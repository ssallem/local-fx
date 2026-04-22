package ops

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"local-fx-host/internal/platform"
	"local-fx-host/internal/protocol"
	"local-fx-host/internal/safety"
)

// removeArgs follows PROTOCOL.md §7.9. Mode distinguishes trash (recoverable
// via OS recycle bin) from permanent (os.Remove). Recursive-permanent
// removal is deferred to Phase 2.3/2.4 because os.RemoveAll makes partial-
// failure handling awkward: Phase 2.1 keeps its semantics narrow (empty
// directory or single file) to eliminate that class of bug.
type removeArgs struct {
	Path            string `json:"path"`
	Mode            string `json:"mode"` // "trash" | "permanent"
	ExplicitConfirm bool   `json:"explicitConfirm,omitempty"`
}

// Remove deletes the entry at args.path. The concrete semantics depend on
// args.mode:
//
//   - "trash"     -> platform.Trash (recycle bin / Finder Trash / etc.).
//                    ErrTrashUnavailable from the platform layer bubbles up
//                    as ErrCodeTrashUnavailable so the UI can suggest
//                    permanent delete instead.
//   - "permanent" -> os.Remove for files or empty directories. Non-empty
//                    directories are refused with EINVAL; os.RemoveAll is
//                    intentionally NOT used in 2.1 (partial failures).
func Remove(ctx context.Context, req protocol.Request) protocol.Response {
	var args removeArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			return protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
				"invalid args: "+err.Error(), false)
		}
	}
	if args.Mode != "trash" && args.Mode != "permanent" {
		return protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
			`mode must be "trash" or "permanent"`, false)
	}

	cleaned, err := safety.CleanPath(args.Path)
	if err != nil {
		return wrapSafetyErr(req.ID, err)
	}
	if err := safety.CheckMutatingOp(cleaned, args.ExplicitConfirm); err != nil {
		return wrapSafetyErr(req.ID, err)
	}

	switch args.Mode {
	case "trash":
		if err := platform.Trash(ctx, cleaned); err != nil {
			if errors.Is(err, platform.ErrTrashUnavailable) {
				return protocol.ErrorResponse(req.ID, protocol.ErrCodeTrashUnavailable,
					err.Error(), false)
			}
			return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}
		}
	case "permanent":
		info, err := os.Lstat(cleaned)
		if err != nil {
			return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}
		}
		if info.IsDir() {
			// Check emptiness before removing. os.Remove on a non-empty
			// directory returns syscall.ENOTEMPTY which mapFSError falls
			// through to EIO — we surface a clearer EINVAL with policy
			// context instead.
			entries, err := os.ReadDir(cleaned)
			if err != nil {
				return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}
			}
			if len(entries) > 0 {
				return protocol.ErrorResponse(req.ID, protocol.ErrCodeEINVAL,
					"permanent delete of non-empty directory not supported in Phase 2.1",
					false)
			}
		}
		if err := os.Remove(cleaned); err != nil {
			return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}
		}
	}

	return protocol.Response{
		ID:   req.ID,
		OK:   true,
		Data: map[string]any{},
	}
}
