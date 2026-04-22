package platform

import "errors"

// ErrUnsupportedOS is returned by the top-level platform helpers when the
// binary was built for an OS that has no per-platform implementation wired
// into Current. Ops map this to protocol.ErrCodeEIO because it's an internal
// configuration error, not something the caller can fix by retrying.
var ErrUnsupportedOS = errors.New("platform: no implementation for this OS")
