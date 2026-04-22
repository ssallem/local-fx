package ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"local-fx-host/internal/protocol"
)

func removeReq(t *testing.T, args map[string]any) protocol.Request {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return protocol.Request{ID: "remove-1", Op: "remove", Args: raw}
}

func TestRemove_PermanentFile(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "goner.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp := Remove(context.Background(), removeReq(t, map[string]any{
		"path": target, "mode": "permanent",
	}))
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("target should be gone: err=%v", err)
	}
}

func TestRemove_PermanentEmptyDir(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "empty")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	resp := Remove(context.Background(), removeReq(t, map[string]any{
		"path": target, "mode": "permanent",
	}))
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("target should be gone: err=%v", err)
	}
}

func TestRemove_PermanentNonEmptyDir(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "full")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "child"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}
	resp := Remove(context.Background(), removeReq(t, map[string]any{
		"path": target, "mode": "permanent",
	}))
	if resp.OK {
		t.Fatalf("expected OK=false for non-empty dir")
	}
	if resp.Error.Code != protocol.ErrCodeEINVAL {
		t.Errorf("code: got %q want EINVAL", resp.Error.Code)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("non-empty dir should still exist after refusal: %v", err)
	}
}

func TestRemove_PermanentMissing(t *testing.T) {
	base := t.TempDir()
	resp := Remove(context.Background(), removeReq(t, map[string]any{
		"path": filepath.Join(base, "nope"), "mode": "permanent",
	}))
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodeENOENT {
		t.Errorf("code: got %q want ENOENT", resp.Error.Code)
	}
}

func TestRemove_BadMode(t *testing.T) {
	base := t.TempDir()
	resp := Remove(context.Background(), removeReq(t, map[string]any{
		"path": filepath.Join(base, "x"), "mode": "nuke",
	}))
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Errorf("code: got %q want E_BAD_REQUEST", resp.Error.Code)
	}
}

func TestRemove_RelativePathRejected(t *testing.T) {
	resp := Remove(context.Background(), removeReq(t, map[string]any{
		"path": "relative", "mode": "permanent",
	}))
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodePathRejected {
		t.Errorf("code: got %q want E_PATH_REJECTED", resp.Error.Code)
	}
}

func TestRemove_RegisteredInRegistry(t *testing.T) {
	if Lookup("remove") == nil {
		t.Fatal("remove handler not registered")
	}
}
