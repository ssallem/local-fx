//go:build darwin

package platform

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// macOS shell integration for Phase 2.1:
//
//   - Trash       -> osascript "tell Finder to delete POSIX file ..."
//   - OpenDefault -> `open <path>`
//   - RevealInOS  -> `open -R <path>`
//
// We deliberately avoid CGo / NSFileManager here: Phase 4 MSI/.pkg signing
// would need extra entitlements for Objective-C bindings, whereas osascript
// and `open` ship on every macOS install and need no entitlements.

// osascriptTrashError codes we care about. Per AppleScript docs, -1743
// is errAEEventNotPermitted, meaning the user denied the Finder automation
// permission prompt on first use. Treat it as ErrTrashUnavailable so the UI
// can offer permanent-delete fallback.
const finderAutomationDeniedCode = "-1743"

func shellTrash(ctx context.Context, path string) error {
	// AppleScript quotes the path as a literal; we must escape any
	// embedded `"` or `\` to avoid breaking out. Newline and carriage
	// return bytes are legal in POSIX file names but terminate an
	// AppleScript string literal, so we translate them to their
	// AppleScript escape form. Paths with `$` are fine because
	// AppleScript does not do shell interpolation.
	escaped := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
	).Replace(path)
	script := fmt.Sprintf(`tell application "Finder" to delete POSIX file "%s"`, escaped)

	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// osascript prints the error code + message on stderr; we merge
		// stdout+stderr above so we can grep the combined output.
		combined := string(out)
		if strings.Contains(combined, finderAutomationDeniedCode) {
			return ErrTrashUnavailable
		}
		return fmt.Errorf("osascript trash: %w: %s", err, strings.TrimSpace(combined))
	}
	return nil
}

func shellOpenDefault(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "open", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		combined := strings.ToLower(string(out))
		if strings.Contains(combined, "no application knows how to open") {
			return ErrNoHandler
		}
		return fmt.Errorf("open: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func shellRevealInOS(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "open", "-R", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("open -R: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
