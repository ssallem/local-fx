//go:build !windows

package ops

import (
	"syscall"
	"testing"

	"local-fx-host/internal/protocol"
)

// TestMapPlatformErrno_Unix verifies the syscall.Errno -> protocol code
// table on POSIX targets.
func TestMapPlatformErrno_Unix(t *testing.T) {
	cases := []struct {
		name     string
		errno    syscall.Errno
		wantCode string
		wantRtry bool
	}{
		{"EEXIST", syscall.EEXIST, protocol.ErrCodeEEXIST, false},
		{"ENOSPC", syscall.ENOSPC, protocol.ErrCodeENOSPC, false},
		{"EBUSY", syscall.EBUSY, protocol.ErrCodeSharingViolation, true},
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

// TestMapPlatformErrno_Unix_Unknown asserts unknown errnos fall through.
func TestMapPlatformErrno_Unix_Unknown(t *testing.T) {
	if _, _, ok := mapPlatformErrno(syscall.Errno(0xffff)); ok {
		t.Errorf("unknown errno: ok=true, want false")
	}
}

// TestMapFSError_EEXIST round-trips through mapFSError to confirm EEXIST
// promotion works on the portable path as well (fs.ErrExist is set by
// os.Mkdir on most Unices so we cover both the fs.ErrExist and the
// syscall.EEXIST Errno branch).
func TestMapFSError_EEXIST(t *testing.T) {
	p := mapFSError(syscall.EEXIST)
	if p.Code != protocol.ErrCodeEEXIST {
		t.Errorf("got %q want EEXIST", p.Code)
	}
}
