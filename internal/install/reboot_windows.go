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

func enableShutdownPrivilege() error {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()

	name, err := windows.UTF16PtrFromString("SeShutdownPrivilege")
	if err != nil {
		return err
	}
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, name, &luid); err != nil {
		return err
	}
	privileges := windows.Tokenprivileges{
		PrivilegeCount: 1,
		Privileges:     [1]windows.LUIDAndAttributes{{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}},
	}
	if err := windows.AdjustTokenPrivileges(token, false, &privileges, 0, nil, nil); err != nil {
		return err
	}
	if err := windows.GetLastError(); err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

func Reboot() error {
	if err := enableShutdownPrivilege(); err == nil && procExitWindowsEx.Find() == nil {
		ret, _, callErr := procExitWindowsEx.Call(uintptr(ewxReboot|ewxForceIfHung), 0)
		if ret != 0 {
			return nil
		}
		if callErr == nil {
			callErr = windows.GetLastError()
		}
		fallbackErr := exec.Command("shutdown.exe", "/r", "/t", "0").Run()
		if fallbackErr == nil {
			return nil
		}
		return fmt.Errorf("ExitWindowsEx failed: %w; shutdown.exe fallback failed: %v", callErr, fallbackErr)
	}
	if err := exec.Command("shutdown.exe", "/r", "/t", "0").Run(); err != nil {
		return fmt.Errorf("rebooting with shutdown.exe: %w", err)
	}
	return nil
}
