//go:build !windows

package usbdrive

import "errors"

var errUnsupported = errors.New("usbdrive: not supported on this platform")

// ListRemovable is unsupported on non-Windows platforms.
func ListRemovable() ([]Drive, error) {
	return nil, errUnsupported
}
