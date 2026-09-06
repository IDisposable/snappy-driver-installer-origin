package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sdio/internal/settings"
)

// optionItem is one toggleable entry on either the options
// (screenOptions, engine flags) or filters (screenFilters, display
// filters) screen - the two dimensions of "config options" the
// original exposes as GUI checkboxes/menu items, split into separate
// screens (see buildFlagItems/buildFilterItems) since a real flag list
// is long enough that filters used to require scrolling well past it
// to reach.
type optionItem struct {
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

// buildFlagItems lists every engine flag (screenOptions), in the same
// order as their declaration in internal/settings/flags.go.
func buildFlagItems() []optionItem {
	var items []optionItem
	for _, f := range settings.FlagOptions() {
		items = append(items, optionItem{
			name: f.Name, help: f.Help, isFlag: true, flagBit: f.Bit, persist: f.Persist,
		})
	}
	return items
}

// buildFilterItems lists every display filter (screenFilters), in the
// same order as the original's "Show" menu, so anyone already
// familiar with SDIO finds each filter where they expect it.
func buildFilterItems() []optionItem {
	var items []optionItem
	for _, f := range settings.FilterOptions() {
		items = append(items, optionItem{
			name: f.Name, help: f.Help, isFlag: false, filterBit: f.Bit,
		})
	}
	return items
}

// updateOptions handles key input while the (engine flags) options
// screen is active. Flags take effect on the next scan, not live (most
// feed into MatchContext/CalcAltSectScore at scan time, not display
// time), so unlike updateFilters this never needs to refresh the table.
func (m model) updateOptions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "o", "esc":
		m.screen = screenTable
		return m, nil
	case "f":
		m.screen = screenFilters
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
		m.options[m.optionIndex].toggle(m.s)
		m.logger.Info().Str("option", m.options[m.optionIndex].name).Bool("enabled", m.options[m.optionIndex].checked(m.s)).Msg("engine option toggled")
	}
	return m, nil
}

// updateFilters handles key input while the display-filters screen is
// active - split out from updateOptions/screenOptions since a real
// flag list is long enough that filters used to require scrolling
// well past it to reach. Unlike a flag, a filter applies immediately,
// so every toggle refreshes the table right away.
func (m model) updateFilters(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "f", "esc":
		m.screen = screenTable
		return m, nil
	case "o":
		m.screen = screenOptions
		return m, nil
	case "up", "k":
		if m.filterIndex > 0 {
			m.filterIndex--
		}
	case "down", "j":
		if m.filterIndex < len(m.filterOptions)-1 {
			m.filterIndex++
		}
	case " ", "enter":
		m.filterOptions[m.filterIndex].toggle(m.s)
		m.logger.Info().Str("filter", m.filterOptions[m.filterIndex].name).Bool("enabled", m.filterOptions[m.filterIndex].checked(m.s)).Msg("display filter toggled")
		m.refreshTable()
	}
	return m, nil
}

// optionItemList renders items as a checkbox list with the cursor row
// (at index selected) bolded - the shared body optionsView/filtersView
// each put their own header above.
func optionItemList(s *settings.Settings, items []optionItem, selected int) string {
	var b strings.Builder
	for i, item := range items {
		box := "[ ]"
		if item.checked(s) {
			box = "[x]"
		}
		line := fmt.Sprintf("  %s %-20s %s\n", box, item.name, item.help)
		if i == selected {
			line = lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("> %s %-20s %s", box, item.name, item.help)) + "\n"
		}
		b.WriteString(line)
	}
	return b.String()
}

// optionsView renders the engine-flags screen (screenOptions) - split
// from filtersView/screenFilters since a real flag list is long enough
// that filters used to require scrolling well past it to reach.
// Flags persist to sdio.cfg; most apply on the next scan, not live.
func (m model) optionsView() string {
	header := "Options (engine flags, persisted to sdio.cfg) - space/enter: toggle, up/down: move, o/esc: back, f: filters, q: quit\n\n"
	return header + optionItemList(m.s, m.options, m.optionIndex)
}

// filtersView renders the display-filters screen (screenFilters) -
// see optionsView's doc comment for why this is a separate screen.
// Unlike a flag, a filter applies to the table immediately.
func (m model) filtersView() string {
	header := "Display Filters (apply immediately) - space/enter: toggle, up/down: move, f/esc: back, o: options, q: quit\n\n"
	return header + optionItemList(m.s, m.filterOptions, m.filterIndex)
}
