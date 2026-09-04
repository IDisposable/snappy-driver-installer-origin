// Command sditui is the TUI front end for the Go rewrite, replacing
// gui.cpp/draw.cpp/theme*.cpp's device-list screen (see
// go/README.md). It runs the same scan (internal/scan) as cmd/sdi,
// then shows the results in a scrollable table instead of a
// plain-text dump. View-only for now: no install wiring (see
// cmd/sdi's -install for that; it will move here once the TUI grows
// an install screen).
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sdio/internal/scan"
	"sdio/internal/settings"
)

// screen selects which of the TUI's two views is active.
type screen int

const (
	screenTable screen = iota
	screenOptions
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

type model struct {
	table   table.Model
	result  scan.Result
	s       *settings.Settings
	matched int
	missing int

	screen      screen
	options     []optionItem
	optionIndex int
}

// deviceRow renders one device as a table row.
func deviceRow(dr scan.DeviceResult) table.Row {
	best := dr.Best()
	if best == nil {
		reason := "no valid candidate found"
		switch {
		case len(dr.Candidates) == 0:
			reason = scan.StatusLabel(dr.Status)
		case dr.Candidates[0].Result.AltSectScore != 0:
			reason = "already has an equal or better driver installed"
		}
		return table.Row{"MISSING", dr.Device.Description, reason, ""}
	}
	return table.Row{"FOUND", dr.Device.Description, best.Driverpack.Filename, best.Result.DriverVersion.String()}
}

// buildRows renders every device visible under filters as a table row,
// ordered best-first within "found" vs. "missing".
func buildRows(devices []scan.DeviceResult, filters settings.FilterShow) []table.Row {
	var rows []table.Row
	for _, dr := range devices {
		if !dr.Visible(filters) {
			continue
		}
		rows = append(rows, deviceRow(dr))
	}
	return rows
}

func newModel(result scan.Result, s *settings.Settings) model {
	columns := []table.Column{
		{Title: "Status", Width: 8},
		{Title: "Device", Width: 45},
		{Title: "Best match", Width: 30},
		{Title: "Version", Width: 14},
	}

	matched, missing := 0, 0
	for _, dr := range result.Devices {
		if dr.Best() != nil {
			matched++
		} else {
			missing++
		}
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(buildRows(result.Devices, s.Filters)),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true).BorderStyle(lipgloss.NormalBorder()).BorderBottom(true)
	styles.Selected = styles.Selected.Bold(true)
	t.SetStyles(styles)

	return model{
		table: t, result: result, s: s, matched: matched, missing: missing,
		options: buildOptionItems(),
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.screen == screenOptions {
			return m.updateOptions(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "o":
			m.screen = screenOptions
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.table.SetWidth(msg.Width)
		if h := msg.Height - 6; h > 0 {
			m.table.SetHeight(h)
		}
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
			m.table.SetRows(buildRows(m.result.Devices, m.s.Filters))
			m.table.GotoTop()
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.screen == screenOptions {
		return m.optionsView()
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
	footer := fmt.Sprintf("\n%d matched, %d missing/no better driver - o: options, q: quit\n", m.matched, m.missing)
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
		cursor := "  "
		line := fmt.Sprintf("%s%s %-20s %s\n", cursor, box, item.name, item.help)
		if i == m.optionIndex {
			line = lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("> %s %-20s %s", box, item.name, item.help)) + "\n"
		}
		b.WriteString(line)
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
