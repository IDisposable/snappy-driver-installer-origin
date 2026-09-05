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
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rs/zerolog"

	"sdio/internal/collection"
	"sdio/internal/common"
	"sdio/internal/hardware"
	"sdio/internal/install"
	"sdio/internal/installflow"
	"sdio/internal/logging"
	"sdio/internal/matcher"
	"sdio/internal/report"
	"sdio/internal/scan"
	"sdio/internal/settings"
	"sdio/internal/update"
	"sdio/internal/usbdrive"
)

// screen selects which of the TUI's views is active.
type screen int

const (
	screenScanning screen = iota
	screenTable
	screenOptions
	screenDetail
	screenConfirmInstall
	screenInstalling
	screenInstallLog
	screenAbout
	screenWelcome
	screenWelcomeConfirmAll
	screenWelcomeDownloading
	screenUSBDrive
	screenUSBDriveConfirm
	screenUSBDriveCopying
)

// optionItem is one toggleable entry in the options screen, wrapping
// either a Settings.Flags bit (settings.FlagOptions) or a
// Settings.Filters bit (settings.FilterOptions) - the two dimensions
// of "config options" the original exposes as GUI checkboxes/menu
// items. section groups entries under a heading in the rendered list.
type optionItem struct {
	section   string
	name      string
	help      string
	isFlag    bool
	flagBit   settings.Flags
	filterBit settings.FilterShow
	persist   bool // only meaningful for flags; filters always persist via -filters=N
}

func (it optionItem) checked(s *settings.Settings) bool {
	if it.isFlag {
		return s.Flags&it.flagBit != 0
	}
	return s.Filters&it.filterBit != 0
}

func (it optionItem) toggle(s *settings.Settings) {
	if it.isFlag {
		s.Flags ^= it.flagBit
		return
	}
	s.Filters ^= it.filterBit
}

// buildOptionItems lists every engine flag and display filter as one
// combined, ordered list - flags first (matching declaration order in
// internal/settings/flags.go), then filters in the same order as the
// original's "Show" menu, so anyone already familiar with SDIO finds
// each filter where they expect it.
func buildOptionItems() []optionItem {
	var items []optionItem
	for _, f := range settings.FlagOptions() {
		items = append(items, optionItem{
			section: "Flags (persisted to sdio.cfg; most apply on the next scan, not live)",
			name:    f.Name, help: f.Help, isFlag: true, flagBit: f.Bit, persist: f.Persist,
		})
	}
	for _, f := range settings.FilterOptions() {
		items = append(items, optionItem{
			section: "Display filters (apply immediately)",
			name:    f.Name, help: f.Help, isFlag: false, filterBit: f.Bit,
		})
	}
	return items
}

// deviceColumnWidth and bestMatchColumnWidth are fixed, not sized off
// the terminal width: device descriptions and driver-pack filenames
// both have a bounded realistic length (the longest real device
// description/filename seen against the reference machine's 371
// devices/124 packs is under 45 characters), so growing these columns
// with the terminal just wastes space on trailing blank padding
// without ever showing more real content. Long outliers still clip
// with an ellipsis rather than distorting the whole layout.
const (
	deviceColumnWidth    = 48
	bestMatchColumnWidth = 38
)

// versionColumnWidth measures the widest cell a version-like column
// actually needs to show, so it's sized to exactly fit real content
// instead of a guessed constant that either clips (too narrow, the
// original complaint) or wastes space (too wide). header is included
// so the column is never narrower than its own title.
func versionColumnWidth(header string, devices []scan.DeviceResult, cell func(scan.DeviceResult) string) int {
	w := len(header)
	for _, dr := range devices {
		if l := len(cell(dr)); l > w {
			w = l
		}
	}
	return w
}

// layoutColumns sizes the table's columns for the given terminal
// width and the devices actually being shown, ported from no original
// equivalent - the original GUI used a fixed-layout window with its
// own resize handling in draw.cpp; this is this rewrite's own design
// for a terminal that can be any width. Only the version-like columns
// grow/shrink with content (see versionColumnWidth); Device and Best
// match stay fixed since a wider terminal doesn't make device names or
// driver-pack filenames any longer. Returns whether there was enough
// room to add the "Installed" (currently-installed driver version)
// column - shown whenever it fits, since unlike Device/Best match it's
// genuinely useful extra information rather than wasted padding.
func layoutColumns(width int, devices []scan.DeviceResult) ([]table.Column, bool) {
	const selWidth, statusWidth = 4, 8
	versionWidth := versionColumnWidth("Version", devices, func(dr scan.DeviceResult) string {
		if best := dr.Best(); best != nil {
			return best.Result.DriverVersion.String()
		}
		return ""
	})
	installedWidth := versionColumnWidth("Installed", devices, func(dr scan.DeviceResult) string {
		if dr.Installed != nil {
			return dr.Installed.Version.String()
		}
		return "not installed"
	})

	base := selWidth + statusWidth + deviceColumnWidth + bestMatchColumnWidth + versionWidth
	showInstalled := width <= 0 || base+installedWidth <= width

	cols := []table.Column{
		{Title: "Sel", Width: selWidth},
		{Title: "Status", Width: statusWidth},
		{Title: "Device", Width: deviceColumnWidth},
		{Title: "Best match", Width: bestMatchColumnWidth},
		{Title: "Version", Width: versionWidth},
	}
	if showInstalled {
		cols = append(cols, table.Column{Title: "Installed", Width: installedWidth})
	}
	return cols, showInstalled
}

// truncateLeading fits s into width columns, replacing however much
// of its start doesn't fit with a leading "…" - bubbles/table's own
// truncation always cuts the end instead, wrong for driver-pack names
// and version numbers, where the end is usually what tells one value
// apart from another sharing the same start.
func truncateLeading(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return "…" + s[len(s)-(width-1):]
}

// displayPackName strips the boilerplate "DP_" prefix and ".7z"
// suffix every driver-pack filename has - the same on every row, so
// dropping them buys back real width - then leading-ellipsis-fits
// whatever's left into width columns.
func displayPackName(filename string, width int) string {
	name := strings.TrimSuffix(filename, ".7z")
	name = strings.TrimPrefix(name, "DP_")
	return truncateLeading(name, width)
}

// deviceRow renders one device as a table row. selected marks it
// ticked for install (meaningful only when it has a Best candidate -
// there's nothing installable about a MISSING row). bestMatchWidth/
// versionWidth are the actual rendered column widths (from
// layoutColumns), needed here to pre-fit cell content ourselves
// rather than let bubbles/table's own trailing-ellipsis truncation
// run - required for the Best match column specifically, since its
// cell is sometimes styled (cautionStyle, see below) and
// bubbles/table's width math isn't ANSI-aware: truncating a styled
// string there miscounts the escape codes as visible characters and
// throws off every column after it.
func deviceRow(dr scan.DeviceResult, selected, showInstalled bool, bestMatchWidth, versionWidth int) table.Row {
	sel := "   "
	best := dr.Best()
	if best != nil {
		sel = "[ ]"
		if selected {
			sel = "[x]"
		}
	}

	description := dr.Device.Description
	if best != nil && isMicrosoftDriver(dr.Installed) {
		// Prepended, not appended, so a long description's ellipsis
		// truncation can't hide this flag - replacing a Microsoft-
		// provided driver is often unnecessary and riskier than
		// keeping it, worth surfacing even under column-width
		// pressure.
		description = "[MS] " + description
	}

	var row table.Row
	if best == nil {
		reason := "no valid candidate found"
		switch {
		case len(dr.Candidates) == 0:
			reason = scan.StatusLabel(dr.Status)
		case dr.Candidates[0].Result.AltSectScore != 0:
			reason = "already has an equal or better driver installed"
		}
		row = table.Row{sel, scan.MatchLabel(nil), description, reason, ""}
	} else {
		fitWidth := bestMatchWidth
		if best.Driverpack.Pending {
			// Reserve room for cautionStyle's escape codes now, so the
			// styled cell's raw length still fits within bestMatchWidth
			// and bubbles/table's own truncation never has to touch it -
			// see this function's doc comment.
			fitWidth -= cautionStyleOverhead
		}
		packName := displayPackName(best.Driverpack.Filename, fitWidth)
		if best.Driverpack.Pending {
			// Its index was fetched ahead of its .7z data (see
			// collection.LoadOnlineIndexes) - installing it means a
			// download first, worth knowing before ticking it. Color
			// rather than a text suffix, so the column doesn't need
			// extra width to say so.
			packName = cautionStyle.Render(packName)
		}
		version := truncateLeading(best.Result.DriverVersion.String(), versionWidth)
		row = table.Row{sel, scan.MatchLabel(best), description, packName, version}
	}
	if showInstalled {
		installedVersion := "not installed"
		if dr.Installed != nil {
			installedVersion = dr.Installed.Version.String()
		}
		row = append(row, installedVersion)
	}
	return row
}

// visibleDevices filters devices to those Visible under filters,
// preserving scan order - the slice cmd/sdigo's table is built from,
// kept alongside the table.Model itself (bubbles/table rows carry no
// back-reference) so a selected table row can be mapped back to its
// scan.DeviceResult via the table's cursor index.
func visibleDevices(devices []scan.DeviceResult, filters settings.FilterShow) []scan.DeviceResult {
	var out []scan.DeviceResult
	for _, dr := range devices {
		if dr.Visible(filters) {
			out = append(out, dr)
		}
	}
	return out
}

func tableRows(devices []scan.DeviceResult, selected map[string]bool, showInstalled bool, bestMatchWidth, versionWidth int) []table.Row {
	rows := make([]table.Row, len(devices))
	for i, dr := range devices {
		rows[i] = deviceRow(dr, selected[dr.Device.InstanceID], showInstalled, bestMatchWidth, versionWidth)
	}
	return rows
}

type model struct {
	s      *settings.Settings
	result scan.Result

	// prepared caches scan.Prepare's hardware-detection output so a
	// rescan after a background download (welcomeDownloadDoneMsg/
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
	options     []optionItem
	optionIndex int

	// opLog holds the captured output of whichever background
	// operation last ran (install or a Welcome-screen download) -
	// screenInstallLog renders it regardless of which one produced it.
	opLog []string

	// dlProgress is set whenever screenInstalling/screenWelcomeDownloading
	// starts a background download, so their View can poll it for a
	// live percent/bytes/speed readout instead of a static message.
	dlProgress *progressTracker

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

	welcomeIndex int

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
		options: buildOptionItems(), selected: map[string]bool{},
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
// picks up m.scanProgress as the collection loads.
func (m model) Init() tea.Cmd {
	scanCmd := func() tea.Msg {
		p, err := scan.Prepare(m.s)
		if err != nil {
			return scanDoneMsg{err: err}
		}
		res, err := scan.MatchWithCollection(m.s, p, m.scanProgress.report)
		return scanDoneMsg{prepared: p, result: res, err: err}
	}
	return tea.Batch(scanCmd, tickProgressCmd())
}

// applyResult populates the model from a fresh scan.Result - the
// initial scan, or a rescan after a background download may have
// changed what's on disk (see welcomeDownloadDoneMsg/installDoneMsg).
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
		case m.s.TorrentFile != "" && m.s.Flags&settings.FlagCheckUpdates != 0:
			// Ported from -checkupdates' documented purpose ("check for
			// driver pack updates") - runs as its own background step
			// after the table's first render instead of blocking it, the
			// way the original's launch-time bootstrap did.
			m.screen = screenWelcomeDownloading
			m.dlProgress = &progressTracker{}
			cmd = tea.Batch(runIndexRefreshCmd(m.s, m.dlProgress, m.alertLogger()), tickProgressCmd())
		case m.s.Flags&settings.FlagAutoUpdate != 0:
			// Ported from the "-autoupdate command line parameter" launch-
			// time auto-trigger (update.cpp) - fires once, right after the
			// first scan.
			m.screen = screenWelcomeDownloading
			m.dlProgress = &progressTracker{}
			cmd = tea.Batch(runWelcomeDownloadCmd(m.s, update.AllDriverPacks, m.dlProgress, m.alertLogger()), tickProgressCmd())
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
		if m.s.Flags&settings.FlagAutoClose != 0 {
			// Ported from Manager::thread_install's PostMessage(WM_CLOSE)
			// once an install finishes - exits straight from here rather
			// than waiting at the log screen, matching an unattended run.
			return m, tea.Quit
		}
		return m, nil

	case welcomeDownloadDoneMsg:
		m.opLog = msg.log
		m.screen = screenInstallLog
		// Whatever just downloaded (indexes, driver packs) may have
		// changed what's on disk - rescan so the table reflects it
		// instead of showing stale "needs download"/missing rows until
		// the next full restart.
		if res, err := scan.MatchWithCollection(m.s, m.prepared, nil); err == nil {
			m.applyResult(res)
		}
		if m.s.Flags&settings.FlagAutoClose != 0 {
			// Ported from Updater_t's "Torrent finished" auto-exit
			// (update.cpp) - the original skips this when -autoinstall is
			// also set, since it closes after the install that follows
			// instead; this rewrite has no -autoinstall to chain into, so
			// there's nothing to wait for.
			return m, tea.Quit
		}
		return m, nil

	case progressTickMsg:
		// Only reschedule while a progress-driven screen is still
		// active - a tick delivered after the operation finished and
		// the screen already moved on would otherwise keep ticking
		// forever.
		if m.screen == screenInstalling || m.screen == screenWelcomeDownloading || m.screen == screenScanning {
			return m, tickProgressCmd()
		}
		return m, nil

	case tea.KeyMsg:
		switch m.screen {
		case screenOptions:
			return m.updateOptions(msg)
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
			case "q", "esc", "enter":
				m.screen = screenTable
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
			return m.updateWelcome(msg)
		case screenWelcomeConfirmAll:
			return m.updateWelcomeConfirmAll(msg)
		case screenWelcomeDownloading:
			return m, nil // ignore input while the download command is running
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

// updateTable handles key input over the main device table.
func (m model) updateTable(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "o":
		m.screen = screenOptions
		return m, nil
	case "?":
		m.screen = screenAbout
		return m, nil
	case "w":
		m.screen = screenWelcome
		m.welcomeIndex = 0
		return m, nil
	case "u":
		drives, err := usbdrive.ListRemovable()
		if err != nil || len(drives) == 0 {
			msg := "No removable drives found."
			if err != nil {
				msg = fmt.Sprintf("Could not list removable drives: %v", err)
			}
			m.opLog = []string{msg}
			m.screen = screenInstallLog
			return m, nil
		}
		m.usbDrives = drives
		m.usbDriveIndex = 0
		m.screen = screenUSBDrive
		return m, nil
	case "enter":
		if dr := m.currentDevice(); dr != nil {
			m.screen = screenDetail
			m.detailViewport.SetContent(m.detailView(*dr))
			m.detailViewport.GotoTop()
		}
		return m, nil
	case " ":
		if dr := m.currentDevice(); dr != nil && dr.Best() != nil {
			id := dr.Device.InstanceID
			if m.selected[id] {
				delete(m.selected, id)
			} else {
				m.selected[id] = true
			}
			m.table.SetRows(tableRows(m.rows, m.selected, m.showInstalledCol, m.bestMatchWidth, m.versionWidth))
		}
		return m, nil
	case "a":
		for _, dr := range m.rows {
			if dr.Best() != nil {
				m.selected[dr.Device.InstanceID] = true
			}
		}
		m.table.SetRows(tableRows(m.rows, m.selected, m.showInstalledCol, m.bestMatchWidth, m.versionWidth))
		return m, nil
	case "n":
		m.selected = map[string]bool{}
		m.table.SetRows(tableRows(m.rows, m.selected, m.showInstalledCol, m.bestMatchWidth, m.versionWidth))
		return m, nil
	case "i":
		if len(m.pendingSelected()) > 0 {
			m.screen = screenConfirmInstall
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// updateOptions handles key input while the options screen is active.
func (m model) updateOptions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "o", "esc":
		m.screen = screenTable
		return m, nil
	case "up", "k":
		if m.optionIndex > 0 {
			m.optionIndex--
		}
	case "down", "j":
		if m.optionIndex < len(m.options)-1 {
			m.optionIndex++
		}
	case " ", "enter":
		item := m.options[m.optionIndex]
		item.toggle(m.s)
		if !item.isFlag {
			// Filters apply immediately; flags take effect on the next
			// scan (most feed into MatchContext/CalcAltSectScore at
			// scan time, not display time) - see buildOptionItems'
			// section heading.
			m.refreshTable()
		}
	}
	return m, nil
}

// updateDetail handles key input on the per-device detail screen.
// Only the keys the footer documents do anything - an unrecognized
// key is ignored rather than closing the screen, so a stray keypress
// can't dismiss it by accident. Scroll keys (arrows/pgup/pgdn/home/
// end) are forwarded to detailViewport, since content routinely runs
// longer than the terminal.
func (m model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	dr := m.currentDevice()
	setSelected := func(v bool) {
		if dr == nil || dr.Best() == nil {
			return
		}
		if v {
			m.selected[dr.Device.InstanceID] = true
		} else {
			delete(m.selected, dr.Device.InstanceID)
		}
		m.table.SetRows(tableRows(m.rows, m.selected, m.showInstalledCol, m.bestMatchWidth, m.versionWidth))
		m.detailViewport.SetContent(m.detailView(*dr))
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case " ":
		if dr != nil && dr.Best() != nil {
			setSelected(!m.selected[dr.Device.InstanceID])
		}
		return m, nil
	case "y":
		setSelected(true)
		m.screen = screenTable
		return m, nil
	case "n":
		setSelected(false)
		m.screen = screenTable
		return m, nil
	case "q", "esc":
		m.screen = screenTable
		return m, nil
	case "up", "down", "k", "j", "pgup", "pgdown", "home", "end", "ctrl+u", "ctrl+d":
		var cmd tea.Cmd
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

// updateConfirmInstall handles the yes/no prompt shown before
// InstallOne is ever called - a real system-modifying action
// (extracts and calls UpdateDriverForPlugAndPlayDevicesW), so it gets
// one more explicit confirmation beyond the space-bar tick, even
// though the original GUI's single "Install (N)" button click is
// arguably the same amount of intent. Only y/enter/n/esc/q do
// anything - an unrecognized key is ignored, not treated as either
// answer.
func (m model) updateConfirmInstall(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		if !install.IsElevated() {
			m.relaunchInstanceIDs = selectedInstanceIDs(m.selected)
			return m, tea.Quit
		}
		pending := m.pendingSelected()
		m.screen = screenInstalling
		m.dlProgress = &progressTracker{}
		return m, tea.Batch(runInstallCmd(m.s, pending, m.dlProgress, m.alertLogger()), tickProgressCmd())
	case "ctrl+c":
		return m, tea.Quit
	case "n", "q", "esc":
		m.screen = screenTable
		return m, nil
	}
	return m, nil
}

// welcomeItems mirrors the original's Welcome dialog buttons, minus
// "Download Indexes" as an all-or-nothing gate: scan.Run already
// fetches the index catalog automatically the first time it finds
// none locally, so this entry is an on-demand refresh instead of a
// first-run necessity.
var welcomeItems = []string{
	"Refresh index catalog now",
	"Download Network Drivers (Net/LAN/WLAN/WWAN - get this PC online quickly)",
	"Download All Driver Packs (the entire collection - large, can take hours)",
}

// updateWelcome handles key input on the Welcome screen.
func (m model) updateWelcome(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q", "esc", "w":
		m.screen = screenTable
		return m, nil
	case "up", "k":
		if m.welcomeIndex > 0 {
			m.welcomeIndex--
		}
	case "down", "j":
		if m.welcomeIndex < len(welcomeItems)-1 {
			m.welcomeIndex++
		}
	case "enter", " ":
		if m.s.TorrentFile == "" {
			m.opLog = []string{"No -torrent-file configured - nothing to download from."}
			m.screen = screenInstallLog
			return m, nil
		}
		switch m.welcomeIndex {
		case 0:
			m.screen = screenWelcomeDownloading
			m.dlProgress = &progressTracker{}
			return m, tea.Batch(runIndexRefreshCmd(m.s, m.dlProgress, m.alertLogger()), tickProgressCmd())
		case 1:
			m.screen = screenWelcomeDownloading
			m.dlProgress = &progressTracker{}
			return m, tea.Batch(runWelcomeDownloadCmd(m.s, update.NetworkDriverPacks, m.dlProgress, m.alertLogger()), tickProgressCmd())
		case 2:
			m.screen = screenWelcomeConfirmAll
			return m, nil
		}
	}
	return m, nil
}

// updateWelcomeConfirmAll confirms before downloading the entire
// driver-pack collection - unlike a single pending candidate's ~tens
// of MB, this is a real, possibly multi-GB, multi-hour operation the
// original's own Welcome dialog warns about too.
func (m model) updateWelcomeConfirmAll(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		m.screen = screenWelcomeDownloading
		m.dlProgress = &progressTracker{}
		return m, tea.Batch(runWelcomeDownloadCmd(m.s, update.AllDriverPacks, m.dlProgress, m.alertLogger()), tickProgressCmd())
	case "ctrl+c":
		return m, tea.Quit
	case "n", "q", "esc":
		m.screen = screenWelcome
		return m, nil
	}
	return m, nil
}

// updateUSBDrive handles key input on the drive-selection screen
// opened by "u".
func (m model) updateUSBDrive(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q", "esc", "u":
		m.screen = screenTable
		return m, nil
	case "up", "k":
		if m.usbDriveIndex > 0 {
			m.usbDriveIndex--
		}
	case "down", "j":
		if m.usbDriveIndex < len(m.usbDrives)-1 {
			m.usbDriveIndex++
		}
	case "enter", " ":
		m.screen = screenUSBDriveConfirm
		return m, nil
	}
	return m, nil
}

// updateUSBDriveConfirm confirms before copying - a real disk write,
// even though (unlike format/delete) it can't destroy anything
// already on the drive.
func (m model) updateUSBDriveConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		m.screen = screenUSBDriveCopying
		return m, runUSBCopyCmd(m.s, m.usbDrives[m.usbDriveIndex].Root)
	case "ctrl+c":
		return m, tea.Quit
	case "n", "q", "esc":
		m.screen = screenUSBDrive
		return m, nil
	}
	return m, nil
}

// usbPortablePaths lists what "Create a USB Drive" copies: the
// running executable plus the configured driver-pack and index
// directories. sdio.cfg itself isn't included - a copy started fresh
// on the destination drive already gets its own portable-layout
// defaults the first time it runs there (see
// Settings.ResolveDataDirs), which is simpler than trying to carry
// the source machine's own paths over to a different drive letter.
func usbPortablePaths(s *settings.Settings) ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("finding the running executable: %w", err)
	}
	return []string{exe, s.DrpDir, s.IndexDir}, nil
}

// runUSBCopyCmd copies usbPortablePaths onto destRoot in the
// background, matching the async pattern every other real
// system-modifying action in this TUI uses (installflow.Run, Welcome
// downloads) so the UI stays responsive.
func runUSBCopyCmd(s *settings.Settings, destRoot string) tea.Cmd {
	return func() tea.Msg {
		var buf bytes.Buffer
		paths, err := usbPortablePaths(s)
		if err != nil {
			fmt.Fprintf(&buf, "error: %v\n", err)
			return welcomeDownloadDoneMsg{log: logLines(&buf)}
		}
		if err := usbdrive.CopyPortable(destRoot, paths, &buf); err != nil {
			fmt.Fprintf(&buf, "error: %v\n", err)
		} else {
			fmt.Fprintf(&buf, "done - copied to %s\n", destRoot)
		}
		return welcomeDownloadDoneMsg{log: logLines(&buf)}
	}
}

// pendingSelected builds the installflow.Pending list for every
// currently-ticked device, looked up from the full (unfiltered)
// device list so a selection made before a filter change is still
// honored.
func (m model) pendingSelected() []installflow.Pending {
	var out []installflow.Pending
	for _, dr := range m.result.Devices {
		if !m.selected[dr.Device.InstanceID] {
			continue
		}
		if best := dr.Best(); best != nil {
			out = append(out, installflow.Pending{Description: dr.Device.Description, Candidate: *best})
		}
	}
	return out
}

// selectedInstanceIDs returns the InstanceIDs currently ticked, for
// carrying a selection across the elevation relaunch (see
// updateConfirmInstall/writeResumeFile) - unlike pendingSelected it
// doesn't need a Best candidate, since sdiGo recomputes that fresh
// after the elevated copy re-scans.
func selectedInstanceIDs(selected map[string]bool) []string {
	var ids []string
	for id, on := range selected {
		if on {
			ids = append(ids, id)
		}
	}
	return ids
}

// progressTracker is mutex-guarded download progress published by a
// background download goroutine (runInstallCmd/runWelcomeDownloadCmd/
// runIndexRefreshCmd, via update.ProgressFunc) and read by the TUI's
// tick loop, so the Installing/Downloading screens can show the same
// kind of live percent/bytes/speed readout update.cpp's ShowProgress
// builds from libtorrent's torrent_status instead of a static
// "please wait".
type progressTracker struct {
	mu        sync.Mutex
	label     string
	completed int64
	total     int64
	rateBps   float64
	sampleAt  time.Time
	sampleBy  int64
	files     []update.FileProgress
}

// report is passed as an update.ProgressFunc to whichever download is
// running.
func (p *progressTracker) report(pr update.Progress) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if p.sampleAt.IsZero() {
		p.sampleAt, p.sampleBy = now, pr.Completed
	} else if dt := now.Sub(p.sampleAt).Seconds(); dt >= 0.2 {
		p.rateBps = float64(pr.Completed-p.sampleBy) / dt
		p.sampleAt, p.sampleBy = now, pr.Completed
	}
	p.label, p.completed, p.total, p.files = pr.Label, pr.Completed, pr.Total, pr.Files
}

// snapshot returns the most recently reported progress.
func (p *progressTracker) snapshot() (label string, completed, total int64, rateBps float64, files []update.FileProgress) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.label, p.completed, p.total, p.rateBps, p.files
}

// scanProgressTracker is the mutex-guarded counterpart of
// progressTracker for the startup scan - Init's background collection
// load reports through it (see collection.LoadCollection's
// onProgress) and screenScanning's View polls it on the same
// progressTickMsg loop.
type scanProgressTracker struct {
	mu             sync.Mutex
	current, total int
	filename       string
}

// report is passed as collection.LoadCollection's onProgress.
func (p *scanProgressTracker) report(current, total int, filename string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current, p.total, p.filename = current, total, filename
}

// snapshot returns the most recently reported scan progress.
func (p *scanProgressTracker) snapshot() (current, total int, filename string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current, p.total, p.filename
}

// progressTickMsg drives periodic re-renders of the Installing/
// Downloading screens while a background download command is
// running - the download itself doesn't send messages as it
// progresses, so View has to instead poll a progressTracker on a
// timer.
type progressTickMsg time.Time

func tickProgressCmd() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(t time.Time) tea.Msg {
		return progressTickMsg(t)
	})
}

// installDoneMsg carries installflow.Run's captured output back to
// Update once the install command finishes.
type installDoneMsg struct{ log []string }

// runInstallCmd runs installflow.Run in the background (bubbletea
// convention: a tea.Cmd is called on its own goroutine and its return
// value delivered back to Update as a message) so the UI stays
// responsive instead of blocking the whole program for however long
// downloads/extraction/install take. Output that installflow.Run
// would otherwise print straight to a terminal is captured into a
// buffer instead, since cmd/sdigo owns the whole screen via bubbletea
// alternate-screen mode - writing to os.Stdout underneath that would
// corrupt the display. progress receives live byte-level status for
// the Installing screen.
func runInstallCmd(s *settings.Settings, pending []installflow.Pending, progress *progressTracker, onAlert func(level, message string)) tea.Cmd {
	return func() tea.Msg {
		var buf bytes.Buffer
		installflow.Run(s, pending, &buf, onAlert, progress.report)
		return installDoneMsg{log: logLines(&buf)}
	}
}

// logLines splits a progress buffer into display lines for
// opLogView, shared by every background operation that captures
// output instead of printing straight to the terminal cmd/sdigo owns.
func logLines(buf *bytes.Buffer) []string {
	return strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
}

// welcomeDownloadDoneMsg carries a Welcome-screen download's captured
// output back to Update once it finishes.
type welcomeDownloadDoneMsg struct{ log []string }

// runIndexRefreshCmd re-runs collection.BootstrapIndexes on request
// (the Welcome screen's "Download Indexes" - scan.Run already does
// this automatically for a genuinely empty index directory, so this
// path matters for an on-demand refresh of an existing catalog).
// progress receives live byte-level status for the Downloading screen.
func runIndexRefreshCmd(s *settings.Settings, progress *progressTracker, onAlert func(level, message string)) tea.Cmd {
	return func() tea.Msg {
		var buf bytes.Buffer
		n, err := collection.BootstrapIndexes(s.TorrentFile, s.IndexDir, s.UpdatesDir, s.Flags&settings.FlagKeepSeeding != 0, onAlert, progress.report)
		if err != nil {
			fmt.Fprintf(&buf, "error refreshing indexes: %v\n", err)
		} else {
			fmt.Fprintf(&buf, "downloaded %d new/updated index file(s)\n", n)
		}
		return welcomeDownloadDoneMsg{log: logLines(&buf)}
	}
}

// runWelcomeDownloadCmd downloads every driver pack filter matches
// and isn't already present, for the Welcome screen's "Download
// Network Drivers"/"Download All Driver Packs" - a real, potentially
// large network operation, run as a background tea.Cmd like install
// so the UI stays responsive. progress receives live byte-level
// status for the Downloading screen.
func runWelcomeDownloadCmd(s *settings.Settings, filter update.DriverPackFilter, progress *progressTracker, onAlert func(level, message string)) tea.Cmd {
	return func() tea.Msg {
		var buf bytes.Buffer
		n, err := update.DownloadDriverPacks(s, onAlert, filter, &buf, 2*time.Hour, progress.report)
		if err != nil {
			fmt.Fprintf(&buf, "error: %v\n", err)
		} else if n == 0 {
			fmt.Fprintf(&buf, "nothing new to download - already up to date\n")
		}
		return welcomeDownloadDoneMsg{log: logLines(&buf)}
	}
}

func (m model) View() string {
	switch m.screen {
	case screenScanning:
		return m.scanningView()
	case screenOptions:
		return m.optionsView()
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
		return m.welcomeView()
	case screenWelcomeConfirmAll:
		return m.welcomeConfirmAllView()
	case screenWelcomeDownloading:
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

// maxActiveDownloadLines caps how many in-progress files
// downloadStatusView lists individually - "Download All Driver Packs"
// selects 100+ files at once, more than a terminal screen can show one
// line each, so only the closest-to-done ones are shown and the rest
// are summarized as a count.
const maxActiveDownloadLines = 8

// downloadStatusView renders live torrent download progress for the
// Installing/Downloading screens - the same percent/bytes/speed
// status update.cpp's ShowProgress (STR_UPD_PROGRES) builds from
// libtorrent's torrent_status, instead of a static "please wait".
// Falls back to that static message until the first progress report
// arrives (metadata/peer discovery can take a few seconds). When more
// than one file is downloading together, the overall percent alone
// can sit still for a long time while individual files actually
// finish and start, so each file still in progress gets its own line
// too (nearest-to-done first).
// scanningView renders screenScanning's "please wait" message, with a
// live "loading pack N of M" line once the background scan reaches
// collection loading - hardware detection alone can be quick, but a
// full collection is 100+ packs and worth showing progress on rather
// than sitting on a bare static message.
func (m model) scanningView() string {
	header := "Scanning hardware and loading driver packs - please wait...\n"
	if m.scanProgress == nil {
		return header
	}
	current, total, filename := m.scanProgress.snapshot()
	if total == 0 {
		return header
	}
	return fmt.Sprintf("%s\nLoading %s (%d of %d)\n", header, filename, current, total)
}

func (m model) downloadStatusView(verb string) string {
	header := verb + " - please wait, this may take a while.\n\n"
	if m.dlProgress == nil {
		return header
	}
	label, completed, total, rateBps, files := m.dlProgress.snapshot()
	if total == 0 {
		return header + "Connecting to the torrent swarm...\n"
	}
	percent := int(completed * 100 / total)
	line := fmt.Sprintf("Downloaded %s out of %s (%d%%)",
		common.BytesToStr(uint64(completed)), common.BytesToStr(uint64(total)), percent)
	if rateBps > 0 {
		line += fmt.Sprintf(" at %s/s", common.BytesToStr(uint64(rateBps)))
	}
	if label != "" {
		line = label + "\n" + line
	}

	var b strings.Builder
	b.WriteString(header)
	b.WriteString(line)
	b.WriteString("\n")
	b.WriteString(activeFileLines(files))
	return b.String()
}

// activeFileLines renders one line per file still short of 100%,
// nearest-to-done first, capped at maxActiveDownloadLines with the
// remainder summarized as a count - the per-file breakdown
// downloadStatusView shows alongside its own overall percent. Returns
// "" for a single-file download (its own line already says the same
// thing as the overall percent).
func activeFileLines(files []update.FileProgress) string {
	if len(files) < 2 {
		return ""
	}
	active := make([]update.FileProgress, 0, len(files))
	done := 0
	for _, f := range files {
		if f.Percent() >= 100 {
			done++
		} else {
			active = append(active, f)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].Percent() > active[j].Percent() })

	var b strings.Builder
	fmt.Fprintf(&b, "\n%d/%d files complete\n", done, len(files))
	shown := active
	if len(shown) > maxActiveDownloadLines {
		shown = shown[:maxActiveDownloadLines]
	}
	for _, f := range shown {
		fmt.Fprintf(&b, "  %-3d%% %s (%s/%s)\n", f.Percent(), filepath.Base(f.Path),
			common.BytesToStr(uint64(f.Completed)), common.BytesToStr(uint64(f.Total)))
	}
	if extra := len(active) - len(shown); extra > 0 {
		fmt.Fprintf(&b, "  ... and %d more in progress\n", extra)
	}
	return b.String()
}

func (m model) usbDriveView() string {
	var b strings.Builder
	b.WriteString("Create a USB Drive - up/down: move, enter/space: select, q/esc/u: back\n\n")
	b.WriteString("Select a removable drive:\n\n")
	for i, d := range m.usbDrives {
		cursor := "  "
		if i == m.usbDriveIndex {
			cursor = "> "
		}
		fmt.Fprintf(&b, "%s%s  (%s free of %s)\n", cursor, d.Root, common.BytesToStr(d.FreeBytes), common.BytesToStr(d.TotalBytes))
	}
	return b.String()
}

func (m model) usbDriveConfirmView() string {
	d := m.usbDrives[m.usbDriveIndex]
	required, err := usbPortablePaths(m.s)
	var requiredBytes uint64
	if err == nil {
		requiredBytes, _ = usbdrive.RequiredBytes(required)
	}
	return fmt.Sprintf("Copy the app and driver-pack collection to %s?\n\n"+
		"Space required:  %s\n"+
		"Space available: %s\n\n"+
		"This only adds/overwrites files - it never deletes anything\n"+
		"already on the drive or formats it.\n\n"+
		"y/enter: copy, n/esc/q: cancel\n",
		d.Root, common.BytesToStr(requiredBytes), common.BytesToStr(d.FreeBytes))
}

func (m model) welcomeView() string {
	var b strings.Builder
	b.WriteString("Welcome - up/down: move, enter/space: select, q/esc/w: back\n\n")
	if m.s.TorrentFile == "" {
		b.WriteString("No -torrent-file is configured, so none of these can fetch anything yet.\n\n")
	}
	for i, item := range welcomeItems {
		cursor := "  "
		if i == m.welcomeIndex {
			cursor = "> "
		}
		fmt.Fprintf(&b, "%s%s\n", cursor, item)
	}
	return b.String()
}

func (m model) welcomeConfirmAllView() string {
	return fmt.Sprintf("Download the entire driver-pack collection? This can be several\n"+
		"gigabytes and take anywhere from an hour to a day depending on\n"+
		"availability and connection speed.\n\n"+
		"Destination: %s\n\n"+
		"y/enter: download, n/esc/q: cancel\n", m.s.DrpDir)
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

func (m model) tableView() string {
	bb, si := m.result.System.BaseBoard, m.result.System.SysInfo
	var bootstrap string
	switch {
	case m.result.BootstrapError != nil:
		bootstrap = fmt.Sprintf("(index update check failed: %v)\n", m.result.BootstrapError)
	case m.result.IndexesDownloaded > 0:
		bootstrap = fmt.Sprintf("(downloaded %d new/updated index file(s))\n", m.result.IndexesDownloaded)
	}
	header := fmt.Sprintf("%s%s %s - Windows %d.%d build %d - %d devices, %d driver packs loaded\n",
		bootstrap, bb.SystemManufacturer, bb.SystemModel, si.Windows.Major, si.Windows.Minor, si.Windows.Build,
		len(m.result.Devices), len(m.result.Collection.Packs))
	footer := fmt.Sprintf("\n%d matched, %d missing/no better driver, %d selected for install\n"+
		"Newer/Older/Better all outrank the installed driver - Newer/Older also means\n"+
		"its own release date is newer/older (enter for the full comparison)\n"+
		"space: tick, a: select all, n: select none, enter: details, i: install, o: options, w: welcome, u: usb drive, ?: about, q: quit\n",
		m.matched, m.missing, len(m.pendingSelected()))
	return header + m.table.View() + footer
}

func (m model) optionsView() string {
	var b strings.Builder
	b.WriteString("Options - space/enter: toggle, up/down: move, o/esc: back, q: quit\n\n")

	lastSection := ""
	for i, item := range m.options {
		if item.section != lastSection {
			if lastSection != "" {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "%s\n", item.section)
			lastSection = item.section
		}

		box := "[ ]"
		if item.checked(m.s) {
			box = "[x]"
		}
		line := fmt.Sprintf("  %s %-20s %s\n", box, item.name, item.help)
		if i == m.optionIndex {
			line = lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("> %s %-20s %s", box, item.name, item.help)) + "\n"
		}
		b.WriteString(line)
	}
	return b.String()
}

// betterStyle/invalidStyle render the detail screen's "greenlight"
// comparison, ported from Manager::draw_hint's cb/POPUP_CMP_INVALID_
// COLOR text colors - the original highlights whichever side of a
// per-field comparison wins in green, and flags a bad signature or
// OS/arch mismatch in red. cautionStyle has no original equivalent -
// it flags the Microsoft-inbox-driver note below.
var (
	betterStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
	invalidStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
	cautionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))

	// cautionStyleOverhead is how many extra bytes cautionStyle.Render
	// adds around its content (the ANSI escape codes) - 0 outside a
	// real terminal, where lipgloss renders plain text unstyled.
	// deviceRow needs this to keep a styled cell's raw length within
	// its column's width - see that function's doc comment.
	cautionStyleOverhead = len(cautionStyle.Render(""))
)

// comparison holds the per-field installed-vs-candidate outcome,
// ported from Manager::draw_hint's cm_date/cm_ver/cm_hwid/cm_score
// locals - answering "whose value for this ONE field is better",
// which is a different, finer-grained question than the overall
// BETTER/WORSE/SAME verdict DeviceResult.Best already resolves: an
// installed inbox driver can carry a numerically higher four-part
// version number (e.g. 10.0.26100.9223 vs. a candidate's
// 10.0.22000.10003) while the candidate still wins overall on date
// and score. 0 means tie or "not comparable" (e.g. no installed
// driver at all); 1 means installed wins that field; 2 means the
// candidate does.
type comparison struct {
	date, version, hwid, score int
}

func compareInstalledVsCandidate(dr scan.DeviceResult, best *collection.Candidate) comparison {
	var c comparison
	if dr.Installed == nil || best == nil {
		return c
	}
	switch r := common.CompareDate(dr.Installed.Version, best.Result.DriverVersion); {
	case r > 0:
		c.date = 1
	case r < 0:
		c.date = 2
	}
	switch r := common.CompareVersion(dr.Installed.Version, best.Result.DriverVersion); {
	case r > 0:
		c.version = 1
	case r < 0:
		c.version = 2
	}
	if dr.InstalledScore != nil {
		// A lower raw score ranks better - see matcher.Result.Cmp's
		// negated CmpUnsigned comparison.
		switch r := matcher.CmpUnsigned(dr.InstalledScore.Score, best.Result.Score); {
		case r < 0:
			c.score = 1
		case r > 0:
			c.score = 2
		}
	}
	c.hwid = hwidComparison(dr.Device.HardwareIDs, dr.Device.CompatibleIDs, dr.Installed.MatchingDeviceID, best.Result.HWID)
	return c
}

// hwidComparison finds whichever of installedID/candidateID matches
// an earlier (more specific) entry in the device's own combined
// hardware-then-compatible ID list, ported from the pp/cm_hwid
// bookkeeping in Manager::draw_hint. Returns 0 if neither matches, or
// both match the very same entry (an exact tie, not "id" - genuinely
// no fresh update: the driver pack targets the identical ID the
// installed driver already used).
func hwidComparison(hardwareIDs, compatibleIDs []string, installedID, candidateID string) int {
	for _, p := range hardwareIDs {
		if pp := hwidMatchBits(p, installedID, candidateID); pp == 1 || pp == 2 {
			return pp
		}
	}
	for _, p := range compatibleIDs {
		if pp := hwidMatchBits(p, installedID, candidateID); pp == 1 || pp == 2 {
			return pp
		}
	}
	return 0
}

func hwidMatchBits(entry, installedID, candidateID string) int {
	pp := 0
	if installedID != "" && strings.EqualFold(installedID, entry) {
		pp |= 1
	}
	if candidateID != "" && strings.EqualFold(candidateID, entry) {
		pp |= 2
	}
	return pp
}

// isMicrosoftDriver reports whether the installed driver's provider
// is Microsoft - almost always an inbox/generic Windows driver rather
// than a vendor one, which replacing is often unnecessary and can be
// riskier than leaving alone (Windows itself keeps it updated via
// Windows Update, and a vendor "upgrade" can be a worse fit than the
// inbox driver Microsoft ships for exactly this hardware class).
func isMicrosoftDriver(inst *hardware.InstalledDriver) bool {
	return inst != nil && strings.EqualFold(strings.TrimSpace(inst.ProviderName), "Microsoft")
}

// styleIf applies betterStyle when winner matches side (1=installed,
// 2=candidate), otherwise renders value unstyled.
func styleIf(value string, winner, side int) string {
	if winner == side {
		return betterStyle.Render(value)
	}
	return value
}

// styleIfMatched highlights an ID-list entry that matched either
// side's matching ID (pp!=0, from hwidMatchBits) - ported from
// Manager::draw_hint's per-entry `pp?POPUP_HWID_COLOR:c0`, which
// marks "this is one of the relevant IDs" rather than comparing which
// side wins (that comparative judgment is the "Matched ID" summary
// line below, via styleIf/cmp.hwid).
func styleIfMatched(value string, pp int) string {
	if pp != 0 {
		return betterStyle.Render(value)
	}
	return value
}

// signatureLabel describes a candidate's catalog-validity in the same
// terms Manager::filter's altsectscore-based visibility rule uses:
// 2 is catalog-signed and confirmed valid for the running OS, 1 is
// present but unsigned or unconfirmed, 0 never reaches here (Best()
// requires IsDriverValid, i.e. AltSectScore>0). Styled red when not
// fully valid, mirroring Manager::draw_hint's isvalidcat check -
// there's no equivalent styling for the installed side below since
// this rewrite never ported Driver::isvalidcat's own catalog lookup.
func signatureLabel(altSectScore int) string {
	switch altSectScore {
	case 2:
		return "catalog-signed, valid for this OS"
	case 1:
		return invalidStyle.Render("unsigned or unconfirmed")
	default:
		return invalidStyle.Render("invalid")
	}
}

// verdictSummary states in one sentence why a candidate is (or isn't)
// recommended, ported from the STR_STATUS_BETTER_NEW/_CUR/_OLD
// sentences itembar_t::str_status builds - the Status column only has
// room for a short word (scan.MatchLabel), which can otherwise read
// as a plain negative ("Older") for a driver that's still the
// recommended pick overall.
func verdictSummary(best *collection.Candidate) string {
	if best == nil {
		return "Not recommended: no candidate outranks the installed driver."
	}
	switch {
	case best.Result.Status&matcher.StatusNew != 0:
		return "Recommended: outranks the installed driver, and is dated more recently."
	case best.Result.Status&matcher.StatusOld != 0:
		return "Recommended: outranks the installed driver overall, even though it's dated older - see Score below for why."
	case best.Result.Status&matcher.StatusCurrent != 0:
		return "Recommended: outranks the installed driver (same release date)."
	default:
		return "Recommended: no driver is currently installed for this device."
	}
}

// scoreDifferences enumerates, in the same priority order
// matcher.Score itself combines them (catalog signature highest,
// then feature number, then hardware-ID match precision), which
// specific factors differ between the installed driver and this
// candidate and which side wins each one - the concrete "why" behind
// the two opaque hex Score values, instead of a description of what
// Score is in the abstract. A tied factor is omitted. Returns nil if
// there's nothing to compare (no installed driver, or its own score
// couldn't be computed).
func scoreDifferences(dr scan.DeviceResult, best *collection.Candidate, is64Bit bool) []string {
	if dr.InstalledScore == nil || best == nil {
		return nil
	}
	inst := dr.InstalledScore
	drp := best.Driverpack

	var lines []string
	side := func(installedWins bool) string {
		if installedWins {
			return "installed"
		}
		return "candidate"
	}

	instSig := matcher.SignatureScore(inst.CatalogFileBits, is64Bit, inst.IsNTSection)
	candIsNTSection := strings.Contains(strings.ToLower(drp.InstallPicked(best.HWIDIndex)), ".nt")
	candSig := matcher.SignatureScore(drp.CatalogFileBits(best.HWIDIndex), is64Bit, candIsNTSection)
	if instSig != candSig {
		lines = append(lines, fmt.Sprintf("Catalog signature: %s is properly signed for this system, the other isn't",
			side(instSig < candSig)))
	}

	candFeature := drp.Feature(best.HWIDIndex)
	if inst.Feature != candFeature {
		lines = append(lines, fmt.Sprintf("Driver pack's priority hint: installed=%d, candidate=%d, 255=default (%s wins - lower is preferred)",
			inst.Feature, candFeature, side(inst.Feature < candFeature)))
	}

	cmp := compareInstalledVsCandidate(dr, best)
	if cmp.hwid != 0 {
		lines = append(lines, fmt.Sprintf("Hardware ID match: %s matched a more specific ID", side(cmp.hwid == 1)))
	}
	if cmp.date != 0 {
		lines = append(lines, fmt.Sprintf("Release date: %s is dated more recently (a separate factor from overall rank)", side(cmp.date == 1)))
	}
	if cmp.score != 0 {
		lines = append(lines, fmt.Sprintf("Overall rank: %s wins (%08X vs %08X, lower wins)", side(cmp.score == 1), inst.Score, best.Result.Score))
	}

	return lines
}

// detailHelpLine is rendered as a fixed header above the scrollable
// detail viewport, rather than as part of its content, so it stays
// visible regardless of scroll position.
const detailHelpLine = "Device detail - space: toggle install, y: mark and back, n: unmark and back, q/esc: back, ↑↓: scroll\n"

// detailView renders the full comparison the original's hover
// tooltip shows (installed vs. available driver), for the device
// under the table's cursor.
func (m model) detailView(dr scan.DeviceResult) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Device\n")
	fmt.Fprintf(&b, "  Description    %s\n", dr.Device.Description)
	fmt.Fprintf(&b, "  Manufacturer   %s\n", dr.Device.Manufacturer)
	fmt.Fprintf(&b, "  Instance ID    %s\n", dr.Device.InstanceID)
	fmt.Fprintf(&b, "  Status         %s\n", dr.Device.Status())
	tick := "not selected for install"
	if m.selected[dr.Device.InstanceID] {
		tick = "SELECTED for install"
	}
	fmt.Fprintf(&b, "  Install        %s\n\n", tick)

	best := dr.Best()
	fmt.Fprintf(&b, "%s\n\n", verdictSummary(best))

	cmp := compareInstalledVsCandidate(dr, best)
	installedID, candidateID := "", ""
	if dr.Installed != nil {
		installedID = dr.Installed.MatchingDeviceID
	}
	if best != nil {
		candidateID = best.Result.HWID
	}

	if len(dr.Device.HardwareIDs) > 0 {
		b.WriteString("Installed hardware ID\n")
		for _, id := range dr.Device.HardwareIDs {
			fmt.Fprintf(&b, "  %s\n", styleIfMatched(id, hwidMatchBits(id, installedID, candidateID)))
		}
		b.WriteString("\n")
	}
	if len(dr.Device.CompatibleIDs) > 0 {
		b.WriteString("Installed compatible ID\n")
		for _, id := range dr.Device.CompatibleIDs {
			fmt.Fprintf(&b, "  %s\n", styleIfMatched(id, hwidMatchBits(id, installedID, candidateID)))
		}
		b.WriteString("\n")
	}

	b.WriteString("Installed driver\n")
	if dr.Installed == nil {
		b.WriteString("  (none)\n\n")
	} else {
		inst := dr.Installed
		fmt.Fprintf(&b, "  Provider       %s\n", inst.ProviderName)
		fmt.Fprintf(&b, "  Date           %s\n", styleIf(inst.Version.DateString(), cmp.date, 1))
		fmt.Fprintf(&b, "  Version        %s\n", styleIf(inst.Version.String(), cmp.version, 1))
		fmt.Fprintf(&b, "  Matched ID     %s\n", styleIf(inst.MatchingDeviceID, cmp.hwid, 1))
		fmt.Fprintf(&b, "  Inf file       %s\n", inst.InfPath)
		fmt.Fprintf(&b, "  Section        %s%s\n", inst.InfSection, inst.InfSectionExt)
		if dr.InstalledScore != nil {
			fmt.Fprintf(&b, "  Score          %s\n", styleIf(fmt.Sprintf("%08X", dr.InstalledScore.Score), cmp.score, 1))
		}
		if isMicrosoftDriver(inst) {
			fmt.Fprintf(&b, "  %s\n", cautionStyle.Render("Microsoft-provided driver - replacing it is often unnecessary and can be riskier than keeping it"))
		}
		b.WriteString("\n")
	}

	b.WriteString("Available driver (best match)\n")
	if best == nil {
		b.WriteString("  (no actionable candidate)\n")
	} else {
		drp := best.Driverpack
		fmt.Fprintf(&b, "  Driver pack    %s\n", drp.Filename)
		fmt.Fprintf(&b, "  Provider       %s\n", drp.Manufacturer(best.HWIDIndex))
		fmt.Fprintf(&b, "  Date           %s\n", styleIf(best.Result.DriverVersion.DateString(), cmp.date, 2))
		fmt.Fprintf(&b, "  Version        %s\n", styleIf(best.Result.DriverVersion.String(), cmp.version, 2))
		fmt.Fprintf(&b, "  Matched ID     %s\n", styleIf(best.Result.HWID, cmp.hwid, 2))
		fmt.Fprintf(&b, "  Inf file       %s\n", drp.InfPath(best.HWIDIndex))
		section := best.Result.Section
		if best.Result.DecorScore == 0 {
			section = invalidStyle.Render(section)
		}
		fmt.Fprintf(&b, "  Section        %s\n", section)
		fmt.Fprintf(&b, "  Score          %s\n", styleIf(fmt.Sprintf("%08X", best.Result.Score), cmp.score, 2))
		fmt.Fprintf(&b, "  Signature      %s\n", signatureLabel(best.Result.AltSectScore))
		if drp.Pending {
			b.WriteString("  (driver pack data not yet downloaded - needs the configured torrent)\n")
		}

		if diffs := scoreDifferences(dr, best, m.result.System.SysInfo.Is64Bit); len(diffs) > 0 {
			b.WriteString("\nWhy the candidate outranks (or doesn't) the installed driver,\nin order of how much each factor counts toward Score:\n")
			for i, line := range diffs {
				fmt.Fprintf(&b, "  %d. %s\n", i+1, line)
			}
		}
	}
	return b.String()
}

func (m model) confirmInstallView() string {
	pending := m.pendingSelected()
	var b strings.Builder
	fmt.Fprintf(&b, "Install %d driver(s)? This modifies the system.\n\n", len(pending))
	for _, p := range pending {
		fmt.Fprintf(&b, "  %s -> %s\n", p.Description, p.Candidate.Driverpack.Filename)
	}
	b.WriteString("\ny/enter: install, n/esc/q: cancel\n")
	return b.String()
}

func (m model) opLogView() string {
	var b strings.Builder
	b.WriteString("Log - enter/esc/q: back to device list\n\n")
	for _, line := range m.opLog {
		fmt.Fprintf(&b, "%s\n", line)
	}
	return b.String()
}

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

	// This logger exists only for -torrentalerts (see alertLogger) -
	// console is always nil since the TUI owns the whole terminal via
	// bubbletea's alternate screen, and -nogui's own output already
	// goes through fmt.Fprint*/report.Print directly, not this logger.
	// -nologfile skips Start entirely rather than opening a file and
	// discarding writes some other way, so "don't write a log file" is
	// literally true, not just unused.
	logger := logging.New(zerolog.WarnLevel, nil)
	if s.Flags&settings.FlagNoLogFile == 0 {
		if err := logger.Start(s.LogDir, logging.Timestamp()); err != nil {
			fmt.Fprintln(os.Stderr, "warning: starting log file:", err)
		}
	}
	defer logger.Stop()
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
		result, err := scan.Run(s)
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
			installflow.Run(s, pending, os.Stdout, alertLogger, nil)
		}
		return 0
	}

	p := tea.NewProgram(newModel(s, resumeSelected, logger), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
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
