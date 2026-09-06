package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"sdio/internal/collection"
	"sdio/internal/logging"
	"sdio/internal/settings"
	"sdio/internal/update"
)

// welcomeItems is the download menu's task list - "indexes, network
// drivers, this machine's drivers, all drivers, and quit". Indexes is
// an on-demand refresh rather than a first-run necessity: scan.Run
// already fetches the index catalog automatically the first time it
// finds none locally (see scan.Result.FirstRun/this screen's own
// auto-jump on it).
var welcomeItems = []string{
	"Indexes (refresh the driver-pack catalog)",
	"Network Drivers (Net/LAN/WLAN/WWAN - get this PC online quickly)",
	"This Machine's Drivers (only what the scanned devices need)",
	"All Driver Packs (the entire collection - large, can take hours)",
	"Quit (back to the driver list)",
}

const (
	welcomeItemIndexes = iota
	welcomeItemNetwork
	welcomeItemThisMachine
	welcomeItemAll
	welcomeItemQuit
)

// thisMachineDriverPacksFilter matches only the driver-pack files the
// scanned devices' own best candidates point at, so "This Machine's
// Drivers" fetches a small, targeted set instead of the entire
// collection - built from the same Best() every install/select-all
// path already uses, not a separate notion of "needed".
func (m model) thisMachineDriverPacksFilter() update.DriverPackFilter {
	needed := map[string]bool{}
	for _, dr := range m.result.Devices {
		if best := dr.Best(); best != nil && best.Driverpack != nil {
			needed[strings.ToLower(best.Driverpack.Filename)] = true
		}
	}
	return func(filename string) bool {
		return needed[strings.ToLower(filename)]
	}
}

// updateWelcome handles key input on the download menu.
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
		if m.welcomeIndex == welcomeItemQuit {
			m.screen = screenTable
			return m, nil
		}
		switch m.welcomeIndex {
		case welcomeItemIndexes:
			m.screen = screenWelcomeDownloading
			m.opLogReturnScreen = screenWelcome
			ctx := m.startDownload()
			return m, tea.Batch(runIndexRefreshCmd(ctx, m.s, m.dlProgress, m.alertLogger(), m.logger), tickProgressCmd())
		case welcomeItemNetwork:
			m.screen = screenWelcomeDownloading
			m.opLogReturnScreen = screenWelcome
			ctx := m.startDownload()
			return m, tea.Batch(runWelcomeDownloadCmd(ctx, m.s, m.downloadFilter(update.NetworkDriverPacks), m.dlProgress, m.alertLogger(), m.logger), tickProgressCmd())
		case welcomeItemThisMachine:
			m.screen = screenWelcomeDownloading
			m.opLogReturnScreen = screenWelcome
			ctx := m.startDownload()
			return m, tea.Batch(runWelcomeDownloadCmd(ctx, m.s, m.downloadFilter(m.thisMachineDriverPacksFilter()), m.dlProgress, m.alertLogger(), m.logger), tickProgressCmd())
		case welcomeItemAll:
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
		m.opLogReturnScreen = screenWelcome
		ctx := m.startDownload()
		return m, tea.Batch(runWelcomeDownloadCmd(ctx, m.s, m.downloadFilter(update.AllDriverPacks), m.dlProgress, m.alertLogger(), m.logger), tickProgressCmd())
	case "ctrl+c":
		return m, tea.Quit
	case "n", "q", "esc":
		m.screen = screenWelcome
		return m, nil
	}
	return m, nil
}

func (m model) downloadFilter(category update.DriverPackFilter) update.DriverPackFilter {
	if m.s.Flags&settings.FlagOnlyUpdates == 0 {
		return category
	}
	newer := update.OnlyUpdates(m.s.DrpDir)
	return func(filename string) bool {
		return category(filename) && newer(filename)
	}
}

// welcomeDownloadDoneMsg carries a Welcome-screen download's captured
// output back to Update once it finishes. isErr is set from the real
// error the download command got (not sniffed from the log text), so
// screenInstallLog can refuse to let "enter" dismiss a failure the
// user hasn't had a chance to read yet - see model.opLogIsError.
type welcomeDownloadDoneMsg struct {
	log   []string
	isErr bool
}

// runIndexRefreshCmd re-runs collection.BootstrapIndexes on request
// (the Welcome screen's "Download Indexes" - scan.Run already does
// this automatically for a genuinely empty index directory, so this
// path matters for an on-demand refresh of an existing catalog).
// progress receives live byte-level status for the Downloading screen.
func runIndexRefreshCmd(ctx context.Context, s *settings.Settings, progress *progressTracker, onAlert func(level, message string), logger *logging.Logger) tea.Cmd {
	return func() tea.Msg {
		defer logPanic(logger, "indexrefresh")
		logger.Info().Str("torrent_source", s.TorrentSourceKind()).Str("indexDir", s.IndexDir).Msg("starting index refresh")

		var buf bytes.Buffer
		n, err := collection.BootstrapIndexes(ctx, s.TorrentSource(), s.IndexDir, s.UpdatesDir, s.Flags&settings.FlagKeepSeeding != 0, onAlert, progress.report, logger)
		switch {
		case errors.Is(err, context.Canceled):
			fmt.Fprintf(&buf, "cancelled - %d new/updated index file(s) already saved\n", n)
			logger.Info().Int("downloaded", n).Msg("index refresh cancelled")
			return welcomeDownloadDoneMsg{log: logLines(&buf)}
		case err != nil:
			fmt.Fprintf(&buf, "error refreshing indexes: %v\n", err)
			logger.Error().Err(err).Msg("index refresh failed")
		default:
			fmt.Fprintf(&buf, "downloaded %d new/updated index file(s)\n", n)
			logger.Info().Int("downloaded", n).Msg("index refresh complete")
		}
		return welcomeDownloadDoneMsg{log: logLines(&buf), isErr: err != nil}
	}
}

// runWelcomeDownloadCmd downloads every driver pack filter matches
// and isn't already present, for the Welcome screen's "Download
// Network Drivers"/"Download All Driver Packs" - a real, potentially
// large network operation, run as a background tea.Cmd like install
// so the UI stays responsive. progress receives live byte-level
// status for the Downloading screen.
func runWelcomeDownloadCmd(ctx context.Context, s *settings.Settings, filter update.DriverPackFilter, progress *progressTracker, onAlert func(level, message string), logger *logging.Logger) tea.Cmd {
	return func() tea.Msg {
		defer logPanic(logger, "welcomedownload")
		logger.Info().Str("torrent_source", s.TorrentSourceKind()).Str("drpDir", s.DrpDir).Msg("starting driver-pack download")

		var buf bytes.Buffer
		n, err := update.DownloadDriverPacks(ctx, s, onAlert, filter, &buf, 2*time.Hour, progress.report, logger)
		switch {
		case err == nil && ctx.Err() != nil:
			// DownloadDriverPacks treats a cancel as a graceful partial
			// run (see its own doc comment) and returns a nil error, so
			// the cancellation itself is only visible via ctx.Err() here.
			logger.Info().Int("downloaded", n).Msg("driver-pack download cancelled")
			return welcomeDownloadDoneMsg{log: logLines(&buf)}
		case err != nil:
			fmt.Fprintf(&buf, "error: %v\n", err)
			logger.Error().Err(err).Msg("driver-pack download failed")
		case n == 0:
			fmt.Fprintf(&buf, "nothing new to download - already up to date\n")
			logger.Info().Msg("driver-pack download: nothing new")
		default:
			logger.Info().Int("downloaded", n).Msg("driver-pack download complete")
		}
		return welcomeDownloadDoneMsg{log: logLines(&buf), isErr: err != nil}
	}
}

func (m model) welcomeView() string {
	var b strings.Builder
	b.WriteString("Download Menu - up/down: move, enter/space: select, q/esc/w: back\n\n")
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
