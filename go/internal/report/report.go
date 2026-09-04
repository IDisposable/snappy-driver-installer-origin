// Package report prints a plain-text scan report, shared by cmd/sdi
// and cmd/sdigo's -nogui mode so there is one implementation of the
// "print what was found" output rather than two that can drift apart.
package report

import (
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

func archLabel(is64Bit bool) string {
	if is64Bit {
		return "amd64"
	}
	return "x86"
}
