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

// SuccessResponse builds an OK Response carrying the given data payload.
// Handlers use it to avoid repeating the struct literal. Pass nil to emit
// an empty data object (json.Marshal will render `"data":null`); most
// handlers prefer an explicit `map[string]any{}` for that case.
func SuccessResponse(id string, data any) Response {
	return Response{ID: id, OK: true, Data: data}
}

// EventFrame is a mid-stream message from a streaming op. Streaming ops emit
// zero or more EventFrames on the wire before their final Response; the
// EventFrame carries the same `id` as its originating Request so that the
// extension can correlate progress/item/done events with the in-flight op.
// See PROTOCOL.md §6.
//
// Event values recognised by the extension:
//   - "progress" — ProgressPayload: bytesDone, fileDone, rate, currentPath
//   - "item"     — op-specific incremental result (e.g. search hit)
//   - "done"     — DonePayload: canceled flag + per-entry failures list
//
// A success path of a streaming op is: 0..N progress/item events -> one
// "done" event -> one final Response{OK:true, Data:{}}. On a hard error
// (e.g. src does not exist), the final Response carries OK:false/Error and
// NO "done" event is emitted. Per-entry partial failures go in
// DonePayload.Failures so callers can distinguish "nothing copied" from
// "copied N of M, here are the skips".
type EventFrame struct {
	ID      string `json:"id"`
	Event   string `json:"event"`
	Payload any    `json:"payload,omitempty"`
}

// ProgressPayload is the payload for Event="progress" frames.
//
// Rate is bytes/sec averaged over the recent emit window. Handlers SHOULD
// debounce progress emissions (typical ~100ms) to keep the wire cheap;
// sub-frame churn is amplified by JSON marshalling + stdout framing.
type ProgressPayload struct {
	BytesDone   int64   `json:"bytesDone"`
	BytesTotal  int64   `json:"bytesTotal"`
	FileDone    int     `json:"fileDone"`
	FileTotal   int     `json:"fileTotal"`
	CurrentPath string  `json:"currentPath,omitempty"`
	Rate        float64 `json:"rate,omitempty"`
}

// DonePayload is the payload for Event="done" frames.
//
// Canceled=true signals the op stopped because ctx was canceled (matching
// cancel request observed by ops.CancelJob). Failures carries per-entry
// errors encountered in non-fail-fast mode, so a streaming copy/move can
// finish with Response{OK:true} while still reporting which children were
// skipped. An empty Failures + Canceled=false is the clean success case.
type DonePayload struct {
	Canceled bool          `json:"canceled,omitempty"`
	Failures []FailureInfo `json:"failures,omitempty"`
}

// FailureInfo is a single per-entry failure inside DonePayload.Failures.
// Code is one of the protocol ErrCode* constants; Path is the affected
// absolute path; Message is a human-readable explanation.
type FailureInfo struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
