//go:build !windows

package hardware

// OpenInstalledDriver is unavailable outside Windows. See ErrWindowsOnly.
func OpenInstalledDriver(driverKeyName string, device Device) (InstalledDriver, error) {
	return InstalledDriver{}, ErrWindowsOnly
}
