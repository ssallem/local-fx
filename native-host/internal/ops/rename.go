package ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"local-fx-host/internal/protocol"
	"local-fx-host/internal/safety"
)

// renameArgs follows PROTOCOL.md §7.8. ExplicitConfirm is checked against
// BOTH src and dst so callers can't smuggle a destructive rename of a system
// path past the allowlist by aiming src at a user dir and dst at a system
// dir (or vice versa).
type renameArgs struct {
	Src             string `json:"src"`
	Dst             string `json:"dst"`
	ExplicitConfirm bool   `json:"explicitConfirm,omitempty"`
}

// Rename renames src -> dst when both sides are in the same parent directory.
//
// Cross-directory / cross-volume moves are intentionally rejected in Phase
// 2.1: os.Rename can return EXDEV across volumes on Unix and Windows handles
// cross-volume moves via MoveFileEx(MOVEFILE_COPY_ALLOWED), which changes the
// failure mode characteristics. Phase 2.3's copy+delete pipeline is the
// proper home for those cases; see harmonic-chasing-narwhal.md §4.
func Rename(_ context.Context, req protocol.Request) protocol.Response {
	var args renameArgs
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

	// Same-directory guard. filepath.Dir is purely lexical, which is what we
	// want here: CleanPath has already canonicalised separators + symlinks,
	// so directory comparison is meaningful without a second stat.
	//
	// Windows file systems (NTFS/ReFS/FAT) are case-insensitive by default,
	// so `C:\Foo` and `C:\foo` refer to the same directory; treating those
	// as "different" would reject an otherwise-valid same-dir rename. We
	// fall back to EqualFold on Windows to match OS semantics. Unix stays
	// byte-exact because case-sensitive file systems are the norm.
	srcDir := filepath.Dir(srcClean)
	dstDir := filepath.Dir(dstClean)
	sameDir := srcDir == dstDir
	if !sameDir && runtime.GOOS == "windows" {
		sameDir = strings.EqualFold(srcDir, dstDir)
	}
	if !sameDir {
		return protocol.ErrorResponse(req.ID, protocol.ErrCodeEINVAL,
			"rename across directories not supported in Phase 2.1", false)
	}

	if err := safety.CheckMutatingOp(srcClean, args.ExplicitConfirm); err != nil {
		return wrapSafetyErr(req.ID, err)
	}
	if err := safety.CheckMutatingOp(dstClean, args.ExplicitConfirm); err != nil {
		return wrapSafetyErr(req.ID, err)
	}

	if err := os.Rename(srcClean, dstClean); err != nil {
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}
	}
	return protocol.Response{
		ID:   req.ID,
		OK:   true,
		Data: map[string]any{},
	}
}
