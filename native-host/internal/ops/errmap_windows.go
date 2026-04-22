//go:build windows

package ops

import (
	"syscall"

	"local-fx-host/internal/protocol"
)

// mapPlatformErrno translates Windows syscall.Errno values (Win32 error
// numbers produced by raw API calls like SHFileOperationW / ShellExecuteW)
// into protocol error codes.
//
// Values are the Win32 error numbers from winerror.h, documented on
// docs.microsoft.com. Only the handful we need for Phase 2.1 mutating ops
// are listed; the generic fallthrough in mapFSError takes care of the rest.
//
// Note: stdlib already translates ERROR_ACCESS_DENIED and ERROR_FILE_NOT_FOUND
// into fs.ErrPermission / fs.ErrNotExist, so callers usually never hit the
// EACCES/ENOENT branches here. They remain as belt-and-braces for hand-rolled
// syscall results where no *os.PathError wrapping happens.
func mapPlatformErrno(errno syscall.Errno) (code string, retryable bool, ok bool) {
	switch errno {
	case 5: // ERROR_ACCESS_DENIED
		return protocol.ErrCodeEACCES, false, true
	case 32: // ERROR_SHARING_VIOLATION
		return protocol.ErrCodeSharingViolation, true, true
	case 80: // ERROR_FILE_EXISTS
		return protocol.ErrCodeEEXIST, false, true
	case 183: // ERROR_ALREADY_EXISTS
		return protocol.ErrCodeEEXIST, false, true
	case 112: // ERROR_DISK_FULL
		return protocol.ErrCodeENOSPC, false, true
	}
	return "", false, false
}
