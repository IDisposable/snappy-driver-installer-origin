package main

import (
	"bytes"
	"context"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sdio/internal/install"
	"sdio/internal/logging"
	"sdio/internal/scan"
	"sdio/internal/settings"
	"sdio/internal/update"
	"sdio/internal/usbdrive"
)

// screen selects which of the TUI's views is active.
type screen int

const (
	// screenSplash is the zero value, so a freshly constructed model
	// (see newModel) starts here before Init's background scan has
	// produced anything to show - the ASCII splash is purely cosmetic
	// and never gates the scan itself, which is already running
	// underneath it (see tickSplashCmd/splashDoneMsg).
	screenSplash screen = iota
	screenScanning
	screenTable
	screenOptions
	screenFilters
	screenDetail
	screenConfirmInstall
	screenInstalling
	screenInstallLog
	screenAbout
	screenWelcome
	screenDownloading
	screenUSBDrive
	screenUSBDriveConfirm
	screenUSBDriveCopying
)

type model struct {
	s      *settings.Settings
	result scan.Result

	// prepared caches scan.Prepare's hardware-detection output so a
	// rescan after a background download (downloadDoneMsg/
	// installDoneMsg) only has to redo collection loading and matching,
	// not device enumeration.
	prepared scan.Prepared

	// pendingResumeSelected is newModel's resumeSelected argument, held
	// here until scanDoneMsg has a result to apply it against - the
	// scan itself is now async (see Init), so it can't be applied at
	// construction time like it used to be.
	pendingResumeSelected map[string]bool

	table            table.Model
	rows             []scan.DeviceResult // parallel to table.Rows(), for cursor -> device lookup
	width, height    int
	showInstalledCol bool
	// bestMatchWidth/versionWidth are the last-computed column widths
	// (layoutColumns), cached here so a selection-only row rebuild
	// (ticking one row, select-all/none) can refit cell content
	// without recomputing the whole layout.
	bestMatchWidth, versionWidth int

	// detailViewport scrolls the detail screen's content - it's often
	// taller than the terminal, so unlike every other screen it needs
	// to be more than a plain string.
	detailViewport viewport.Model

	matched, missing int
	selected         map[string]bool // keyed by Device.InstanceID

	screen      screen
	options     []optionItem // engine flags (screenOptions) - see buildFlagItems
	optionIndex int

	filterOptions []optionItem // display filters (screenFilters) - see buildFilterItems
	filterIndex   int

	// opLog holds the captured output of whichever background
	// operation last ran (install or a Welcome-screen download) -
	// screenInstallLog renders it regardless of which one produced it.
	opLog []string

	// opLogIsError is true when opLog reports a failure rather than a
	// completed operation - screenInstallLog then refuses to treat
	// "enter" as a dismiss key (see updateTable's screenInstallLog
	// case), only "esc"/"q". A real download failure was observed to
	// flash by unread: "enter" both confirms a Welcome-screen menu
	// choice and dismisses the log screen that immediately follows a
	// fast failure, so an ordinary press-and-release (or a repeated
	// keypress out of habit) can blow straight through an error message
	// nobody had a chance to read.
	opLogIsError bool

	// opLogReturnScreen is where "q"/"esc"/"enter" sends the user once
	// they dismiss screenInstallLog - screenTable for anything launched
	// from the main table (install, USB copy, a startup auto-download),
	// screenWelcome for a download-menu task, so each menu item returns
	// to the menu it came from once it finishes instead of always
	// dropping back to the driver list.
	opLogReturnScreen screen

	// dlProgress is set whenever screenInstalling/screenDownloading
	// starts a background download, so their View can poll it for a
	// live percent/bytes/speed readout instead of a static message.
	dlProgress *progressTracker

	// dlCancel stops a screenDownloading run early (see
	// startDownload/"esc" in updateTable's screenDownloading
	// case) - scoped to Welcome-screen downloads only, not
	// screenInstalling, since interrupting an in-progress driver
	// install (rather than just a download feeding it) risks leaving a
	// device half-configured. Already-completed files are still saved,
	// not discarded - see update.DownloadDriverPacks/
	// collection.BootstrapIndexes's ctx parameter. nil when no
	// cancelable download is running.
	dlCancel context.CancelFunc
	// dlCancelling is set the moment "esc" requests a stop, so the view
	// can say so immediately instead of looking unresponsive for
	// however long the in-flight torrent I/O takes to actually unwind.
	dlCancelling bool

	// scanProgress is populated by Init's background collection load,
	// so screenScanning's View can show which driver pack is loading
	// instead of a static message for however long a full collection
	// (100+ packs) takes.
	scanProgress *scanProgressTracker

	// logger is file-only (console suppressed - this TUI owns the
	// terminal via the alternate screen buffer). Always non-nil and
	// safe to log through even with FlagNoLogFile set: sdiGo simply
	// never calls Start in that case, so writes go to logging.New's
	// default io.Discard sink instead of a file.
	logger *logging.Logger

	downloadIndex int

	usbDrives     []usbdrive.Drive
	usbDriveIndex int

	// relaunchInstanceIDs is set by updateConfirmInstall when install
	// is confirmed without an elevated token, instead of installing
	// directly - sdiGo checks it once p.Run() returns and, if set,
	// hands off to an elevated relaunch carrying this selection (see
	// relaunchElevated/writeResumeFile).
	relaunchInstanceIDs []string
}

// refreshTable recomputes the visible device list and the table's
// columns/rows together, since which columns exist (Installed) and
// which rows are shown (filters) can each change independently but
// both require rebuilding rows in lockstep to stay aligned.
func (m *model) refreshTable() {
	m.rows = visibleDevices(m.result.Devices, m.s.Filters)
	cols, showInstalled := layoutColumns(m.width, m.rows)
	m.showInstalledCol = showInstalled
	m.bestMatchWidth, m.versionWidth = cols[3].Width, cols[4].Width

	// SetColumns re-renders immediately against whatever rows are
	// already loaded. If the column count is changing (showInstalled
	// flipping on a resize) those old rows are the wrong shape for the
	// new columns, and bubbles/table indexes off the end of them.
	// Clearing first means SetColumns has nothing to render, and the
	// real SetRows call below always matches the columns already in
	// place. SetRows(nil) also resets the cursor to -1 with nothing to
	// clamp it back afterward, so it's saved and restored explicitly.
	cursor := m.table.Cursor()
	m.table.SetRows(nil)
	m.table.SetColumns(cols)
	m.table.SetRows(tableRows(m.rows, m.selected, showInstalled, m.bestMatchWidth, m.versionWidth))
	m.table.SetCursor(cursor)
}

// logPanic recovers a panic in a background tea.Cmd goroutine and logs
// it via zerolog (full stack trace, structured fields) before letting
// it continue to propagate. bubbletea's own recovery (see its tea.go)
// still catches it after that, restores the terminal, and reports a
// generic "program panicked" error - but its own diagnostic print goes
// to stdout while still inside the alt-screen buffer, which can be
// overwritten or never actually flushed to a visible terminal before
// the process exits. Reported live: a resumed download crashed with
// nothing in the log file but the startup marker, nothing on screen
// either. op names which background command panicked, since a log
// covers many different operations over a session. Call as the first
// line of a tea.Cmd closure via defer.
func logPanic(logger *logging.Logger, op string) {
	if r := recover(); r != nil {
		logger.Error().
			Str("op", op).
			Interface("panic", r).
			Str("stack", string(debug.Stack())).
			Msg("recovered panic in background command")
		panic(r)
	}
}

// alertLogger builds an update.Config.OnAlert callback that logs
// through m.logger when -torrentalerts is set, and otherwise silently
// discards the torrent client's own Warning-or-higher events (see
// update.Config.OnAlert's doc comment for why they can't just go to
// the default stderr handler). Checks the flag per call, not once at
// construction, so toggling it live on the options screen takes
// effect on the next event instead of needing a fresh download.
func (m model) alertLogger() func(level, message string) {
	return func(level, message string) {
		if m.s.Flags&settings.FlagTorrentAlerts != 0 {
			m.logger.Warn().Str("level", level).Msg(message)
		}
	}
}

// startDownload prepares model state for a screenDownloading
// run - a fresh progress tracker and a cancelable context, so "esc"
// can stop the operation early (see updateTable's screenDownloading
// case) rather than the only way out being to force-kill the whole
// process, which was observed to leave the torrent client's on-disk
// state in a shape that crashed on the next resume. Returns the
// context to pass into whichever run*Cmd is about to start.
func (m *model) startDownload() context.Context {
	m.dlProgress = &progressTracker{}
	m.dlCancelling = false
	ctx, cancel := context.WithCancel(context.Background())
	m.dlCancel = cancel
	return ctx
}

// currentDevice returns the device under the table's cursor, or nil
// if there are no visible rows.
func (m *model) currentDevice() *scan.DeviceResult {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.rows) {
		return nil
	}
	return &m.rows[i]
}

// newModel builds the initial TUI state. resumeSelected, when
// non-empty, restores a device selection confirmed just before an
// elevation relaunch (see sdiGo/relaunchElevated) - the model starts
// straight on the confirm-install screen instead of the table, so the
// user's "y" isn't silently dropped by the elevation round trip.
// newModel builds the initial TUI state before anything has been
// scanned yet - Init kicks off the real scan work in the background
// (see scanDoneMsg) so the program can render a "please wait" screen
// right away instead of leaving the terminal silent for however long
// hardware detection and driver-pack matching take.
func newModel(s *settings.Settings, resumeSelected map[string]bool, logger *logging.Logger) model {
	t := table.New(table.WithFocused(true), table.WithHeight(20))
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true).BorderStyle(lipgloss.NormalBorder()).BorderBottom(true)
	styles.Selected = styles.Selected.Bold(true)
	t.SetStyles(styles)

	return model{
		table: t, s: s,
		options: buildFlagItems(), filterOptions: buildFilterItems(), selected: map[string]bool{},
		width: 100, height: 30,
		detailViewport:        viewport.New(100, 24),
		scanProgress:          &scanProgressTracker{},
		logger:                logger,
		pendingResumeSelected: resumeSelected,
	}
}

// scanDoneMsg carries scan.Prepare/MatchWithCollection's result back
// to Update once the background scan Init launches finishes.
type scanDoneMsg struct {
	prepared scan.Prepared
	result   scan.Result
	err      error
}

// Init runs the real scan in the background instead of blocking
// program startup on it - see scanDoneMsg and screenScanning's View.
// It also starts the progressTickMsg loop so screenScanning's View
// picks up m.scanProgress as the collection loads, and the splash
// timer that moves screenSplash along to screenScanning on its own.
func (m model) Init() tea.Cmd {
	scanCmd := func() tea.Msg {
		defer logPanic(m.logger, "scan")
		m.logger.Info().Msg("starting hardware scan")
		p, err := scan.Prepare(m.s, m.logger)
		if err != nil {
			m.logger.Error().Err(err).Msg("hardware scan failed")
			return scanDoneMsg{err: err}
		}
		res, err := scan.MatchWithCollection(m.s, p, m.scanProgress.report)
		if err != nil {
			m.logger.Error().Err(err).Msg("loading driver-pack collection failed")
		} else {
			m.logger.Info().
				Int("devices", len(res.Devices)).
				Int("packsLoaded", len(res.Collection.Packs)).
				Int("packsSkipped", len(res.Collection.Skipped)).
				Msg("scan complete")
		}
		return scanDoneMsg{prepared: p, result: res, err: err}
	}
	return tea.Batch(scanCmd, tickProgressCmd(), tickSplashCmd())
}

// applyResult populates the model from a fresh scan.Result - the
// initial scan, or a rescan after a background download may have
// changed what's on disk (see downloadDoneMsg/installDoneMsg).
func (m *model) applyResult(result scan.Result) {
	m.result = result
	m.matched, m.missing = 0, 0
	for _, dr := range result.Devices {
		if dr.Best() != nil {
			m.matched++
		} else {
			m.missing++
		}
	}
	m.refreshTable()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.table.SetWidth(m.width)
		if h := m.height - 6; h > 0 {
			m.table.SetHeight(h)
		}
		m.refreshTable()

		m.detailViewport.Width = m.width
		if h := m.height - 2; h > 0 {
			m.detailViewport.Height = h
		} else {
			m.detailViewport.Height = 1
		}
		return m, nil

	case scanDoneMsg:
		if msg.err != nil {
			m.opLog = []string{fmt.Sprintf("error: %v", msg.err)}
			m.opLogIsError = true
			m.opLogReturnScreen = screenTable
			m.screen = screenInstallLog
			return m, nil
		}
		m.prepared = msg.prepared
		m.applyResult(msg.result)
		m.screen = screenTable
		if msg.result.FirstRun {
			m.screen = screenWelcome
		}

		var cmd tea.Cmd
		switch {
		case m.s.Flags&settings.FlagCheckUpdates != 0:
			// Ported from -checkupdates' documented purpose ("check for
			// driver pack updates") - runs as its own background step
			// after the table's first render instead of blocking it, the
			// way the original's launch-time bootstrap did.
			m.screen = screenDownloading
			m.opLogReturnScreen = screenTable
			ctx := m.startDownload()
			cmd = tea.Batch(runIndexRefreshCmd(ctx, m.s, m.dlProgress, m.alertLogger(), m.logger), tickProgressCmd())
		case m.s.Flags&settings.FlagAutoUpdate != 0:
			// Ported from the "-autoupdate command line parameter" launch-
			// time auto-trigger (update.cpp) - fires once, right after the
			// first scan.
			m.screen = screenDownloading
			m.opLogReturnScreen = screenTable
			ctx := m.startDownload()
			cmd = tea.Batch(runDownloadCmd(ctx, m.s, m.downloadFilter(update.AllDriverPacks), m.dlProgress, m.alertLogger(), m.logger), tickProgressCmd())
		}
		if len(m.pendingResumeSelected) > 0 {
			// An elevation relaunch resuming a confirmed selection wins
			// over any auto-triggered download - the user already
			// committed to installing before elevating.
			m.selected = m.pendingResumeSelected
			m.screen = screenConfirmInstall
			cmd = nil
		}
		return m, cmd

	case installDoneMsg:
		m.opLog = msg.log
		m.opLogIsError = msg.isErr
		m.opLogReturnScreen = screenTable
		m.screen = screenInstallLog
		// A completed install invalidates the ticked devices' old
		// candidate state (they may now already have that driver), and
		// may have changed the Installed comparison for other devices
		// too - MatchWithCollection re-reads each device's installed
		// driver from the registry, so this also picks up the version
		// just installed instead of showing stale "not installed"/older
		// data until the next full restart.
		m.selected = map[string]bool{}
		if res, err := scan.MatchWithCollection(m.s, m.prepared, nil); err == nil {
			m.applyResult(res)
		}
		if m.s.Flags&settings.FlagAutoClose != 0 && !msg.isErr {
			// Ported from Manager::thread_install's PostMessage(WM_CLOSE)
			// once an install finishes - exits straight from here rather
			// than waiting at the log screen, matching an unattended run.
			// Skipped on failure - -autoclose means "close once done," not
			// "close and hide that it failed."
			return m, tea.Quit
		}
		if m.s.Flags&settings.FlagFinishReboot != 0 && !msg.isErr {
			if err := install.Reboot(); err != nil {
				m.opLog = append(m.opLog, fmt.Sprintf("error: rebooting after install: %v", err))
				m.opLogIsError = true
			}
		}
		return m, nil

	case downloadDoneMsg:
		m.opLog = msg.log
		m.opLogIsError = msg.isErr
		m.screen = screenInstallLog
		m.dlCancel = nil
		m.dlCancelling = false
		// Whatever just downloaded (indexes, driver packs) may have
		// changed what's on disk - rescan so the table reflects it
		// instead of showing stale "needs download"/missing rows until
		// the next full restart.
		if res, err := scan.MatchWithCollection(m.s, m.prepared, nil); err == nil {
			m.applyResult(res)
		}
		if m.s.Flags&settings.FlagAutoClose != 0 && !msg.isErr {
			// Ported from Updater_t's "Torrent finished" auto-exit
			// (update.cpp) - the original skips this when -autoinstall is
			// also set, since it closes after the install that follows
			// instead; this rewrite has no -autoinstall to chain into, so
			// there's nothing to wait for. Skipped on a failed download -
			// -autoclose means "close once done," not "close and hide
			// that it failed."
			return m, tea.Quit
		}
		return m, nil

	case progressTickMsg:
		// Only reschedule while a progress-driven screen is still
		// active - a tick delivered after the operation finished and
		// the screen already moved on would otherwise keep ticking
		// forever.
		if m.screen == screenInstalling || m.screen == screenDownloading || m.screen == screenScanning {
			return m, tickProgressCmd()
		}
		return m, nil

	case splashDoneMsg:
		// Only advance if still on the splash - a scan that finished
		// first already moved m.screen on to screenTable/screenWelcome/
		// screenInstallLog, and this stale timer firing after that must
		// not knock it back to screenScanning.
		if m.screen == screenSplash {
			m.screen = screenScanning
		}
		return m, nil

	case tea.KeyMsg:
		if m.screen == screenSplash {
			// Any key dismisses the splash early, straight to whatever
			// screenScanning currently has to show (still loading, or
			// already past it if the scan won the race).
			m.screen = screenScanning
			return m, nil
		}
		switch m.screen {
		case screenOptions:
			return m.updateOptions(msg)
		case screenFilters:
			return m.updateFilters(msg)
		case screenDetail:
			return m.updateDetail(msg)
		case screenConfirmInstall:
			return m.updateConfirmInstall(msg)
		case screenInstalling:
			return m, nil // ignore input while the install command is running
		case screenInstallLog:
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "q", "esc":
				m.screen = m.opLogReturnScreen
				return m, nil
			case "enter":
				// Not honored on a failure (see model.opLogIsError's doc
				// comment) - "enter" is also the key that starts most
				// operations this screen reports on, so a fast failure
				// followed by a habitual second "enter" would otherwise
				// dismiss the error before anyone could read it.
				if !m.opLogIsError {
					m.screen = m.opLogReturnScreen
				}
				return m, nil
			}
			return m, nil
		case screenAbout:
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "q", "esc", "?":
				m.screen = screenTable
				return m, nil
			}
			return m, nil
		case screenWelcome:
			return m.updateDownload(msg)
		case screenDownloading:
			switch msg.String() {
			case "esc", "ctrl+c":
				// Only stop the operation, not the whole process - the
				// running command still finishes on its own (see
				// downloadDoneMsg) once WaitDownload notices ctx is
				// done, saving whatever completed first rather than
				// discarding it. m.dlCancel is nil once already called
				// (or if somehow reached with nothing running), so a
				// repeated esc is a harmless no-op instead of a panic.
				if m.dlCancel != nil {
					m.dlCancel()
					m.dlCancelling = true
				}
			}
			return m, nil // otherwise ignore input while the download command is running
		case screenUSBDrive:
			return m.updateUSBDrive(msg)
		case screenUSBDriveConfirm:
			return m.updateUSBDriveConfirm(msg)
		case screenUSBDriveCopying:
			return m, nil // ignore input while the copy command is running
		}
		return m.updateTable(msg)
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// logLines splits a progress buffer into display lines for
// opLogView, shared by every background operation that captures
// output instead of printing straight to the terminal cmd/sdigo owns.
func logLines(buf *bytes.Buffer) []string {
	return strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
}

func (m model) View() string {
	switch m.screen {
	case screenSplash:
		return m.splashView()
	case screenScanning:
		return m.scanningView()
	case screenOptions:
		return m.optionsView()
	case screenFilters:
		return m.filtersView()
	case screenDetail:
		if m.currentDevice() != nil {
			return detailHelpLine + "\n" + m.detailViewport.View()
		}
	case screenConfirmInstall:
		return m.confirmInstallView()
	case screenInstalling:
		return m.downloadStatusView("Installing")
	case screenInstallLog:
		return m.opLogView()
	case screenAbout:
		return aboutView
	case screenWelcome:
		return m.downloadView()
	case screenDownloading:
		return m.downloadStatusView("Downloading")
	case screenUSBDrive:
		return m.usbDriveView()
	case screenUSBDriveConfirm:
		return m.usbDriveConfirmView()
	case screenUSBDriveCopying:
		return "Copying... please wait.\n"
	}
	return m.tableView()
}

// aboutView credits the original project this is a Go reimplementation
// of. Constant, not a method: it has nothing to do with the current
// scan/device state.
const aboutView = `Snappy Driver Installer: Go Forth
A Go reimplementation of Snappy Driver Installer Origin

Source: github.com/IDisposable/snappy-driver-installer-origin

Based on Snappy Driver Installer Origin
  Home page: www.snappy-driver-installer.org

Snappy Driver Installer Origin is free software: you can redistribute
it and/or modify it under the terms of the GNU General Public License
as published by the Free Software Foundation, either version 3 of the
License or (at your option) any later version. See
https://www.gnu.org/licenses/ for the full text.

This reimplementation carries the same license, being a derivative
work of the original source.

Built with:
  github.com/anacrolix/torrent
  github.com/bodgit/sevenzip
  github.com/charmbracelet/bubbles
  github.com/charmbracelet/bubbletea
  github.com/charmbracelet/lipgloss
  github.com/rs/zerolog
  github.com/ulikunitz/xz
  github.com/yusufpapurcu/wmi
  golang.org/x/sys
  golang.org/x/time

esc/q/?: back
`

func (m model) opLogView() string {
	dest := "device list"
	if m.opLogReturnScreen == screenWelcome {
		dest = "download menu"
	}
	var b strings.Builder
	if m.opLogIsError {
		b.WriteString(cautionStyle.Render("FAILED") + " - esc/q: back to " + dest + "\n\n")
	} else {
		b.WriteString("Log - enter/esc/q: back to " + dest + "\n\n")
	}
	for _, line := range m.opLog {
		fmt.Fprintf(&b, "%s\n", line)
	}
	return b.String()
}
