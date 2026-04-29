package ops

import (
	"context"
	"runtime"
	"testing"

	"local-fx-host/internal/protocol"
	"local-fx-host/internal/version"
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
	if v, _ := data["version"].(string); v != version.Version {
		t.Errorf("version: got %q, want %q", v, version.Version)
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
	if hv != version.Version {
		t.Errorf("hostVersion: got %q, want %q", hv, version.Version)
	}

	// JSON numbers in a map[string]any are float64 after round-trip, but the
	// live handler returns int directly (we haven't marshalled yet). Accept
	// both so the test is robust if we later wrap Ping with a marshal step.
	switch v := data["hostMaxProtocolVersion"].(type) {
	case int:
		if v != version.MaxProtocolVersion {
			t.Errorf("hostMaxProtocolVersion: got %d, want %d", v, version.MaxProtocolVersion)
		}
	case float64:
		if int(v) != version.MaxProtocolVersion {
			t.Errorf("hostMaxProtocolVersion: got %v, want %d", v, version.MaxProtocolVersion)
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

// TestPing_HostMaxProtocolVersionIs2 locks in the Phase 2.1 bump. The
// version constant itself doubles as the wire-format contract; any future
// bump MUST update PROTOCOL.md + extension/src/ui/ipc.ts in the same commit.
func TestPing_HostMaxProtocolVersionIs2(t *testing.T) {
	if version.MaxProtocolVersion != 2 {
		t.Errorf("MaxProtocolVersion: got %d, want 2 (Phase 2.1 bump)", version.MaxProtocolVersion)
	}
}

// TestPing_VersionIs0_3_0 locks in the v0.3.0 release bump (T2 hybrid CI +
// T6 opt-in update check). Previous 0.0.2 was the Phase 2 read-write baseline.
func TestPing_VersionIs0_3_0(t *testing.T) {
	if version.Version != "0.3.0" {
		t.Errorf("Version: got %q, want %q (v0.3.0 release bump)", version.Version, "0.3.0")
	}
}
