//go:build windows

package hardware

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	modkernel32 = windows.NewLazySystemDLL("kernel32.dll")
	moduser32   = windows.NewLazySystemDLL("user32.dll")

	procGetSystemPowerStatus = modkernel32.NewProc("GetSystemPowerStatus")
	procGetUserDefaultLCID   = modkernel32.NewProc("GetUserDefaultLCID")
	procEnumDisplayDevicesW  = moduser32.NewProc("EnumDisplayDevicesW")
)

// GetSysInfoFast gathers system information available without a WMI
// round trip: battery status, monitor physical sizes (via EDID),
// Windows version, locale, environment paths, and architecture. Ported
// from State::getsysinfo_fast.
func GetSysInfoFast() (SysInfo, error) {
	var info SysInfo
	var err error

	if info.Battery, err = getBatteryStatus(); err != nil {
		return info, fmt.Errorf("getting battery status: %w", err)
	}
	info.Monitors = getMonitorSizes()

	if info.Windows, err = getWindowsVersion(); err != nil {
		return info, fmt.Errorf("getting Windows version: %w", err)
	}

	info.LocaleID = getLocaleID()

	info.WinDir = os.Getenv("windir") + `\inf\`

	tempDir := os.Getenv("TEMP")
	if tempDir == "" {
		tempDir = os.Getenv("SystemDrive") + `\temp`
	}
	info.TempDir = tempDir

	info.Is64Bit = strings.EqualFold(os.Getenv("PROCESSOR_ARCHITECTURE"), "AMD64") ||
		os.Getenv("PROCESSOR_ARCHITEW6432") != ""

	return info, nil
}

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

func getBatteryStatus() (BatteryStatus, error) {
	var raw systemPowerStatus
	ret, _, err := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&raw)))
	if ret == 0 {
		return BatteryStatus{}, err
	}

	bs := BatteryStatus{
		ACOnline:            raw.ACLineStatus == 1,
		Flags:               int(raw.BatteryFlag),
		NoBattery:           raw.BatteryFlag&128 != 0,
		ChargePercent:       -1,
		LifeTimeSeconds:     -1,
		FullLifeTimeSeconds: -1,
	}
	if raw.BatteryLifePercent != 255 {
		bs.ChargePercent = int(raw.BatteryLifePercent)
	}
	if raw.BatteryLifeTime != 0xFFFFFFFF {
		bs.LifeTimeSeconds = int(raw.BatteryLifeTime)
	}
	if raw.BatteryFullLifeTime != 0xFFFFFFFF {
		bs.FullLifeTimeSeconds = int(raw.BatteryFullLifeTime)
	}
	return bs, nil
}

func getLocaleID() uint32 {
	ret, _, _ := procGetUserDefaultLCID.Call()
	return uint32(ret)
}

// getWindowsVersion reads the release info straight from the registry
// rather than calling GetVersionEx, which silently lies about the OS
// version unless the calling process' manifest declares support for
// it. CurrentMajorVersionNumber exists from Windows 10 onward; older
// releases fall back to parsing the "CurrentVersion" string (e.g.
// "6.1"). Windows 11 still reports itself as major=10 here, so build
// number is used to correct it, same as the original.
func getWindowsVersion() (WindowsVersionInfo, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return WindowsVersionInfo{}, err
	}
	defer k.Close()

	var v WindowsVersionInfo
	v.ProductType = 1
	if installType, _, err := k.GetStringValue("InstallationType"); err == nil && strings.HasPrefix(installType, "Server") {
		v.ProductType = 3
	}

	if buildStr, _, err := k.GetStringValue("CurrentBuildNumber"); err == nil {
		v.Build, _ = strconv.Atoi(buildStr)
	}

	if major, _, err := k.GetIntegerValue("CurrentMajorVersionNumber"); err == nil {
		minor, _, _ := k.GetIntegerValue("CurrentMinorVersionNumber")
		v.Major, v.Minor = int(major), int(minor)
	} else if verStr, _, err := k.GetStringValue("CurrentVersion"); err == nil {
		if maj, min, ok := strings.Cut(verStr, "."); ok {
			v.Major, _ = strconv.Atoi(maj)
			v.Minor, _ = strconv.Atoi(min)
		}
	}

	if v.Build >= 22000 {
		v.Major, v.Minor = 11, 0
	}

	return v, nil
}

// displayDevice mirrors the Win32 DISPLAY_DEVICEW struct.
type displayDevice struct {
	cb           uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

const (
	displayDeviceActive   = 0x00000001
	displayDeviceAttached = 0x00000002
)

func enumDisplayDevices(deviceName *uint16, devNum uint32, dd *displayDevice) bool {
	dd.cb = uint32(unsafe.Sizeof(*dd))
	ret, _, _ := procEnumDisplayDevicesW.Call(
		uintptr(unsafe.Pointer(deviceName)),
		uintptr(devNum),
		uintptr(unsafe.Pointer(dd)),
		0,
	)
	return ret != 0
}

// getMonitorDevice finds the active, attached monitor for the given
// adapter.
func getMonitorDevice(adapterName *uint16) (displayDevice, bool) {
	var dd displayDevice
	var devMon uint32
	for enumDisplayDevices(adapterName, devMon, &dd) {
		if dd.StateFlags&displayDeviceActive != 0 && dd.StateFlags&displayDeviceAttached != 0 {
			break
		}
		devMon++
	}
	return dd, dd.DeviceID[0] != 0
}

func getMonitorSizes() []MonitorSize {
	var sizes []MonitorSize

	var adapter displayDevice
	var adapterIdx uint32
	for enumDisplayDevices(nil, adapterIdx, &adapter) {
		adapterName := windows.UTF16ToString(adapter.DeviceName[:])
		if adapterNamePtr, err := windows.UTF16PtrFromString(adapterName); err == nil {
			if mon, found := getMonitorDevice(adapterNamePtr); found {
				deviceID := windows.UTF16ToString(mon.DeviceID[:])
				if size, ok := monitorSizeFromEDID(deviceID); ok && size.WidthCM > 0 && size.HeightCM > 0 {
					sizes = append(sizes, size)
				}
			}
		}
		adapterIdx++
	}
	return sizes
}

// monitorSizeFromEDID reads the physical size (in cm) out of a
// monitor's EDID. deviceID looks like
// "MONITOR\ACI27E2\{4d36e96e-...}\0001"; the segment after "MONITOR\"
// is both the monitor model used to verify a
// registry EDID match, and (combined with the remainder) the path to
// find it under SYSTEM\CurrentControlSet\Enum\DISPLAY.
func monitorSizeFromEDID(deviceID string) (MonitorSize, bool) {
	_, rest, ok := strings.Cut(deviceID, `\`)
	if !ok {
		return MonitorSize{}, false
	}
	model, driverSuffix, ok := strings.Cut(rest, `\`)
	if !ok {
		return MonitorSize{}, false
	}

	displayKey, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Enum\DISPLAY\`+model, registry.READ)
	if err != nil {
		return MonitorSize{}, false
	}
	defer displayKey.Close()

	subkeys, err := displayKey.ReadSubKeyNames(-1)
	if err != nil {
		return MonitorSize{}, false
	}

	for _, sub := range subkeys {
		size, ok := edidSizeIfMatchingInstance(displayKey, sub, driverSuffix, model)
		if ok {
			return size, true
		}
	}
	return MonitorSize{}, false
}

func edidSizeIfMatchingInstance(displayKey registry.Key, subkeyName, wantDriver, wantModel string) (MonitorSize, bool) {
	instKey, err := registry.OpenKey(displayKey, subkeyName, registry.READ)
	if err != nil {
		return MonitorSize{}, false
	}
	defer instKey.Close()

	if driver, _, err := instKey.GetStringValue("Driver"); err != nil || driver != wantDriver {
		return MonitorSize{}, false
	}

	paramKey, err := registry.OpenKey(instKey, "Device Parameters", registry.READ)
	if err != nil {
		return MonitorSize{}, false
	}
	defer paramKey.Close()

	edid, _, err := paramKey.GetBinaryValue("EDID")
	if err != nil || len(edid) < 23 {
		return MonitorSize{}, false
	}

	if edidModel(edid) != wantModel {
		return MonitorSize{}, false
	}
	return MonitorSize{WidthCM: int(edid[22]), HeightCM: int(edid[21])}, true
}

// edidModel decodes the 3-letter manufacturer code and 4-hex-digit
// product code from an EDID's manufacturer/product ID bytes (offsets
// 8-11), to match against the PNP device ID's monitor model segment.
func edidModel(edid []byte) string {
	b1, b2 := edid[8], edid[9]
	letters := [3]byte{
		((b1 & 0x7C) >> 2) + 64,
		((b1&3)<<3 | (b2&0xE0)>>5) + 64,
		(b2 & 0x1F) + 64,
	}
	return fmt.Sprintf("%s%X%X%X%X", letters[:], (edid[11]&0xf0)>>4, edid[11]&0xf, (edid[10]&0xf0)>>4, edid[10]&0x0f)
}
