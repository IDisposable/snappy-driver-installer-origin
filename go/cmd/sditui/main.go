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

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sdio/internal/scan"
	"sdio/internal/settings"
)

type model struct {
	table   table.Model
	result  scan.Result
	matched int
	missing int
}

func newModel(result scan.Result) model {
	columns := []table.Column{
		{Title: "Status", Width: 8},
		{Title: "Device", Width: 45},
		{Title: "Best match", Width: 30},
		{Title: "Version", Width: 14},
	}

	var rows []table.Row
	matched, missing := 0, 0
	for _, dr := range result.Devices {
		best := dr.Best()
		if best == nil {
			missing++
			reason := "no valid candidate found"
			switch {
			case len(dr.Candidates) == 0:
				reason = scan.StatusLabel(dr.Status)
			case dr.Candidates[0].Result.AltSectScore != 0:
				reason = "already has an equal or better driver installed"
			}
			rows = append(rows, table.Row{"MISSING", dr.Device.Description, reason, ""})
			continue
		}
		matched++
		rows = append(rows, table.Row{
			"FOUND", dr.Device.Description, best.Driverpack.Filename, best.Result.DriverVersion.String(),
		})
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true).BorderStyle(lipgloss.NormalBorder()).BorderBottom(true)
	styles.Selected = styles.Selected.Bold(true)
	t.SetStyles(styles)

	return model{table: t, result: result, matched: matched, missing: missing}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
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

func (m model) View() string {
	bb, si := m.result.System.BaseBoard, m.result.System.SysInfo
	header := fmt.Sprintf("%s %s - Windows %d.%d build %d - %d devices, %d driver packs loaded\n",
		bb.SystemManufacturer, bb.SystemModel, si.Windows.Major, si.Windows.Minor, si.Windows.Build,
		len(m.result.Devices), len(m.result.Collection.Packs))
	footer := fmt.Sprintf("\n%d matched, %d missing/no better driver - q: quit\n", m.matched, m.missing)
	return header + m.table.View() + footer
}

func main() {
	s := settings.New()
	if err := s.LoadDefaultCfg(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: loading sdio.cfg:", err)
	}
	if err := s.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	result, err := scan.Run(s)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	p := tea.NewProgram(newModel(result), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
