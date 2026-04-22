//go:build !windows

package ops

import (
	"syscall"

	"local-fx-host/internal/protocol"
)

// mapPlatformErrno translates Unix syscall.Errno values into protocol error
// codes. Covers the mutating-op-specific errnos that the portable
// fs.Err* sentinels do not already capture.
//
// EBUSY is surfaced as E_ERROR_SHARING_VIOLATION so that Windows and Unix
// clients see the same "file is in use by another process" code — the
// wire protocol deliberately uses the Windows-style name across platforms
// (see PROTOCOL.md §8).
func mapPlatformErrno(errno syscall.Errno) (code string, retryable bool, ok bool) {
	switch errno {
	case syscall.EEXIST:
		return protocol.ErrCodeEEXIST, false, true
	case syscall.ENOSPC:
		return protocol.ErrCodeENOSPC, false, true
	case syscall.EBUSY:
		return protocol.ErrCodeSharingViolation, true, true
	}
	return "", false, false
}
