package hardware

import "math"

// MonitorSize is a display's physical size in centimeters, decoded from
// its EDID. Note: the original C++ (GetMonitorSizeFromEDID) assigns
// *Width from EDID byte 22 and *Height from EDID byte 21, which is
// inverted relative to the VESA EDID spec (byte 21 is the horizontal
// size, byte 22 the vertical). That inversion is preserved here for
// consistency with isWide/IsLaptop's ratio math, which was written
// against the original's (mis-)labeling - do not "fix" one without the
// other.
type MonitorSize struct {
	WidthCM, HeightCM int
}

// isWide reports whether a monitor is widescreen (aspect ratio > 1.35).
func isWide(m MonitorSize) bool {
	if m.WidthCM == 0 {
		return false
	}
	return float64(m.HeightCM)/float64(m.WidthCM) > 1.35
}

// BatteryStatus mirrors the SYSTEM_POWER_STATUS fields this package
// uses. ChargePercent, LifeTimeSeconds, and FullLifeTimeSeconds are -1
// when Windows reports them as unknown (the 255/0xFFFFFFFF sentinels).
type BatteryStatus struct {
	ACOnline            bool
	Flags               int
	NoBattery           bool
	ChargePercent       int
	LifeTimeSeconds     int
	FullLifeTimeSeconds int
}

// WindowsVersionInfo describes the running Windows release, ported from
// the fields State::getsysinfo_fast populates (originally via
// GetVersionEx plus a registry-based Windows 11 correction; this
// rewrite reads the registry directly instead, since GetVersionEx lies
// about the version unless the calling process has an app manifest
// declaring support for the current Windows release).
type WindowsVersionInfo struct {
	Major, Minor, Build int
	// ProductType is 1 for a workstation, 3 for a server (the
	// VER_NT_WORKSTATION/VER_NT_SERVER values from winnt.h).
	ProductType int
}

// IsServer reports whether this is a Windows Server release.
func (w WindowsVersionInfo) IsServer() bool { return w.ProductType == 3 }

// SysInfo is the "fast" system information gathered without a WMI
// round trip.
type SysInfo struct {
	Battery  BatteryStatus
	Monitors []MonitorSize
	Windows  WindowsVersionInfo
	LocaleID uint32
	WinDir   string // %windir%\inf\
	TempDir  string
	Is64Bit  bool
}

// IsLaptop decides desktop vs. laptop. hasACPIBatteryDevice should
// report whether any enumerated device's hardware ID contains
// "*ACPI0003" (a control-method battery); pass
// false if devices haven't been enumerated yet.
func IsLaptop(chassisType int, monitors []MonitorSize, battery BatteryStatus, hasACPIBatteryDevice bool) bool {
	// Chassis types 3 (Desktop) and 10 (Notebook) are unambiguous.
	switch chassisType {
	case 3:
		return false
	case 10:
		return true
	}

	minDiagonal := 99
	var smallest MonitorSize
	for _, m := range monitors {
		diag := int(math.Sqrt(float64(m.WidthCM*m.WidthCM+m.HeightCM*m.HeightCM)) / 2.54)
		if diag < minDiagonal || (diag == minDiagonal && isWide(m)) {
			minDiagonal = diag
			smallest = m
		}
	}

	if !battery.NoBattery || hasACPIBatteryDevice {
		switch {
		case len(monitors) == 0:
			return true
		case isWide(smallest):
			return minDiagonal <= 18
		default:
			return false
		}
	}
	return false
}
