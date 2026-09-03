// Package hardware queries the host's hardware identity and devices via
// WMI, replacing the raw COM/IWbemLocator calls in baseboard.cpp,
// system.cpp, and enum.cpp. WMI only exists on Windows, so each query
// has a real Windows implementation plus a non-Windows stub, keeping the
// package buildable and its pure logic testable on any platform.
package hardware

// BaseBoard holds identifying information about the system's
// motherboard, overall computer system, and chassis, ported from
// State::getbaseboard in baseboard.cpp (Win32_BaseBoard,
// Win32_ComputerSystem, Win32_SystemEnclosure).
type BaseBoard struct {
	Manufacturer string
	Model        string
	Product      string

	SystemManufacturer string
	SystemModel        string

	// ChassisType is a Win32_SystemEnclosure ChassisTypes code (e.g. 8
	// for "Portable", 10 for "Notebook"); 0 if unknown.
	ChassisType int
}

// lastChassisType mirrors the original's loop over the ChassisTypes
// array: it assigns *type on every iteration without breaking, so
// whichever element comes last in the array wins.
func lastChassisType(types []int) int {
	if len(types) == 0 {
		return 0
	}
	return types[len(types)-1]
}
