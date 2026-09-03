//go:build !windows

package hardware

import (
	"errors"
	"testing"
)

func TestScanDevicesStubOnNonWindows(t *testing.T) {
	_, err := ScanDevices()
	if !errors.Is(err, ErrWindowsOnly) {
		t.Fatalf("ScanDevices() error = %v, want ErrWindowsOnly", err)
	}
}
