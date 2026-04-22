package safety

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// absExample returns a platform-appropriate absolute path fixture so the same
// test body covers both Windows (C:\foo) and POSIX (/tmp/foo).
func absExample(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return `C:\Windows`
	}
	return "/tmp"
}

func TestCleanPath_Empty(t *testing.T) {
	_, err := CleanPath("")
	if !errors.Is(err, ErrEmptyPath) {
		t.Fatalf("want ErrEmptyPath, got %v", err)
	}
}

func TestCleanPath_Whitespace(t *testing.T) {
	_, err := CleanPath("   ")
	if !errors.Is(err, ErrEmptyPath) {
		t.Fatalf("want ErrEmptyPath, got %v", err)
	}
}

func TestCleanPath_NullByte(t *testing.T) {
	_, err := CleanPath("/tmp\x00/foo")
	if !errors.Is(err, ErrNullByte) {
		t.Fatalf("want ErrNullByte, got %v", err)
	}
}

func TestCleanPath_Relative(t *testing.T) {
	_, err := CleanPath("foo/bar")
	if !errors.Is(err, ErrNotAbsolute) {
		t.Fatalf("want ErrNotAbsolute, got %v", err)
	}
}

func TestCleanPath_DotDot_AfterClean(t *testing.T) {
	// Absolute path containing ".." should be collapsed by filepath.Clean and
	// accepted (no policy against it for read ops in Phase 1).
	base := absExample(t)
	input := filepath.Join(base, "sub", "..")
	got, err := CleanPath(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != base && got != filepath.Clean(base) {
		// On Windows EvalSymlinks may canonicalize case; accept anything
		// that equals base after cleaning.
		if filepath.Clean(got) != filepath.Clean(base) {
			t.Errorf("expected %q, got %q", base, got)
		}
	}
}

func TestCleanPath_AbsoluteExistingDir(t *testing.T) {
	dir := t.TempDir()
	got, err := CleanPath(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// EvalSymlinks may resolve TempDir through a system symlink (e.g. macOS
	// /var -> /private/var). Accept either the raw or resolved form.
	resolved, _ := filepath.EvalSymlinks(dir)
	if got != dir && got != resolved {
		t.Errorf("unexpected path: got %q, want %q or %q", got, dir, resolved)
	}
}

func TestCleanPath_NonexistentAbsolute(t *testing.T) {
	// A path whose EvalSymlinks fails (doesn't exist) should still return a
	// cleaned absolute string — the actual existence check belongs to the
	// caller's Stat/ReadDir.
	base := absExample(t)
	missing := filepath.Join(base, "definitely-missing-xyz-"+t.Name())
	got, err := CleanPath(missing)
	if err != nil {
		t.Fatalf("want nil err, got %v", err)
	}
	if got != filepath.Clean(missing) {
		t.Errorf("got %q, want %q", got, filepath.Clean(missing))
	}
}

func TestCleanPath_SymlinkResolved(t *testing.T) {
	// Create tmp/real + tmp/link -> tmp/real, ensure CleanPath returns the
	// real target.
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		// Windows without Developer Mode cannot create symlinks; skip.
		t.Skipf("symlink unsupported on this platform: %v", err)
	}
	got, err := CleanPath(link)
	if err != nil {
		t.Fatalf("CleanPath: %v", err)
	}
	// real may itself resolve through a system symlink, so canonicalise.
	want, _ := filepath.EvalSymlinks(real)
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
