package ops

import (
	"context"
	"errors"

	"local-fx-host/internal/platform"
	"local-fx-host/internal/protocol"
)

// listDrivesData is the `data` payload returned by the `listDrives` op. The
// outer object wraps a `drives` array per PROTOCOL.md §7.2; the extension
// side reads `resp.data.drives` rather than a top-level array.
type listDrivesData struct {
	Drives []platform.Drive `json:"drives"`
}

// ListDrives enumerates mounted volumes via the platform abstraction and
// packages them into a Response.
//
// It takes no args (args may be `{}` or omitted). Any decoding failure is
// reported back as E_BAD_REQUEST, consistent with how other ops handle
// unexpected payload shapes.
//
// Errors from the platform layer are mapped through mapFSError so that a
// permission-denied enumeration on macOS (e.g. restricted /Volumes) surfaces
// as EACCES rather than the catch-all EIO.
func ListDrives(ctx context.Context, req protocol.Request) protocol.Response {
	drives, err := platform.ListDrives(ctx)
	if err != nil {
		// ErrUnsupportedOS is a build-configuration problem, not something
		// retrying will fix. Map it to EIO with retryable=false so the UI
		// surfaces a distinct message instead of the generic transient-I/O
		// copy mapFSError would apply.
		if errors.Is(err, platform.ErrUnsupportedOS) {
			return errResp(req.ID, protocol.ErrCodeEIO,
				"listDrives is not supported on this host OS", false)
		}
		return protocol.Response{
			ID:    req.ID,
			OK:    false,
			Error: mapFSError(err),
		}
	}
	// Normalise to non-nil slice so the JSON output is `"drives": []`
	// instead of `"drives": null` when no volumes are visible.
	if drives == nil {
		drives = []platform.Drive{}
	}
	return protocol.Response{
		ID:   req.ID,
		OK:   true,
		Data: listDrivesData{Drives: drives},
	}
}
