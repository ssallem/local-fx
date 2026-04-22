//go:build windows

package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"local-fx-host/internal/protocol"
)

// TestRename_SameDirCaseInsensitive covers the Windows case-insensitivity
// fix: callers who submit src and dst whose parent directories differ only
// in letter case (e.g. C:\Foo vs C:\foo) must not be rejected by the
// cross-directory guard, because NTFS treats those as the same directory.
func TestRename_SameDirCaseInsensitive(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "a.txt")
	if err := os.WriteFile(src, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	// Flip the case of the parent-directory component in dst so Dir(src)
	// and Dir(dst) are byte-different but refer to the same Windows dir.
	// We rely on t.TempDir returning a path with at least one letter in
	// its final segment; crash with a skip if the env violates that.
	parent, leaf := filepath.Split(base)
	if leaf == "" || strings.ToUpper(leaf) == strings.ToLower(leaf) {
		t.Skipf("temp dir leaf %q has no case-bearing letters", leaf)
	}
	flipped := filepath.Join(parent, flipCase(leaf))
	if flipped == base {
		t.Skip("case-flip produced identical string")
	}
	dst := filepath.Join(flipped, "b.txt")

	resp := Rename(context.Background(), renameReq(t, map[string]any{
		"src": src, "dst": dst,
	}))
	if !resp.OK {
		t.Fatalf("expected OK on case-only parent diff, got err=%+v", resp.Error)
	}
	// Either the case-flipped or original-case path should resolve to the
	// same renamed file on disk.
	if _, err := os.Stat(filepath.Join(base, "b.txt")); err != nil {
		t.Errorf("renamed file missing: %v", err)
	}
}

// TestRename_CrossDirRejected_Windows confirms that genuinely distinct
// parent directories (not just case differences) are still rejected on
// Windows. Guards against the EqualFold relaxation going too far.
func TestRename_CrossDirRejected_Windows(t *testing.T) {
	base := t.TempDir()
	subA := filepath.Join(base, "Alpha")
	subB := filepath.Join(base, "Bravo")
	for _, d := range []string{subA, subB} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	src := filepath.Join(subA, "x.txt")
	if err := os.WriteFile(src, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	dst := filepath.Join(subB, "x.txt")
	resp := Rename(context.Background(), renameReq(t, map[string]any{
		"src": src, "dst": dst,
	}))
	if resp.OK {
		t.Fatalf("expected OK=false across distinct dirs even on Windows")
	}
	if resp.Error.Code != protocol.ErrCodeEINVAL {
		t.Errorf("code: got %q want EINVAL", resp.Error.Code)
	}
}

// flipCase toggles the case of every ASCII letter in s. Good enough for a
// test helper that just wants a string that EqualFold-matches the input but
// is not byte-equal.
func flipCase(s string) string {
	b := []byte(s)
	for i := range b {
		switch {
		case b[i] >= 'a' && b[i] <= 'z':
			b[i] -= 'a' - 'A'
		case b[i] >= 'A' && b[i] <= 'Z':
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
