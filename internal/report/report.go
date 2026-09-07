// Package report prints the scan report cmd/sdigo shows with -nogui,
// as plain text (Print) or as JSON for scripts/CI (PrintJSON). Both
// report a stable, hand-picked view of a scan.Result rather than
// marshaling it directly, so internal field changes don't silently
// change either output's contract.
package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"sdio/internal/collection"
	"sdio/internal/hardware"
	"sdio/internal/installflow"
	"sdio/internal/scan"
)

func missingReason(candidate *collection.Candidate) string {
	if candidate.Result.AltSectScore == 0 || !candidate.Result.IsDriverValid() {
		return "no valid candidate found"
	}
	return "already has an equal or better driver installed"
}

// WriteDeviceList writes one stable tab-separated row per scanned device.
func WriteDeviceList(w io.Writer, result scan.Result) error {
	csvWriter := csv.NewWriter(w)
	csvWriter.Comma = '\t'
	if err := csvWriter.Write([]string{"status", "description", "instance_id", "hardware_ids", "installed_provider", "installed_version", "installed_inf", "candidate_pack", "candidate_section", "candidate_version", "needs_download"}); err != nil {
		return err
	}
	for _, device := range result.Devices {
		best := device.Best()
		status := scan.StatusLabel(device.Status)
		if best != nil {
			status = scan.MatchLabel(best)
		}
		row := []string{status, device.Device.Description, device.Device.InstanceID, strings.Join(device.Device.HardwareIDs, "|")}
		if device.Installed != nil {
			row = append(row, device.Installed.ProviderName, device.Installed.Version.String(), device.Installed.InfPath)
		} else {
			row = append(row, "", "", "")
		}
		if best != nil {
			row = append(row, best.Driverpack.Filename, best.Result.Section, best.Result.DriverVersion.String(), fmt.Sprint(best.Driverpack.Pending))
		} else {
			row = append(row, "", "", "", "false")
		}
		if err := csvWriter.Write(row); err != nil {
			return err
		}
	}
	csvWriter.Flush()
	return csvWriter.Error()
}

// WriteDeviceListJSON writes the same structured JSON shape as -json.
func WriteDeviceListJSON(w io.Writer, result scan.Result) error {
	_, err := PrintJSON(w, result)
	return err
}

// Print writes a plain-text report of result to w, and returns every
// device with an actionable best candidate as an installflow.Pending
// list, for a caller that wants to install them.
func Print(w io.Writer, result scan.Result) []installflow.Pending {
	bb, si := result.System.BaseBoard, result.System.SysInfo

	if result.BootstrapError != nil {
		fmt.Fprintf(w, "warning: checking for index updates: %v\n", result.BootstrapError)
	} else if result.IndexesDownloaded > 0 {
		fmt.Fprintf(w, "Downloaded %d new/updated index file(s)\n", result.IndexesDownloaded)
	}

	fmt.Fprintf(w, "System: %s %s, Windows %d.%d build %d (%s), %d devices found\n",
		bb.SystemManufacturer, bb.SystemModel, si.Windows.Major, si.Windows.Minor, si.Windows.Build,
		archLabel(si.Is64Bit), len(result.Devices))
	fmt.Fprintf(w, "Driver packs: %d loaded, %d skipped\n", len(result.Collection.Packs), len(result.Collection.Skipped))
	for _, skipped := range result.Collection.Skipped {
		fmt.Fprintf(w, "  skipped %s: %v\n", skipped.Filename, skipped.Err)
	}

	var missing, matched int
	var pending []installflow.Pending
	for _, dr := range result.Devices {
		best := dr.Best()
		if best == nil {
			missing++
			switch {
			case len(dr.Candidates) == 0:
				fmt.Fprintf(w, "Missing  %-50s (%s)\n", dr.Device.Description, scan.StatusLabel(dr.Status))
			default:
				fmt.Fprintf(w, "Missing  %-50s (%s)\n", dr.Device.Description, missingReason(&dr.Candidates[0]))
			}
			continue
		}
		matched++
		onTorrent := ""
		if best.Driverpack.Pending {
			onTorrent = " [needs download]"
		}
		msNote := ""
		if hardware.IsMicrosoftDriver(dr.Installed) {
			// Excluded from -install's automatic pending list below (see
			// that comment) - flagged here too so a -install run's output
			// doesn't silently install fewer devices than "matched"
			// implies with no explanation.
			msNote = " [kept: Microsoft-provided driver]"
		}
		fmt.Fprintf(w, "%-8s %-50s -> %s (%s, %s)%s%s\n",
			scan.MatchLabel(best), dr.Device.Description, best.Driverpack.Filename, best.Result.Section, best.Result.DriverVersion, onTorrent, msNote)
		if msNote == "" {
			// A Microsoft-provided driver is often unnecessary and
			// riskier to replace than to keep (see cmd/sdigo's [MS] tag
			// and select-all exclusion for the same reasoning) - -install
			// has no per-device selection to opt out with, so the safe
			// default is to leave these alone rather than replace every
			// matched device unconditionally.
			pending = append(pending, installflow.Pending{Description: dr.Device.Description, Candidate: *best})
		}
	}

	fmt.Fprintf(w, "\n%d devices matched, %d missing/no driver found\n", matched, missing)
	return pending
}

// JSONReport is the machine-readable equivalent of Print's output, for
// scripts/CI that need a scan result without parsing terminal text.
type JSONReport struct {
	System       JSONSystem   `json:"system"`
	PacksLoaded  int          `json:"packsLoaded"`
	PacksSkipped int          `json:"packsSkipped"`
	Devices      []JSONDevice `json:"devices"`
	Matched      int          `json:"matched"`
	Missing      int          `json:"missing"`
}

// JSONSystem is the machine's identity as reported by JSONReport.
type JSONSystem struct {
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	WindowsMajor int    `json:"windowsMajor"`
	WindowsMinor int    `json:"windowsMinor"`
	WindowsBuild int    `json:"windowsBuild"`
	Arch         string `json:"arch"`
}

// JSONDevice is one device's scan outcome as reported by JSONReport.
// Reason and DriverPack/Section/Version/NeedsDownload are mutually
// exclusive: a device either has no best candidate (Reason explains
// why) or it does (the other fields describe it).
type JSONDevice struct {
	Description   string `json:"description"`
	MatchLabel    string `json:"matchLabel"`
	Reason        string `json:"reason,omitempty"`
	DriverPack    string `json:"driverPack,omitempty"`
	Section       string `json:"section,omitempty"`
	Version       string `json:"version,omitempty"`
	NeedsDownload bool   `json:"needsDownload,omitempty"`

	// KeptMicrosoftDriver is true when this device matched but was
	// excluded from the returned pending list because its currently
	// installed driver is Microsoft-provided (see
	// hardware.IsMicrosoftDriver) - replacing it is often unnecessary
	// and riskier than keeping it, so -install's automatic action
	// leaves it alone. Still counted in Matched; a caller that wants it
	// installed anyway has no per-device switch to ask for that with
	// today's -install (see cmd/sdigo's TUI for manual per-row control).
	KeptMicrosoftDriver bool `json:"keptMicrosoftDriver,omitempty"`
}

// PrintJSON writes result to w as indented JSON (see JSONReport) and
// returns the same installflow.Pending list Print returns, for a
// caller that wants to install the matched devices.
func PrintJSON(w io.Writer, result scan.Result) ([]installflow.Pending, error) {
	bb, si := result.System.BaseBoard, result.System.SysInfo
	rep := JSONReport{
		System: JSONSystem{
			Manufacturer: bb.SystemManufacturer,
			Model:        bb.SystemModel,
			WindowsMajor: si.Windows.Major,
			WindowsMinor: si.Windows.Minor,
			WindowsBuild: si.Windows.Build,
			Arch:         archLabel(si.Is64Bit),
		},
		PacksLoaded:  len(result.Collection.Packs),
		PacksSkipped: len(result.Collection.Skipped),
	}

	var pending []installflow.Pending
	for _, dr := range result.Devices {
		best := dr.Best()
		if best == nil {
			rep.Missing++
			jd := JSONDevice{Description: dr.Device.Description, MatchLabel: scan.StatusLabel(dr.Status)}
			switch {
			case len(dr.Candidates) == 0:
				jd.Reason = "no candidate driver pack found"
			default:
				jd.Reason = missingReason(&dr.Candidates[0])
			}
			rep.Devices = append(rep.Devices, jd)
			continue
		}
		rep.Matched++
		msDriver := hardware.IsMicrosoftDriver(dr.Installed)
		rep.Devices = append(rep.Devices, JSONDevice{
			Description:         dr.Device.Description,
			MatchLabel:          scan.MatchLabel(best),
			DriverPack:          best.Driverpack.Filename,
			Section:             best.Result.Section,
			Version:             best.Result.DriverVersion.String(),
			NeedsDownload:       best.Driverpack.Pending,
			KeptMicrosoftDriver: msDriver,
		})
		if !msDriver {
			pending = append(pending, installflow.Pending{Description: dr.Device.Description, Candidate: *best})
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		return nil, err
	}
	return pending, nil
}

func archLabel(is64Bit bool) string {
	if is64Bit {
		return "amd64"
	}
	return "x86"
}
