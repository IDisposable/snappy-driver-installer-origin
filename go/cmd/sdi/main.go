// Command sdi is the CLI entry point for the Go rewrite: scans the
// current hardware, loads a driver-pack collection, matches every
// device to the best available driver, and prints a plain-text
// report. This replaces gui.cpp's "scan and show results" screen for
// now; a TUI (see go/README.md) isn't built yet.
//
// By default this only scans and reports - nothing on the system is
// changed. Pass -install to actually extract and install matched
// drivers (internal/install), a real system-modifying action; see
// runInstall's doc comment.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sdio/internal/archive"
	"sdio/internal/collection"
	"sdio/internal/install"
	"sdio/internal/scan"
	"sdio/internal/settings"
)

func main() {
	s := settings.New()
	fs := s.FlagSet("sdi")
	doInstall := fs.Bool("install", false, "install matched drivers (modifies the system; without this flag, only scan and report)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	s.ExpandDirs()

	if err := run(s, *doInstall); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(s *settings.Settings, doInstall bool) error {
	result, err := scan.Run(s)
	if err != nil {
		return err
	}
	bb, si := result.System.BaseBoard, result.System.SysInfo

	fmt.Printf("System: %s %s, Windows %d.%d build %d (%s), %d devices found\n",
		bb.SystemManufacturer, bb.SystemModel, si.Windows.Major, si.Windows.Minor, si.Windows.Build,
		archLabel(si.Is64Bit), len(result.Devices))
	fmt.Printf("Driver packs: %d loaded, %d skipped\n", len(result.Collection.Packs), len(result.Collection.Skipped))
	for _, skipped := range result.Collection.Skipped {
		fmt.Printf("  skipped %s: %v\n", skipped.Filename, skipped.Err)
	}

	var missing, matched int
	var pending []pendingInstall
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
		fmt.Printf("FOUND    %-50s -> %s (%s, %s)\n",
			dr.Device.Description, best.Driverpack.Filename, best.Result.Section, best.Result.DriverVersion)
		pending = append(pending, pendingInstall{description: dr.Device.Description, candidate: *best})
	}

	fmt.Printf("\n%d devices matched, %d missing/no driver found\n", matched, missing)

	if doInstall && len(pending) > 0 {
		runInstall(s, pending)
	}
	return nil
}

// pendingInstall is a device matched to a candidate driver, queued for
// installation if -install was given.
type pendingInstall struct {
	description string
	candidate   collection.Candidate
}

// runInstall extracts and installs every pending candidate, ported
// from the per-device loop in Manager::thread_install: create one
// restore point up front (skipped if FlagDisableInstall is set,
// matching the original), then call internal/install.Driver for each
// device. This modifies the system - it is only reached when the
// caller passed -install explicitly.
func runInstall(s *settings.Settings, pending []pendingInstall) {
	if s.Flags&settings.FlagDisableInstall == 0 {
		if err := install.CreateRestorePoint(install.RestorePointDescription); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not create a restore point: %v\n", err)
		}
	}

	for _, p := range pending {
		if err := installOne(s, p); err != nil {
			fmt.Fprintf(os.Stderr, "install %s: %v\n", p.description, err)
		}
	}
}

func installOne(s *settings.Settings, p pendingInstall) error {
	drp := p.candidate.Driverpack
	infPath := drp.InfPath(p.candidate.HWIDIndex)   // e.g. `dt\allx64\DtPort_1.0.0.6\`
	infName := drp.InfName(p.candidate.HWIDIndex)   // e.g. "dtport.inf"
	prefix := strings.ReplaceAll(infPath, `\`, "/") // archive entries use "/"

	packPath := filepath.Join(drp.Path, drp.Filename)
	r, err := archive.Open(packPath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", packPath, err)
	}
	defer r.Close()

	destDir := filepath.Join(s.ExtractDir, filepath.FromSlash(strings.TrimSuffix(prefix, "/")))
	if _, err := r.ExtractPrefix(prefix, destDir); err != nil {
		return fmt.Errorf("extracting %s: %w", prefix, err)
	}
	extractedInf := filepath.Join(destDir, infName)

	if s.Flags&settings.FlagDisableInstall != 0 {
		fmt.Printf("INSTALL  %-50s (-disableinstall set, not actually installing)\n", p.description)
		return nil
	}

	res, err := install.Driver(0, p.candidate.Result.HWID, extractedInf)
	if err != nil {
		return err
	}
	reboot := ""
	if res.NeedsReboot {
		reboot = " (reboot required)"
	}
	fmt.Printf("INSTALLED %-50s%s\n", p.description, reboot)
	return nil
}

func archLabel(is64Bit bool) string {
	if is64Bit {
		return "amd64"
	}
	return "x86"
}
