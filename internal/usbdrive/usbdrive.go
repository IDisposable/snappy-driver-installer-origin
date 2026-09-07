// Package usbdrive finds removable drives and copies a portable app tree.
// Copying overwrites same-path files but does not format or clear a drive.
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
