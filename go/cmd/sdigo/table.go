package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"sdio/internal/hardware"
	"sdio/internal/scan"
	"sdio/internal/settings"
	"sdio/internal/usbdrive"
)

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
	if best != nil && hardware.IsMicrosoftDriver(dr.Installed) {
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

// updateTable handles key input over the main device table.
func (m model) updateTable(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "o":
		m.screen = screenOptions
		return m, nil
	case "f":
		m.screen = screenFilters
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
			m.opLogIsError = err != nil
			m.opLogReturnScreen = screenTable
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
			// A Microsoft-provided driver is excluded from select-all for
			// the same reason it gets the [MS] tag (see deviceRow) -
			// replacing it is often unnecessary and riskier than keeping
			// it, so it shouldn't be swept up by a bulk action; a space
			// bar tick on the row still installs it if that's genuinely
			// wanted.
			if dr.Best() != nil && !hardware.IsMicrosoftDriver(dr.Installed) {
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
		"space: tick, a: select all, n: select none, enter: details, i: install, o: options, f: filters, w: downloads, u: usb drive, ?: about, q: quit\n",
		m.matched, m.missing, len(m.pendingSelected()))
	return header + m.table.View() + footer
}
