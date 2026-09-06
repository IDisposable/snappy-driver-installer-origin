// Package usbdrive finds removable drives and copies a portable copy
// of the app onto one, ported from the non-destructive parts of
// USBWizard (usbwizard.cpp). USBWizard::QuickFormatTarget (opens
// Windows' own SHFormatDrive dialog) and its recursive delete-
// existing-files step are deliberately not ported: formatting/erasing
// a drive is a real destructive action this rewrite has not built a
// safe confirmation flow for yet. Copying overwrites files with the
// same destination path, but does not remove unrelated files.
package usbdrive

// Drive describes one removable drive found by ListRemovable.
type Drive struct {
	// Root is the drive's root path, e.g. `E:\`.
	Root string
	// TotalBytes/FreeBytes describe the drive's filesystem, from
	// GetDiskFreeSpaceEx.
	TotalBytes uint64
	FreeBytes  uint64
}
