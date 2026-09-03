// Package scan runs the full "detect hardware, load a driver-pack
// collection, match every device" pipeline that both cmd/sdi's
// plain-text report and any future TUI front end need, so that
// orchestration lives in one place rather than being duplicated across
// front ends.
package scan

import (
	"fmt"
	"strings"

	"sdio/internal/collection"
	"sdio/internal/hardware"
	"sdio/internal/indexing"
	"sdio/internal/matcher"
	"sdio/internal/settings"
)

// System summarizes the machine scan.cpp/enum.cpp gathers, the parts a
// front end needs to display and to build a MatchContext from.
type System struct {
	BaseBoard hardware.BaseBoard
	SysInfo   hardware.SysInfo
	IsLaptop  bool
}

// DeviceResult is one enumerated device plus its match outcome, ported
// from what a Devicematch (matcher.cpp) exposes to the UI.
type DeviceResult struct {
	Device     hardware.Device
	Status     int // matcher.Status* bits
	Candidates []collection.Candidate
}

// Best returns the top-ranked valid candidate, or nil if there isn't
// one (no candidates at all, or the best one has AltSectScore 0).
func (r DeviceResult) Best() *collection.Candidate {
	if len(r.Candidates) == 0 || r.Candidates[0].Result.AltSectScore == 0 {
		return nil
	}
	return &r.Candidates[0]
}

// Result is the outcome of a full scan.
type Result struct {
	System     System
	Collection collection.LoadResult
	Devices    []DeviceResult
}

// Run performs the full pipeline: hardware detection, driver-pack
// collection loading, and per-device matching, ported from the
// combination of State::getsysinfo_fast/Device enumeration
// (enum.cpp), Collection::scanfolder (indexing.cpp), and
// MatcherImp::populate (matcher.cpp) that a full SDIO run performs
// before showing its device list.
func Run(s *settings.Settings) (Result, error) {
	var res Result

	bb, err := hardware.GetBaseBoard()
	if err != nil {
		return res, fmt.Errorf("reading base board info: %w", err)
	}
	si, err := hardware.GetSysInfoFast()
	if err != nil {
		return res, fmt.Errorf("reading system info: %w", err)
	}
	devices, err := hardware.ScanDevices()
	if err != nil {
		return res, fmt.Errorf("scanning devices: %w", err)
	}

	hasACPIBattery := false
	for _, d := range devices {
		for _, id := range d.HardwareIDs {
			if strings.Contains(id, "*ACPI0003") {
				hasACPIBattery = true
			}
		}
	}
	isLaptop := hardware.IsLaptop(bb.ChassisType, si.Monitors, si.Battery, hasACPIBattery)
	marker := ""
	if isLaptop {
		marker = matcher.NotebookOEMMarker(bb.SystemManufacturer)
	}
	res.System = System{BaseBoard: bb, SysInfo: si, IsLaptop: isLaptop}

	res.Collection, err = collection.LoadCollection(s.DrpDir, s.IndexDir)
	if err != nil {
		return res, fmt.Errorf("loading driver-pack collection: %w", err)
	}

	ctx := indexing.MatchContext{
		Major: si.Windows.Major, Minor: si.Windows.Minor, Build: si.Windows.Build,
		IsAMD64: si.Is64Bit, IsLaptop: isLaptop, NotebookMarker: marker,
		FilterSP: s.Flags&settings.FlagFilterSP != 0,
	}

	res.Devices = make([]DeviceResult, 0, len(devices))
	for _, d := range devices {
		var installed *hardware.InstalledDriver
		if d.DriverKeyName != "" {
			if drv, err := hardware.OpenInstalledDriver(d.DriverKeyName, d); err == nil {
				installed = &drv
			}
		}

		dm := collection.Match(d, installed, res.Collection.Packs, ctx, s.IgnoreList)
		if dm.Status == matcher.StatusIgnored {
			continue
		}
		res.Devices = append(res.Devices, DeviceResult{Device: d, Status: dm.Status, Candidates: dm.Candidates})
	}

	return res, nil
}

// StatusLabel renders a device's status as a short human-readable
// string, for status values that carry meaning with no candidates
// (see DeviceResult.Best for the has-a-candidate case).
func StatusLabel(status int) string {
	switch status {
	case matcher.StatusNFMissing:
		return "driver missing"
	case matcher.StatusNFUnknown:
		return "unknown driver installed"
	case matcher.StatusNFStandard:
		return "standard driver installed, nothing better found"
	default:
		return fmt.Sprintf("status %#x", status)
	}
}
