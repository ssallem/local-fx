//go:build darwin

package ops

import (
	"os"
	"syscall"
	"time"
)

// populateTimestamps fills CreatedTs (from Birthtimespec) and AccessedTs
// (from Atimespec) on macOS via the Stat_t struct that Go embeds in each
// FileInfo.Sys().
func populateTimestamps(d *statData, info os.FileInfo) {
	sys, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	d.CreatedTs = time.Unix(sys.Birthtimespec.Sec, sys.Birthtimespec.Nsec).UnixMilli()
	d.AccessedTs = time.Unix(sys.Atimespec.Sec, sys.Atimespec.Nsec).UnixMilli()
}
