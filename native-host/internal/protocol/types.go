package protocol

import "encoding/json"

// Request is a single op invocation from the extension.
//
// Args is kept as json.RawMessage so that each op handler can decode it into
// its own strongly-typed argument struct without the dispatcher needing to
// know every op's schema.
//
// Stream and ProtocolVersion are both `omitempty` so that Phase 0 frames (which
// carry neither field) round-trip without introducing "stream":false or
// "protocolVersion":0 noise on the wire. See PROTOCOL.md §4 (version
// negotiation) and §6 (streaming contract).
type Request struct {
	ID              string          `json:"id"`
	Op              string          `json:"op"`
	Args            json.RawMessage `json:"args,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
	ProtocolVersion int             `json:"protocolVersion,omitempty"`
}

// Response is the reply written back for each Request. Exactly one of Data or
// Error should be set; OK must match Error == nil so that the extension can
// branch on either field.
type Response struct {
	ID    string        `json:"id"`
	OK    bool          `json:"ok"`
	Data  any           `json:"data,omitempty"`
	Error *ErrorPayload `json:"error,omitempty"`
}

// ErrorPayload describes a failed op. Retryable is a hint to the caller that
// the same request might succeed if sent again (e.g. transient I/O errors),
// as opposed to programmer errors like E_UNKNOWN_OP.
//
// Details is an optional structured context bag keyed/typed by the specific
// error code (see PROTOCOL.md §8). It MUST be JSON-serialisable; handlers
// should prefer scalar-valued maps over nested objects.
type ErrorPayload struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Retryable bool                   `json:"retryable"`
	Details   map[string]interface{} `json:"details,omitempty"`
}
