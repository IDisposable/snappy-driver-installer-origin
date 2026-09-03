package hardware

import "fmt"

// GUID mirrors a Win32 GUID. It's defined independently of
// golang.org/x/sys/windows (rather than reusing windows.GUID) so this
// file builds on any platform; device_windows.go converts between the
// two.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// String renders the GUID in the standard hyphenated hex form, ported
// from the raw-fallback branch of Device::print_guid (used when
// SetupDiGetClassDescription can't resolve a friendly class name).
func (g GUID) String() string {
	return fmt.Sprintf("%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X",
		g.Data1, g.Data2, g.Data3,
		g.Data4[0], g.Data4[1],
		g.Data4[2], g.Data4[3], g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7])
}

// DeviceStatus summarizes a device's runtime state, ported from
// Device::print_status() in enum.cpp.
type DeviceStatus int

const (
	DeviceNotPresent DeviceStatus = iota
	DeviceDisabled
	DeviceHasProblem
	DevicePrivateProblem
	DeviceRunning
	DeviceStopped
)

func (s DeviceStatus) String() string {
	switch s {
	case DeviceNotPresent:
		return "not present"
	case DeviceDisabled:
		return "disabled"
	case DeviceHasProblem:
		return "has a problem"
	case DevicePrivateProblem:
		return "has a private problem"
	case DeviceRunning:
		return "running"
	case DeviceStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// deviceCMProbDisabled is CM_PROB_DISABLED from cfgmgr32.h (Device
// Manager's well-known "Code 22": this device is disabled). Not
// exported by x/sys/windows, unlike the CR_*/DN_* constants used below.
const deviceCMProbDisabled = 0x16

const (
	dnStarted        = 0x00000008
	dnHasProblem     = 0x00000400
	dnPrivateProblem = 0x00008000
)

// deviceStatus computes a DeviceStatus from raw CM_Get_DevNode_Status
// output, ported from Device::print_status(). notPresent should be set
// when CM_Get_DevNode_Status failed with CR_NO_SUCH_DEVINST or
// CR_NO_SUCH_VALUE (a "phantom" device - no longer plugged in, but
// still listed).
func deviceStatus(notPresent bool, statusFlags, problem uint32) DeviceStatus {
	if notPresent {
		return DeviceNotPresent
	}
	if statusFlags&dnHasProblem != 0 && problem == deviceCMProbDisabled {
		return DeviceDisabled
	}
	if statusFlags&dnHasProblem != 0 {
		return DeviceHasProblem
	}
	if statusFlags&dnPrivateProblem != 0 {
		return DevicePrivateProblem
	}
	if statusFlags&dnStarted != 0 {
		return DeviceRunning
	}
	return DeviceStopped
}

// Device describes one enumerated Plug and Play device: the parts of
// the original Device class independent of driver-pack indexing.
// Currently-installed-driver introspection (the original's Driver
// class: a registry lookup plus .inf scanning against a Driverpack) is
// deferred until the indexing package it depends on is ported.
type Device struct {
	InstanceID    string
	Description   string
	HardwareIDs   []string
	CompatibleIDs []string
	DriverKeyName string // empty if the device has no driver registry key
	Manufacturer  string
	FriendlyName  string
	ClassGUID     GUID
	Capabilities  uint32
	ConfigFlags   uint32

	NotPresent     bool
	RawStatusFlags uint32
	Problem        uint32
}

// Status summarizes the device's runtime state.
func (d Device) Status() DeviceStatus {
	return deviceStatus(d.NotPresent, d.RawStatusFlags, d.Problem)
}
