//go:build windows

package platform

import (
	"context"
	"syscall"
	"unsafe"
)

// Windows drive enumeration using direct kernel32 bindings so we keep the
// module dependency-free (no golang.org/x/sys/windows). The three entry
// points we need are:
//
//	GetLogicalDriveStringsW  -> "A:\\\0B:\\\0...\0\0" buffer of mounted drives
//	GetDriveTypeW             -> one of DRIVE_* constants identifying media type
//	GetDiskFreeSpaceExW       -> free/total byte counters
//	GetVolumeInformationW     -> label, serial, filesystem name, flags
//
// All four are ANSI-free (W = wide/UTF-16) to support non-ASCII volume labels.

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procGetLogicalDriveStringsW = kernel32.NewProc("GetLogicalDriveStringsW")
	procGetDriveTypeW           = kernel32.NewProc("GetDriveTypeW")
	procGetDiskFreeSpaceExW     = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetVolumeInformationW   = kernel32.NewProc("GetVolumeInformationW")
)

// DRIVE_* constants from WinBase.h. We map these to PROTOCOL.md-friendly
// strings inside driveTypeFSType.
const (
	driveUnknown     = 0
	driveNoRootDir   = 1
	driveRemovable   = 2
	driveFixed       = 3
	driveRemote      = 4
	driveCDROM       = 5
	driveRAMDisk     = 6
)

// FILE_READ_ONLY_VOLUME flag from GetVolumeInformationW's fileSystemFlags.
const fileReadOnlyVolume = 0x00080000

type windowsOS struct{}

func init() {
	Current = windowsOS{}
}

// ListDrives enumerates A: through Z: filtered by GetLogicalDriveStringsW.
func (windowsOS) ListDrives(_ context.Context) ([]Drive, error) {
	mask, err := getLogicalDriveStrings()
	if err != nil {
		return nil, err
	}
	out := make([]Drive, 0, len(mask))
	for _, root := range mask {
		d := Drive{Path: root}
		populateWindowsDrive(&d)
		out = append(out, d)
	}
	return out, nil
}

// Trash / OpenDefault / RevealInOS are Phase 2. Declared here so the
// windowsOS struct satisfies the OS interface; each returns the Phase 2
// placeholder error so accidental Phase 1 callers fail fast rather than
// silently doing nothing.
func (windowsOS) Trash(context.Context, string) error       { return ErrUnsupportedOS }
func (windowsOS) OpenDefault(context.Context, string) error { return ErrUnsupportedOS }
func (windowsOS) RevealInOS(context.Context, string) error  { return ErrUnsupportedOS }

// getLogicalDriveStrings returns each mounted drive as "C:\\", "D:\\", ...
func getLogicalDriveStrings() ([]string, error) {
	// First call with 0/nil asks for the required buffer size in chars.
	n, _, errno := procGetLogicalDriveStringsW.Call(0, 0)
	if n == 0 {
		return nil, error(errno)
	}
	buf := make([]uint16, n+1)
	n2, _, errno := procGetLogicalDriveStringsW.Call(uintptr(n), uintptr(unsafe.Pointer(&buf[0])))
	if n2 == 0 {
		return nil, error(errno)
	}
	// The buffer is a sequence of NUL-terminated wide strings ending with an
	// extra NUL (double-NUL terminator). Split on '\0' and drop empty trailer.
	var roots []string
	start := 0
	for i := 0; i < int(n2); i++ {
		if buf[i] == 0 {
			if i > start {
				roots = append(roots, syscall.UTF16ToString(buf[start:i]))
			}
			start = i + 1
		}
	}
	return roots, nil
}

// populateWindowsDrive fills Label / FSType / *Bytes / ReadOnly fields.
// Failures are swallowed so that a single flaky drive (disconnected network
// share, empty CD tray) doesn't hide the rest of the list.
func populateWindowsDrive(d *Drive) {
	// Drive type (fixed/removable/cdrom/remote/ramdisk).
	pathPtr, err := syscall.UTF16PtrFromString(d.Path)
	if err != nil {
		return
	}
	kind, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(pathPtr)))
	if uint32(kind) == driveCDROM {
		// CD-ROM is inherently read-only for our purposes.
		d.ReadOnly = true
	}

	// Volume label + filesystem name. Buffers sized per MSDN
	// recommendations (MAX_PATH / small fs name).
	var (
		volName        [261]uint16
		volSerial      uint32
		maxComponent   uint32
		fsFlags        uint32
		fsName         [32]uint16
	)
	r1, _, _ := procGetVolumeInformationW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&volName[0])),
		uintptr(len(volName)),
		uintptr(unsafe.Pointer(&volSerial)),
		uintptr(unsafe.Pointer(&maxComponent)),
		uintptr(unsafe.Pointer(&fsFlags)),
		uintptr(unsafe.Pointer(&fsName[0])),
		uintptr(len(fsName)),
	)
	if r1 != 0 {
		d.Label = syscall.UTF16ToString(volName[:])
		d.FSType = syscall.UTF16ToString(fsName[:])
		if fsFlags&fileReadOnlyVolume != 0 {
			d.ReadOnly = true
		}
	}

	// Free / total bytes. Uses the "Ex" variant that handles >4GB volumes.
	var (
		freeBytesAvailable   uint64
		totalNumberOfBytes   uint64
		totalNumberOfFreeBytes uint64
	)
	r2, _, _ := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)
	if r2 != 0 {
		d.TotalBytes = int64(totalNumberOfBytes)
		d.FreeBytes = int64(freeBytesAvailable)
	}
}
