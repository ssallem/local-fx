// Package safety contains input-sanitisation helpers used before any op
// touches the filesystem.
//
// Phase 1 scope (read-only ops):
//   - reject empty / relative / null-byte-bearing paths
//   - collapse separators via filepath.Clean
//   - best-effort symlink resolution via filepath.EvalSymlinks (failures are
//     tolerated so that callers can still Lstat dangling symlinks and report
//     ENOENT themselves — EvalSymlinks on a broken link errors out, which is
//     the wrong place to surface the mismatch)
//
// Phase 2 (mutating ops) will layer allow-list checks on top of this; those
// checks deliberately do NOT belong here because Phase 1 read paths have no
// allow-list policy.
package safety

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrEmptyPath is returned by CleanPath when given "" or whitespace-only input.
var ErrEmptyPath = errors.New("safety: empty path")

// ErrNotAbsolute is returned by CleanPath when the input is not an absolute
// path. All ops require absolute paths: the host has no meaningful working
// directory for relative-path resolution.
var ErrNotAbsolute = errors.New("safety: path is not absolute")

// ErrNullByte is returned by CleanPath when the input contains a NUL byte.
// POSIX and Win32 APIs truncate at NUL, which makes NUL-bearing paths a
// classic vector for bypassing allow-list checks.
var ErrNullByte = errors.New("safety: path contains null byte")

// CleanPath validates p and returns a normalised absolute path.
//
// The returned value is suitable for passing directly to os.Stat / os.ReadDir.
// Symlinks are resolved on a best-effort basis: if EvalSymlinks succeeds, the
// resolved target is returned; if it fails (broken symlink, permission
// denied, etc.), the lexically cleaned path is returned instead so that the
// caller's Lstat can decide how to report the condition.
func CleanPath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", ErrEmptyPath
	}
	if strings.ContainsRune(p, 0) {
		return "", ErrNullByte
	}
	if !filepath.IsAbs(p) {
		return "", ErrNotAbsolute
	}
	cleaned := filepath.Clean(p)
	// Re-check for NUL after Clean just to be defence-in-depth; Clean itself
	// does not strip NUL but future refactors might introduce a transform.
	if strings.ContainsRune(cleaned, 0) {
		return "", ErrNullByte
	}
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		cleaned = resolved
	}
	return cleaned, nil
}
