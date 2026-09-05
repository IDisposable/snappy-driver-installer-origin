//go:build !windows

package install

import "errors"

var errUnsupported = errors.New("install: not supported on this platform")

// CheckAvailable is unsupported on non-Windows platforms.
func CheckAvailable() error {
	return errUnsupported
}

// Driver is unsupported on non-Windows platforms.
func Driver(hwndParent uintptr, hardwareID, infPath string) (Result, error) {
	return Result{}, errUnsupported
}

// CreateRestorePoint is unsupported on non-Windows platforms.
func CreateRestorePoint(description string) error {
	return errUnsupported
}

// GetRestorePointCreationFrequency is unsupported on non-Windows platforms.
func GetRestorePointCreationFrequency() (int, error) {
	return 0, errUnsupported
}

// SetRestorePointCreationFrequency is unsupported on non-Windows platforms.
func SetRestorePointCreationFrequency(freq int) error {
	return errUnsupported
}
