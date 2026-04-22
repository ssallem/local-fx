package ops

import (
	"context"
	"runtime"
	"testing"

	"local-fx-host/internal/protocol"
)

func TestPing(t *testing.T) {
	req := protocol.Request{ID: "req-1", Op: "ping"}
	resp := Ping(context.Background(), req)

	if resp.ID != "req-1" {
		t.Errorf("ID: got %q want %q", resp.ID, "req-1")
	}
	if !resp.OK {
		t.Fatalf("OK: got false, want true (error=%+v)", resp.Error)
	}
	if resp.Error != nil {
		t.Errorf("Error: got %+v, want nil", resp.Error)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data: got %T, want map[string]any", resp.Data)
	}
	if pong, _ := data["pong"].(bool); !pong {
		t.Errorf("pong: got %v, want true", data["pong"])
	}
	if v, _ := data["version"].(string); v != Version {
		t.Errorf("version: got %q, want %q", v, Version)
	}
	if os, _ := data["os"].(string); os != runtime.GOOS {
		t.Errorf("os: got %q, want %q", os, runtime.GOOS)
	}
}

// TestPing_Phase1HandshakeFields verifies PROTOCOL.md §7.1 requires the host
// to advertise hostVersion/hostMaxProtocolVersion/serverTs so the extension
// can detect version skew on the very first frame.
func TestPing_Phase1HandshakeFields(t *testing.T) {
	req := protocol.Request{ID: "req-hs", Op: "ping"}
	resp := Ping(context.Background(), req)
	if !resp.OK {
		t.Fatalf("OK: got false, want true (error=%+v)", resp.Error)
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data: got %T, want map[string]any", resp.Data)
	}

	hv, ok := data["hostVersion"].(string)
	if !ok {
		t.Fatalf("hostVersion missing or wrong type: got %T (%v)", data["hostVersion"], data["hostVersion"])
	}
	if hv != Version {
		t.Errorf("hostVersion: got %q, want %q", hv, Version)
	}

	// JSON numbers in a map[string]any are float64 after round-trip, but the
	// live handler returns int directly (we haven't marshalled yet). Accept
	// both so the test is robust if we later wrap Ping with a marshal step.
	switch v := data["hostMaxProtocolVersion"].(type) {
	case int:
		if v != HostMaxProtocolVersion {
			t.Errorf("hostMaxProtocolVersion: got %d, want %d", v, HostMaxProtocolVersion)
		}
	case float64:
		if int(v) != HostMaxProtocolVersion {
			t.Errorf("hostMaxProtocolVersion: got %v, want %d", v, HostMaxProtocolVersion)
		}
	default:
		t.Fatalf("hostMaxProtocolVersion missing or wrong type: got %T (%v)", data["hostMaxProtocolVersion"], data["hostMaxProtocolVersion"])
	}

	switch v := data["serverTs"].(type) {
	case int64:
		if v <= 0 {
			t.Errorf("serverTs: got %d, want positive unix millis", v)
		}
	case float64:
		if v <= 0 {
			t.Errorf("serverTs: got %v, want positive unix millis", v)
		}
	default:
		t.Fatalf("serverTs missing or wrong type: got %T (%v)", data["serverTs"], data["serverTs"])
	}
}

func TestPing_RegisteredInRegistry(t *testing.T) {
	if Lookup("ping") == nil {
		t.Fatal("ping handler not registered; registry init() did not run")
	}
}
