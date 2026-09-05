//go:build windows

package hardware

import (
	"fmt"

	"github.com/yusufpapurcu/wmi"
)

type win32BaseBoard struct {
	Manufacturer string
	Model        string
	Product      string
}

type win32ComputerSystem struct {
	Manufacturer string
	Model        string
}

type win32SystemEnclosure struct {
	ChassisTypes []int
}

// GetBaseBoard queries WMI for motherboard, computer system, and
// chassis identification. A failed query aborts the rest rather than
// leaving that section zero-valued, so a caller never mistakes an
// unqueried section for a real "unknown" answer; wmi.Query owns the
// underlying COM lifecycle, so there's no manual Release() bookkeeping
// here.
func GetBaseBoard() (BaseBoard, error) {
	var info BaseBoard

	var boards []win32BaseBoard
	if err := wmi.Query("SELECT Manufacturer, Model, Product FROM Win32_BaseBoard", &boards); err != nil {
		return info, fmt.Errorf("querying Win32_BaseBoard: %w", err)
	}
	if len(boards) > 0 {
		info.Manufacturer = boards[0].Manufacturer
		info.Model = boards[0].Model
		info.Product = boards[0].Product
	}

	var systems []win32ComputerSystem
	if err := wmi.Query("SELECT Manufacturer, Model FROM Win32_ComputerSystem", &systems); err != nil {
		return info, fmt.Errorf("querying Win32_ComputerSystem: %w", err)
	}
	if len(systems) > 0 {
		info.SystemManufacturer = systems[0].Manufacturer
		info.SystemModel = systems[0].Model
	}

	var enclosures []win32SystemEnclosure
	if err := wmi.Query("SELECT ChassisTypes FROM Win32_SystemEnclosure", &enclosures); err != nil {
		return info, fmt.Errorf("querying Win32_SystemEnclosure: %w", err)
	}
	if len(enclosures) > 0 {
		info.ChassisType = lastChassisType(enclosures[0].ChassisTypes)
	}

	return info, nil
}
