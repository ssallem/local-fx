// Package version exposes the host binary version and supported protocol
// version. Kept in its own package (not internal/ops) so non-op callers
// (installer checks, main.go log prefixes, future health endpoints) can
// reference version metadata without pulling in the ops registry.
//
// Bumping requires cross-layer coordination:
//   - Host: change Version string here
//   - Host: bump MaxProtocolVersion when adding incompatible op changes
//   - Extension: update PROTOCOL_VERSION in src/ui/ipc.ts
//   - Docs: reflect the bump in docs/PROTOCOL.md §4/§7.1
package version

// Version is the semantic version of the native host binary. Bumped to
// 0.3.0 alongside the extension v0.3.0 release (T2 hybrid CI + T6 opt-in
// update check). Previous 0.0.2 was the Phase 2 read-write baseline.
const Version = "0.3.0"

// MaxProtocolVersion is the highest IPC protocol version this host supports.
// See docs/PROTOCOL.md §4 for handshake semantics. The ping op advertises
// this value so the extension can detect version skew on the first frame.
const MaxProtocolVersion = 2
