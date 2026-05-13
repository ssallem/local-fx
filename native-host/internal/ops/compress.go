package ops

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"local-fx-host/internal/protocol"
	"local-fx-host/internal/safety"
)

// CompressArgs follows mission 16 design.
//
// Paths is a non-empty list of absolute source paths (files and/or
// directories). DestDir is the absolute directory where the resulting
// archive lands. ArchiveName is optional — when empty, the handler
// auto-derives a name (see auto-naming rules below). Overwrite controls
// what happens when an archive of the same name already exists in DestDir:
// false (default) picks a "(N).zip" suffix via UniqueName; true clobbers
// the existing file by truncating on os.Create.
//
// Auto-naming:
//   - 1 path: filepath.Base(paths[0]) + ".zip" (or unchanged if the basename
//     already ends in .zip — defends against doubled extensions on a folder
//     literally called "x.zip").
//   - >=2 paths: "archive_YYYYMMDD_HHMMSS.zip" with the local time at the
//     moment of name resolution.
type CompressArgs struct {
	Paths           []string `json:"paths"`
	DestDir         string   `json:"destDir"`
	ArchiveName     string   `json:"archiveName,omitempty"`
	Overwrite       bool     `json:"overwrite,omitempty"`
	ExplicitConfirm bool     `json:"explicitConfirm,omitempty"`
}

// compressMaxArchiveName mirrors the typical Windows path component limit
// (255 chars) so that auto-named archives can't exceed what the filesystem
// will accept downstream. The check fires for caller-supplied names only;
// auto-naming derives from sources that already passed CleanPath.
const compressMaxArchiveName = 255

// Compress is the streaming handler for the "compress" op. Emission contract
// mirrors Copy:
//
//  1. On success: zero or more "progress" events, then a single "done" event
//     (carrying per-entry failures, if any), then a final Response{OK:true,
//     Data:{archivePath: "..."}}.
//  2. On cancel: zero or more "progress" events, a "done" event with
//     canceled=true, then Response{OK:true, Data:{}}. The partially-written
//     zip is removed.
//  3. On setup error (bad args, ENOENT on paths[0], rejected destDir, etc.):
//     the final Response carries Error and NO "done" event is emitted, again
//     matching the Copy contract.
//
// Per-entry strategy:
//   - EACCES, ERROR_SHARING_VIOLATION (Windows 32), and ENOENT (file
//     disappeared mid-walk) are accumulated into DonePayload.Failures and
//     the walk continues. Any other write error is fatal: the partially
//     written zip is removed and an Error Response is returned.
//   - Non-regular files (symlinks, devices, sockets) are skipped with a
//     FailureInfo entry, the same pattern recursiveCopy uses.
//
// rel computation: each top-level path's basename becomes the root entry
// in the archive. So compress paths=["C:\a\foo", "C:\b\bar.txt"] produces
// entries "foo/..." and "bar.txt". This prevents collisions when two
// sources share a parent directory and keeps the archive structure intuitive
// when extracting (one folder per source).
func Compress(ctx context.Context, req protocol.Request, emit func(protocol.EventFrame) error) protocol.Response {
	var args CompressArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			return protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
				"invalid args: "+err.Error(), false)
		}
	}

	if len(args.Paths) == 0 {
		return protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
			"paths must not be empty", false)
	}

	// Validate + clean each source path.
	cleanedPaths := make([]string, 0, len(args.Paths))
	for _, p := range args.Paths {
		cp, err := safety.CleanPath(p)
		if err != nil {
			return wrapSafetyErr(req.ID, err)
		}
		cleanedPaths = append(cleanedPaths, cp)
	}

	destClean, err := safety.CleanPath(args.DestDir)
	if err != nil {
		return wrapSafetyErr(req.ID, err)
	}
	if cerr := safety.CheckMutatingOp(destClean, args.ExplicitConfirm); cerr != nil {
		return wrapSafetyErr(req.ID, cerr)
	}

	// archiveName validation. Empty means "auto-name". Anything else must
	// not contain separators or NUL, and must fit the filesystem's name limit.
	archiveName := args.ArchiveName
	if archiveName != "" {
		if strings.ContainsAny(archiveName, "\x00/\\") {
			return protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
				"archiveName must not contain path separators or NUL", false)
		}
		if len(archiveName) > compressMaxArchiveName {
			return protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
				"archiveName exceeds 255 character limit", false)
		}
	}

	// Sort + dedupe parent-child collisions. After CleanPath the paths are
	// absolute, so isSubPath gives us case-correct nesting checks. We
	// preserve the original input order for stable error reporting by
	// remembering the first-seen index when collapsing duplicates.
	deduped := dedupeSubPaths(cleanedPaths)

	// Guard: destDir must not lie inside any source path. Compressing into
	// a child of a source would mean we'd be writing the archive into the
	// tree we are about to walk.
	for _, p := range deduped {
		if isSubPath(p, destClean) {
			return protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
				"destDir must not be inside a source path", false)
		}
	}

	// paths[0] ENOENT is treated as a fatal setup error per spec; later
	// paths can fail during walk and contribute to FailureInfo instead.
	if _, lerr := os.Lstat(deduped[0]); lerr != nil {
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(lerr)}
	}

	// Auto-name resolution. Done before pre-walk so an early caller error
	// (e.g. destDir doesn't exist on first os.Create) surfaces cheaply.
	if archiveName == "" {
		archiveName = autoArchiveName(deduped)
	}
	archivePath := filepath.Join(destClean, archiveName)
	if !args.Overwrite {
		unique, uerr := UniqueName(destClean, archiveName)
		if uerr != nil {
			return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(uerr)}
		}
		archivePath = unique
	}

	// Guard: archivePath must not lie inside any source path. This catches
	// the case where destDir == source root or destDir is a sibling that
	// would still cause UniqueName to land inside.
	for _, p := range deduped {
		if isSubPath(p, archivePath) {
			return protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
				"destination archive would be inside a source path", false)
		}
	}

	// Pre-walk: tally bytesTotal and fileTotal so progress payloads carry
	// meaningful denominators. Pre-walk failures (e.g. EACCES on a subtree
	// we can't even stat) abort the whole op — at this stage we have not
	// written anything to disk yet, so the failure mode is clean.
	var bytesTotal int64
	var fileTotal int
	for _, p := range deduped {
		if walkErr := filepath.WalkDir(p, func(path string, d fs.DirEntry, werr error) error {
			if werr != nil {
				return werr
			}
			if d.IsDir() {
				return nil
			}
			info, ierr := d.Info()
			if ierr != nil {
				return ierr
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			bytesTotal += info.Size()
			fileTotal++
			return nil
		}); walkErr != nil {
			// Tolerate "vanished mid-walk" on non-leading paths the same way
			// the main walk will: surface them as failures rather than abort.
			// But the spec says pre-walk failure is fatal, so any error here
			// (including for the first path) is fatal.
			return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(walkErr)}
		}
	}

	// Create the output archive. os.Create truncates if a file already exists,
	// which is exactly the semantics we want for overwrite=true and is
	// harmless for overwrite=false (UniqueName already picked a fresh path).
	file, ferr := os.Create(archivePath)
	if ferr != nil {
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(ferr)}
	}
	zw := zip.NewWriter(file)

	// cleanup closes the zip writer + file and deletes the partial archive.
	// Used on cancel and on any fatal error; safe to call multiple times
	// (Close on an already-closed *os.File returns an error we discard).
	cleanup := func() {
		_ = zw.Close()
		_ = file.Close()
		_ = os.Remove(archivePath)
	}

	prog := &copyProgress{
		bytesTotal:  bytesTotal,
		fileTotal:   fileTotal,
		windowStart: time.Now(),
	}

	var failures []protocol.FailureInfo
	var canceled bool

	for _, root := range deduped {
		rootBase := filepath.Base(root)
		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
			select {
			case <-ctx.Done():
				canceled = true
				return filepath.SkipAll
			default:
			}

			if werr != nil {
				// Mid-walk descent failure: degrade to a per-entry failure
				// when the error is one of the soft codes; otherwise abort.
				if isSoftWalkErr(werr) {
					failures = append(failures, protocol.FailureInfo{
						Path:    path,
						Code:    mapFSError(werr).Code,
						Message: werr.Error(),
					})
					if d != nil && d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				return werr
			}

			// Defensive: never include the archive we are currently writing.
			// The setup guard above should have rejected this, but Windows
			// case-insensitivity + UniqueName interleaving makes a defence-
			// in-depth cheap.
			if archivePathEqual(path, archivePath) {
				return nil
			}

			// Compute the archive-relative path. The root itself becomes
			// rootBase; children get rootBase + "/" + filepath.Rel(root, path).
			var rel string
			if path == root {
				rel = rootBase
			} else {
				r, rerr := filepath.Rel(root, path)
				if rerr != nil {
					failures = append(failures, protocol.FailureInfo{
						Path:    path,
						Code:    protocol.ErrCodeEINVAL,
						Message: rerr.Error(),
					})
					return nil
				}
				rel = filepath.Join(rootBase, r)
			}
			entryName := strings.ReplaceAll(rel, "\\", "/")

			info, ierr := d.Info()
			if ierr != nil {
				if isSoftWalkErr(ierr) {
					failures = append(failures, protocol.FailureInfo{
						Path:    path,
						Code:    mapFSError(ierr).Code,
						Message: ierr.Error(),
					})
					return nil
				}
				return ierr
			}

			if d.IsDir() {
				// Emit a directory entry so unzippers can preserve empty
				// folders. Zip convention: trailing slash signals a dir.
				header := &zip.FileHeader{
					Name:     entryName + "/",
					Method:   zip.Store,
					Modified: info.ModTime(),
				}
				header.Flags |= 0x800 // UTF-8 filename
				header.SetMode(info.Mode())
				if _, werr := zw.CreateHeader(header); werr != nil {
					cleanup()
					return werr
				}
				return nil
			}

			if !info.Mode().IsRegular() {
				failures = append(failures, protocol.FailureInfo{
					Path:    path,
					Code:    protocol.ErrCodeEINVAL,
					Message: "non-regular file skipped",
				})
				return nil
			}

			// Open source. Per-file soft errors degrade to FailureInfo +
			// continue; anything else is fatal.
			src, oerr := os.Open(path)
			if oerr != nil {
				if isSoftWalkErr(oerr) {
					failures = append(failures, protocol.FailureInfo{
						Path:    path,
						Code:    mapFSError(oerr).Code,
						Message: oerr.Error(),
					})
					return nil
				}
				cleanup()
				return oerr
			}

			header := &zip.FileHeader{
				Name:     entryName,
				Method:   zip.Deflate,
				Modified: info.ModTime(),
			}
			header.Flags |= 0x800
			header.SetMode(info.Mode())
			w, herr := zw.CreateHeader(header)
			if herr != nil {
				_ = src.Close()
				cleanup()
				return herr
			}

			// Stream the file. We keep a small local buffer so per-iteration
			// cancel checks fire reasonably often without paying per-byte
			// syscall overhead. 64KB matches copy.go's choice.
			buf := make([]byte, copyBufSize)
			var copyErr error
			for {
				select {
				case <-ctx.Done():
					_ = src.Close()
					canceled = true
					return filepath.SkipAll
				default:
				}

				n, rerr := src.Read(buf)
				if n > 0 {
					if _, werr := w.Write(buf[:n]); werr != nil {
						copyErr = werr
						break
					}
					prog.bytesDone += int64(n)
					prog.windowBytes += int64(n)
				}
				if rerr == io.EOF {
					break
				}
				if rerr != nil {
					copyErr = rerr
					break
				}

				// Debounced progress emit. Same shape as copy.go so the
				// extension's progress UI doesn't need a separate parser.
				if now := time.Now(); now.Sub(prog.lastEmit) >= progressInterval {
					rate := 0.0
					if dur := now.Sub(prog.windowStart).Seconds(); dur > 0 {
						rate = float64(prog.windowBytes) / dur
					}
					_ = emit(protocol.EventFrame{
						ID:    req.ID,
						Event: "progress",
						Payload: protocol.ProgressPayload{
							BytesDone:   prog.bytesDone,
							BytesTotal:  prog.bytesTotal,
							FileDone:    prog.fileDone,
							FileTotal:   prog.fileTotal,
							CurrentPath: path,
							Rate:        rate,
						},
					})
					prog.lastEmit = now
					prog.windowStart = now
					prog.windowBytes = 0
				}
			}
			_ = src.Close()

			if copyErr != nil {
				if isSoftWalkErr(copyErr) {
					failures = append(failures, protocol.FailureInfo{
						Path:    path,
						Code:    mapFSError(copyErr).Code,
						Message: copyErr.Error(),
					})
					return nil
				}
				cleanup()
				return copyErr
			}

			prog.fileDone++
			prog.currentPath = path
			return nil
		})

		if canceled {
			break
		}
		if walkErr != nil && !errors.Is(walkErr, filepath.SkipAll) {
			cleanup()
			return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(walkErr)}
		}
	}

	if canceled {
		cleanup()
		_ = emit(protocol.EventFrame{
			ID:      req.ID,
			Event:   "done",
			Payload: protocol.DonePayload{Canceled: true},
		})
		return protocol.SuccessResponse(req.ID, map[string]any{})
	}

	if cerr := zw.Close(); cerr != nil {
		_ = file.Close()
		_ = os.Remove(archivePath)
		return protocol.ErrorResponse(req.ID, protocol.ErrCodeArchiveFailed,
			"zip finalisation failed: "+cerr.Error(), false)
	}
	if cerr := file.Close(); cerr != nil {
		_ = os.Remove(archivePath)
		return protocol.ErrorResponse(req.ID, protocol.ErrCodeArchiveFailed,
			"archive close failed: "+cerr.Error(), false)
	}

	_ = emit(protocol.EventFrame{
		ID:      req.ID,
		Event:   "done",
		Payload: protocol.DonePayload{Failures: failures},
	})
	return protocol.SuccessResponse(req.ID, map[string]any{"archivePath": archivePath})
}

// dedupeSubPaths drops any path that lies under another path in the same list.
// E.g. ["C:\\a", "C:\\a\\sub"] collapses to ["C:\\a"]. The result preserves
// the relative ordering of survivors so the first valid path remains paths[0]
// (which the caller relies on for the leading-ENOENT check).
//
// The implementation sorts a working copy by length to ensure the shorter
// (parent) candidate appears first when checking, then filters in the
// original order against the set of accepted parents.
func dedupeSubPaths(in []string) []string {
	if len(in) <= 1 {
		// Even for len=1, return a fresh slice so callers don't accidentally
		// alias the caller's input.
		out := make([]string, len(in))
		copy(out, in)
		return out
	}
	// Sort a copy ascending by length — that way isSubPath(parent, child)
	// receives strict parents before their descendants.
	sorted := make([]string, len(in))
	copy(sorted, in)
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i]) < len(sorted[j])
	})

	accepted := make([]string, 0, len(sorted))
	for _, p := range sorted {
		shadowed := false
		for _, parent := range accepted {
			if isSubPath(parent, p) {
				shadowed = true
				break
			}
		}
		if !shadowed {
			accepted = append(accepted, p)
		}
	}
	// Now reconstruct output in the caller's original order, retaining
	// only the survivors. Use a set for O(1) membership.
	keep := make(map[string]struct{}, len(accepted))
	for _, p := range accepted {
		keep[p] = struct{}{}
	}
	out := make([]string, 0, len(accepted))
	for _, p := range in {
		if _, ok := keep[p]; ok {
			out = append(out, p)
			delete(keep, p) // guard against duplicate inputs
		}
	}
	return out
}

// autoArchiveName picks the archive filename when the caller didn't supply
// one. See the CompressArgs comment for the rule set.
func autoArchiveName(paths []string) string {
	if len(paths) == 1 {
		base := filepath.Base(paths[0])
		if strings.HasSuffix(strings.ToLower(base), ".zip") {
			return base
		}
		return base + ".zip"
	}
	return "archive_" + time.Now().Local().Format("20060102_150405") + ".zip"
}

// archivePathEqual compares a walk path against the in-progress archive
// path. Windows compares case-insensitively to match the filesystem; other
// platforms use exact equality.
func archivePathEqual(a, b string) bool {
	if a == b {
		return true
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return false
}

// isSoftWalkErr reports whether err is one of the "skip this entry, keep
// going" failures during compression. Hard errors (e.g. ENOSPC) abort the
// whole op and clean up the partial archive.
func isSoftWalkErr(err error) bool {
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EACCES) {
		return true
	}
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOENT) {
		return true
	}
	// Windows ERROR_SHARING_VIOLATION (32). We can't import the Win32
	// constant on non-Windows builds, so compare by numeric Errno value.
	var errno syscall.Errno
	if errors.As(err, &errno) {
		if errno == 32 {
			return true
		}
	}
	return false
}
