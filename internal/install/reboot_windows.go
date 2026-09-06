//go:build windows

package install

import (
	"fmt"
	"os/exec"

	"golang.org/x/sys/windows"
)

var procExitWindowsEx = windows.NewLazySystemDLL("user32.dll").NewProc("ExitWindowsEx")

const (
	ewxReboot      = 0x00000002
	ewxForceIfHung = 0x00000010
)

func Reboot() error {
	if err := procExitWindowsEx.Find(); err == nil {
		ret, _, callErr := procExitWindowsEx.Call(uintptr(ewxReboot|ewxForceIfHung), 0)
		if ret != 0 {
			return nil
		}
		if callErr == nil {
			callErr = windows.GetLastError()
		}
		if err := exec.Command("shutdown.exe", "/r", "/t", "0").Run(); err == nil {
			return nil
		}
		return fmt.Errorf("ExitWindowsEx failed: %w", callErr)
	}
	if err := exec.Command("shutdown.exe", "/r", "/t", "0").Run(); err != nil {
		return fmt.Errorf("rebooting with shutdown.exe: %w", err)
	}
	return nil
}
