// Package platform declares the OS-abstraction interface that Phase 1+
// implementations (Windows via syscall + golang.org/x/sys/windows, macOS via
// syscall.Statfs_t + /Volumes scan) satisfy.
//
// Phase 1 wires up ListDrives only. Trash / OpenDefault / RevealInOS remain
// declarations and become Phase 2 once mutating ops are implemented.
package platform

import "context"

// Drive describes a mounted volume returned by ListDrives.
//
// Field shape mirrors PROTOCOL.md §7.2 exactly so that op handlers can return
// a slice of Drive without a second projection. The zero value of FSType is
// a valid wire output ("") — callers MUST NOT emit "unknown" as a sentinel.
type Drive struct {
	Path       string `json:"path"`                 // e.g. "C:\\" or "/Volumes/Foo" or "/"
	Label      string `json:"label"`                // volume label, may be empty
	FSType     string `json:"fsType"`               // "NTFS", "APFS", "exFAT", ...
	TotalBytes int64  `json:"totalBytes,omitempty"` // 0 when unavailable
	FreeBytes  int64  `json:"freeBytes,omitempty"`  // 0 when unavailable
	ReadOnly   bool   `json:"readOnly"`             // true for CD-ROM / read-only mounts
}

// OS abstracts the handful of platform-specific filesystem operations we need.
//
// The interface is deliberately narrow: anything that can be expressed with
// stdlib (os, io, filepath) should live elsewhere. This exists solely for
// operations that require calling native APIs on at least one target OS.
type OS interface {
	// ListDrives enumerates currently mounted volumes suitable for showing
	// in a drive picker.
	ListDrives(ctx context.Context) ([]Drive, error)

	// Trash moves path to the platform recycle bin / trash.
	// Must be atomic from the user's perspective (no partial moves).
	// Phase 2.
	Trash(ctx context.Context, path string) error

	// OpenDefault opens path with the OS's default handler (Explorer on
	// Windows, Finder/`open` on macOS). Phase 2.
	OpenDefault(ctx context.Context, path string) error

	// RevealInOS opens a file manager window focused on path. Phase 2.
	RevealInOS(ctx context.Context, path string) error
}

// Current is wired by a per-OS build-tagged file (platform_windows.go,
// platform_darwin.go). Callers should prefer the top-level ListDrives helper
// below rather than poking Current directly, so that unsupported-OS failures
// are handled uniformly.
var Current OS

// ListDrives is a thin wrapper that returns an error rather than panicking
// when Current is nil (unsupported GOOS build). Phase 1 ops call this.
func ListDrives(ctx context.Context) ([]Drive, error) {
	if Current == nil {
		return nil, ErrUnsupportedOS
	}
	return Current.ListDrives(ctx)
}

// Trash moves path to the platform recycle bin via the active OS impl.
// Returns ErrUnsupportedOS on platforms with no implementation,
// ErrTrashUnavailable when the trash is disabled/unreachable, or a
// native syscall error that mapFSError will classify.
func Trash(ctx context.Context, path string) error {
	if Current == nil {
		return ErrUnsupportedOS
	}
	return Current.Trash(ctx, path)
}

// OpenDefault launches path with the OS's default handler.
func OpenDefault(ctx context.Context, path string) error {
	if Current == nil {
		return ErrUnsupportedOS
	}
	return Current.OpenDefault(ctx, path)
}

// RevealInOS opens a file manager window focused on path.
func RevealInOS(ctx context.Context, path string) error {
	if Current == nil {
		return ErrUnsupportedOS
	}
	return Current.RevealInOS(ctx, path)
}
