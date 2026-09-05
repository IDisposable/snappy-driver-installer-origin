//go:build !windows

package hardware

// ScanDevices is unavailable outside Windows. See ErrWindowsOnly.
func ScanDevices() ([]Device, error) {
	return nil, ErrWindowsOnly
}
