package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// UniqueName finds a free name in dir by appending " (1)", " (2)", ...
// e.g., "file.txt" -> "file (1).txt" if file.txt exists.
// Returns the full path to use (dir joined with the unique name).
// Errors on I/O failures other than "not exist"; returns an error if no free
// variant is found within a sane limit.
func UniqueName(dir, base string) (string, error) {
	candidate := filepath.Join(dir, base)
	if _, err := os.Lstat(candidate); err != nil {
		if os.IsNotExist(err) {
			return candidate, nil
		}
		return "", err
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; i < 10000; i++ {
		name := fmt.Sprintf("%s (%d)%s", stem, i, ext)
		candidate = filepath.Join(dir, name)
		if _, err := os.Lstat(candidate); err != nil {
			if os.IsNotExist(err) {
				return candidate, nil
			}
			return "", err
		}
	}
	return "", fmt.Errorf("exhausted unique name variants for %q", base)
}

// pathsEqual compares two cleaned paths for equality.
// On Windows the comparison is case-insensitive; elsewhere case-sensitive.
// Both inputs are assumed to have been passed through safety.CleanPath or
// filepath.Clean already.
func pathsEqual(a, b string) bool {
	if a == b {
		return true
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return false
}
