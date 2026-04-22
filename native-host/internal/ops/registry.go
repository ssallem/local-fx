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

var (
	mu        sync.RWMutex
	handlers  = map[string]Handler{}
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

// Registered returns all registered op names in lexicographic order.
// Go map iteration is randomised, so we sort explicitly to give callers a
// deterministic view (used by diagnostics and tests).
func Registered() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(handlers))
	for k := range handlers {
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
}
