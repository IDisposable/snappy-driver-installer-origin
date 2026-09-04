// Command sdi is the CLI entry point for the Go rewrite: scans the
// current hardware, loads a driver-pack collection, matches every
// device to the best available driver, and prints a plain-text
// report (internal/report - shared with cmd/sdigo's -nogui mode).
//
// By default this only scans and reports - nothing on the system is
// changed. Pass -install to actually extract and install matched
// drivers via internal/installflow, a real system-modifying action.
package main

import (
	"fmt"
	"os"

	"sdio/internal/installflow"
	"sdio/internal/report"
	"sdio/internal/scan"
	"sdio/internal/settings"
)

func main() {
	os.Exit(mainErr())
}

func mainErr() int {
	s := settings.New()
	cfgPath, err := s.LoadDefaultCfgResolved()
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: loading sdio.cfg:", err)
	}

	fs := s.FlagSet("sdi")
	doInstall := fs.Bool("install", false, "install matched drivers (modifies the system; without this flag, only scan and report)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}
	s.ExpandDirs()

	// Persists on exit so switches given on the command line survive
	// to the next run (Settings.Save honors -preservecfg and only ever
	// writes the persistent subset of fields - never GUI-only state,
	// which this rewrite doesn't have). Deferred so it still runs even
	// if run() below returns an error. cfgPath is wherever
	// LoadDefaultCfgResolved decided sdio.cfg lives (exe-adjacent for
	// a portable install, %LOCALAPPDATA%\SDIO otherwise).
	defer func() {
		if err := s.Save(cfgPath); err != nil {
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
	pending := report.Print(os.Stdout, result)
	if doInstall && len(pending) > 0 {
		installflow.Run(s, pending, os.Stdout)
	}
	return nil
}
