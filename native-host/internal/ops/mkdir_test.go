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

func mkdirReq(t *testing.T, args map[string]any) protocol.Request {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return protocol.Request{ID: "mkdir-1", Op: "mkdir", Args: raw}
}

func TestMkdir_CreatesDirectory(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "new")
	resp := Mkdir(context.Background(), mkdirReq(t, map[string]any{"path": target}))
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat new dir: %v", err)
	}
	if !fi.IsDir() {
		t.Errorf("expected directory, got mode %v", fi.Mode())
	}
}

func TestMkdir_ExistsWithoutRecursive(t *testing.T) {
	base := t.TempDir()
	// base itself already exists; try to mkdir on it without recursive.
	resp := Mkdir(context.Background(), mkdirReq(t, map[string]any{"path": base}))
	if resp.OK {
		t.Fatalf("expected OK=false, got ok")
	}
	if resp.Error.Code != protocol.ErrCodeEEXIST {
		t.Errorf("code: got %q want EEXIST", resp.Error.Code)
	}
}

func TestMkdir_RecursiveTolerantOfExistence(t *testing.T) {
	base := t.TempDir()
	// MkdirAll on an existing directory is a no-op success.
	resp := Mkdir(context.Background(), mkdirReq(t, map[string]any{
		"path":      base,
		"recursive": true,
	}))
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
}

func TestMkdir_RecursiveCreatesIntermediateDirs(t *testing.T) {
	base := t.TempDir()
	deep := filepath.Join(base, "a", "b", "c")
	resp := Mkdir(context.Background(), mkdirReq(t, map[string]any{
		"path":      deep,
		"recursive": true,
	}))
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	if fi, err := os.Stat(deep); err != nil || !fi.IsDir() {
		t.Errorf("deep dir missing: err=%v", err)
	}
}

func TestMkdir_RelativePathRejected(t *testing.T) {
	resp := Mkdir(context.Background(), mkdirReq(t, map[string]any{"path": "relative/path"}))
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodePathRejected {
		t.Errorf("code: got %q want E_PATH_REJECTED", resp.Error.Code)
	}
}

func TestMkdir_EmptyPathRejected(t *testing.T) {
	resp := Mkdir(context.Background(), mkdirReq(t, map[string]any{"path": ""}))
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodePathRejected {
		t.Errorf("code: got %q want E_PATH_REJECTED", resp.Error.Code)
	}
}

func TestMkdir_BadJSON(t *testing.T) {
	req := protocol.Request{ID: "x", Op: "mkdir", Args: json.RawMessage(`{not json`)}
	resp := Mkdir(context.Background(), req)
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Errorf("code: got %q want E_BAD_REQUEST", resp.Error.Code)
	}
}

func TestMkdir_SystemPathRequiresConfirm(t *testing.T) {
	var sysPath string
	switch runtime.GOOS {
	case "windows":
		sysPath = `C:\Windows\this-should-not-be-created`
	case "darwin":
		sysPath = "/System/this-should-not-be-created"
	default:
		t.Skip("no allowlist on this OS")
	}
	resp := Mkdir(context.Background(), mkdirReq(t, map[string]any{"path": sysPath}))
	if resp.OK {
		t.Fatalf("expected OK=false on system path")
	}
	if resp.Error.Code != protocol.ErrCodeSystemPathConfirmRequired {
		t.Errorf("code: got %q want E_SYSTEM_PATH_CONFIRM_REQUIRED", resp.Error.Code)
	}
	// Ensure we did NOT actually try to create anything.
	if _, err := os.Stat(sysPath); !os.IsNotExist(err) {
		t.Errorf("side effect: system path should still be missing, got err=%v", err)
	}
}

func TestMkdir_RegisteredInRegistry(t *testing.T) {
	if Lookup("mkdir") == nil {
		t.Fatal("mkdir handler not registered")
	}
}
