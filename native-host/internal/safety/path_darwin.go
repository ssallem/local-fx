//go:build darwin

package safety

// systemAllowlist on macOS covers the Apple-managed directories where
// direct writes can destabilise the OS or trip System Integrity Protection.
// User data lives in /Users, which is NOT on the list.
//
// /Applications is included because third-party app bundles there are not
// SIP-protected: mutating a *.app's Contents/MacOS or Contents/Resources
// breaks code signing and leaves the app in an unlaunchable state. This is
// the macOS equivalent of C:\Program Files on Windows. The explicitConfirm
// two-step gate in CheckMutatingOp still lets power users proceed deliberately.
var systemAllowlist = []string{
	"/System",
	"/usr",
	"/Library",
	"/private",
	"/bin",
	"/sbin",
	"/Applications",
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
