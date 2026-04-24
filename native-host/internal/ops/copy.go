package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"local-fx-host/internal/protocol"
	"local-fx-host/internal/safety"
)

// CopyArgs follows PROTOCOL.md §6/§7. Phase 2.4 scope:
//
//   - single file copy
//   - directory recursive copy
//   - conflict strategies: "overwrite" | "skip" | "rename"
//     ("prompt" must be resolved by the UI via pre-scan — sending
//     conflict="prompt" to the host is a bug and returns E_BAD_REQUEST.)
//
// Overwrite is a legacy boolean flag kept for API parity with Phase 2.1
// semantics. If Conflict is empty, Overwrite=true is interpreted as
// conflict="overwrite" and Overwrite=false as conflict="skip".
type CopyArgs struct {
	Src             string `json:"src"`
	Dst             string `json:"dst"`
	Overwrite       bool   `json:"overwrite,omitempty"`
	ExplicitConfirm bool   `json:"explicitConfirm,omitempty"`
	Conflict        string `json:"conflict,omitempty"` // "overwrite"|"skip"|"rename"
}

// copyBufSize is the per-iteration read buffer. 64KB is a good fit for
// Windows pipe/IO semantics: large enough to keep syscall overhead low on
// multi-MB files, small enough that a cancel check fires within a few ms
// on any modern SSD. Matches the default io.Copy chunk (which we avoid so
// we can thread progress+cancel through the loop).
const copyBufSize = 64 * 1024

// progressInterval is the debounce window for progress events. Smaller
// means smoother UI but higher wire overhead; 100ms lines up with what
// humans perceive as "instant" and keeps even 1GB copies under ~10k
// frames on a fast disk.
const progressInterval = 100 * time.Millisecond

// resolveConflictStrategy normalises the CopyArgs conflict/overwrite fields
// into one of {"overwrite","skip","rename"} or returns an error Response if
// the pair is invalid. "prompt" is explicitly rejected (see PROTOCOL.md §7.10:
// UI is responsible for pre-scan resolution).
func resolveConflictStrategy(reqID string, args CopyArgs) (string, *protocol.Response) {
	strategy := args.Conflict
	if strategy == "" {
		if args.Overwrite {
			strategy = "overwrite"
		} else {
			strategy = "skip"
		}
	}
	switch strategy {
	case "overwrite", "skip", "rename":
		return strategy, nil
	case "prompt":
		r := protocol.ErrorResponse(reqID, protocol.ErrCodeBadRequest,
			`conflict="prompt" must be resolved by UI before sending`, false)
		return "", &r
	default:
		r := protocol.ErrorResponse(reqID, protocol.ErrCodeBadRequest,
			`conflict must be "overwrite", "skip", or "rename"`, false)
		return "", &r
	}
}

// Copy is the streaming handler for the "copy" op. Emission contract:
//
//  1. On success: zero or more "progress" events, then one "done" event
//     with an empty payload (or failures list for recursive copies with
//     per-entry skips), then the final Response{OK:true, Data:{}}.
//  2. On cancel: zero or more "progress" events, then one "done" event
//     with canceled=true, then the final Response{OK:true, Data:{}}
//     (cancellation is a normal termination, not an error).
//  3. On setup error (bad args, ENOENT, EEXIST+skip for a single file,
//     EACCES): NO "done" event; the final Response carries Error.
//
// Atomicity (per-file): each byte stream lands in a unique `.<base>.part-<reqID>-*`
// temp file created by os.CreateTemp in the same directory as the final dst,
// then os.Rename onto dst. On any error (including cancel), the temp file is
// removed so callers never see a half-written destination.
func Copy(ctx context.Context, req protocol.Request, emit func(protocol.EventFrame) error) protocol.Response {
	var args CopyArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			return protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
				"invalid args: "+err.Error(), false)
		}
	}

	srcClean, err := safety.CleanPath(args.Src)
	if err != nil {
		return wrapSafetyErr(req.ID, err)
	}
	dstClean, err := safety.CleanPath(args.Dst)
	if err != nil {
		return wrapSafetyErr(req.ID, err)
	}
	// Copy mutates the destination (it creates/overwrites files there), so
	// the system-path gate is checked against dst only. Reading from a
	// system path into a user dir is a legitimate use case (e.g.
	// exporting a log) and does not require explicitConfirm.
	if err := safety.CheckMutatingOp(dstClean, args.ExplicitConfirm); err != nil {
		return wrapSafetyErr(req.ID, err)
	}

	strategy, errResp := resolveConflictStrategy(req.ID, args)
	if errResp != nil {
		return *errResp
	}

	srcInfo, err := os.Lstat(srcClean)
	if err != nil {
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}
	}

	// Cancel registration lives in the caller (runStreamHandler in main.go
	// or the integration harness). We simply use the ctx we were handed —
	// this avoids a double RegisterJob race where a late UnregisterJob from
	// the handler could wipe an entry the dispatcher has already re-used
	// for a different in-flight request.
	opCtx := ctx

	if srcInfo.IsDir() {
		// Reject obvious recursion: dst cannot be inside src, or src itself.
		if isSubPath(srcClean, dstClean) {
			return protocol.ErrorResponse(req.ID, protocol.ErrCodeEINVAL,
				"destination is inside source directory", false)
		}

		failures, canceled, fatal := recursiveCopy(opCtx, srcClean, dstClean, strategy, emit, req.ID)
		if canceled {
			_ = emit(protocol.EventFrame{
				ID:      req.ID,
				Event:   "done",
				Payload: protocol.DonePayload{Canceled: true},
			})
			return protocol.SuccessResponse(req.ID, map[string]any{})
		}
		if fatal != nil {
			return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(fatal)}
		}
		_ = emit(protocol.EventFrame{
			ID:      req.ID,
			Event:   "done",
			Payload: protocol.DonePayload{Failures: failures},
		})
		return protocol.SuccessResponse(req.ID, map[string]any{})
	}

	if !srcInfo.Mode().IsRegular() {
		return protocol.ErrorResponse(req.ID, protocol.ErrCodeEINVAL,
			"only regular files or directories can be copied", false)
	}

	// Single-file path: conflict handling.
	finalDst := dstClean
	if info, lerr := os.Lstat(dstClean); lerr == nil {
		switch strategy {
		case "skip":
			return protocol.ErrorResponse(req.ID, protocol.ErrCodeEEXIST,
				"destination already exists", false)
		case "rename":
			dir := filepath.Dir(dstClean)
			base := filepath.Base(dstClean)
			unique, uerr := UniqueName(dir, base)
			if uerr != nil {
				return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(uerr)}
			}
			finalDst = unique
		case "overwrite":
			// proceed; clobber happens after temp rename.
			_ = info
		}
	} else if !errors.Is(lerr, fs.ErrNotExist) {
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(lerr)}
	}

	progress := &copyProgress{
		bytesTotal:  srcInfo.Size(),
		fileTotal:   1,
		currentPath: finalDst,
	}
	progress.windowStart = time.Now()

	if err := copySingleFile(opCtx, srcClean, finalDst, req.ID, strategy, progress, emit); err != nil {
		if errors.Is(err, errCopyCanceled) {
			_ = emit(protocol.EventFrame{
				ID:      req.ID,
				Event:   "done",
				Payload: protocol.DonePayload{Canceled: true},
			})
			return protocol.SuccessResponse(req.ID, map[string]any{})
		}
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}
	}

	// Final done event with empty payload = clean success.
	_ = emit(protocol.EventFrame{
		ID:      req.ID,
		Event:   "done",
		Payload: protocol.DonePayload{},
	})
	return protocol.SuccessResponse(req.ID, map[string]any{})
}

// errCopyCanceled is an internal sentinel returned by copySingleFile when
// the supplied context was canceled mid-stream. Callers of the per-file
// helper translate it into the appropriate wire response (SuccessResponse
// with DonePayload{Canceled:true}) — inside the recursive walker it simply
// aborts the walk.
var errCopyCanceled = errors.New("copy canceled")

// copyProgress is a tiny state bag threaded through copySingleFile and
// recursiveCopy so that per-file helpers can update the shared totals /
// debounce window without a long parameter list.
type copyProgress struct {
	bytesDone   int64
	bytesTotal  int64
	fileDone    int
	fileTotal   int
	currentPath string
	lastEmit    time.Time
	windowStart time.Time
	windowBytes int64
}

// copySingleFile streams one regular file from src to dst using the
// temp+rename atomic dance. It updates prog in place and emits debounced
// progress events via emit. Returns errCopyCanceled iff ctx has been
// canceled (caller is responsible for emitting the terminal done/canceled
// frame); any other error is a real failure.
func copySingleFile(
	ctx context.Context,
	src, dst, reqID, strategy string,
	prog *copyProgress,
	emit func(protocol.EventFrame) error,
) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// CRITICAL-P2-R4-3 — use os.CreateTemp so the stdlib guarantees a
	// collision-free temp path. Pathological recursive walks where two
	// subtrees share a basename used to race on the fixed
	// ".part-<reqID>" name and exhaust the 100-iteration fallback. The
	// temp lives in the same directory as the final destination so the
	// os.Rename below stays an intra-directory (atomic) operation.
	dir := filepath.Dir(dst)
	base := filepath.Base(dst)
	pattern := fmt.Sprintf(".%s.part-%s-*", base, reqID)
	tmpFile, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	cleanupTemp := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}

	buf := make([]byte, copyBufSize)

	for {
		select {
		case <-ctx.Done():
			cleanupTemp()
			return errCopyCanceled
		default:
		}

		n, rerr := srcFile.Read(buf)
		if n > 0 {
			if _, werr := tmpFile.Write(buf[:n]); werr != nil {
				cleanupTemp()
				return werr
			}
			prog.bytesDone += int64(n)
			prog.windowBytes += int64(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			cleanupTemp()
			return rerr
		}

		// Debounced progress emit. Rate is instantaneous over the last
		// window (not a moving average) so the UI can show the current
		// throughput rather than a cumulative figure skewed by startup
		// overhead.
		if now := time.Now(); now.Sub(prog.lastEmit) >= progressInterval {
			rate := 0.0
			if dur := now.Sub(prog.windowStart).Seconds(); dur > 0 {
				rate = float64(prog.windowBytes) / dur
			}
			_ = emit(protocol.EventFrame{
				ID:    reqID,
				Event: "progress",
				Payload: protocol.ProgressPayload{
					BytesDone:   prog.bytesDone,
					BytesTotal:  prog.bytesTotal,
					FileDone:    prog.fileDone,
					FileTotal:   prog.fileTotal,
					CurrentPath: prog.currentPath,
					Rate:        rate,
				},
			})
			prog.lastEmit = now
			prog.windowStart = now
			prog.windowBytes = 0
		}
	}

	// Flush and close the temp file BEFORE renaming. On Windows the Rename
	// will fail with ERROR_SHARING_VIOLATION if any handle is still open.
	if err := tmpFile.Sync(); err != nil {
		cleanupTemp()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	// Final cancel check before the atomic swap.
	select {
	case <-ctx.Done():
		_ = os.Remove(tmpPath)
		return errCopyCanceled
	default:
	}

	// For the "overwrite" strategy on an existing dst, delete the old dst
	// just before the rename. On Unix Rename silently replaces; on Windows
	// it fails with ERROR_ALREADY_EXISTS unless we Remove first.
	if strategy == "overwrite" {
		if _, lerr := os.Lstat(dst); lerr == nil {
			if rerr := os.Remove(dst); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
				_ = os.Remove(tmpPath)
				return rerr
			}
		}
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// recursiveCopy walks a source tree and copies every regular file into dst,
// preserving the directory structure. Returns any per-entry failures it
// encountered (rather than aborting), unless fatal is non-nil.
//
// Walk ordering: directories are created first (so child files can land
// inside them), then files are copied. symlinks and non-regular files are
// skipped with a FailureInfo entry.
//
// Conflict handling:
//   - dst file exists + strategy=overwrite → copied with temp+rename clobber
//   - dst file exists + strategy=skip      → FailureInfo{EEXIST}, continue
//   - dst file exists + strategy=rename    → UniqueName(dir, base) target
//   - dst dir exists (any strategy)        → reused in place (no error)
func recursiveCopy(
	ctx context.Context,
	src, dst string,
	strategy string,
	emit func(protocol.EventFrame) error,
	jobID string,
) (failures []protocol.FailureInfo, canceled bool, fatal error) {
	// Pre-walk: count total bytes + files so the progress event's
	// bytesTotal/fileTotal are meaningful.
	var bytesTotal int64
	var fileTotal int
	_ = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // tolerate unreadable subtrees; actual walk will re-surface
		}
		if d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		if info.Mode().IsRegular() {
			bytesTotal += info.Size()
			fileTotal++
		}
		return nil
	})

	prog := &copyProgress{
		bytesTotal:  bytesTotal,
		fileTotal:   fileTotal,
		windowStart: time.Now(),
	}

	// We walk src and mirror its shape into dst. Directories are created
	// eagerly so that subsequent file copies succeed.
	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, werr error) error {
		select {
		case <-ctx.Done():
			canceled = true
			return filepath.SkipAll
		default:
		}

		if werr != nil {
			// Cannot descend into this subtree. Record and skip.
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

		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			failures = append(failures, protocol.FailureInfo{
				Path:    path,
				Code:    protocol.ErrCodeEINVAL,
				Message: rerr.Error(),
			})
			return nil
		}
		target := dst
		if rel != "." {
			target = filepath.Join(dst, rel)
		}

		if d.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				failures = append(failures, protocol.FailureInfo{
					Path:    target,
					Code:    mapFSError(err).Code,
					Message: err.Error(),
				})
				// Can't land children if the parent dir failed. Skip the
				// subtree but continue walking siblings.
				return filepath.SkipDir
			}
			return nil
		}

		info, ierr := d.Info()
		if ierr != nil {
			failures = append(failures, protocol.FailureInfo{
				Path:    path,
				Code:    mapFSError(ierr).Code,
				Message: ierr.Error(),
			})
			return nil
		}
		if !info.Mode().IsRegular() {
			// Symlinks, devices, sockets, pipes: not supported.
			failures = append(failures, protocol.FailureInfo{
				Path:    path,
				Code:    protocol.ErrCodeEINVAL,
				Message: "non-regular file skipped",
			})
			return nil
		}

		// Per-file conflict resolution.
		finalTarget := target
		if _, lerr := os.Lstat(target); lerr == nil {
			switch strategy {
			case "skip":
				failures = append(failures, protocol.FailureInfo{
					Path:    target,
					Code:    protocol.ErrCodeEEXIST,
					Message: "destination already exists",
				})
				prog.fileDone++
				return nil
			case "rename":
				dir := filepath.Dir(target)
				base := filepath.Base(target)
				unique, uerr := UniqueName(dir, base)
				if uerr != nil {
					failures = append(failures, protocol.FailureInfo{
						Path:    target,
						Code:    mapFSError(uerr).Code,
						Message: uerr.Error(),
					})
					return nil
				}
				finalTarget = unique
			case "overwrite":
				// fine
			}
		} else if !errors.Is(lerr, fs.ErrNotExist) {
			failures = append(failures, protocol.FailureInfo{
				Path:    target,
				Code:    mapFSError(lerr).Code,
				Message: lerr.Error(),
			})
			return nil
		}

		prog.fileDone++
		prog.currentPath = finalTarget

		if cerr := copySingleFile(ctx, path, finalTarget, jobID, strategy, prog, emit); cerr != nil {
			if errors.Is(cerr, errCopyCanceled) {
				canceled = true
				return filepath.SkipAll
			}
			failures = append(failures, protocol.FailureInfo{
				Path:    path,
				Code:    mapFSError(cerr).Code,
				Message: cerr.Error(),
			})
			return nil
		}
		return nil
	})

	// filepath.SkipAll is sentinel-only, not an error to surface.
	if walkErr != nil && !errors.Is(walkErr, filepath.SkipAll) {
		// Fatal: e.g. stat on src itself failed catastrophically.
		// Callers surface this as a top-level Error Response.
		return failures, canceled, walkErr
	}
	return failures, canceled, nil
}

// isSubPath returns true iff candidate is equal to or nested beneath
// parent. It's used to block dst-inside-src recursion. Case-insensitive on
// Windows to match the filesystem's semantics; exact match elsewhere.
//
// Both inputs are expected to be cleaned absolute paths (callers have
// already been through safety.CleanPath).
func isSubPath(parent, candidate string) bool {
	if parent == "" || candidate == "" {
		return false
	}
	sep := string(filepath.Separator)
	p := strings.TrimRight(parent, sep)
	c := candidate
	if pathsEqual(p, c) {
		return true
	}
	if len(c) > len(p) && pathsEqual(c[:len(p)], p) && c[len(p)] == filepath.Separator {
		return true
	}
	return false
}
