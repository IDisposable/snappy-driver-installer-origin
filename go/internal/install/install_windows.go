//go:build windows

package install

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	newdev                                 = windows.NewLazySystemDLL("newdev.dll")
	procUpdateDriverForPlugAndPlayDevicesW = newdev.NewProc("UpdateDriverForPlugAndPlayDevicesW")

	srClient               = windows.NewLazySystemDLL("SrClient.dll")
	procSRSetRestorePointW = srClient.NewProc("SRSetRestorePointW")
)

// installFlagForce is INSTALLFLAG_FORCE from newdev.h - reinstall even
// if an equal-or-better driver is already present, matching
// driver_install's own call.
const installFlagForce = 0x00000001

// Restore point event/type constants from srrestoreptapi.h, matching
// the fixed values Manager::thread_install passes to
// SRSetRestorePointW.
const (
	beginSystemChange   = 100
	deviceDriverInstall = 10
)

// CheckAvailable resolves every DLL/function this package calls
// (newdev.dll!UpdateDriverForPlugAndPlayDevicesW,
// SrClient.dll!SRSetRestorePointW) without invoking them, for
// diagnosing whether Driver/CreateRestorePoint can work on this
// machine at all before actually installing anything or touching
// System Restore state.
func CheckAvailable() error {
	if err := newdev.Load(); err != nil {
		return fmt.Errorf("loading newdev.dll: %w", err)
	}
	if err := procUpdateDriverForPlugAndPlayDevicesW.Find(); err != nil {
		return fmt.Errorf("finding UpdateDriverForPlugAndPlayDevicesW: %w", err)
	}
	if err := srClient.Load(); err != nil {
		return fmt.Errorf("loading SrClient.dll: %w", err)
	}
	if err := procSRSetRestorePointW.Find(); err != nil {
		return fmt.Errorf("finding SRSetRestorePointW: %w", err)
	}
	return nil
}

// Driver installs infPath as the driver for hardwareID via
// UpdateDriverForPlugAndPlayDevicesW - see the package doc comment for
// what isn't ported (the WOW64 helper/Autoclicker).
func Driver(hwndParent uintptr, hardwareID, infPath string) (Result, error) {
	hwidPtr, err := windows.UTF16PtrFromString(hardwareID)
	if err != nil {
		return Result{}, fmt.Errorf("encoding hardware ID: %w", err)
	}
	infPtr, err := windows.UTF16PtrFromString(infPath)
	if err != nil {
		return Result{}, fmt.Errorf("encoding inf path: %w", err)
	}

	var needReboot int32
	ret, _, callErr := procUpdateDriverForPlugAndPlayDevicesW.Call(
		hwndParent,
		uintptr(unsafe.Pointer(hwidPtr)),
		uintptr(unsafe.Pointer(infPtr)),
		uintptr(installFlagForce),
		uintptr(unsafe.Pointer(&needReboot)),
	)
	if ret == 0 {
		return Result{}, fmt.Errorf("UpdateDriverForPlugAndPlayDevices: %w", callErr)
	}
	return Result{Installed: true, NeedsReboot: needReboot != 0}, nil
}

// restorePointInfo mirrors RESTOREPTINFOW from srrestoreptapi.h - field
// layout (two DWORDs, an INT64, then a 256-wchar_t buffer) must match
// exactly for the raw syscall below to read it correctly.
type restorePointInfo struct {
	eventType      uint32
	restorePtType  uint32
	sequenceNumber int64
	description    [256]uint16
}

// stateMgrStatus mirrors STATEMGRSTATUS from srrestoreptapi.h.
type stateMgrStatus struct {
	status         uint32
	sequenceNumber int64
}

// CreateRestorePoint creates a Windows System Restore point of a fixed
// event/type (BEGIN_SYSTEM_CHANGE, DEVICE_DRIVER_INSTALL - not
// configurable, since every caller creates the same kind of restore
// point) with the given description. Returns an error if SrClient.dll
// isn't present (System Restore is disabled or unavailable on this
// system) or the call itself fails.
func CreateRestorePoint(description string) error {
	descPtr, err := windows.UTF16FromString(description)
	if err != nil {
		return fmt.Errorf("encoding description: %w", err)
	}
	if len(descPtr) > len(restorePointInfo{}.description) {
		return fmt.Errorf("description too long (%d chars, max %d)", len(descPtr)-1, len(restorePointInfo{}.description)-1)
	}

	if err := srClient.Load(); err != nil {
		return fmt.Errorf("loading SrClient.dll: %w", err)
	}
	if err := procSRSetRestorePointW.Find(); err != nil {
		return fmt.Errorf("finding SRSetRestorePointW: %w", err)
	}

	var info restorePointInfo
	info.eventType = beginSystemChange
	info.restorePtType = deviceDriverInstall
	copy(info.description[:], descPtr)

	var status stateMgrStatus
	ret, _, callErr := procSRSetRestorePointW.Call(
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Pointer(&status)),
	)
	if ret == 0 {
		return fmt.Errorf("SRSetRestorePointW: status=%d: %w", status.status, callErr)
	}
	return nil
}

// restoreKeyPath is the registry key GetRestorePointCreationFrequency/
// SetRestorePointCreationFrequency read and write.
const restoreKeyPath = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\SystemRestore`
const restoreValueName = "SystemRestorePointCreationFrequency"

// GetRestorePointCreationFrequency reads the system's minimum-interval-
// between-restore-points setting (minutes). Returns -1 if the value
// isn't set (no throttling configured).
func GetRestorePointCreationFrequency() (int, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, restoreKeyPath, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", restoreKeyPath, err)
	}
	defer k.Close()

	v, _, err := k.GetIntegerValue(restoreValueName)
	if err != nil {
		return -1, nil
	}
	return int(v), nil
}

// SetRestorePointCreationFrequency sets (or, if freq is -1, deletes)
// the system's minimum-interval-between-restore-points setting, ported
// from SystemImp::SetRestorePointCreationFrequency. SDIO calls this
// with 0 before creating its own restore point, to bypass the OS's
// default "at most one automatic restore point per day" throttle, then
// restores the original value afterward.
func SetRestorePointCreationFrequency(freq int) error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, restoreKeyPath, registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return fmt.Errorf("opening %s: %w", restoreKeyPath, err)
	}
	defer k.Close()

	if freq == -1 {
		if err := k.DeleteValue(restoreValueName); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("deleting %s: %w", restoreValueName, err)
		}
		return nil
	}
	return k.SetDWordValue(restoreValueName, uint32(freq))
}
