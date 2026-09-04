// Package usbdrive finds removable drives and copies a portable copy
// of the app onto one, ported from the non-destructive parts of
// USBWizard (usbwizard.cpp). USBWizard::QuickFormatTarget (opens
// Windows' own SHFormatDrive dialog) and its recursive delete-
// existing-files step are deliberately not ported: formatting/erasing
// a drive is a real destructive action this rewrite hasn't built
// (or had reviewed) a safe confirmation flow for yet. Preparing an
// empty destination is left to the user; this package only ever adds
// files, never removes or overwrites the drive's own filesystem.
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
