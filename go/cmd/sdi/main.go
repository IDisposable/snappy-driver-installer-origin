// Command sdi is the CLI entry point for the Go rewrite: scans the
// current hardware, loads a driver-pack collection, matches every
// device to the best available driver, and prints a plain-text
// report. This replaces gui.cpp's "scan and show results" screen for
// now; a TUI (see go/README.md) isn't built yet.
//
// By default this only scans and reports - nothing on the system is
// changed. Pass -install to actually extract and install matched
// drivers via internal/installflow, a real system-modifying action.
package main

import (
	"fmt"
	"os"

	"sdio/internal/installflow"
	"sdio/internal/scan"
	"sdio/internal/settings"
)

func main() {
	os.Exit(mainErr())
}

func mainErr() int {
	s := settings.New()
	if err := s.LoadDefaultCfg(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: loading sdio.cfg:", err)
	}

	fs := s.FlagSet("sdi")
	doInstall := fs.Bool("install", false, "install matched drivers (modifies the system; without this flag, only scan and report)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}
	s.ExpandDirs()

	// Ported from main()'s unconditional Settings.save() after a run
	// completes, so switches given on the command line persist into
	// sdio.cfg for next time (Settings.Save itself honors
	// -preservecfg and only ever writes the persistent subset of
	// fields - never GUI-only state, which this rewrite doesn't have).
	// Deferred so it still runs even if run() below returns an error.
	defer func() {
		if err := s.Save(settings.DefaultCfgFilename); err != nil {
			fmt.Fprintln(os.Stderr, "warning: saving sdio.cfg:", err)
		}
	}()

	if err := run(s, *doInstall); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func run(s *settings.Settings, doInstall bool) error {
	result, err := scan.Run(s)
	if err != nil {
		return err
	}
	bb, si := result.System.BaseBoard, result.System.SysInfo

	if result.BootstrapError != nil {
		fmt.Fprintf(os.Stderr, "warning: checking for index updates: %v\n", result.BootstrapError)
	} else if result.IndexesDownloaded > 0 {
		fmt.Printf("Downloaded %d new/updated index file(s)\n", result.IndexesDownloaded)
	}

	fmt.Printf("System: %s %s, Windows %d.%d build %d (%s), %d devices found\n",
		bb.SystemManufacturer, bb.SystemModel, si.Windows.Major, si.Windows.Minor, si.Windows.Build,
		archLabel(si.Is64Bit), len(result.Devices))
	fmt.Printf("Driver packs: %d loaded, %d skipped\n", len(result.Collection.Packs), len(result.Collection.Skipped))
	for _, skipped := range result.Collection.Skipped {
		fmt.Printf("  skipped %s: %v\n", skipped.Filename, skipped.Err)
	}

	var missing, matched int
	var pending []installflow.Pending
	for _, dr := range result.Devices {
		best := dr.Best()
		if best == nil {
			missing++
			switch {
			case len(dr.Candidates) == 0:
				fmt.Printf("MISSING  %-50s (%s)\n", dr.Device.Description, scan.StatusLabel(dr.Status))
			case dr.Candidates[0].Result.AltSectScore == 0:
				fmt.Printf("MISSING  %-50s (no valid candidate found)\n", dr.Device.Description)
			default:
				fmt.Printf("MISSING  %-50s (already has an equal or better driver installed)\n", dr.Device.Description)
			}
			continue
		}
		matched++
		onTorrent := ""
		if best.Driverpack.Pending {
			onTorrent = " [needs download]"
		}
		fmt.Printf("FOUND    %-50s -> %s (%s, %s)%s\n",
			dr.Device.Description, best.Driverpack.Filename, best.Result.Section, best.Result.DriverVersion, onTorrent)
		pending = append(pending, installflow.Pending{Description: dr.Device.Description, Candidate: *best})
	}

	fmt.Printf("\n%d devices matched, %d missing/no driver found\n", matched, missing)

	if doInstall && len(pending) > 0 {
		installflow.Run(s, pending, os.Stdout)
	}
	return nil
}

func archLabel(is64Bit bool) string {
	if is64Bit {
		return "amd64"
	}
	return "x86"
}
