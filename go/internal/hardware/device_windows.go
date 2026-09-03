//go:build windows

package hardware

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// ScanDevices enumerates all present Plug and Play devices, ported
// from the device-enumeration half of State::scanDevices (the
// currently-installed-driver half - the original's Driver class - is
// deferred, see Device's doc comment).
func ScanDevices() ([]Device, error) {
	set, err := windows.SetupDiGetClassDevsEx(nil, "", 0, windows.DIGCF_PRESENT|windows.DIGCF_ALLCLASSES, 0, "")
	if err != nil {
		return nil, fmt.Errorf("SetupDiGetClassDevsEx: %w", err)
	}
	defer set.Close()

	var devices []Device
	for i := 0; ; i++ {
		data, err := set.EnumDeviceInfo(i)
		if err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
				break
			}
			continue
		}
		devices = append(devices, deviceFromInfo(set, data))
	}
	return devices, nil
}

func deviceFromInfo(set windows.DevInfo, data *windows.DevInfoData) Device {
	var d Device

	d.ClassGUID = GUID{
		Data1: data.ClassGUID.Data1,
		Data2: data.ClassGUID.Data2,
		Data3: data.ClassGUID.Data3,
		Data4: data.ClassGUID.Data4,
	}

	if id, err := set.DeviceInstanceID(data); err == nil {
		d.InstanceID = id
	}

	d.Description = regPropertyString(set, data, windows.SPDRP_DEVICEDESC)
	d.HardwareIDs = regPropertyStrings(set, data, windows.SPDRP_HARDWAREID)
	d.CompatibleIDs = regPropertyStrings(set, data, windows.SPDRP_COMPATIBLEIDS)
	d.DriverKeyName = regPropertyString(set, data, windows.SPDRP_DRIVER)
	d.Manufacturer = regPropertyString(set, data, windows.SPDRP_MFG)
	d.FriendlyName = regPropertyString(set, data, windows.SPDRP_FRIENDLYNAME)
	d.Capabilities = regPropertyUint32(set, data, windows.SPDRP_CAPABILITIES)
	d.ConfigFlags = regPropertyUint32(set, data, windows.SPDRP_CONFIGFLAGS)

	var status, problem uint32
	if err := windows.CM_Get_DevNode_Status(&status, &problem, data.DevInst, 0); err != nil {
		var cr windows.CONFIGRET
		if errors.As(err, &cr) && (cr == windows.CR_NO_SUCH_DEVINST || cr == windows.CR_NO_SUCH_VALUE) {
			d.NotPresent = true
		}
	}
	d.RawStatusFlags = status
	d.Problem = problem

	return d
}

func regPropertyString(set windows.DevInfo, data *windows.DevInfoData, prop windows.SPDRP) string {
	v, err := set.DeviceRegistryProperty(data, prop)
	if err != nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func regPropertyStrings(set windows.DevInfo, data *windows.DevInfoData, prop windows.SPDRP) []string {
	v, err := set.DeviceRegistryProperty(data, prop)
	if err != nil {
		return nil
	}
	switch s := v.(type) {
	case []string:
		return s
	case string:
		// Some drivers store a single-valued ID as REG_SZ rather than
		// the usual REG_MULTI_SZ.
		if s != "" {
			return []string{s}
		}
	}
	return nil
}

func regPropertyUint32(set windows.DevInfo, data *windows.DevInfoData, prop windows.SPDRP) uint32 {
	v, err := set.DeviceRegistryProperty(data, prop)
	if err != nil {
		return 0
	}
	n, _ := v.(uint32)
	return n
}
