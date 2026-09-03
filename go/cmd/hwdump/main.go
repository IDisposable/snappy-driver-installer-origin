// Command hwdump prints everything the internal/hardware package can
// detect about the current machine as JSON. It exists to cross-check
// the Go rewrite's hardware detection against a real Windows host: the
// dev environment is WSL, which can't call WMI/SetupAPI directly, but
// can execute a GOOS=windows binary through its PE interop and get
// real answers from the actual machine. See go/scripts/test-windows.sh.
package main

import (
	"encoding/json"
	"os"
	"strings"

	"sdio/internal/hardware"
)

type result struct {
	BaseBoard      *hardware.BaseBoard `json:"base_board,omitempty"`
	BaseBoardError string              `json:"base_board_error,omitempty"`

	SysInfo      *hardware.SysInfo `json:"sys_info,omitempty"`
	SysInfoError string            `json:"sys_info_error,omitempty"`

	DeviceCount      int      `json:"device_count,omitempty"`
	SampleDeviceDesc []string `json:"sample_device_desc,omitempty"`
	DevicesError     string   `json:"devices_error,omitempty"`

	IsLaptop *bool `json:"is_laptop,omitempty"`
}

func main() {
	var r result

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
		for i, d := range devices {
			if i < 10 {
				r.SampleDeviceDesc = append(r.SampleDeviceDesc, d.Description)
			}
			for _, id := range d.HardwareIDs {
				if strings.Contains(id, "*ACPI0003") {
					hasACPIBattery = true
				}
			}
		}
	}

	if r.BaseBoard != nil && r.SysInfo != nil {
		laptop := hardware.IsLaptop(r.BaseBoard.ChassisType, r.SysInfo.Monitors, r.SysInfo.Battery, hasACPIBattery)
		r.IsLaptop = &laptop
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		os.Exit(1)
	}
}
