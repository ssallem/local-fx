package ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"local-fx-host/internal/protocol"
)

// mustRequest marshals a readdir args struct and wraps it in a Request. The
// helper isolates test bodies from the json.Marshal boilerplate.
func mustRequest(t *testing.T, id string, args readdirArgs) protocol.Request {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return protocol.Request{ID: id, Op: "readdir", Args: raw}
}

// parseData decodes resp.Data back into readdirData so tests read strongly-
// typed fields instead of traversing map[string]any chains.
func parseData(t *testing.T, resp protocol.Response) readdirData {
	t.Helper()
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var d readdirData
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	return d
}

// seedDir populates a temp directory with a known set of entries, returning
// the dir path. Subsequent helpers rely on the exact name set.
func seedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]int{
		"alpha.txt": 10,
		"Bravo.md":  200,
		"charlie":   3000, // directory (size ignored)
	}
	for name, size := range files {
		full := filepath.Join(dir, name)
		if name == "charlie" {
			if err := os.Mkdir(full, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			continue
		}
		data := make([]byte, size)
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatalf("writefile %s: %v", name, err)
		}
	}
	// A hidden (leading-dot) file to exercise the includeHidden flag.
	if err := os.WriteFile(filepath.Join(dir, ".secret"), []byte("x"), 0o644); err != nil {
		t.Fatalf("writefile .secret: %v", err)
	}
	return dir
}

func TestReaddir_Happy(t *testing.T) {
	dir := seedDir(t)
	resp := Readdir(context.Background(), mustRequest(t, "r1", readdirArgs{Path: dir}))
	if !resp.OK {
		t.Fatalf("OK=false: %+v", resp.Error)
	}
	d := parseData(t, resp)
	if d.Total != 3 { // .secret filtered out
		t.Errorf("Total: got %d, want 3", d.Total)
	}
	if len(d.Entries) != 3 {
		t.Fatalf("Entries: got %d, want 3", len(d.Entries))
	}
	// Default sort is name asc, case-insensitive. Expected: alpha, Bravo, charlie
	wantNames := []string{"alpha.txt", "Bravo.md", "charlie"}
	for i, w := range wantNames {
		if d.Entries[i].Name != w {
			t.Errorf("Entries[%d]: got %q, want %q", i, d.Entries[i].Name, w)
		}
	}
	// Directory entry has SizeBytes==nil; files have a pointer.
	for _, e := range d.Entries {
		if e.Type == "directory" && e.SizeBytes != nil {
			t.Errorf("directory %q should have SizeBytes=nil, got %v", e.Name, *e.SizeBytes)
		}
		if e.Type == "file" && e.SizeBytes == nil {
			t.Errorf("file %q should have SizeBytes set", e.Name)
		}
	}
}

func TestReaddir_IncludeHidden(t *testing.T) {
	dir := seedDir(t)
	resp := Readdir(context.Background(), mustRequest(t, "r", readdirArgs{Path: dir, IncludeHidden: true}))
	if !resp.OK {
		t.Fatalf("OK=false: %+v", resp.Error)
	}
	d := parseData(t, resp)
	if d.Total != 4 {
		t.Errorf("Total with hidden: got %d, want 4", d.Total)
	}
	var found bool
	for _, e := range d.Entries {
		if e.Name == ".secret" {
			found = true
			if !e.Hidden {
				t.Errorf(".secret not flagged hidden")
			}
		}
	}
	if !found {
		t.Errorf(".secret missing from entries")
	}
}

func TestReaddir_SortSizeDesc(t *testing.T) {
	dir := seedDir(t)
	resp := Readdir(context.Background(), mustRequest(t, "r", readdirArgs{
		Path: dir,
		Sort: readdirSort{Field: "size", Order: "desc"},
	}))
	if !resp.OK {
		t.Fatalf("OK=false: %+v", resp.Error)
	}
	d := parseData(t, resp)
	// Bravo.md (200) > alpha.txt (10) > charlie (dir, 0) in desc order.
	want := []string{"Bravo.md", "alpha.txt", "charlie"}
	for i, w := range want {
		if d.Entries[i].Name != w {
			t.Errorf("Entries[%d]: got %q, want %q", i, d.Entries[i].Name, w)
		}
	}
}

func TestReaddir_SortModified(t *testing.T) {
	dir := t.TempDir()
	// Write files with deliberately staggered mtimes so the sort is well-defined.
	names := []string{"first", "second", "third"}
	for i, n := range names {
		full := filepath.Join(dir, n)
		if err := os.WriteFile(full, []byte{1}, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		future := time.Now().Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(full, future, future); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}
	resp := Readdir(context.Background(), mustRequest(t, "r", readdirArgs{
		Path: dir,
		Sort: readdirSort{Field: "modified", Order: "asc"},
	}))
	if !resp.OK {
		t.Fatalf("OK=false: %+v", resp.Error)
	}
	d := parseData(t, resp)
	for i, n := range names {
		if d.Entries[i].Name != n {
			t.Errorf("Entries[%d]: got %q want %q", i, d.Entries[i].Name, n)
		}
	}
}

func TestReaddir_Paging(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 12; i++ {
		name := filepath.Join(dir, strings.Repeat("a", i+1))
		if err := os.WriteFile(name, nil, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	// Page 0, pageSize 5 -> 5 entries, nextCursor="1"
	resp := Readdir(context.Background(), mustRequest(t, "r", readdirArgs{
		Path: dir, Page: 0, PageSize: 5,
	}))
	if !resp.OK {
		t.Fatalf("OK=false: %+v", resp.Error)
	}
	d := parseData(t, resp)
	if len(d.Entries) != 5 {
		t.Errorf("page 0: got %d entries, want 5", len(d.Entries))
	}
	if d.Total != 12 {
		t.Errorf("Total: got %d want 12", d.Total)
	}
	if d.NextCursor == nil || *d.NextCursor != "1" {
		t.Errorf("NextCursor: got %v, want \"1\"", d.NextCursor)
	}
	// Last page (2) should have 2 entries and nextCursor=nil.
	resp = Readdir(context.Background(), mustRequest(t, "r", readdirArgs{
		Path: dir, Page: 2, PageSize: 5,
	}))
	d = parseData(t, resp)
	if len(d.Entries) != 2 {
		t.Errorf("last page: got %d entries, want 2", len(d.Entries))
	}
	if d.NextCursor != nil {
		t.Errorf("NextCursor: got %v, want nil", *d.NextCursor)
	}
}

func TestReaddir_PageSizeTooLarge(t *testing.T) {
	resp := Readdir(context.Background(), mustRequest(t, "r", readdirArgs{
		Path: t.TempDir(), PageSize: readdirMaxPageSize + 1,
	}))
	if resp.OK {
		t.Fatalf("expected failure")
	}
	if resp.Error.Code != protocol.ErrCodeTooLarge {
		t.Errorf("code: got %q want %q", resp.Error.Code, protocol.ErrCodeTooLarge)
	}
}

func TestReaddir_NegativePageSize(t *testing.T) {
	resp := Readdir(context.Background(), mustRequest(t, "r", readdirArgs{
		Path: t.TempDir(), PageSize: -1,
	}))
	if resp.OK || resp.Error.Code != protocol.ErrCodeEINVAL {
		t.Errorf("want EINVAL, got %+v", resp.Error)
	}
}

func TestReaddir_NotADirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp := Readdir(context.Background(), mustRequest(t, "r", readdirArgs{Path: file}))
	if resp.OK || resp.Error.Code != protocol.ErrCodeEINVAL {
		t.Errorf("want EINVAL, got %+v", resp.Error)
	}
}

func TestReaddir_Missing(t *testing.T) {
	resp := Readdir(context.Background(), mustRequest(t, "r", readdirArgs{
		Path: filepath.Join(t.TempDir(), "does-not-exist"),
	}))
	if resp.OK || resp.Error.Code != protocol.ErrCodeENOENT {
		t.Errorf("want ENOENT, got %+v", resp.Error)
	}
}

func TestReaddir_RelativePathRejected(t *testing.T) {
	resp := Readdir(context.Background(), mustRequest(t, "r", readdirArgs{Path: "relative/path"}))
	if resp.OK || resp.Error.Code != protocol.ErrCodePathRejected {
		t.Errorf("want E_PATH_REJECTED, got %+v", resp.Error)
	}
}

func TestReaddir_BadJSON(t *testing.T) {
	req := protocol.Request{ID: "r", Op: "readdir", Args: json.RawMessage(`{"path":`)}
	resp := Readdir(context.Background(), req)
	if resp.OK || resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Errorf("want E_BAD_REQUEST, got %+v", resp.Error)
	}
}

func TestReaddir_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows permission semantics differ enough that chmod 0 on a
		// directory doesn't reliably produce EACCES; covered by the
		// integration story instead.
		t.Skip("unix-only chmod(0) scenario")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Populate first, then lock so readdir has content to refuse.
	if err := os.WriteFile(filepath.Join(locked, "x"), nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	resp := Readdir(context.Background(), mustRequest(t, "r", readdirArgs{Path: locked}))
	if resp.OK {
		t.Fatalf("expected failure")
	}
	if resp.Error.Code != protocol.ErrCodeEACCES {
		t.Errorf("code: got %q want EACCES", resp.Error.Code)
	}
}

func TestReaddir_SymlinkEntryFlagged(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	resp := Readdir(context.Background(), mustRequest(t, "r", readdirArgs{Path: dir}))
	if !resp.OK {
		t.Fatalf("OK=false: %+v", resp.Error)
	}
	d := parseData(t, resp)
	var got *readdirEntry
	for i, e := range d.Entries {
		if e.Name == "link" {
			got = &d.Entries[i]
		}
	}
	if got == nil {
		t.Fatalf("link not present in entries")
	}
	if !got.Symlink || got.Type != "symlink" {
		t.Errorf("expected symlink=true type=symlink, got %+v", got)
	}
}

// TestReaddir_Registered exercises the init() wiring.
func TestReaddir_Registered(t *testing.T) {
	if Lookup("readdir") == nil {
		t.Fatal("readdir handler not registered")
	}
}

// TestReaddir_ContextCancelled asserts that a canceled context short-circuits
// the per-entry loop. We feed a large-ish directory and a pre-cancelled ctx
// so the very first iteration of the loop observes Done().
func TestReaddir_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		_ = os.WriteFile(filepath.Join(dir, strings.Repeat("f", i+1)), nil, 0o644)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp := Readdir(ctx, mustRequest(t, "r", readdirArgs{Path: dir}))
	if resp.OK {
		t.Fatalf("expected failure after cancel")
	}
	if resp.Error.Code != protocol.ErrCodeCanceled {
		t.Errorf("code: got %q want E_CANCELED", resp.Error.Code)
	}
}

