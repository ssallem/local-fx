//go:build windows

package ops

import (
	"os"
	"syscall"
)

// FILE_ATTRIBUTE_HIDDEN from fileapi.h. Declared locally to avoid pulling in
// golang.org/x/sys/windows just for one constant.
const fileAttributeHidden = 0x2

// hasWindowsHiddenAttr inspects the Win32-specific FileAttributes field
// attached to every FileInfo produced by Go's os package on Windows. Other
// platforms' hidden-file semantics go through isHidden's name prefix rule.
func hasWindowsHiddenAttr(info os.FileInfo) bool {
	sys, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false
	}
	return sys.FileAttributes&fileAttributeHidden != 0
}
