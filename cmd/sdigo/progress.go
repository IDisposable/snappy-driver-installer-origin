package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"sdio/internal/common"
	"sdio/internal/update"
)

// progressTracker is mutex-guarded download progress published by a
// background download goroutine (runInstallCmd/runDownloadCmd/
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

// scanRecentLines caps how many recently loaded driver-pack filenames
// scanProgressTracker keeps - screenScanning renders these as a
// scrolling list (oldest at the top, newest at the bottom), the same
// "watch the file list go by" feel a classic installer's file-copy
// screen gives, rather than a single line overwriting itself.
const scanRecentLines = 12

// scanProgressTracker is the mutex-guarded counterpart of
// progressTracker for the startup scan - Init's background collection
// load reports through it (see collection.LoadCollection's
// onProgress) and screenScanning's View polls it on the same
// progressTickMsg loop.
type scanProgressTracker struct {
	mu             sync.Mutex
	current, total int
	recent         []string // bounded to scanRecentLines, oldest first
}

// report is passed as collection.LoadCollection's onProgress.
func (p *scanProgressTracker) report(current, total int, filename string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current, p.total = current, total
	p.recent = append(p.recent, filename)
	if len(p.recent) > scanRecentLines {
		p.recent = p.recent[len(p.recent)-scanRecentLines:]
	}
}

// snapshot returns the most recently reported scan progress, and up
// to scanRecentLines of the most recently loaded filenames (oldest
// first) - a copy, safe for the caller to hold onto past the next
// report call.
func (p *scanProgressTracker) snapshot() (current, total int, recent []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current, p.total, append([]string(nil), p.recent...)
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

// maxActiveDownloadLines is the fallback cap on how many in-progress
// files downloadStatusView lists individually, used only before the
// terminal's real size is known (m.height<=0) - see
// activeFileLinesBudget for the normal, height-aware sizing.
// "Download All Driver Packs" selects 100+ files at once, so on a
// reasonably tall terminal most of that fits at once rather than
// always truncating to a small fixed handful.
const maxActiveDownloadLines = 20

// activeFileLinesBudget returns how many in-progress files
// activeFileLines should list individually, sized to the terminal's
// actual height (minus the handful of lines downloadStatusView's own
// header/percent/summary text always takes) instead of a small fixed
// cap - a taller terminal should show more of a large batch, not the
// same truncated handful every time.
func (m model) activeFileLinesBudget() int {
	if m.height <= 0 {
		return maxActiveDownloadLines
	}
	const reservedLines = 8 // header, blank line, percent/label, "N/M complete", "...and N more", margin
	if budget := m.height - reservedLines; budget > 5 {
		return budget
	}
	return 5
}

// scanningView renders screenScanning's "please wait" message, with a
// scrolling list of the most recently loaded driver-pack filenames
// once the background scan reaches collection loading - hardware
// detection alone can be quick, but a full collection is 100+ packs
// and worth showing real progress on rather than sitting on a bare
// static message.
func (m model) scanningView() string {
	header := "Scanning hardware and loading driver packs - please wait...\n"
	if m.scanProgress == nil {
		return header
	}
	current, total, recent := m.scanProgress.snapshot()
	if total == 0 {
		return header
	}

	var b strings.Builder
	b.WriteString(header)
	fmt.Fprintf(&b, "\n%d of %d driver packs loaded\n\n", current, total)
	for _, name := range recent {
		b.WriteString("  ")
		b.WriteString(name)
		b.WriteString("\n")
	}
	return b.String()
}

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
func (m model) downloadStatusView(verb string) string {
	cancelHint := ""
	if m.dlCancel != nil {
		cancelHint = " - esc: stop"
	}
	header := verb + cancelHint + " - please wait, this may take a while.\n\n"
	if m.dlCancelling {
		header = verb + " - stopping, please wait...\n\n"
	}
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
	b.WriteString(activeFileLines(files, m.activeFileLinesBudget()))
	return b.String()
}

// activeFileLines renders one line per file still short of 100%,
// nearest-to-done first, capped at maxLines with the remainder
// summarized as a count - the per-file breakdown downloadStatusView
// shows alongside its own overall percent (see activeFileLinesBudget
// for how maxLines is normally chosen). Returns "" for a single-file
// download (its own line already says the same thing as the overall
// percent).
func activeFileLines(files []update.FileProgress, maxLines int) string {
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
	if len(shown) > maxLines {
		shown = shown[:maxLines]
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
