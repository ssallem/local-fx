package platform

import "errors"

// ErrUnsupportedOS is returned by the top-level platform helpers when the
// binary was built for an OS that has no per-platform implementation wired
// into Current. Ops map this to protocol.ErrCodeEIO because it's an internal
// configuration error, not something the caller can fix by retrying.
var ErrUnsupportedOS = errors.New("platform: no implementation for this OS")

// ErrTrashUnavailable is returned by Trash when the platform's recycle bin
// is disabled or unreachable (Windows: GP policy, WSL2; macOS: user denied
// Finder automation permission). Callers translate it into
// protocol.ErrCodeTrashUnavailable so the UI can fall back to permanent
// delete with a second confirmation.
var ErrTrashUnavailable = errors.New("platform: trash is unavailable")

// ErrNoHandler is returned by OpenDefault when no OS-registered application
// can handle the file (Windows: ShellExecuteW returns SE_ERR_NOASSOC=31;
// macOS: `open` exits with code 1 and "no application knows how to open").
// Maps to protocol.ErrCodeNoHandler.
var ErrNoHandler = errors.New("platform: no default handler")
