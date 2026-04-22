package ops

import (
	"context"
	"runtime"
	"time"

	"local-fx-host/internal/protocol"
)

// Version is the host binary version reported by the ping op. Bump on every
// released build so the extension can detect stale installs.
//
// TODO(phase1): promote to a dedicated `internal/version` package so non-op
// code (installer checks, log prefixes) can reference it without importing
// the ops package. Kept in ops/ for Phase 0 to minimise surface area.
const Version = "0.0.1"

// HostMaxProtocolVersion is the highest protocol version this host binary
// speaks. PROTOCOL.md §7.1 requires the ping response to advertise it so the
// extension can detect version skew at handshake time.
//
// Bumped to 2 for Phase 2.1: the host now understands mkdir/rename/remove/
// open/revealInOsExplorer. Extensions speaking PROTOCOL_VERSION=1 still work
// for the Phase 0/1 read-only op subset.
const HostMaxProtocolVersion = 2

// Ping is a liveness/identity probe used during extension <-> host handshake.
// It intentionally takes no arguments so it can double as the smoke test in
// installation docs and CI integration checks.
//
// The response Data carries both the legacy fields (`version`, `os`) and the
// Phase 1 handshake trio (`hostVersion`, `hostMaxProtocolVersion`, `serverTs`)
// so older extensions keep working while new ones can negotiate properly.
// See PROTOCOL.md §7.1.
func Ping(_ context.Context, req protocol.Request) protocol.Response {
	return protocol.Response{
		ID: req.ID,
		OK: true,
		Data: map[string]any{
			// Legacy fields — retained for backward compatibility.
			"pong":    true,
			"version": Version,
			"os":      runtime.GOOS,
			"arch":    runtime.GOARCH,
			// Phase 1 handshake fields (PROTOCOL.md §7.1).
			"hostVersion":            Version,
			"hostMaxProtocolVersion": HostMaxProtocolVersion,
			"serverTs":               time.Now().UnixMilli(),
		},
	}
}
