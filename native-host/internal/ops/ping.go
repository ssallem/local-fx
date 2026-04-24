package ops

import (
	"context"
	"runtime"
	"time"

	"local-fx-host/internal/protocol"
	"local-fx-host/internal/version"
)

// Ping is a liveness/identity probe used during extension <-> host handshake.
// It intentionally takes no arguments so it can double as the smoke test in
// installation docs and CI integration checks.
//
// The response Data carries both the legacy fields (`version`, `os`) and the
// Phase 1 handshake trio (`hostVersion`, `hostMaxProtocolVersion`, `serverTs`)
// so older extensions keep working while new ones can negotiate properly.
// See PROTOCOL.md §7.1.
//
// Version metadata lives in internal/version (promoted out of this file in
// W1-4) so installer / log prefix / future health endpoints can reference it
// without importing ops.
func Ping(_ context.Context, req protocol.Request) protocol.Response {
	return protocol.Response{
		ID: req.ID,
		OK: true,
		Data: map[string]any{
			// Legacy fields — retained for backward compatibility.
			"pong":    true,
			"version": version.Version,
			"os":      runtime.GOOS,
			"arch":    runtime.GOARCH,
			// Phase 1 handshake fields (PROTOCOL.md §7.1).
			"hostVersion":            version.Version,
			"hostMaxProtocolVersion": version.MaxProtocolVersion,
			"serverTs":               time.Now().UnixMilli(),
		},
	}
}
