package ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"local-fx-host/internal/protocol"
	"local-fx-host/internal/safety"
)

// statArgs / statData follow PROTOCOL.md §7.4. The wire shape is flat (no
// nested "entry" object) so that extension-side type narrowing works with
// the same interface for both readdir rows and stat results.
//
// Times are emitted as unix milliseconds for consistency with readdir. RFC3339
// strings are more human-readable but complicate client-side arithmetic,
// and the existing extension types already expect `modifiedTs: number`.

type statArgs struct {
	Path string `json:"path"`
}

type statData struct {
	Path        string `json:"path"`
	Type        string `json:"type"` // "file" | "directory" | "symlink"
	SizeBytes   *int64 `json:"sizeBytes"`
	ModifiedTs  int64  `json:"modifiedTs"`
	CreatedTs   int64  `json:"createdTs,omitempty"`
	AccessedTs  int64  `json:"accessedTs,omitempty"`
	ReadOnly    bool   `json:"readOnly"`
	Hidden      bool   `json:"hidden"`
	Symlink     bool   `json:"symlink"`
	Target      string `json:"target,omitempty"`      // populated when Symlink=true
	Permissions string `json:"permissions,omitempty"` // "drwxr-xr-x" style
}

// Stat returns a single entry's metadata using Lstat so that symbolic links
// are reported rather than followed. When the entry IS a symlink we also
// populate the `target` field via os.Readlink.
func Stat(_ context.Context, req protocol.Request) protocol.Response {
	var args statArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			return errResp(req.ID, protocol.ErrCodeBadRequest,
				"invalid args: "+err.Error(), false)
		}
	}

	cleaned, err := safety.CleanPath(args.Path)
	if err != nil {
		return errResp(req.ID, protocol.ErrCodePathRejected, err.Error(), false)
	}

	info, err := os.Lstat(cleaned)
	if err != nil {
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}
	}

	d := statData{
		Path:        cleaned,
		ModifiedTs:  info.ModTime().UnixMilli(),
		ReadOnly:    info.Mode().Perm()&0o200 == 0,
		Hidden:      isHidden(filepath.Base(cleaned), info),
		Permissions: info.Mode().String(),
	}
	isLink := info.Mode()&os.ModeSymlink != 0
	switch {
	case isLink:
		d.Type = "symlink"
		d.Symlink = true
		// Best-effort readlink. Broken or unreadable symlinks just leave
		// Target empty rather than failing the whole stat call.
		if tgt, err := os.Readlink(cleaned); err == nil {
			d.Target = tgt
		}
	case info.IsDir():
		d.Type = "directory"
	default:
		d.Type = "file"
		sz := info.Size()
		d.SizeBytes = &sz
	}

	// Platform-specific ctime/atime are optional niceties. Populated via
	// build-tagged helpers so that non-POSIX future targets compile too.
	populateTimestamps(&d, info)

	return protocol.Response{
		ID:   req.ID,
		OK:   true,
		Data: d,
	}
}
