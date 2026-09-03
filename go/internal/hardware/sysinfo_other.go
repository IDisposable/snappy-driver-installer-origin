//go:build !windows

package hardware

// GetSysInfoFast is unavailable outside Windows. See ErrWindowsOnly.
func GetSysInfoFast() (SysInfo, error) {
	return SysInfo{}, ErrWindowsOnly
}
