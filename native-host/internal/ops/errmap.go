package ops

import (
	"errors"
	"io/fs"
	"os"
	"syscall"

	"local-fx-host/internal/platform"
	"local-fx-host/internal/protocol"
	"local-fx-host/internal/safety"
)

// mapFSError converts a filesystem error raised by the stdlib (os.Stat,
// os.ReadDir, os.Readlink, ...) into a protocol.ErrorPayload.
//
// Rules:
//   - fs.ErrNotExist     -> ENOENT (retryable=false)
//   - fs.ErrPermission   -> EACCES (retryable=false)
//   - fs.ErrExist        -> EEXIST (retryable=false)
//   - syscall.EACCES     -> EACCES
//   - syscall.ENOENT     -> ENOENT
//   - syscall.EEXIST     -> EEXIST
//   - syscall.ENOSPC     -> ENOSPC
//   - syscall.ENOTDIR    -> EINVAL (caller targeted a non-dir with readdir)
//   - Platform-specific syscall.Errno (Windows ERROR_ACCESS_DENIED=5,
//     ERROR_SHARING_VIOLATION=32, ERROR_FILE_EXISTS=80, ERROR_ALREADY_EXISTS=183;
//     Unix EEXIST / ENOSPC / EBUSY) are resolved via the build-tagged
//     mapPlatformErrno helper before the generic fallthrough.
//   - ERROR_ACCESS_DENIED / ERROR_FILE_NOT_FOUND / ERROR_PATH_NOT_FOUND
//     (Windows) collapse into EACCES / ENOENT via the errors.Is checks above,
//     because os.PathError wraps them as syscall.Errno already translated by
//     Go's runtime into fs.ErrPermission / fs.ErrNotExist.
//   - platform.ErrTrashUnavailable -> E_TRASH_UNAVAILABLE
//   - platform.ErrNoHandler        -> E_NO_HANDLER
//   - anything else       -> EIO (retryable=true) with details.wrapped = err.Error()
//
// The function deliberately uses errors.Is / errors.As instead of type
// switches so wrapped errors (os.PathError, fs.PathError) are handled
// transparently, per PROTOCOL.md §8 "catch-all 금지, errors.Is/As 사용".
func mapFSError(err error) *protocol.ErrorPayload {
	if err == nil {
		return nil
	}

	// safety.CleanPath rejections translate directly.
	if errors.Is(err, safety.ErrEmptyPath) ||
		errors.Is(err, safety.ErrNotAbsolute) ||
		errors.Is(err, safety.ErrNullByte) {
		return protocol.NewError(protocol.ErrCodePathRejected, err.Error(), false)
	}

	// Platform sentinels exposed via the platform package. Checked before
	// the generic syscall cascade so a trash-unavailable/no-handler miss
	// doesn't degrade to EIO.
	if errors.Is(err, platform.ErrTrashUnavailable) {
		return protocol.NewError(protocol.ErrCodeTrashUnavailable, err.Error(), false)
	}
	if errors.Is(err, platform.ErrNoHandler) {
		return protocol.NewError(protocol.ErrCodeNoHandler, err.Error(), false)
	}

	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOENT) {
		return protocol.NewError(protocol.ErrCodeENOENT, err.Error(), false)
	}
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EACCES) {
		return protocol.NewError(protocol.ErrCodeEACCES, err.Error(), false)
	}
	if errors.Is(err, fs.ErrExist) || errors.Is(err, syscall.EEXIST) {
		return protocol.NewError(protocol.ErrCodeEEXIST, err.Error(), false)
	}
	if errors.Is(err, syscall.ENOSPC) {
		return protocol.NewError(protocol.ErrCodeENOSPC, err.Error(), false)
	}
	// ENOTDIR means "you tried to readdir a file". That's a caller bug, so
	// surface it as EINVAL rather than EIO (which would imply retry).
	if errors.Is(err, syscall.ENOTDIR) {
		return protocol.NewError(protocol.ErrCodeEINVAL, err.Error(), false)
	}

	// Platform-specific Errno values that stdlib did not rewrap into one of
	// the portable sentinels above (e.g. Windows ERROR_SHARING_VIOLATION has
	// no fs.Err* equivalent). Handled via build-tagged helper so the
	// Windows code path can test Errno literals that don't exist on Unix.
	var errno syscall.Errno
	if errors.As(err, &errno) {
		if code, retryable, ok := mapPlatformErrno(errno); ok {
			return protocol.NewError(code, err.Error(), retryable)
		}
	}

	// Unknown low-level failure. Preserve the original message for
	// diagnostics but flag the error as retryable per §8.
	return protocol.NewErrorWithDetails(
		protocol.ErrCodeEIO,
		"i/o error",
		true,
		map[string]interface{}{"wrapped": err.Error()},
	)
}

// pathErrorCode returns the protocol error code that corresponds to the
// syscall.Errno wrapped inside an *os.PathError, if any. Tests use this to
// assert that Windows-specific ERROR_* values map correctly.
func pathErrorCode(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return mapFSError(pe).Code
	}
	return mapFSError(err).Code
}

// wrapSafetyErr converts an error produced by the safety package into a
// protocol.Response. Specifically handles the sentinel
// ErrSystemPathConfirmRequired (→ E_SYSTEM_PATH_CONFIRM_REQUIRED) and the
// CleanPath-family sentinels (→ E_PATH_REJECTED). Anything else falls
// through to mapFSError.
func wrapSafetyErr(id string, err error) protocol.Response {
	if errors.Is(err, safety.ErrSystemPathConfirmRequired) {
		return protocol.ErrorResponse(id,
			protocol.ErrCodeSystemPathConfirmRequired,
			err.Error(), false)
	}
	if errors.Is(err, safety.ErrEmptyPath) ||
		errors.Is(err, safety.ErrNotAbsolute) ||
		errors.Is(err, safety.ErrNullByte) {
		return protocol.ErrorResponse(id, protocol.ErrCodePathRejected, err.Error(), false)
	}
	p := mapFSError(err)
	return protocol.Response{ID: id, OK: false, Error: p}
}
