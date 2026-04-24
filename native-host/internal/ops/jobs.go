// Package ops — jobs tracks in-flight cancelable operations.
//
// Streaming op handlers (copy, move, search) register their context.CancelFunc
// under the request ID on entry and unregister on exit. The separate `cancel`
// op (see cancel.go, PROTOCOL.md §6) looks up the ID and triggers cancellation
// out-of-band from the streaming handler's goroutine.
//
// A sync.Map is sufficient here: hot-path lookups happen only on the Cancel
// op (rare vs. progress events), and Register/Unregister occur once per
// streaming op lifetime. No tuning knobs needed.
package ops

import (
	"context"
	"sync"
)

// jobs maps Request.ID -> context.CancelFunc. Keyed by string, the value is
// stored as a context.CancelFunc but retrieved through LoadAndDelete so a
// racing Cancel cannot fire twice on the same func.
var jobs sync.Map

// RegisterJob records a cancel function under the job's request ID. Call
// this from the streaming op handler's goroutine once ctx is established.
// Re-registering the same id overwrites the previous entry (shouldn't happen
// with unique request IDs, but defensive).
func RegisterJob(id string, cancel context.CancelFunc) {
	jobs.Store(id, cancel)
}

// UnregisterJob removes the cancel function. Safe to call multiple times
// and safe to call even if CancelJob already consumed the entry.
func UnregisterJob(id string) {
	jobs.Delete(id)
}

// CancelJob invokes the cancel function for the given job ID and removes
// the entry atomically. Returns true if a job was found and canceled,
// false if no such ID was registered (stale cancel, race with completion).
func CancelJob(id string) bool {
	v, ok := jobs.LoadAndDelete(id)
	if !ok {
		return false
	}
	cancel, _ := v.(context.CancelFunc)
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

// ActiveJobCount returns the number of currently tracked jobs. Used by
// diagnostics and tests; not part of the wire protocol.
func ActiveJobCount() int {
	n := 0
	jobs.Range(func(_, _ any) bool { n++; return true })
	return n
}
