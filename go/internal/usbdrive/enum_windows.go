//go:build windows

package usbdrive

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// ListRemovable enumerates removable drives (USB flash drives, SD
// cards, etc. - not fixed hard disks, network shares, or optical
// drives), ported from USBWizard's target-drive dropdown population.
func ListRemovable() ([]Drive, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("enumerating drives: %w", err)
	}

	var drives []Drive
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		rootPtr, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		if windows.GetDriveType(rootPtr) != windows.DRIVE_REMOVABLE {
			continue
		}

		var free, total, totalFree uint64
		if err := windows.GetDiskFreeSpaceEx(rootPtr, &free, &total, &totalFree); err != nil {
			// Drive letter exists (e.g. an empty card reader slot) but
			// isn't ready/formatted - skip rather than show 0/0.
			continue
		}
		drives = append(drives, Drive{Root: root, TotalBytes: total, FreeBytes: free})
	}
	return drives, nil
}
