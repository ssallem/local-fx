package ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"local-fx-host/internal/protocol"
)

func mustStatRequest(t *testing.T, id, path string) protocol.Request {
	t.Helper()
	raw, err := json.Marshal(statArgs{Path: path})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return protocol.Request{ID: id, Op: "stat", Args: raw}
}

func parseStatData(t *testing.T, resp protocol.Response) statData {
	t.Helper()
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var d statData
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return d
}

func TestStat_File(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp := Stat(context.Background(), mustStatRequest(t, "r", p))
	if !resp.OK {
		t.Fatalf("OK=false: %+v", resp.Error)
	}
	d := parseStatData(t, resp)
	if d.Type != "file" {
		t.Errorf("Type: got %q want file", d.Type)
	}
	if d.SizeBytes == nil || *d.SizeBytes != 5 {
		t.Errorf("SizeBytes: got %v want 5", d.SizeBytes)
	}
	if d.Symlink {
		t.Errorf("Symlink should be false for plain file")
	}
	if d.ModifiedTs == 0 {
		t.Errorf("ModifiedTs should be populated")
	}
	if d.Permissions == "" {
		t.Errorf("Permissions should be populated")
	}
}

func TestStat_Directory(t *testing.T) {
	dir := t.TempDir()
	resp := Stat(context.Background(), mustStatRequest(t, "r", dir))
	if !resp.OK {
		t.Fatalf("OK=false: %+v", resp.Error)
	}
	d := parseStatData(t, resp)
	if d.Type != "directory" {
		t.Errorf("Type: got %q want directory", d.Type)
	}
	if d.SizeBytes != nil {
		t.Errorf("SizeBytes: got %v want nil for directory", *d.SizeBytes)
	}
}

func TestStat_Symlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	// Important: stat uses Lstat so the `link` path (not a path that would
	// get resolved by safety.CleanPath first) needs to be produced carefully.
	// We request via the symlink directly; CleanPath's EvalSymlinks follows
	// it to the real target, at which point Stat would report the file, not
	// the link. This is documented Phase-1 behaviour: CleanPath resolves.
	resp := Stat(context.Background(), mustStatRequest(t, "r", link))
	if !resp.OK {
		t.Fatalf("OK=false: %+v", resp.Error)
	}
	d := parseStatData(t, resp)
	// Because CleanPath evaluates symlinks, the reported type is the target's.
	if d.Type == "symlink" {
		t.Errorf("after symlink resolution, Type should reflect target, not %q", d.Type)
	}
}

// TestStat_BrokenSymlink exercises the path where EvalSymlinks fails (target
// missing). CleanPath should fall back to the lexical path, and os.Lstat
// should then succeed on the broken link itself, yielding Type=symlink.
func TestStat_BrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "broken")
	missing := filepath.Join(dir, "nope")
	if err := os.Symlink(missing, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	resp := Stat(context.Background(), mustStatRequest(t, "r", link))
	if !resp.OK {
		t.Fatalf("OK=false: %+v", resp.Error)
	}
	d := parseStatData(t, resp)
	if !d.Symlink || d.Type != "symlink" {
		t.Errorf("broken symlink: got symlink=%v type=%q, want true/symlink", d.Symlink, d.Type)
	}
	if d.Target != missing {
		t.Errorf("Target: got %q want %q", d.Target, missing)
	}
}

func TestStat_Missing(t *testing.T) {
	resp := Stat(context.Background(), mustStatRequest(t, "r",
		filepath.Join(t.TempDir(), "does-not-exist")))
	if resp.OK || resp.Error.Code != protocol.ErrCodeENOENT {
		t.Errorf("want ENOENT, got %+v", resp.Error)
	}
}

func TestStat_EmptyPath(t *testing.T) {
	resp := Stat(context.Background(), mustStatRequest(t, "r", ""))
	if resp.OK || resp.Error.Code != protocol.ErrCodePathRejected {
		t.Errorf("want E_PATH_REJECTED, got %+v", resp.Error)
	}
}

func TestStat_Relative(t *testing.T) {
	resp := Stat(context.Background(), mustStatRequest(t, "r", "relative/dir"))
	if resp.OK || resp.Error.Code != protocol.ErrCodePathRejected {
		t.Errorf("want E_PATH_REJECTED, got %+v", resp.Error)
	}
}

func TestStat_Registered(t *testing.T) {
	if Lookup("stat") == nil {
		t.Fatal("stat handler not registered")
	}
}

func TestStat_BadJSON(t *testing.T) {
	req := protocol.Request{ID: "r", Op: "stat", Args: json.RawMessage(`{bad`)}
	resp := Stat(context.Background(), req)
	if resp.OK || resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Errorf("want E_BAD_REQUEST, got %+v", resp.Error)
	}
}
