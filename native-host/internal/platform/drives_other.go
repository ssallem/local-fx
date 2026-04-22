//go:build !windows && !darwin

package platform

// Stub implementation for any GOOS that is not windows or darwin. The host
// does not officially support Linux / BSD yet, but allowing the package to
// compile on those targets keeps `go build ./...` working inside developer
// Docker images and CI containers.
//
// Current is left nil so the top-level ListDrives helper returns
// ErrUnsupportedOS without an extra runtime.GOOS switch.
