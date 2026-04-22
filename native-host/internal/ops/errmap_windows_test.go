//go:build windows

package ops

import (
	"syscall"
	"testing"

	"local-fx-host/internal/protocol"
)

// TestMapPlatformErrno_Windows verifies the Win32 error number -> protocol
// code table (winerror.h) for the subset Phase 2.1 mutating ops care about.
//
// The values here match winerror.h verbatim and are what SHFileOperationW /
// ShellExecuteW / MoveFileExW return when they fail; Go stdlib does NOT
// wrap them into fs.Err* sentinels because those calls bypass the os
// package entirely.
func TestMapPlatformErrno_Windows(t *testing.T) {
	cases := []struct {
		name     string
		errno    syscall.Errno
		wantCode string
		wantRtry bool
	}{
		{"ERROR_ACCESS_DENIED", syscall.Errno(5), protocol.ErrCodeEACCES, false},
		{"ERROR_SHARING_VIOLATION", syscall.Errno(32), protocol.ErrCodeSharingViolation, true},
		{"ERROR_FILE_EXISTS", syscall.Errno(80), protocol.ErrCodeEEXIST, false},
		{"ERROR_ALREADY_EXISTS", syscall.Errno(183), protocol.ErrCodeEEXIST, false},
		{"ERROR_DISK_FULL", syscall.Errno(112), protocol.ErrCodeENOSPC, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, retryable, ok := mapPlatformErrno(c.errno)
			if !ok {
				t.Fatalf("mapPlatformErrno(%v): ok=false, want true", c.errno)
			}
			if code != c.wantCode {
				t.Errorf("code: got %q, want %q", code, c.wantCode)
			}
			if retryable != c.wantRtry {
				t.Errorf("retryable: got %v, want %v", retryable, c.wantRtry)
			}
		})
	}
}

// TestMapPlatformErrno_Windows_Unknown asserts unknown errnos fall through.
func TestMapPlatformErrno_Windows_Unknown(t *testing.T) {
	if _, _, ok := mapPlatformErrno(syscall.Errno(99999)); ok {
		t.Errorf("unknown errno: ok=true, want false (must fall through to EIO)")
	}
}

// TestMapFSError_SharingViolation round-trips through mapFSError to confirm
// Windows ERROR_SHARING_VIOLATION surfaces the dedicated code rather than
// degrading to EIO.
func TestMapFSError_SharingViolation(t *testing.T) {
	p := mapFSError(syscall.Errno(32))
	if p.Code != protocol.ErrCodeSharingViolation {
		t.Errorf("got %q want %s", p.Code, protocol.ErrCodeSharingViolation)
	}
	if !p.Retryable {
		t.Errorf("SharingViolation should be retryable")
	}
}
