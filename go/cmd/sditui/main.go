// Command sditui is the TUI front end for the Go rewrite, replacing
// gui.cpp/draw.cpp/theme*.cpp's device-list screen (see
// go/README.md). It runs the same scan (internal/scan) as cmd/sdi,
// then shows the results in a scrollable table with an options screen
// (all engine flags and display filters), a per-device detail screen,
// and per-row selection wired to the real install path
// (internal/installflow) - the same one cmd/sdi's -install uses.
package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sdio/internal/installflow"
	"sdio/internal/scan"
	"sdio/internal/settings"
)

// screen selects which of the TUI's views is active.
type screen int

const (
	screenTable screen = iota
	screenOptions
	screenDetail
	screenConfirmInstall
	screenInstalling
	screenInstallLog
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
// internal/settings/flags.go), then filters (matching the original's
// "Show" menu order).
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

// minTableWidth is the narrowest terminal width this layout tries to
// support before columns start clipping content rather than shrinking
// gracefully.
const minTableWidth = 70

// wideInstalledColumnWidth is the terminal width at and above which
// there's room to add a fifth "Installed" column without squeezing
// Device/Best match down to unreadable widths.
const wideInstalledColumnWidth = 130

// layoutColumns sizes the table's columns for the given terminal
// width, ported from no original equivalent - the original GUI used a
// fixed-layout window with its own resize handling in draw.cpp; this
// is this rewrite's own design for a terminal that can be any width.
// Status, Sel, and Version get fixed widths since their content is
// bounded; Device and Best match share whatever's left at a roughly
// 3:2 ratio (matching the original 45:30 MVP column proportions), so
// growing the terminal mostly benefits the two free-text columns
// rather than making numbers/labels needlessly wide. Returns whether
// there was enough room to add the "Installed" (currently-installed
// driver version) column.
func layoutColumns(width int) ([]table.Column, bool) {
	if width < minTableWidth {
		width = minTableWidth
	}
	showInstalled := width >= wideInstalledColumnWidth

	const selWidth, statusWidth, versionWidth = 4, 8, 14
	fixed := selWidth + statusWidth + versionWidth
	if showInstalled {
		fixed += versionWidth
	}
	flex := width - fixed - 4 // 4: rough per-row border/padding budget
	if flex < 30 {
		flex = 30
	}
	deviceWidth := flex * 3 / 5
	bestWidth := flex - deviceWidth

	cols := []table.Column{
		{Title: "Sel", Width: selWidth},
		{Title: "Status", Width: statusWidth},
		{Title: "Device", Width: deviceWidth},
		{Title: "Best match", Width: bestWidth},
		{Title: "Version", Width: versionWidth},
	}
	if showInstalled {
		cols = append(cols, table.Column{Title: "Installed", Width: versionWidth})
	}
	return cols, showInstalled
}

// deviceRow renders one device as a table row. selected marks it
// ticked for install (meaningful only when it has a Best candidate -
// there's nothing installable about a MISSING row).
func deviceRow(dr scan.DeviceResult, selected, showInstalled bool) table.Row {
	sel := "   "
	best := dr.Best()
	if best != nil {
		sel = "[ ]"
		if selected {
			sel = "[x]"
		}
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
		row = table.Row{sel, "MISSING", dr.Device.Description, reason, ""}
	} else {
		row = table.Row{sel, "FOUND", dr.Device.Description, best.Driverpack.Filename, best.Result.DriverVersion.String()}
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
// preserving scan order - the slice cmd/sditui's table is built from,
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

func tableRows(devices []scan.DeviceResult, selected map[string]bool, showInstalled bool) []table.Row {
	rows := make([]table.Row, len(devices))
	for i, dr := range devices {
		rows[i] = deviceRow(dr, selected[dr.Device.InstanceID], showInstalled)
	}
	return rows
}

type model struct {
	s      *settings.Settings
	result scan.Result

	table            table.Model
	rows             []scan.DeviceResult // parallel to table.Rows(), for cursor -> device lookup
	width, height    int
	showInstalledCol bool

	matched, missing int
	selected         map[string]bool // keyed by Device.InstanceID

	screen      screen
	options     []optionItem
	optionIndex int

	installLog []string
}

// refreshTable recomputes the visible device list and the table's
// columns/rows together, since which columns exist (Installed) and
// which rows are shown (filters) can each change independently but
// both require rebuilding rows in lockstep to stay aligned.
func (m *model) refreshTable() {
	cols, showInstalled := layoutColumns(m.width)
	m.showInstalledCol = showInstalled
	m.table.SetColumns(cols)
	m.rows = visibleDevices(m.result.Devices, m.s.Filters)
	m.table.SetRows(tableRows(m.rows, m.selected, showInstalled))
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

func newModel(result scan.Result, s *settings.Settings) model {
	matched, missing := 0, 0
	for _, dr := range result.Devices {
		if dr.Best() != nil {
			matched++
		} else {
			missing++
		}
	}

	t := table.New(table.WithFocused(true), table.WithHeight(20))
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true).BorderStyle(lipgloss.NormalBorder()).BorderBottom(true)
	styles.Selected = styles.Selected.Bold(true)
	t.SetStyles(styles)

	m := model{
		table: t, result: result, s: s, matched: matched, missing: missing,
		options: buildOptionItems(), selected: map[string]bool{},
		width: 100, height: 30,
	}
	m.refreshTable()
	return m
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.table.SetWidth(m.width)
		if h := m.height - 6; h > 0 {
			m.table.SetHeight(h)
		}
		m.refreshTable()
		return m, nil

	case installDoneMsg:
		m.installLog = msg.log
		m.screen = screenInstallLog
		// A completed install invalidates the ticked devices' old
		// candidate state (they may now already have that driver) -
		// clearing avoids re-offering the same install as still
		// pending. This rewrite doesn't auto-rescan afterward; the log
		// screen tells the user what happened.
		m.selected = map[string]bool{}
		m.refreshTable()
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
			case "q", "ctrl+c":
				return m, tea.Quit
			default:
				m.screen = screenTable
				return m, nil
			}
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
	case "enter":
		if m.currentDevice() != nil {
			m.screen = screenDetail
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
			m.table.SetRows(tableRows(m.rows, m.selected, m.showInstalledCol))
		}
		return m, nil
	case "a":
		for _, dr := range m.rows {
			if dr.Best() != nil {
				m.selected[dr.Device.InstanceID] = true
			}
		}
		m.table.SetRows(tableRows(m.rows, m.selected, m.showInstalledCol))
		return m, nil
	case "n":
		m.selected = map[string]bool{}
		m.table.SetRows(tableRows(m.rows, m.selected, m.showInstalledCol))
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
func (m model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case " ":
		if dr := m.currentDevice(); dr != nil && dr.Best() != nil {
			id := dr.Device.InstanceID
			if m.selected[id] {
				delete(m.selected, id)
			} else {
				m.selected[id] = true
			}
			m.table.SetRows(tableRows(m.rows, m.selected, m.showInstalledCol))
		}
		return m, nil
	default:
		m.screen = screenTable
		return m, nil
	}
}

// updateConfirmInstall handles the yes/no prompt shown before
// InstallOne is ever called - a real system-modifying action
// (extracts and calls UpdateDriverForPlugAndPlayDevicesW), so it gets
// one more explicit confirmation beyond the space-bar tick, even
// though the original GUI's single "Install (N)" button click is
// arguably the same amount of intent.
func (m model) updateConfirmInstall(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		pending := m.pendingSelected()
		m.screen = screenInstalling
		return m, runInstallCmd(m.s, pending)
	case "q", "ctrl+c":
		return m, tea.Quit
	default:
		m.screen = screenTable
		return m, nil
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

// installDoneMsg carries installflow.Run's captured output back to
// Update once the install command finishes.
type installDoneMsg struct{ log []string }

// runInstallCmd runs installflow.Run in the background (bubbletea
// convention: a tea.Cmd is called on its own goroutine and its return
// value delivered back to Update as a message) so the UI stays
// responsive instead of blocking the whole program for however long
// downloads/extraction/install take. Output that installflow.Run
// would otherwise print straight to a terminal is captured into a
// buffer instead, since cmd/sditui owns the whole screen via bubbletea
// alternate-screen mode - writing to os.Stdout underneath that would
// corrupt the display.
func runInstallCmd(s *settings.Settings, pending []installflow.Pending) tea.Cmd {
	return func() tea.Msg {
		var buf bytes.Buffer
		installflow.Run(s, pending, &buf)
		log := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		return installDoneMsg{log: log}
	}
}

func (m model) View() string {
	switch m.screen {
	case screenOptions:
		return m.optionsView()
	case screenDetail:
		if dr := m.currentDevice(); dr != nil {
			return m.detailView(*dr)
		}
	case screenConfirmInstall:
		return m.confirmInstallView()
	case screenInstalling:
		return "Installing... please wait.\n"
	case screenInstallLog:
		return m.installLogView()
	}
	return m.tableView()
}

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
		"space: tick, a: select all, n: select none, enter: details, i: install, o: options, q: quit\n",
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

// signatureLabel describes a candidate's catalog-validity in the same
// terms Manager::filter's altsectscore-based visibility rule uses:
// 2 is catalog-signed and confirmed valid for the running OS, 1 is
// present but unsigned or unconfirmed, 0 never reaches here (Best()
// requires IsDriverValid, i.e. AltSectScore>0).
func signatureLabel(altSectScore int) string {
	switch altSectScore {
	case 2:
		return "catalog-signed, valid for this OS"
	case 1:
		return "unsigned or unconfirmed"
	default:
		return "invalid"
	}
}

// detailView renders the full comparison the original's hover
// tooltip shows (installed vs. available driver), for the device
// under the table's cursor.
func (m model) detailView(dr scan.DeviceResult) string {
	var b strings.Builder
	b.WriteString("Device detail - space: toggle install, any other key: back, q: quit\n\n")

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

	if len(dr.Device.HardwareIDs) > 0 {
		b.WriteString("Installed hardware ID\n")
		for _, id := range dr.Device.HardwareIDs {
			fmt.Fprintf(&b, "  %s\n", id)
		}
		b.WriteString("\n")
	}
	if len(dr.Device.CompatibleIDs) > 0 {
		b.WriteString("Installed compatible ID\n")
		for _, id := range dr.Device.CompatibleIDs {
			fmt.Fprintf(&b, "  %s\n", id)
		}
		b.WriteString("\n")
	}

	b.WriteString("Installed driver\n")
	if dr.Installed == nil {
		b.WriteString("  (none)\n\n")
	} else {
		inst := dr.Installed
		fmt.Fprintf(&b, "  Provider       %s\n", inst.ProviderName)
		fmt.Fprintf(&b, "  Date           %s\n", inst.Version.DateString())
		fmt.Fprintf(&b, "  Version        %s\n", inst.Version.String())
		fmt.Fprintf(&b, "  Matched ID     %s\n", inst.MatchingDeviceID)
		fmt.Fprintf(&b, "  Inf file       %s\n", inst.InfPath)
		fmt.Fprintf(&b, "  Section        %s%s\n\n", inst.InfSection, inst.InfSectionExt)
	}

	b.WriteString("Available driver (best match)\n")
	best := dr.Best()
	if best == nil {
		b.WriteString("  (no actionable candidate)\n")
	} else {
		drp := best.Driverpack
		fmt.Fprintf(&b, "  Driver pack    %s\n", drp.Filename)
		fmt.Fprintf(&b, "  Provider       %s\n", drp.Manufacturer(best.HWIDIndex))
		fmt.Fprintf(&b, "  Date           %s\n", best.Result.DriverVersion.DateString())
		fmt.Fprintf(&b, "  Version        %s\n", best.Result.DriverVersion.String())
		fmt.Fprintf(&b, "  Matched ID     %s\n", best.Result.HWID)
		fmt.Fprintf(&b, "  Inf file       %s\n", drp.InfPath(best.HWIDIndex))
		fmt.Fprintf(&b, "  Section        %s\n", best.Result.Section)
		fmt.Fprintf(&b, "  Signature      %s\n", signatureLabel(best.Result.AltSectScore))
		if drp.Pending {
			b.WriteString("  (driver pack data not yet downloaded - needs the configured torrent)\n")
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
	b.WriteString("\ny/enter: install, any other key: cancel, q: quit\n")
	return b.String()
}

func (m model) installLogView() string {
	var b strings.Builder
	b.WriteString("Install log - any key: back to device list, q: quit\n\n")
	for _, line := range m.installLog {
		fmt.Fprintf(&b, "%s\n", line)
	}
	return b.String()
}

func main() {
	os.Exit(mainErr())
}

func mainErr() int {
	s := settings.New()
	if err := s.LoadDefaultCfg(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: loading sdio.cfg:", err)
	}
	if err := s.Parse(os.Args[1:]); err != nil {
		return 2
	}

	// Ported from main()'s unconditional Settings.save() after a run
	// completes; deferred so it still runs even on an error return. This
	// is also how options-screen toggles persist, since the TUI has no
	// separate "save" action.
	defer func() {
		if err := s.Save(settings.DefaultCfgFilename); err != nil {
			fmt.Fprintln(os.Stderr, "warning: saving sdio.cfg:", err)
		}
	}()

	result, err := scan.Run(s)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	p := tea.NewProgram(newModel(result, s), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}
