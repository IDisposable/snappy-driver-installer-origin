//go:build !windows

package hardware

import "errors"

// ErrWindowsOnly is returned by hardware queries on non-Windows
// platforms, where the underlying WMI APIs don't exist. SDIO itself is
// a Windows-only tool; these stubs exist so the package still builds
// and its platform-independent logic is testable elsewhere.
var ErrWindowsOnly = errors.New("hardware: this query requires Windows (WMI)")

// GetBaseBoard is unavailable outside Windows. See ErrWindowsOnly.
func GetBaseBoard() (BaseBoard, error) {
	return BaseBoard{}, ErrWindowsOnly
}
