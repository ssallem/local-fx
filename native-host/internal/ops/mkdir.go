package ops

import (
	"context"
	"encoding/json"
	"os"

	"local-fx-host/internal/protocol"
	"local-fx-host/internal/safety"
)

// mkdirArgs follows PROTOCOL.md §7.5:
//
//   { "path": "...", "recursive": true, "explicitConfirm": false }
//
// Recursive mirrors `mkdir -p`: intermediate directories are created and
// an already-existing target is NOT an error. Without Recursive, the
// existence of the target is surfaced as EEXIST so callers can branch.
type mkdirArgs struct {
	Path            string `json:"path"`
	Recursive       bool   `json:"recursive,omitempty"`
	ExplicitConfirm bool   `json:"explicitConfirm,omitempty"`
}

// Mkdir creates a directory at args.path. It is the first mutating op in
// Phase 2.1, so it also exercises the full mutating pipeline:
//   1. safety.CleanPath   — reject relative/empty/null-byte paths
//   2. safety.CheckMutatingOp — gate system allowlist paths behind explicit
//      confirmation
//   3. os.Mkdir / os.MkdirAll — actual filesystem op
//   4. mapFSError            — uniform error translation
//
// Response data is an empty object (PROTOCOL.md §7.5); handlers that need
// to return the created path should refetch via stat.
func Mkdir(_ context.Context, req protocol.Request) protocol.Response {
	var args mkdirArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			return protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
				"invalid args: "+err.Error(), false)
		}
	}

	cleaned, err := safety.CleanPath(args.Path)
	if err != nil {
		return wrapSafetyErr(req.ID, err)
	}
	if err := safety.CheckMutatingOp(cleaned, args.ExplicitConfirm); err != nil {
		return wrapSafetyErr(req.ID, err)
	}

	if args.Recursive {
		err = os.MkdirAll(cleaned, 0o755)
	} else {
		err = os.Mkdir(cleaned, 0o755)
	}
	if err != nil {
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}
	}

	return protocol.Response{
		ID:   req.ID,
		OK:   true,
		Data: map[string]any{},
	}
}
