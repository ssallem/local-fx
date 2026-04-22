package ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"local-fx-host/internal/protocol"
)

func renameReq(t *testing.T, args map[string]any) protocol.Request {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return protocol.Request{ID: "rename-1", Op: "rename", Args: raw}
}

func TestRename_SameDirSuccess(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "old.txt")
	if err := os.WriteFile(src, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	dst := filepath.Join(base, "new.txt")
	resp := Rename(context.Background(), renameReq(t, map[string]any{
		"src": src, "dst": dst,
	}))
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("src should be gone: err=%v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("dst missing: %v", err)
	}
}

func TestRename_CrossDirRejected(t *testing.T) {
	base := t.TempDir()
	subA := filepath.Join(base, "a")
	subB := filepath.Join(base, "b")
	for _, d := range []string{subA, subB} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	src := filepath.Join(subA, "x.txt")
	if err := os.WriteFile(src, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	dst := filepath.Join(subB, "x.txt")
	resp := Rename(context.Background(), renameReq(t, map[string]any{
		"src": src, "dst": dst,
	}))
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodeEINVAL {
		t.Errorf("code: got %q want EINVAL", resp.Error.Code)
	}
	// src should still exist untouched.
	if _, err := os.Stat(src); err != nil {
		t.Errorf("src should be intact: %v", err)
	}
}

func TestRename_SrcMissingENOENT(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "missing.txt")
	dst := filepath.Join(base, "dest.txt")
	resp := Rename(context.Background(), renameReq(t, map[string]any{
		"src": src, "dst": dst,
	}))
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodeENOENT {
		t.Errorf("code: got %q want ENOENT", resp.Error.Code)
	}
}

func TestRename_RelativeSrcRejected(t *testing.T) {
	resp := Rename(context.Background(), renameReq(t, map[string]any{
		"src": "relative", "dst": filepath.Join(t.TempDir(), "x"),
	}))
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodePathRejected {
		t.Errorf("code: got %q want E_PATH_REJECTED", resp.Error.Code)
	}
}

func TestRename_SystemPathRequiresConfirm(t *testing.T) {
	var sysParent, sysDst string
	switch runtime.GOOS {
	case "windows":
		sysParent = `C:\Windows\fx-rename-src-should-not-exist`
		sysDst = `C:\Windows\fx-rename-dst-should-not-exist`
	case "darwin":
		sysParent = "/System/fx-rename-src-should-not-exist"
		sysDst = "/System/fx-rename-dst-should-not-exist"
	default:
		t.Skip("no allowlist on this OS")
	}
	resp := Rename(context.Background(), renameReq(t, map[string]any{
		"src": sysParent, "dst": sysDst,
	}))
	if resp.OK {
		t.Fatalf("expected OK=false on system path rename")
	}
	if resp.Error.Code != protocol.ErrCodeSystemPathConfirmRequired {
		t.Errorf("code: got %q want E_SYSTEM_PATH_CONFIRM_REQUIRED", resp.Error.Code)
	}
}

func TestRename_RegisteredInRegistry(t *testing.T) {
	if Lookup("rename") == nil {
		t.Fatal("rename handler not registered")
	}
}
