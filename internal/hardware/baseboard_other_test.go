//go:build !windows

package hardware

import (
	"errors"
	"testing"
)

func TestGetBaseBoardStubOnNonWindows(t *testing.T) {
	_, err := GetBaseBoard()
	if !errors.Is(err, ErrWindowsOnly) {
		t.Fatalf("GetBaseBoard() error = %v, want ErrWindowsOnly", err)
	}
}
