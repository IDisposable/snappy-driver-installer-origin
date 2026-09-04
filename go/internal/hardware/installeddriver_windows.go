//go:build windows

package hardware

import (
	"fmt"

	"golang.org/x/sys/windows/registry"

	"sdio/internal/indexing"
)

// OpenInstalledDriver opens driverKeyName under
// SYSTEM\CurrentControlSet\Control\Class and reads the currently-
// installed driver info from it. device should be the Device this
// driver is installed for (its DriverKeyName field gives
// driverKeyName).
func OpenInstalledDriver(driverKeyName string, device Device) (InstalledDriver, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Class\`+driverKeyName, registry.QUERY_VALUE)
	if err != nil {
		return InstalledDriver{}, fmt.Errorf("opening driver registry key %q: %w", driverKeyName, err)
	}
	defer key.Close()

	return readInstalledDriver(key, device), nil
}

// readInstalledDriver reads a currently-installed driver's info from
// its already-open registry key (typically
// SYSTEM\CurrentControlSet\Control\Class\<ClassGUID>\<Index>) - see
// InstalledDriver's doc comment for what's deferred.
// DriverDate/DriverVersion are parsed with the same
// indexing.ParseDate/ParseVersionNumber used for .inf DriverVer
// fields, so an installed driver's version compares consistently
// against a candidate driver-pack entry's.
func readInstalledDriver(key registry.Key, device Device) InstalledDriver {
	var d InstalledDriver

	d.Desc = readRegString(key, "DriverDesc")
	d.ProviderName = readRegString(key, "ProviderName")
	d.MatchingDeviceID = readRegString(key, "MatchingDeviceId")
	d.InfPath = readRegString(key, "InfPath")
	d.InfSection = readRegString(key, "InfSection")
	d.InfSectionExt = readRegString(key, "InfSectionExt")

	d.DevPos, d.IsHardwareID = MatchDeviceID(device.HardwareIDs, device.CompatibleIDs, d.MatchingDeviceID)

	if driverDate := readRegString(key, "DriverDate"); driverDate != "" {
		d.Version = indexing.ParseDate(driverDate)
	}
	if driverVersion := readRegString(key, "DriverVersion"); driverVersion != "" {
		v := indexing.ParseVersionNumber(driverVersion)
		d.Version.SetVersion(v.V1, v.V2, v.V3, v.V4)
	}

	return d
}

// readRegString reads a string registry value, returning "" for any
// error (missing value, wrong type, etc.) rather than failing the
// whole InstalledDriver read - matching the original's read_reg_val,
// which logs but leaves the field unset on failure instead of
// aborting Driver construction.
func readRegString(key registry.Key, name string) string {
	v, _, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	return v
}
