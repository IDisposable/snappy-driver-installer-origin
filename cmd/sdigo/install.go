package main

import (
	"bytes"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"sdio/internal/install"
	"sdio/internal/installflow"
	"sdio/internal/logging"
	"sdio/internal/settings"
)

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
		m.logger.Info().Int("devices", len(m.pendingSelected())).Msg("install confirmation accepted")
		if !install.IsElevated() {
			m.relaunchInstanceIDs = selectedInstanceIDs(m.selected)
			return m, tea.Quit
		}
		pending := m.pendingSelected()
		m.screen = screenInstalling
		m.dlProgress = &progressTracker{}
		return m, tea.Batch(runInstallCmd(m.s, pending, m.dlProgress, m.alertLogger(), m.logger), tickProgressCmd())
	case "ctrl+c":
		return m, tea.Quit
	case "n", "q", "esc":
		m.logger.Info().Msg("install confirmation cancelled")
		m.screen = screenTable
		return m, nil
	}
	return m, nil
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

// installDoneMsg carries installflow.Run's captured output back to
// Update once the install command finishes. isErr is Run's own return
// value (not sniffed from the log text), so screenInstallLog can
// refuse to let "enter" dismiss a failure the user hasn't had a
// chance to read yet - see model.opLogIsError.
type installDoneMsg struct {
	log   []string
	isErr bool
}

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
func runInstallCmd(s *settings.Settings, pending []installflow.Pending, progress *progressTracker, onAlert func(level, message string), logger *logging.Logger) tea.Cmd {
	return func() tea.Msg {
		defer logPanic(logger, "install")
		logger.Info().Int("devices", len(pending)).Msg("starting install")

		var buf bytes.Buffer
		ok := installflow.Run(s, pending, &buf, onAlert, progress.report, logger)
		logger.Info().Bool("ok", ok).Msg("install finished")
		if !ok {
			logger.Warn().Str("log", buf.String()).Msg("install reported at least one failure")
		}
		return installDoneMsg{log: logLines(&buf), isErr: !ok}
	}
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
