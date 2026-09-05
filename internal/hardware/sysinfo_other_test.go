//go:build !windows

package hardware

import (
	"errors"
	"testing"
)

func TestGetSysInfoFastStubOnNonWindows(t *testing.T) {
	_, err := GetSysInfoFast()
	if !errors.Is(err, ErrWindowsOnly) {
		t.Fatalf("GetSysInfoFast() error = %v, want ErrWindowsOnly", err)
	}
}
