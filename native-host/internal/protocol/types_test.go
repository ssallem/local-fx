package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestErrorPayload_DetailsRoundTrip verifies that the Details field added in
// F-10 marshals to and unmarshals from the JSON shape the extension's
// TypeScript ErrorFrame.error.details field expects:
//   details?: Record<string, unknown>
func TestErrorPayload_DetailsRoundTrip(t *testing.T) {
	in := ErrorPayload{
		Code:      ErrCodeProtocol,
		Message:   "unsupported protocol version",
		Retryable: false,
		Details: map[string]interface{}{
			"hostMaxVersion": float64(1), // JSON numbers decode to float64
			"requested":      float64(3),
			"reason":         "hostMaxVersion < requested",
		},
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// The field name must be "details" (lowercase) per PROTOCOL.md §3.
	if !strings.Contains(string(raw), `"details":`) {
		t.Fatalf("expected marshalled JSON to contain \"details\" key, got: %s", raw)
	}

	var out ErrorPayload
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Code != in.Code || out.Message != in.Message || out.Retryable != in.Retryable {
		t.Errorf("scalar round-trip mismatch: got %+v want %+v", out, in)
	}
	if len(out.Details) != len(in.Details) {
		t.Fatalf("Details length: got %d want %d", len(out.Details), len(in.Details))
	}
	for k, v := range in.Details {
		if out.Details[k] != v {
			t.Errorf("Details[%q]: got %v want %v", k, out.Details[k], v)
		}
	}
}

// TestErrorPayload_DetailsOmittedWhenNil confirms the `omitempty` JSON tag
// keeps the wire format minimal: handlers that have no structured context
// should not produce a `"details": null` field in the output.
func TestErrorPayload_DetailsOmittedWhenNil(t *testing.T) {
	in := ErrorPayload{
		Code:      ErrCodeUnknownOp,
		Message:   "unknown op",
		Retryable: false,
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(raw), "details") {
		t.Errorf("expected Details to be omitted, got: %s", raw)
	}
}

// TestNewError_NoDetails documents that the legacy NewError helper still
// produces a payload with Details == nil, so existing Phase-0 call sites
// keep their on-wire shape unchanged.
func TestNewError_NoDetails(t *testing.T) {
	p := NewError(ErrCodeInternal, "boom", false)
	if p == nil {
		t.Fatal("NewError returned nil")
	}
	if p.Details != nil {
		t.Errorf("Details: got %+v, want nil", p.Details)
	}
}

// TestNewErrorWithDetails_SetsDetails confirms the F-10 helper variant wires
// the details map through without copying/mutating it.
func TestNewErrorWithDetails_SetsDetails(t *testing.T) {
	d := map[string]interface{}{"wrapped": "stat: permission denied"}
	p := NewErrorWithDetails(ErrCodeEIO, "io failure", true, d)
	if p == nil {
		t.Fatal("NewErrorWithDetails returned nil")
	}
	if !p.Retryable {
		t.Errorf("Retryable: got false, want true")
	}
	if got := p.Details["wrapped"]; got != "stat: permission denied" {
		t.Errorf("Details[wrapped]: got %v", got)
	}
}

// TestErrorResponseWithDetails_WrapsPayload exercises the Response-level
// helper so that dispatcher code migrating to the richer API has a
// reference fixture.
func TestErrorResponseWithDetails_WrapsPayload(t *testing.T) {
	resp := ErrorResponseWithDetails(
		"req-1",
		ErrCodeProtocol,
		"version too high",
		false,
		map[string]interface{}{"hostMaxVersion": float64(1)},
	)
	if resp.OK {
		t.Errorf("OK: got true, want false")
	}
	if resp.Error == nil {
		t.Fatal("Error: got nil")
	}
	if resp.Error.Code != ErrCodeProtocol {
		t.Errorf("Code: got %q want %q", resp.Error.Code, ErrCodeProtocol)
	}
	if resp.Error.Details["hostMaxVersion"] != float64(1) {
		t.Errorf("Details[hostMaxVersion]: got %v", resp.Error.Details["hostMaxVersion"])
	}
}

// TestRequest_StreamAndProtocolVersionRoundTrip verifies that the Stream /
// ProtocolVersion fields added for PROTOCOL.md §4/§6 marshal to and unmarshal
// from the JSON shape the extension's TypeScript Request interface uses, and
// that the `omitempty` tag keeps Phase 0 (no stream, no version) frames free
// of those keys on the wire.
func TestRequest_StreamAndProtocolVersionRoundTrip(t *testing.T) {
	// Populated variant — both fields must be present after round-trip.
	in := Request{
		ID:              "req-1",
		Op:              "copy",
		Args:            json.RawMessage(`{"src":"a","dst":"b"}`),
		Stream:          true,
		ProtocolVersion: 2,
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal populated: %v", err)
	}
	if !strings.Contains(string(raw), `"stream":true`) {
		t.Errorf("expected \"stream\":true in %s", raw)
	}
	if !strings.Contains(string(raw), `"protocolVersion":2`) {
		t.Errorf("expected \"protocolVersion\":2 in %s", raw)
	}
	var out Request
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal populated: %v", err)
	}
	if out.ID != in.ID || out.Op != in.Op || out.Stream != in.Stream || out.ProtocolVersion != in.ProtocolVersion {
		t.Errorf("round-trip mismatch: got %+v want %+v", out, in)
	}

	// Phase 0 variant — both fields absent on the wire (omitempty).
	ping := Request{ID: "req-2", Op: "ping"}
	pingRaw, err := json.Marshal(ping)
	if err != nil {
		t.Fatalf("Marshal phase-0: %v", err)
	}
	if strings.Contains(string(pingRaw), "stream") {
		t.Errorf("expected \"stream\" to be omitted for Phase 0 ping, got: %s", pingRaw)
	}
	if strings.Contains(string(pingRaw), "protocolVersion") {
		t.Errorf("expected \"protocolVersion\" to be omitted for Phase 0 ping, got: %s", pingRaw)
	}

	// Incoming Phase 0 frame (no stream/protocolVersion keys) must decode to
	// the Go zero values without error, so handlers added in Phase 1+ can
	// still read Phase 0 traffic untouched.
	const phase0Wire = `{"id":"req-3","op":"ping"}`
	var decoded Request
	if err := json.Unmarshal([]byte(phase0Wire), &decoded); err != nil {
		t.Fatalf("Unmarshal phase-0 wire: %v", err)
	}
	if decoded.Stream != false || decoded.ProtocolVersion != 0 {
		t.Errorf("phase-0 defaults: got stream=%v protocolVersion=%d, want false/0",
			decoded.Stream, decoded.ProtocolVersion)
	}
}

// TestErrorCatalogue_NoDuplicateValues guards against accidental string
// collisions when the catalogue grows. Every declared constant must map to
// a distinct on-wire code; PROTOCOL.md §8 is the authority.
func TestErrorCatalogue_NoDuplicateValues(t *testing.T) {
	codes := []string{
		ErrCodeHostNotFound,
		ErrCodeHostCrash,
		ErrCodeProtocol,
		ErrCodeFrameTooLarge,
		ErrCodeUnknownOp,
		ErrCodeBadRequest,
		ErrCodeInternal,
		ErrCodeCanceled,
		ErrCodeEACCES,
		ErrCodeENOENT,
		ErrCodeEIO,
		ErrCodeTooLarge,
		ErrCodeEEXIST,
		ErrCodeENOSPC,
		ErrCodeSharingViolation,
		ErrCodeTrashUnavailable,
		ErrCodeEINVAL,
		ErrCodeNoHandler,
		ErrCodePathRejected,
		ErrCodeSystemPathConfirmRequired,
	}
	seen := make(map[string]bool, len(codes))
	for _, c := range codes {
		if c == "" {
			t.Errorf("empty error code constant")
			continue
		}
		if seen[c] {
			t.Errorf("duplicate error code value: %q", c)
		}
		seen[c] = true
	}
}
