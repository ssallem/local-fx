//go:build darwin

package safety

// systemAllowlist on macOS covers the Apple-managed directories where
// direct writes can destabilise the OS or trip System Integrity Protection.
// User data lives in /Users, which is NOT on the list.
var systemAllowlist = []string{
	"/System",
	"/usr",
	"/Library",
	"/private",
	"/bin",
	"/sbin",
}

// isSystemPathOS is the Darwin implementation of IsSystemPath. Comparison
// is case-sensitive because the default APFS mount is case-sensitive-aware
// even if the filesystem itself is case-insensitive; we err on the side of
// strictness so "/usr/bin" is blocked but "/USR/bin" is not (the latter
// doesn't exist on a standard macOS install).
func isSystemPathOS(p string) bool {
	for _, prefix := range systemAllowlist {
		if hasPrefixBoundary(p, prefix, false) {
			return true
		}
	}
	return false
}
