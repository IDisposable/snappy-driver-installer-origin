package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"sdio/internal/common"
	"sdio/internal/logging"
	"sdio/internal/settings"
	"sdio/internal/usbdrive"
)

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
		m.logger.Info().Str("dest", m.usbDrives[m.usbDriveIndex].Root).Msg("USB copy confirmation accepted")
		m.screen = screenUSBDriveCopying
		m.opLogReturnScreen = screenTable
		return m, runUSBCopyCmd(m.s, m.usbDrives[m.usbDriveIndex].Root, m.logger)
	case "ctrl+c":
		return m, tea.Quit
	case "n", "q", "esc":
		m.logger.Info().Msg("USB copy confirmation cancelled")
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
func runUSBCopyCmd(s *settings.Settings, destRoot string, logger *logging.Logger) tea.Cmd {
	return func() tea.Msg {
		defer logPanic(logger, "usbcopy")
		logger.Info().Str("dest", destRoot).Msg("starting USB drive copy")

		var buf bytes.Buffer
		paths, err := usbPortablePaths(s)
		if err != nil {
			fmt.Fprintf(&buf, "error: %v\n", err)
			logger.Error().Err(err).Msg("resolving paths to copy for USB drive failed")
			return downloadDoneMsg{log: logLines(&buf), isErr: true}
		}
		if err := usbdrive.CopyPortable(destRoot, paths, &buf); err != nil {
			fmt.Fprintf(&buf, "error: %v\n", err)
			logger.Error().Err(err).Str("dest", destRoot).Msg("USB drive copy failed")
			return downloadDoneMsg{log: logLines(&buf), isErr: true}
		}
		fmt.Fprintf(&buf, "done - copied to %s\n", destRoot)
		logger.Info().Str("dest", destRoot).Msg("USB drive copy complete")
		return downloadDoneMsg{log: logLines(&buf)}
	}
}

func copyUSBHeadless(s *settings.Settings, destRoot string, out io.Writer, logger *logging.Logger) error {
	paths, err := usbPortablePaths(s)
	if err != nil {
		return err
	}
	logger.Info().Str("dest", destRoot).Msg("starting headless USB drive copy")
	if err := usbdrive.CopyPortable(destRoot, paths, out); err != nil {
		logger.Error().Err(err).Str("dest", destRoot).Msg("headless USB drive copy failed")
		return err
	}
	logger.Info().Str("dest", destRoot).Msg("headless USB drive copy complete")
	return nil
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
