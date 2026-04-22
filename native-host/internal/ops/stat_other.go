//go:build !windows && !darwin

package ops

import "os"

// populateTimestamps is a stub for GOOSes that don't have a per-platform
// birth-time extraction. Stat responses on those targets simply omit the
// optional Created/Accessed fields (the `,omitempty` tag keeps the wire
// format tidy).
func populateTimestamps(_ *statData, _ os.FileInfo) {}
