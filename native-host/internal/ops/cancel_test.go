package ops

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"local-fx-host/internal/protocol"
)

func cancelReq(t *testing.T, args map[string]any) protocol.Request {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return protocol.Request{ID: "cancel-req-1", Op: "cancel", Args: raw}
}

// TestCancel_AcceptedWhenJobRegistered stores a dummy cancel under a known
// ID, invokes Cancel, and verifies accepted=true + cancel func fired.
func TestCancel_AcceptedWhenJobRegistered(t *testing.T) {
	const target = "job-to-be-canceled"
	var fired int32
	RegisterJob(target, func() { atomic.StoreInt32(&fired, 1) })
	t.Cleanup(func() { UnregisterJob(target) })

	resp := Cancel(context.Background(), cancelReq(t, map[string]any{"targetId": target}))
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	data, ok := resp.Data.(cancelData)
	if !ok {
		t.Fatalf("Data: got %T, want cancelData", resp.Data)
	}
	if !data.Accepted {
		t.Errorf("Accepted: got false, want true")
	}
	if atomic.LoadInt32(&fired) != 1 {
		t.Errorf("cancel func was not invoked")
	}
}

// TestCancel_NotAcceptedWhenJobMissing returns accepted=false for an
// unknown ID. This is the "stale cancel" path — caller raced the op to
// completion.
func TestCancel_NotAcceptedWhenJobMissing(t *testing.T) {
	resp := Cancel(context.Background(), cancelReq(t, map[string]any{
		"targetId": "this-id-never-existed-zzz",
	}))
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	data, ok := resp.Data.(cancelData)
	if !ok {
		t.Fatalf("Data: got %T, want cancelData", resp.Data)
	}
	if data.Accepted {
		t.Errorf("Accepted: got true, want false for unknown ID")
	}
}

// TestCancel_MissingTargetIDRejected enforces the required field check.
func TestCancel_MissingTargetIDRejected(t *testing.T) {
	resp := Cancel(context.Background(), cancelReq(t, map[string]any{}))
	if resp.OK {
		t.Fatalf("expected OK=false, got OK")
	}
	if resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Errorf("code: got %q want E_BAD_REQUEST", resp.Error.Code)
	}
}

// TestCancel_BadJSON confirms invalid args surface E_BAD_REQUEST.
func TestCancel_BadJSON(t *testing.T) {
	req := protocol.Request{
		ID:   "x",
		Op:   "cancel",
		Args: json.RawMessage(`{not json`),
	}
	resp := Cancel(context.Background(), req)
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Errorf("code: got %q want E_BAD_REQUEST", resp.Error.Code)
	}
}

// TestCancel_RegisteredInRegistry confirms the dispatcher can find it.
func TestCancel_RegisteredInRegistry(t *testing.T) {
	if Lookup("cancel") == nil {
		t.Fatal("cancel handler not registered")
	}
}
