package main

import (
	"encoding/json"
	"os"
	"strings"

	"sdio/internal/hardware"
	"sdio/internal/install"
)

// hwdumpMain prints everything internal/hardware can detect about the
// current machine as JSON ("sdigo hwdump"), for cross-checking this
// rewrite's hardware detection against a real Windows host - the dev
// environment is WSL, which can't call WMI/SetupAPI directly, but can
// execute a GOOS=windows binary through PE interop and get real
// answers from the actual machine.
func hwdumpMain() int {
	var r hwdumpResult

	bb, err := hardware.GetBaseBoard()
	if err != nil {
		r.BaseBoardError = err.Error()
	} else {
		r.BaseBoard = &bb
	}

	si, err := hardware.GetSysInfoFast()
	if err != nil {
		r.SysInfoError = err.Error()
	} else {
		r.SysInfo = &si
	}

	devices, err := hardware.ScanDevices()
	hasACPIBattery := false
	if err != nil {
		r.DevicesError = err.Error()
	} else {
		r.DeviceCount = len(devices)
		driversSampled := 0
		for _, d := range devices {
			if len(r.SampleDeviceDesc) < 10 {
				r.SampleDeviceDesc = append(r.SampleDeviceDesc, d.Description)
			}
			for _, id := range d.HardwareIDs {
				if strings.Contains(id, "*ACPI0003") {
					hasACPIBattery = true
				}
			}

			if d.DriverKeyName != "" && driversSampled < 5 {
				driversSampled++
				sample := hwdumpSampleDriver{Device: d.Description}
				drv, err := hardware.OpenInstalledDriver(d.DriverKeyName, d)
				if err != nil {
					sample.Error = err.Error()
				} else {
					sample.Desc = drv.Desc
					sample.Provider = drv.ProviderName
					sample.Version = drv.Version.String()
					sample.DevPos = drv.DevPos
					sample.IsHardwareID = drv.IsHardwareID
				}
				r.SampleInstalledDrivers = append(r.SampleInstalledDrivers, sample)
			}
		}
	}

	if r.BaseBoard != nil && r.SysInfo != nil {
		laptop := hardware.IsLaptop(r.BaseBoard.ChassisType, r.SysInfo.Monitors, r.SysInfo.Battery, hasACPIBattery)
		r.IsLaptop = &laptop
	}

	if err := install.CheckAvailable(); err != nil {
		r.InstallAPIError = err.Error()
	} else {
		r.InstallAPIAvailable = true
	}
	if freq, err := install.GetRestorePointCreationFrequency(); err != nil {
		r.RestorePointFreqErr = err.Error()
	} else {
		r.RestorePointFreqMin = freq
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return 1
	}
	return 0
}

type hwdumpResult struct {
	BaseBoard      *hardware.BaseBoard `json:"base_board,omitempty"`
	BaseBoardError string              `json:"base_board_error,omitempty"`

	SysInfo      *hardware.SysInfo `json:"sys_info,omitempty"`
	SysInfoError string            `json:"sys_info_error,omitempty"`

	DeviceCount      int      `json:"device_count,omitempty"`
	SampleDeviceDesc []string `json:"sample_device_desc,omitempty"`
	DevicesError     string   `json:"devices_error,omitempty"`

	SampleInstalledDrivers []hwdumpSampleDriver `json:"sample_installed_drivers,omitempty"`

	IsLaptop *bool `json:"is_laptop,omitempty"`

	InstallAPIAvailable bool   `json:"install_api_available"`
	InstallAPIError     string `json:"install_api_error,omitempty"`
	RestorePointFreqMin int    `json:"restore_point_frequency_min,omitempty"`
	RestorePointFreqErr string `json:"restore_point_frequency_error,omitempty"`
}

type hwdumpSampleDriver struct {
	Device       string `json:"device"`
	Desc         string `json:"desc"`
	Provider     string `json:"provider"`
	Version      string `json:"version"`
	DevPos       int    `json:"dev_pos"`
	IsHardwareID bool   `json:"is_hardware_id"`
	Error        string `json:"error,omitempty"`
}
