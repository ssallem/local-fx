package ops

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"local-fx-host/internal/protocol"
	"local-fx-host/internal/safety"
)

func TestMapFSError_Nil(t *testing.T) {
	if got := mapFSError(nil); got != nil {
		t.Errorf("mapFSError(nil): got %+v, want nil", got)
	}
}

func TestMapFSError_NotExist(t *testing.T) {
	_, err := os.Open(filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatal("expected Open to fail on missing file")
	}
	p := mapFSError(err)
	if p.Code != protocol.ErrCodeENOENT {
		t.Errorf("code: got %q want ENOENT", p.Code)
	}
	if p.Retryable {
		t.Errorf("retryable: got true, want false for ENOENT")
	}
}

func TestMapFSError_SentinelWrapping(t *testing.T) {
	// Wrap fs.ErrPermission a couple of layers deep to confirm errors.Is
	// unwraps through fmt.Errorf sentinels.
	wrapped := errors.Join(errors.New("outer"), fs.ErrPermission)
	p := mapFSError(wrapped)
	if p.Code != protocol.ErrCodeEACCES {
		t.Errorf("wrapped ErrPermission: got %q want EACCES", p.Code)
	}
}

func TestMapFSError_NotDir(t *testing.T) {
	p := mapFSError(syscall.ENOTDIR)
	if p.Code != protocol.ErrCodeEINVAL {
		t.Errorf("ENOTDIR: got %q want EINVAL", p.Code)
	}
}

func TestMapFSError_SafetyRejection(t *testing.T) {
	p := mapFSError(safety.ErrNullByte)
	if p.Code != protocol.ErrCodePathRejected {
		t.Errorf("ErrNullByte: got %q want E_PATH_REJECTED", p.Code)
	}
}

func TestMapFSError_Fallback(t *testing.T) {
	p := mapFSError(errors.New("some brand new thing"))
	if p.Code != protocol.ErrCodeEIO {
		t.Errorf("unknown: got %q want EIO", p.Code)
	}
	if !p.Retryable {
		t.Errorf("unknown: retryable should be true")
	}
	if got := p.Details["wrapped"]; got != "some brand new thing" {
		t.Errorf("Details[wrapped]: got %v", got)
	}
}

func TestPathErrorCode_WrapsPathError(t *testing.T) {
	_, err := os.Open(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected error")
	}
	if got := pathErrorCode(err); got != protocol.ErrCodeENOENT {
		t.Errorf("got %q want ENOENT", got)
	}
}
