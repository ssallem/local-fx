//go:build windows

package safety

// systemAllowlist is the set of Windows system directories that require
// explicit user confirmation for mutating ops (see SECURITY.md §5). The
// list is deliberately small: C:\Users is NOT included because that's
// where user documents live; callers should only see confirm prompts for
// genuinely dangerous paths.
//
// All entries are absolute paths with no trailing separator. Comparison
// in isSystemPathOS is case-insensitive (strings.EqualFold) so "c:\WINDOWS"
// and "C:\Windows" both match — Win32 file systems treat paths
// case-insensitively.
var systemAllowlist = []string{
	`C:\Windows`,
	`C:\Program Files`,
	`C:\Program Files (x86)`,
	`C:\ProgramData`,
}

// isSystemPathOS is the Windows implementation of IsSystemPath. It walks
// the allowlist and returns true on the first prefix-boundary match.
// Case-insensitive comparison is required because "C:\windows" and
// "C:\Windows" refer to the same directory on Win32.
func isSystemPathOS(p string) bool {
	for _, prefix := range systemAllowlist {
		if hasPrefixBoundary(p, prefix, true) {
			return true
		}
	}
	return false
}
