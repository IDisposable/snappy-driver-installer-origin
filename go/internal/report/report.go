// Package report prints the scan report cmd/sdigo shows with -nogui,
// as plain text (Print) or as JSON for scripts/CI (PrintJSON). Both
// report a stable, hand-picked view of a scan.Result rather than
// marshaling it directly, so internal field changes don't silently
// change either output's contract.
package report

import (
	"encoding/json"
	"fmt"
	"io"

	"sdio/internal/installflow"
	"sdio/internal/scan"
)

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
			case dr.Candidates[0].Result.AltSectScore == 0:
				fmt.Fprintf(w, "Missing  %-50s (no valid candidate found)\n", dr.Device.Description)
			default:
				fmt.Fprintf(w, "Missing  %-50s (already has an equal or better driver installed)\n", dr.Device.Description)
			}
			continue
		}
		matched++
		onTorrent := ""
		if best.Driverpack.Pending {
			onTorrent = " [needs download]"
		}
		fmt.Fprintf(w, "%-8s %-50s -> %s (%s, %s)%s\n",
			scan.MatchLabel(best), dr.Device.Description, best.Driverpack.Filename, best.Result.Section, best.Result.DriverVersion, onTorrent)
		pending = append(pending, installflow.Pending{Description: dr.Device.Description, Candidate: *best})
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
			case dr.Candidates[0].Result.AltSectScore == 0:
				jd.Reason = "no valid candidate found"
			default:
				jd.Reason = "already has an equal or better driver installed"
			}
			rep.Devices = append(rep.Devices, jd)
			continue
		}
		rep.Matched++
		rep.Devices = append(rep.Devices, JSONDevice{
			Description:   dr.Device.Description,
			MatchLabel:    scan.MatchLabel(best),
			DriverPack:    best.Driverpack.Filename,
			Section:       best.Result.Section,
			Version:       best.Result.DriverVersion.String(),
			NeedsDownload: best.Driverpack.Pending,
		})
		pending = append(pending, installflow.Pending{Description: dr.Device.Description, Candidate: *best})
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
