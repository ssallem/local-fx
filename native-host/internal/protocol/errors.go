package protocol

// Error code catalogue. The authoritative list lives in PROTOCOL.md §8 — any
// code used on the wire MUST be declared here, and any constant here MUST map
// to a row in that table. Keep the two in lockstep on every change.
//
// Grouping below mirrors the three tiers the protocol distinguishes:
//   - Transport / framing (host process + wire integrity)
//   - Dispatch (request routing, cancellation, version negotiation)
//   - Filesystem / op-level (surfaced by individual op handlers)
const (
	// --- Transport / framing (PROTOCOL.md §8) ---

	// ErrCodeHostNotFound: Host binary or Native Messaging manifest is
	// missing. Emitted by the extension side, never by the host itself;
	// declared here so Go code that constructs E2E test fixtures has a
	// single source of truth. retryable=false.
	ErrCodeHostNotFound = "E_HOST_NOT_FOUND"

	// ErrCodeHostCrash: Host terminated abnormally. Like E_HOST_NOT_FOUND,
	// observed only from the extension side. retryable=true (auto-reconnect
	// once per PROTOCOL.md §9 step 6).
	ErrCodeHostCrash = "E_HOST_CRASH"

	// ErrCodeProtocol: JSON parse failure, schema mismatch, or
	// protocolVersion out of range (see PROTOCOL.md §4). retryable=false.
	ErrCodeProtocol = "E_PROTOCOL"

	// ErrCodeFrameTooLarge: Declared or supplied frame body exceeds the
	// 1 MiB Chrome Native Messaging ceiling (see codec.go MaxFrameSize).
	// retryable=false — callers must redesign around streaming/chunk ops.
	ErrCodeFrameTooLarge = "E_FRAME_TOO_LARGE"

	// --- Dispatch (PROTOCOL.md §8) ---

	// ErrCodeUnknownOp: Dispatcher has no handler registered for req.Op.
	// retryable=false.
	ErrCodeUnknownOp = "E_UNKNOWN_OP"

	// ErrCodeBadRequest: Frame body cannot be parsed as a Request, or
	// required fields (id, op) are missing/empty. Distinct from
	// E_PROTOCOL, which covers version/schema drift rather than malformed
	// JSON. retryable=false.
	ErrCodeBadRequest = "E_BAD_REQUEST"

	// ErrCodeInternal: Catch-all for unexpected host-side failures
	// (panics, logic bugs). Handlers should prefer a more specific code
	// whenever possible. retryable=false.
	ErrCodeInternal = "E_INTERNAL"

	// ErrCodeCanceled: Op was aborted because the extension sent a
	// matching `cancel` request (see PROTOCOL.md §6). retryable=false —
	// the host treats cancellation as normal termination.
	ErrCodeCanceled = "E_CANCELED"

	// --- Filesystem / op-level (PROTOCOL.md §8) ---

	// ErrCodeEACCES: OS reported EACCES / access denied. retryable=false.
	ErrCodeEACCES = "EACCES"

	// ErrCodeENOENT: Path does not exist. retryable=false.
	ErrCodeENOENT = "ENOENT"

	// ErrCodeEIO: Low-level I/O failure. retryable=true — host performs up
	// to two internal retries before surfacing this per PROTOCOL.md §8.
	ErrCodeEIO = "EIO"

	// ErrCodeTooLarge: Directory listing exceeds the protocol's soft
	// threshold; caller must page. retryable=false.
	ErrCodeTooLarge = "E_TOO_LARGE"

	// ErrCodeEEXIST: Destination already exists (copy/move/mkdir/rename
	// without overwrite). retryable=false.
	ErrCodeEEXIST = "EEXIST"

	// ErrCodeENOSPC: Target volume is out of space. retryable=false.
	ErrCodeENOSPC = "ENOSPC"

	// ErrCodeSharingViolation: Windows-specific ERROR_SHARING_VIOLATION
	// (file is locked by another process). retryable=true.
	ErrCodeSharingViolation = "ERROR_SHARING_VIOLATION"

	// ErrCodeTrashUnavailable: Platform trash is disabled or unreachable;
	// caller must fall back to permanent delete. retryable=false.
	ErrCodeTrashUnavailable = "E_TRASH_UNAVAILABLE"

	// ErrCodeEINVAL: Argument validation failure (bad path chars, out of
	// range enum, etc.). retryable=false.
	ErrCodeEINVAL = "EINVAL"

	// ErrCodeNoHandler: No OS-registered application for the requested
	// file type (`open` op). retryable=false.
	ErrCodeNoHandler = "E_NO_HANDLER"

	// ErrCodePathRejected: safety.path rejected the input (traversal
	// attempt, unsupported root, allowlist miss). retryable=false.
	ErrCodePathRejected = "E_PATH_REJECTED"

	// ErrCodeSystemPathConfirmRequired: Mutating op targets a system
	// allowlist path without `explicitConfirm: true` (see
	// PROTOCOL.md §8 + SECURITY.md §5). retryable=false.
	ErrCodeSystemPathConfirmRequired = "E_SYSTEM_PATH_CONFIRM_REQUIRED"
)

// NewError builds an *ErrorPayload with the given code/message/retryable flag
// and no structured details. It exists so that handlers don't have to repeat
// the struct-literal boilerplate.
func NewError(code, message string, retryable bool) *ErrorPayload {
	return &ErrorPayload{
		Code:      code,
		Message:   message,
		Retryable: retryable,
	}
}

// NewErrorWithDetails is the Details-carrying variant of NewError. Callers
// should pass nil rather than an empty map when there is no context to attach
// (the JSON encoder will then omit the field entirely).
func NewErrorWithDetails(code, message string, retryable bool, details map[string]interface{}) *ErrorPayload {
	return &ErrorPayload{
		Code:      code,
		Message:   message,
		Retryable: retryable,
		Details:   details,
	}
}

// ErrorResponse builds a failed Response for the given request id. It is the
// canonical way to report a top-level dispatch failure (unknown op, bad JSON).
func ErrorResponse(id, code, message string, retryable bool) Response {
	return Response{
		ID:    id,
		OK:    false,
		Error: NewError(code, message, retryable),
	}
}

// ErrorResponseWithDetails is the Details-carrying variant of ErrorResponse,
// used when the top-level dispatch failure wants to surface structured
// context (e.g. hostMaxVersion on E_PROTOCOL mismatch).
func ErrorResponseWithDetails(id, code, message string, retryable bool, details map[string]interface{}) Response {
	return Response{
		ID:    id,
		OK:    false,
		Error: NewErrorWithDetails(code, message, retryable, details),
	}
}
