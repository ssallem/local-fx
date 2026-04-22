package ops

import (
	"context"
	"encoding/json"
	"testing"

	"local-fx-host/internal/protocol"
)

func openReq(t *testing.T, op string, args map[string]any) protocol.Request {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return protocol.Request{ID: "open-1", Op: op, Args: raw}
}

// We intentionally do NOT exercise the actual ShellExecuteW / osascript
// calls in unit tests — they would bring up GUI windows, require a user
// session, and would be environment-dependent. Instead we verify args
// validation surfaces proper errors before the platform layer is touched.
//
// End-to-end open/reveal verification lives in docs/DEV.md as a manual
// checkpoint (see harmonic-chasing-narwhal.md §Phase 2.1 step 6).

func TestOpen_RelativePathRejected(t *testing.T) {
	resp := Open(context.Background(), openReq(t, "open", map[string]any{"path": "relative"}))
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodePathRejected {
		t.Errorf("code: got %q want E_PATH_REJECTED", resp.Error.Code)
	}
}

func TestOpen_EmptyPathRejected(t *testing.T) {
	resp := Open(context.Background(), openReq(t, "open", map[string]any{"path": ""}))
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodePathRejected {
		t.Errorf("code: got %q want E_PATH_REJECTED", resp.Error.Code)
	}
}

func TestOpen_BadJSON(t *testing.T) {
	req := protocol.Request{ID: "x", Op: "open", Args: json.RawMessage(`{bad`)}
	resp := Open(context.Background(), req)
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Errorf("code: got %q want E_BAD_REQUEST", resp.Error.Code)
	}
}

func TestReveal_RelativePathRejected(t *testing.T) {
	resp := RevealInOsExplorer(context.Background(),
		openReq(t, "revealInOsExplorer", map[string]any{"path": "relative"}))
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodePathRejected {
		t.Errorf("code: got %q want E_PATH_REJECTED", resp.Error.Code)
	}
}

func TestOpen_RegisteredInRegistry(t *testing.T) {
	if Lookup("open") == nil {
		t.Fatal("open handler not registered")
	}
	if Lookup("revealInOsExplorer") == nil {
		t.Fatal("revealInOsExplorer handler not registered")
	}
}
