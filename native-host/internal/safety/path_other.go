//go:build !windows && !darwin

package safety

// systemAllowlist is empty on non-Windows/non-Darwin builds: we don't
// officially support Linux/BSD yet, but letting `go build ./...` work
// there keeps CI containers happy. No path is treated as a system path,
// so CheckMutatingOp becomes a no-op.
var systemAllowlist = []string{}

func isSystemPathOS(p string) bool {
	return false
}
