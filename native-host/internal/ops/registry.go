// Package ops is the dispatch table that maps op names to handler functions.
//
// Handlers register themselves from init() via Register. The main loop calls
// Lookup for each incoming Request and invokes the returned handler. Unknown
// ops are reported with protocol.ErrCodeUnknownOp by the caller, not here.
package ops

import (
	"context"
	"sort"
	"sync"

	"local-fx-host/internal/protocol"
)

// Handler is the signature every op implements. It must never panic; any
// failure must be returned as a Response with OK=false and a populated Error.
// The ctx is passed through from main for future cancellation support.
type Handler func(ctx context.Context, req protocol.Request) protocol.Response

// StreamHandler is the signature for ops that emit mid-stream events
// (progress, item, done) before their final Response. It runs in its own
// goroutine and may call `emit` any number of times over its lifetime. The
// returned Response is the final frame on the wire; a successful streaming
// op returns Response{OK:true, Data: map[string]any{}} after emitting a
// "done" event, while a setup error (bad args, ENOENT on src) returns an
// Error Response WITHOUT emitting a "done" event. See PROTOCOL.md §6.
//
// The `emit` func returns the underlying WriteFrame error so the handler
// can abort early if the peer has disconnected; handlers typically ignore
// the return value because the next iteration's ctx.Err() check will
// short-circuit anyway.
type StreamHandler func(ctx context.Context, req protocol.Request, emit func(protocol.EventFrame) error) protocol.Response

var (
	mu             sync.RWMutex
	handlers       = map[string]Handler{}
	streamHandlers = map[string]StreamHandler{}
)

// Register associates op with h. Re-registering the same op overwrites the
// previous entry; this is intentional to simplify test setup.
func Register(op string, h Handler) {
	mu.Lock()
	defer mu.Unlock()
	handlers[op] = h
}

// Lookup returns the handler for op, or nil if none is registered.
func Lookup(op string) Handler {
	mu.RLock()
	defer mu.RUnlock()
	return handlers[op]
}

// RegisterStream associates op with a streaming handler h. An op may have
// BOTH a regular handler and a stream handler registered; the dispatcher
// picks based on Request.Stream. Re-registering overwrites.
func RegisterStream(op string, h StreamHandler) {
	mu.Lock()
	defer mu.Unlock()
	streamHandlers[op] = h
}

// LookupStream returns the stream handler for op, or nil if none is
// registered. The dispatcher falls back to E_UNKNOWN_OP when a request has
// Stream=true but no stream handler exists.
func LookupStream(op string) StreamHandler {
	mu.RLock()
	defer mu.RUnlock()
	return streamHandlers[op]
}

// Registered returns all registered op names in lexicographic order.
// Go map iteration is randomised, so we sort explicitly to give callers a
// deterministic view (used by diagnostics and tests). Stream-only ops are
// included under their op name.
func Registered() []string {
	mu.RLock()
	defer mu.RUnlock()
	seen := make(map[string]struct{}, len(handlers)+len(streamHandlers))
	for k := range handlers {
		seen[k] = struct{}{}
	}
	for k := range streamHandlers {
		seen[k] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for k := range seen {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func init() {
	// Phase 0/1 — read-only ops.
	Register("ping", Ping)
	Register("listDrives", ListDrives)
	Register("readdir", Readdir)
	Register("stat", Stat)
	// Phase 2.1 — mutating + OS-integration ops. See PROTOCOL.md §7.5–§7.11.
	Register("mkdir", Mkdir)
	Register("rename", Rename)
	Register("remove", Remove)
	Register("open", Open)
	Register("revealInOsExplorer", RevealInOsExplorer)
	// Phase 2.3 — streaming ops + out-of-band cancel. See PROTOCOL.md §6.
	RegisterStream("copy", Copy)
	Register("cancel", Cancel)
	// Phase 2.4 — move (same-volume rename with cross-volume copy-then-delete fallback).
	RegisterStream("move", Move)
	// T6 — opt-in update check (default OFF, gated extension-side; host
	// honours LOCALFX_DISABLE_UPDATE_CHECK=1 as defence-in-depth).
	Register("checkUpdate", CheckUpdate)
}
