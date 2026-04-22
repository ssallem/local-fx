//go:build !windows && !darwin

package platform

import "context"

// Non-Windows/non-Darwin targets have no Trash/Open/Reveal implementation.
// The per-OS OS struct (if any) would call these if Current were wired.
// Since drives_other.go leaves Current == nil, the top-level helpers short
// circuit with ErrUnsupportedOS before reaching here — these stubs exist
// solely to satisfy any future build where Current IS wired for a partial
// Linux port.
func shellTrash(_ context.Context, _ string) error       { return ErrUnsupportedOS }
func shellOpenDefault(_ context.Context, _ string) error { return ErrUnsupportedOS }
func shellRevealInOS(_ context.Context, _ string) error  { return ErrUnsupportedOS }
