package ops

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"local-fx-host/internal/protocol"
	"local-fx-host/internal/safety"
)

// MoveArgs follows PROTOCOL.md §7.12.
//
//   - same-volume: backed by os.Rename (atomic, fast).
//   - cross-volume: fall back to recursive copy + delete of source when the
//     OS reports EXDEV / ERROR_NOT_SAME_DEVICE.
//
// Conflict semantics mirror CopyArgs: "overwrite" | "skip" | "rename". The
// legacy Overwrite boolean is normalised via resolveConflictStrategy so the
// wire API stays consistent with copy.
type MoveArgs struct {
	Src             string `json:"src"`
	Dst             string `json:"dst"`
	Overwrite       bool   `json:"overwrite,omitempty"`
	ExplicitConfirm bool   `json:"explicitConfirm,omitempty"`
	Conflict        string `json:"conflict,omitempty"` // overwrite|skip|rename
}

// Move is a streaming op. On same-volume it uses os.Rename (atomic, fast).
// On cross-volume (EXDEV / ERROR_NOT_SAME_DEVICE) it falls back to a
// recursive copy followed by delete of the source.
//
// Emission contract matches Copy: zero events on the fast path other than a
// terminal "done"; the cross-volume fallback streams progress events through
// recursiveCopy exactly like a copy op.
func Move(ctx context.Context, req protocol.Request, emit func(protocol.EventFrame) error) protocol.Response {
	var args MoveArgs
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
	// Move mutates BOTH sides: src loses its data, dst gains it. Gate both
	// paths against the system allowlist so a move can't remove a system
	// file or overwrite one without explicitConfirm.
	if cerr := safety.CheckMutatingOp(srcClean, args.ExplicitConfirm); cerr != nil {
		return wrapSafetyErr(req.ID, cerr)
	}
	if cerr := safety.CheckMutatingOp(dstClean, args.ExplicitConfirm); cerr != nil {
		return wrapSafetyErr(req.ID, cerr)
	}

	// Normalise conflict/overwrite via the same helper Copy uses so the wire
	// semantics stay identical across the two ops.
	strategy, errResp := resolveConflictStrategy(req.ID, CopyArgs{
		Overwrite: args.Overwrite,
		Conflict:  args.Conflict,
	})
	if errResp != nil {
		return *errResp
	}

	if pathsEqual(srcClean, dstClean) {
		return protocol.ErrorResponse(req.ID, protocol.ErrCodeEINVAL,
			"src and dst are the same path", false)
	}
	if isUnder(dstClean, srcClean) {
		return protocol.ErrorResponse(req.ID, protocol.ErrCodeEINVAL,
			"dst is inside src (cycle)", false)
	}

	// Cancel registration lives in the caller (runStreamHandler in main.go
	// or the integration harness). We re-use the ctx as-is.
	opCtx := ctx

	// CRITICAL-P2-R4-3 — conflict strategy must be honoured BEFORE the fast
	// os.Rename path. POSIX rename(2) atomically replaces dst when it exists,
	// which silently destroys data when the user asked for "skip" or "rename".
	// Gate the fast path on an explicit Lstat and handle each strategy
	// according to the wire contract.
	if _, lerr := os.Lstat(dstClean); lerr == nil {
		switch strategy {
		case "skip":
			// Keep src and dst untouched. Surface the conflict via a done
			// frame with a single FailureInfo so the UI can report it.
			_ = emit(protocol.EventFrame{
				ID:    req.ID,
				Event: "done",
				Payload: protocol.DonePayload{
					Failures: []protocol.FailureInfo{{
						Path:    dstClean,
						Code:    protocol.ErrCodeEEXIST,
						Message: "skipped: destination exists",
					}},
				},
			})
			return protocol.SuccessResponse(req.ID, map[string]any{})
		case "rename":
			parent := filepath.Dir(dstClean)
			base := filepath.Base(dstClean)
			newDst, uerr := UniqueName(parent, base)
			if uerr != nil {
				return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(uerr)}
			}
			dstClean = newDst
		case "overwrite":
			// POSIX rename(2) / Windows MoveFileEx with MOVEFILE_REPLACE_EXISTING
			// (the Go stdlib behaviour) will clobber the destination atomically.
			// Proceed into the fast path unchanged.
		}
	} else if !errors.Is(lerr, fs.ErrNotExist) {
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(lerr)}
	}

	// Fast path: same-volume rename. One syscall, atomic on both Windows
	// (MoveFileEx) and POSIX (rename(2)).
	renameErr := os.Rename(srcClean, dstClean)
	if renameErr == nil {
		_ = emit(protocol.EventFrame{
			ID:      req.ID,
			Event:   "done",
			Payload: protocol.DonePayload{},
		})
		return protocol.SuccessResponse(req.ID, map[string]any{})
	}

	// Cross-volume fallback: recursive copy, then delete source. We only
	// take this path when the OS tells us the rename crossed a volume
	// boundary; all other os.Rename failures surface as hard errors.
	if !isCrossDevice(renameErr) {
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(renameErr)}
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

	// Only remove the source when the copy was completely clean. A
	// partial-failure set means data exists only in src for some entries —
	// deleting src would destroy it. Better to leave both trees in place
	// and let the UI reconcile via the Failures list.
	if len(failures) == 0 {
		if rmErr := os.RemoveAll(srcClean); rmErr != nil {
			ep := mapFSError(rmErr)
			failures = append(failures, protocol.FailureInfo{
				Path:    srcClean,
				Code:    ep.Code,
				Message: "copied but failed to delete source: " + rmErr.Error(),
			})
		}
	}

	_ = emit(protocol.EventFrame{
		ID:      req.ID,
		Event:   "done",
		Payload: protocol.DonePayload{Failures: failures},
	})
	return protocol.SuccessResponse(req.ID, map[string]any{})
}

// isCrossDevice reports whether err is the OS's way of saying "this rename
// crossed a volume/device boundary". On Windows the code is
// ERROR_NOT_SAME_DEVICE (17); on POSIX it's EXDEV (18 on Linux). Go's
// syscall package exposes both as syscall.Errno so we can compare by number
// without importing platform-specific constants.
func isCrossDevice(err error) bool {
	const (
		errorNotSameDevice syscall.Errno = 17 // Windows ERROR_NOT_SAME_DEVICE
		exdev              syscall.Errno = 18 // Linux/Darwin EXDEV
	)
	var le *os.LinkError
	if errors.As(err, &le) {
		var se syscall.Errno
		if errors.As(le.Err, &se) {
			if se == errorNotSameDevice || se == exdev {
				return true
			}
		}
	}
	return errors.Is(err, errorNotSameDevice) || errors.Is(err, exdev)
}

// isUnder reports whether child is strictly inside parent (equal paths
// return false). Comparison is case-insensitive on Windows to match
// filesystem semantics. Both inputs are assumed cleaned.
func isUnder(child, parent string) bool {
	if pathsEqual(child, parent) {
		return false
	}
	sep := string(os.PathSeparator)
	if runtime.GOOS == "windows" {
		return strings.HasPrefix(strings.ToLower(child), strings.ToLower(parent)+sep)
	}
	return strings.HasPrefix(child, parent+sep)
}
