package ops

import (
	"context"
	"encoding/json"

	"local-fx-host/internal/platform"
	"local-fx-host/internal/protocol"
	"local-fx-host/internal/safety"
)

// openArgs is the shared argument shape for `open` and `revealInOsExplorer`
// (PROTOCOL.md §7.11). Both ops take only a path; the difference is which
// OS-side call executes (default handler vs. file-manager highlight).
type openArgs struct {
	Path string `json:"path"`
}

// Open launches path with the OS's default handler. These are intentionally
// NOT gated by CheckMutatingOp: reading / viewing a system file (e.g.
// opening a .log under C:\ProgramData) is allowed without confirmation,
// matching how a user would double-click it in Explorer/Finder directly.
// Mutations via the opened app are the responsibility of that app, not
// this host.
func Open(ctx context.Context, req protocol.Request) protocol.Response {
	cleaned, perr := parseOpenPath(req)
	if perr != nil {
		return *perr
	}
	if err := platform.OpenDefault(ctx, cleaned); err != nil {
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}
	}
	return protocol.Response{
		ID:   req.ID,
		OK:   true,
		Data: map[string]any{},
	}
}

// RevealInOsExplorer opens Explorer (Windows) / Finder (macOS) focused on
// the given path. The OS implementation is responsible for raising a new
// window rather than reusing an existing one.
func RevealInOsExplorer(ctx context.Context, req protocol.Request) protocol.Response {
	cleaned, perr := parseOpenPath(req)
	if perr != nil {
		return *perr
	}
	if err := platform.RevealInOS(ctx, cleaned); err != nil {
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}
	}
	return protocol.Response{
		ID:   req.ID,
		OK:   true,
		Data: map[string]any{},
	}
}

// parseOpenPath factors the shared decode/CleanPath prologue out of both
// handlers. Returns a cleaned absolute path or a pre-built error response.
func parseOpenPath(req protocol.Request) (string, *protocol.Response) {
	var args openArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			resp := protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
				"invalid args: "+err.Error(), false)
			return "", &resp
		}
	}
	cleaned, err := safety.CleanPath(args.Path)
	if err != nil {
		resp := wrapSafetyErr(req.ID, err)
		return "", &resp
	}
	return cleaned, nil
}
