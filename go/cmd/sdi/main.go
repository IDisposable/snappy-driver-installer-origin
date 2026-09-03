// Command sdi is the CLI entry point for the Go rewrite: scans the
// current hardware, loads a driver-pack collection, matches every
// device to the best available driver, and prints a plain-text
// report. This replaces gui.cpp's "scan and show results" screen for
// now; a TUI (see go/README.md) isn't built yet. Installing drivers
// isn't wired in here yet either - see internal/install.
package main

import (
	"fmt"
	"os"
	"strings"

	"sdio/internal/collection"
	"sdio/internal/hardware"
	"sdio/internal/indexing"
	"sdio/internal/matcher"
	"sdio/internal/settings"
)

func main() {
	s := settings.New()
	if err := s.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if err := run(s); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(s *settings.Settings) error {
	bb, err := hardware.GetBaseBoard()
	if err != nil {
		return fmt.Errorf("reading base board info: %w", err)
	}
	si, err := hardware.GetSysInfoFast()
	if err != nil {
		return fmt.Errorf("reading system info: %w", err)
	}
	devices, err := hardware.ScanDevices()
	if err != nil {
		return fmt.Errorf("scanning devices: %w", err)
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

	fmt.Printf("System: %s %s, Windows %d.%d build %d (%s), %d devices found\n",
		bb.SystemManufacturer, bb.SystemModel, si.Windows.Major, si.Windows.Minor, si.Windows.Build,
		archLabel(si.Is64Bit), len(devices))

	result, err := collection.LoadCollection(s.DrpDir, s.IndexDir)
	if err != nil {
		return fmt.Errorf("loading driver-pack collection: %w", err)
	}
	fmt.Printf("Driver packs: %d loaded, %d skipped\n", len(result.Packs), len(result.Skipped))
	for _, skipped := range result.Skipped {
		fmt.Printf("  skipped %s: %v\n", skipped.Filename, skipped.Err)
	}

	ctx := indexing.MatchContext{
		Major: si.Windows.Major, Minor: si.Windows.Minor, Build: si.Windows.Build,
		IsAMD64: si.Is64Bit, IsLaptop: isLaptop, NotebookMarker: marker,
		FilterSP: s.Flags&settings.FlagFilterSP != 0,
	}

	var missing, matched int
	for _, d := range devices {
		var installed *hardware.InstalledDriver
		if d.DriverKeyName != "" {
			if drv, err := hardware.OpenInstalledDriver(d.DriverKeyName, d); err == nil {
				installed = &drv
			}
		}

		dm := collection.Match(d, installed, result.Packs, ctx, s.IgnoreList)
		switch {
		case dm.Status == matcher.StatusIgnored:
			continue
		case len(dm.Candidates) == 0:
			missing++
			fmt.Printf("MISSING  %-50s (%s)\n", d.Description, statusLabel(dm.Status))
		default:
			best := dm.Candidates[0]
			if best.Result.AltSectScore == 0 {
				missing++
				fmt.Printf("MISSING  %-50s (no valid candidate found)\n", d.Description)
				continue
			}
			matched++
			fmt.Printf("FOUND    %-50s -> %s (%s, %s)\n",
				d.Description, best.Driverpack.Filename, best.Result.Section, best.Result.DriverVersion)
		}
	}

	fmt.Printf("\n%d devices matched, %d missing/no driver found\n", matched, missing)
	return nil
}

func archLabel(is64Bit bool) string {
	if is64Bit {
		return "amd64"
	}
	return "x86"
}

func statusLabel(status int) string {
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
