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
// Phase 2 (mutating ops) layers allow-list checks on top. IsSystemPath and
// CheckMutatingOp are the public surface: mutating ops call
// CheckMutatingOp(cleanedPath, args.ExplicitConfirm) right after CleanPath.
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

// ErrSystemPathConfirmRequired is returned by CheckMutatingOp when the caller
// targets a system allowlist path without passing explicitConfirm=true.
// Handlers translate it into protocol.ErrCodeSystemPathConfirmRequired.
var ErrSystemPathConfirmRequired = errors.New("safety: system path requires explicit confirm")

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

// IsSystemPath reports whether p lies under any platform-specific system
// allowlist prefix (e.g. C:\Windows on Windows, /System on macOS). The
// comparison uses per-OS semantics (case-insensitive on Windows) and
// requires the allowlist prefix to end at a path-separator boundary so
// that "C:\ProgramData" does NOT incorrectly match "C:\ProgramDataX".
//
// Callers should pass a path that has already been run through CleanPath;
// IsSystemPath itself does a second filepath.Clean as a defence-in-depth.
func IsSystemPath(p string) bool {
	if p == "" {
		return false
	}
	return isSystemPathOS(filepath.Clean(p))
}

// CheckMutatingOp gates mutating ops against the system allowlist. When the
// path matches a system prefix and the caller did NOT set explicitConfirm,
// the call is rejected with ErrSystemPathConfirmRequired. All other cases
// return nil; callers combine this with mapFSError for OS-level failures.
func CheckMutatingOp(p string, explicitConfirm bool) error {
	if !IsSystemPath(p) {
		return nil
	}
	if explicitConfirm {
		return nil
	}
	return ErrSystemPathConfirmRequired
}

// hasPrefixBoundary returns true iff full == prefix or full starts with
// prefix + separator. It's used by per-OS isSystemPathOS implementations
// so that "C:\ProgramDataX" doesn't match the "C:\ProgramData" allowlist
// entry. eqFold controls case sensitivity (true on Windows, false on Unix).
func hasPrefixBoundary(full, prefix string, eqFold bool) bool {
	// Normalise trailing separators on the prefix so that callers can list
	// allowlist entries with or without them.
	prefix = strings.TrimRight(prefix, string(filepath.Separator))
	if eqFold {
		if strings.EqualFold(full, prefix) {
			return true
		}
		if len(full) > len(prefix) &&
			strings.EqualFold(full[:len(prefix)], prefix) &&
			full[len(prefix)] == filepath.Separator {
			return true
		}
		return false
	}
	if full == prefix {
		return true
	}
	if len(full) > len(prefix) && full[:len(prefix)] == prefix &&
		full[len(prefix)] == filepath.Separator {
		return true
	}
	return false
}
