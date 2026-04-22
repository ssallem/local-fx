//go:build windows

package platform

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"
)

// Windows shell integration for Phase 2.1:
//
//   - Trash       -> SHFileOperationW (FO_DELETE + FOF_ALLOWUNDO)
//   - OpenDefault -> ShellExecuteW("open", path, ...)
//   - RevealInOS  -> explorer.exe /select,<path>
//
// We bind shell32.dll directly (not golang.org/x/sys) to keep the module
// dependency-free. The two Win32 APIs we need are SHFileOperationW and
// ShellExecuteW; both are wide-char (W suffix) so non-ASCII file names work.

var (
	shell32                  = syscall.NewLazyDLL("shell32.dll")
	procSHFileOperationW     = shell32.NewProc("SHFileOperationW")
	procShellExecuteW        = shell32.NewProc("ShellExecuteW")
)

// SHFileOperation flags / ops (shellapi.h). We set the no-UI subset so the
// operation completes without prompting Explorer to show a dialog even when
// the host runs in an interactive session.
const (
	foDelete           = 0x0003
	fofAllowUndo       = 0x0040
	fofNoConfirmation  = 0x0010
	fofSilent          = 0x0004
	fofNoErrorUI       = 0x0400
	fofNoConfirmMkDir  = 0x0200
	fofWantNukeWarning = 0x4000

	// Combined flag set used for Trash. ALLOWUNDO puts the entry in the
	// Recycle Bin rather than nuking it; NO_UI disables all interactive
	// dialogs so the host process doesn't hang waiting for user input.
	trashFlags = fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI | fofNoConfirmMkDir
)

// shFileOpStruct matches SHFILEOPSTRUCTW layout on 64-bit Windows. All
// pointer fields are uintptr so the struct lines up with the C ABI when
// passed to SHFileOperationW via syscall.Syscall.
//
// Reference: https://learn.microsoft.com/en-us/windows/win32/api/shellapi/ns-shellapi-shfileopstructw
type shFileOpStruct struct {
	Hwnd                  uintptr
	Func                  uint32
	From                  *uint16
	To                    *uint16
	Flags                 uint16
	AnyOperationsAborted  int32
	HNameMappings         uintptr
	ProgressTitle         *uint16
}

// ShellExecute SW_ constants (from ShowWindow docs).
const (
	swShowNormal = 1
)

// shellTrash moves path to the Recycle Bin via SHFileOperationW.
//
// Three subtle requirements:
//  1. SHFILEOPSTRUCT.pFrom must be a **double-NUL-terminated** wide-char
//     buffer. syscall.UTF16FromString adds exactly one NUL, so we append
//     a second one manually. Skipping this causes SHFileOperationW to
//     read past the end of the buffer and either fail or operate on
//     garbage.
//  2. The fAnyOperationsAborted field is an int32 OUT param. We deliberately
//     ignore it: under the FOF_NOCONFIRMATION|FOF_SILENT|FOF_NOERRORUI flag
//     combination Windows can flip fAnyOperationsAborted internally even
//     when SHFileOperationW returns 0 (success) — for example when deleting
//     an item whose Recycle Bin entry was already present. Trusting the
//     return value alone is the documented way to distinguish success from
//     failure; trusting the OUT flag produces false positives here.
//  3. The return value is a Win32 error code when nonzero, but it's NOT
//     a GetLastError — it's from a dedicated table in the SHFileOperation
//     docs. We surface it as a generic "shell op failed" for unknown
//     values so diagnostic messages include the numeric code.
func shellTrash(_ context.Context, path string) error {
	utf16Path, err := syscall.UTF16FromString(path)
	if err != nil {
		return fmt.Errorf("trash: utf16: %w", err)
	}
	// UTF16FromString already null-terminates, but SHFileOperation requires
	// a DOUBLE null: the string list ends with "" (empty wide string), which
	// renders as two consecutive NUL words.
	utf16Path = append(utf16Path, 0)

	op := shFileOpStruct{
		Func:  foDelete,
		From:  &utf16Path[0],
		Flags: trashFlags,
	}
	ret, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if ret != 0 {
		// Common values documented at
		// https://learn.microsoft.com/en-us/windows/win32/api/shellapi/nf-shellapi-shfileoperationw#return-value
		// 0x7C (ERROR_INVALID_INDEX): invalid source path
		// 0x78 (DE_ACCESSDENIEDSRC): insufficient permissions
		// We return a generic error with the code embedded; callers
		// that care about the precise mapping can parse it. For the
		// Phase 2.1 surface, mapFSError's EIO fallback is acceptable
		// for rare codes not covered by errmap_windows.go.
		if ret == 120 /* ERROR_CALL_NOT_IMPLEMENTED */ {
			return ErrTrashUnavailable
		}
		return fmt.Errorf("SHFileOperationW: error 0x%X on %q", ret, path)
	}
	// Intentionally skip the op.AnyOperationsAborted check — see comment above.
	return nil
}

// shellOpenDefault invokes ShellExecuteW with the "open" verb, which asks
// Windows to launch the default handler for the file's extension (or for
// directories, opens an Explorer window).
//
// ShellExecuteW returns an HINSTANCE (uintptr here). Per MSDN, values <= 32
// indicate failure: common ones are SE_ERR_NOASSOC (31) when no app is
// registered for the file type, and SE_ERR_FNF (2) for file-not-found.
func shellOpenDefault(_ context.Context, path string) error {
	verb, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return fmt.Errorf("open: utf16 verb: %w", err)
	}
	file, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("open: utf16 path: %w", err)
	}
	ret, _, _ := procShellExecuteW.Call(
		0,                                  // hwnd
		uintptr(unsafe.Pointer(verb)),      // lpOperation
		uintptr(unsafe.Pointer(file)),      // lpFile
		0,                                  // lpParameters
		0,                                  // lpDirectory
		uintptr(swShowNormal),              // nShowCmd
	)
	if ret <= 32 {
		switch ret {
		case 31: // SE_ERR_NOASSOC
			return ErrNoHandler
		case 2, 3: // SE_ERR_FNF, SE_ERR_PNF
			return fmt.Errorf("ShellExecuteW: file not found: %q", path)
		case 5: // SE_ERR_ACCESSDENIED
			return fmt.Errorf("ShellExecuteW: access denied on %q", path)
		}
		return fmt.Errorf("ShellExecuteW: error %d on %q", ret, path)
	}
	return nil
}

// shellRevealInOS launches explorer.exe with "/select,<path>" so the target
// appears highlighted inside its parent folder.
//
// The "/select," + path form is a single argument to explorer.exe: the comma
// is a separator, not a CLI flag delimiter. exec.Command on Windows CreateProcess
// with the default arg quoter preserves it verbatim, which is what explorer
// expects. DO NOT split this into two args or explorer will open /select, in
// the address bar.
func shellRevealInOS(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "explorer.exe", "/select,"+path)
	// explorer.exe returns non-zero exit codes even on successful selection
	// (it forks a new window and the parent exits with 1). Start() instead
	// of Run() so we don't misreport that as an error.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("explorer /select: %w", err)
	}
	// Abandon the child; Windows cleans it up on exit. Wait() would block
	// until the user closes the Explorer window, which is never what we
	// want.
	go func() { _ = cmd.Wait() }()
	return nil
}
