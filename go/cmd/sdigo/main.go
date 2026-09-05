// Command sdigo is Snappy Driver Installer: Go Forth, the single-EXE
// entry point for this Go rewrite of Snappy Driver Installer Origin:
// an interactive TUI by default (replacing gui.cpp/draw.cpp/theme*.cpp's
// device-list screen - see go/README.md), or a plain-text report
// (internal/report.Print) when -nogui is set. The TUI shows a
// scrollable table with an options screen (all engine flags and
// display filters), a per-device detail screen, and per-row selection
// wired to the real install path (internal/installflow). "sdigo
// cleandrivers" removes superseded driver-pack files (replacing
// del_old_driverpacks.bat); "sdigo hwdump"/"sdigo torrenttest" are
// dev/diagnostic subcommands with no end-user purpose of their own.
//
// The TUI's implementation is split across this package's other
// files by screen/responsibility: model.go (core model type and the
// Init/Update/View lifecycle), table.go (main device table),
// options.go (engine-flag and filter screens), detail.go (per-device
// detail screen and installed-vs-candidate comparison), install.go
// (confirm-install and the shared operation log screen), downloadmenu.go
// (the download menu and its background downloads), progress.go
// (shared download/scan progress tracking), usb.go (USB portable-copy
// screens), splash.go (startup splash screen), and styles.go (shared
// lipgloss styles).
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rs/zerolog"

	"sdio/internal/install"
	"sdio/internal/installflow"
	"sdio/internal/logging"
	"sdio/internal/report"
	"sdio/internal/scan"
	"sdio/internal/settings"
)

func main() {
	os.Exit(mainErr())
}

// mainErr dispatches "sdigo hwdump"/"sdigo torrenttest"/"sdigo
// cleandrivers" as subcommands before falling through to the normal
// scan/TUI/-nogui path, so this is the only executable a release
// needs to build. hwdump and torrenttest are dev/diagnostic tools
// with no end-user purpose of their own; cleandrivers replaces the
// original's del_old_driverpacks.bat.
func mainErr() int {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hwdump":
			return hwDump()
		case "torrenttest":
			return torrentTest(os.Args[2:])
		case "cleandrivers":
			return cleanDrivers(os.Args[2:])
		}
	}
	return sdiGo(os.Args[1:])
}

func sdiGo(args []string) int {
	s := settings.New()
	cfgPath, err := s.LoadDefaultCfgResolved()
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: loading sdio.cfg:", err)
	}

	fs := s.FlagSet("sdigo")
	doInstall := fs.Bool("install", false, "with -nogui, install matched drivers (modifies the system; without this flag, only scan and report)")
	doJSON := fs.Bool("json", false, "with -nogui, report as JSON instead of plain text (for scripts/CI)")
	resumeFile := fs.String("elevated-resume", "", "internal use only: set by sdigo's own elevation relaunch to restore the device selection confirmed before elevating")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	s.ExpandDirs()

	// The manifest requests no elevation at launch, since browsing scan
	// results needs none - only an actual install does. resumeSelected
	// carries a TUI selection across the elevation relaunch triggered
	// by updateConfirmInstall (see below); the file is one-shot and
	// consumed here regardless of whether elevation actually succeeded
	// this time, so a failed relaunch doesn't leave it behind.
	var resumeSelected map[string]bool
	if *resumeFile != "" {
		resumeSelected = readResumeFile(*resumeFile)
		os.Remove(*resumeFile)
	}

	// Persists on exit even on an error return, so switches given on
	// the command line and options-screen toggles both survive to the
	// next run - the TUI has no separate "save" action. cfgPath is
	// wherever LoadDefaultCfgResolved decided sdio.cfg lives (exe-
	// adjacent for a portable install, %LOCALAPPDATA%\SDIO otherwise).
	defer func() {
		if err := s.Save(cfgPath); err != nil {
			fmt.Fprintln(os.Stderr, "warning: saving sdio.cfg:", err)
		}
	}()

	// console is always nil since the TUI owns the whole terminal via
	// bubbletea's alternate screen, and -nogui's own output already
	// goes through fmt.Fprint*/report.Print directly, not this logger.
	// -nologfile skips Start entirely rather than opening a file and
	// discarding writes some other way, so "don't write a log file" is
	// literally true, not just unused. InfoLevel (not WarnLevel) so the
	// operational trail every background command now writes (scan,
	// install, download start/complete/failure, recovered panics -
	// see logPanic) actually reaches the file: a WarnLevel logger was
	// found live to produce a file with nothing in it but the start
	// marker even through a real crash, since every one of those calls
	// is Info-level.
	logger := logging.New(zerolog.InfoLevel, nil)
	if s.Flags&settings.FlagNoLogFile == 0 {
		if err := logger.Start(s.LogDir, logging.Timestamp()); err != nil {
			fmt.Fprintln(os.Stderr, "warning: starting log file:", err)
		}
	}
	defer logger.Stop()
	// Covers a panic anywhere in sdiGo's own synchronous code (notably
	// the whole -nogui path, which never reaches bubbletea/logPanic's
	// per-command recovery at all) - logged before re-panicking so the
	// log file still has it even though a -nogui panic already prints
	// its own stack trace to stderr, unlike the TUI's alt-screen case.
	defer func() {
		if r := recover(); r != nil {
			logger.Error().
				Interface("panic", r).
				Str("stack", string(debug.Stack())).
				Msg("recovered panic in sdiGo")
			panic(r)
		}
	}()
	alertLogger := func(level, message string) {
		if s.Flags&settings.FlagTorrentAlerts != 0 {
			logger.Warn().Str("level", level).Msg(message)
		}
	}

	// -nogui prints a scan report instead of launching the interactive
	// table - plain text by default, or JSON with -json for scripts/CI
	// that need to parse the result without a terminal. Unlike the TUI
	// (see newModel/Init), there's no terminal to keep responsive here,
	// so scan.Run's synchronous bootstrap-then-scan is fine as is.
	if s.Flags&settings.FlagNoGUI != 0 {
		result, err := scan.Run(s, logger)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		var pending []installflow.Pending
		if *doJSON {
			var err error
			if pending, err = report.PrintJSON(os.Stdout, result); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				return 1
			}
		} else {
			pending = report.Print(os.Stdout, result)
		}
		if *doInstall && len(pending) > 0 {
			if !install.IsElevated() {
				return relaunchElevated(s, cfgPath, args)
			}
			if !installflow.Run(s, pending, os.Stdout, alertLogger, nil, logger) {
				return 1
			}
		}
		return 0
	}

	p := tea.NewProgram(newModel(s, resumeSelected, logger), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		// Covers a panic bubbletea's own recovery caught (see logPanic's
		// doc comment for why that alone isn't enough to diagnose one) as
		// well as any other way the program loop itself failed - either
		// way, the log file is the only place this is guaranteed to still
		// be readable once the terminal's been restored.
		logger.Error().Err(err).Msg("program exited with an error")
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if fm, ok := final.(model); ok && len(fm.relaunchInstanceIDs) > 0 {
		return relaunchElevated(s, cfgPath, append(append([]string{}, args...), "-elevated-resume", writeResumeFile(fm.relaunchInstanceIDs)))
	}
	return 0
}

// relaunchElevated saves the current settings (so the elevated copy
// sees any options-screen change made before confirming install, not
// a stale on-disk file) and starts an elevated copy of this same
// binary with relaunchArgs as its command line. The original process
// exits right after; the two processes share no further state beyond
// sdio.cfg and, when present, the -elevated-resume file relaunchArgs
// points at.
func relaunchElevated(s *settings.Settings, cfgPath string, relaunchArgs []string) int {
	if err := s.Save(cfgPath); err != nil {
		fmt.Fprintln(os.Stderr, "warning: saving sdio.cfg:", err)
	}
	if err := install.RelaunchElevated(relaunchArgs); err != nil {
		fmt.Fprintln(os.Stderr, "error: relaunching elevated for install:", err)
		return 1
	}
	return 0
}

// writeResumeFile saves ids (ticked device InstanceIDs) to a one-shot
// temp file for -elevated-resume to read back after the elevation
// relaunch, and returns its path. A write failure is reported to
// stderr and yields an empty path, which -elevated-resume treats the
// same as "no selection to restore" rather than crashing the relaunch.
func writeResumeFile(ids []string) string {
	f, err := os.CreateTemp("", "sdigo-resume-*.txt")
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: preparing elevated relaunch:", err)
		return ""
	}
	defer f.Close()
	if _, err := f.WriteString(strings.Join(ids, "\n")); err != nil {
		fmt.Fprintln(os.Stderr, "warning: preparing elevated relaunch:", err)
		return ""
	}
	return f.Name()
}

// readResumeFile reads back what writeResumeFile wrote, as a selected
// map keyed the same way as model.selected. A missing/unreadable file
// (relaunch never happened, or -elevated-resume was passed by hand
// with a bad path) yields an empty selection rather than an error -
// this flag has no other effect, so there's nothing to fail.
func readResumeFile(path string) map[string]bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	selected := map[string]bool{}
	for _, id := range strings.Split(string(data), "\n") {
		if id != "" {
			selected[id] = true
		}
	}
	return selected
}
