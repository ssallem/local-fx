//go:build windows

package ops

import (
	"os"
	"syscall"
	"time"
)

// populateTimestamps fills CreatedTs/AccessedTs on Windows via the
// Win32FileAttributeData struct that Go embeds in every FileInfo.Sys().
func populateTimestamps(d *statData, info os.FileInfo) {
	sys, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return
	}
	d.CreatedTs = filetimeToUnixMilli(sys.CreationTime)
	d.AccessedTs = filetimeToUnixMilli(sys.LastAccessTime)
}

// filetimeToUnixMilli converts the FILETIME (100-ns ticks since 1601-01-01)
// used throughout the Win32 API into a unix millisecond timestamp.
func filetimeToUnixMilli(ft syscall.Filetime) int64 {
	// Nanoseconds() returns time since Unix epoch already adjusted for the
	// FILETIME epoch offset, so we just rescale to milliseconds.
	return time.Unix(0, ft.Nanoseconds()).UnixMilli()
}
