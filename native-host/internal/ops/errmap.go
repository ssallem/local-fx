package ops

import (
	"errors"
	"io/fs"
	"os"
	"syscall"

	"local-fx-host/internal/protocol"
	"local-fx-host/internal/safety"
)

// mapFSError converts a filesystem error raised by the stdlib (os.Stat,
// os.ReadDir, os.Readlink, ...) into a protocol.ErrorPayload.
//
// Rules:
//   - fs.ErrNotExist     -> ENOENT (retryable=false)
//   - fs.ErrPermission   -> EACCES (retryable=false)
//   - syscall.EACCES     -> EACCES
//   - syscall.ENOENT     -> ENOENT
//   - syscall.ENOTDIR    -> EINVAL (caller targeted a non-dir with readdir)
//   - ERROR_ACCESS_DENIED / ERROR_FILE_NOT_FOUND / ERROR_PATH_NOT_FOUND
//     (Windows) collapse into EACCES / ENOENT via the errors.Is checks above,
//     because os.PathError wraps them as syscall.Errno already translated by
//     Go's runtime into fs.ErrPermission / fs.ErrNotExist.
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

	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOENT) {
		return protocol.NewError(protocol.ErrCodeENOENT, err.Error(), false)
	}
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EACCES) {
		return protocol.NewError(protocol.ErrCodeEACCES, err.Error(), false)
	}
	// ENOTDIR means "you tried to readdir a file". That's a caller bug, so
	// surface it as EINVAL rather than EIO (which would imply retry).
	if errors.Is(err, syscall.ENOTDIR) {
		return protocol.NewError(protocol.ErrCodeEINVAL, err.Error(), false)
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
