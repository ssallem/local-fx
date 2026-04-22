//go:build darwin

package platform

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
)

// macOS drive enumeration: always include "/" as the boot volume, then scan
// /Volumes/* for additional mounts (the boot volume is also symlinked there
// under its display name, so we de-duplicate by resolved path).
//
// We intentionally do NOT call `mount(2)` or `getmntinfo` (CGo) here so that
// cross-compilation remains trivial. Statfs_t plus the /Volumes scan covers
// >95% of user-visible mounts and matches what Finder shows in the sidebar.

type darwinOS struct{}

func init() {
	Current = darwinOS{}
}

// ListDrives enumerates "/" plus each entry under /Volumes. Symlinks at
// /Volumes (macOS creates one for the boot volume) are de-duplicated so the
// boot drive never appears twice.
func (darwinOS) ListDrives(_ context.Context) ([]Drive, error) {
	seen := make(map[string]bool)
	var out []Drive

	// Root volume first so pickers can default to it.
	if d, ok := statfsDrive("/"); ok {
		d.Label = volumeLabel("/", d.Label)
		seen[d.Path] = true
		out = append(out, d)
	}

	entries, err := os.ReadDir("/Volumes")
	if err != nil {
		// /Volumes should always exist; treat absence as fatal so the
		// caller sees EIO rather than a silently-truncated list.
		return out, err
	}
	for _, e := range entries {
		// Name-based label preserves diacritics (e.g. "Time Machine Backups")
		// that statfs might mangle on case-insensitive HFS+ sibling mounts.
		name := e.Name()
		mountPoint := filepath.Join("/Volumes", name)

		// Resolve symlinks so that the boot volume's /Volumes alias doesn't
		// create a duplicate entry.
		resolved, err := filepath.EvalSymlinks(mountPoint)
		if err != nil {
			continue
		}
		if seen[resolved] {
			continue
		}

		d, ok := statfsDrive(mountPoint)
		if !ok {
			continue
		}
		// Prefer the /Volumes display name over whatever statfs reports: on
		// macOS, the /Volumes entry name is the user-facing label.
		d.Label = name
		seen[d.Path] = true
		out = append(out, d)
	}
	return out, nil
}

func (darwinOS) Trash(ctx context.Context, path string) error {
	return shellTrash(ctx, path)
}
func (darwinOS) OpenDefault(ctx context.Context, path string) error {
	return shellOpenDefault(ctx, path)
}
func (darwinOS) RevealInOS(ctx context.Context, path string) error {
	return shellRevealInOS(ctx, path)
}

// statfsDrive populates the size/fs-type fields for a given mount point.
// Returns ok=false when the mount is inaccessible (permission denied on a
// networked share, etc.) so callers can skip it cleanly.
func statfsDrive(path string) (Drive, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Drive{Path: path}, false
	}
	d := Drive{
		Path:       path,
		FSType:     cStringToGo(st.Fstypename[:]),
		TotalBytes: int64(st.Blocks) * int64(st.Bsize),
		FreeBytes:  int64(st.Bavail) * int64(st.Bsize),
		ReadOnly:   st.Flags&syscall.MNT_RDONLY != 0,
	}
	return d, true
}

// volumeLabel returns the sidebar-friendly name for a mount point. We try
// the directory basename first (matches Finder) and fall back to the caller-
// supplied default when we're looking at "/".
func volumeLabel(path, fallback string) string {
	if path == "/" {
		// macOS exposes the boot volume name at /Volumes/<name>; surface a
		// stable default when that lookup hasn't happened yet.
		if fallback != "" {
			return fallback
		}
		return "Macintosh HD"
	}
	return filepath.Base(path)
}

// cStringToGo decodes a null-terminated []int8 / []uint8 (syscall differs by
// field) into a Go string. Statfs_t.Fstypename is declared as [16]int8 on
// Darwin so we take a byte view.
func cStringToGo(b []int8) string {
	bytes := make([]byte, 0, len(b))
	for _, c := range b {
		if c == 0 {
			break
		}
		bytes = append(bytes, byte(c))
	}
	return string(bytes)
}
