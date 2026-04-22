//go:build !windows

package ops

import "os"

// hasWindowsHiddenAttr is a no-op on non-Windows platforms: those rely on
// the "leading dot" convention that isHidden already handles.
func hasWindowsHiddenAttr(_ os.FileInfo) bool { return false }
