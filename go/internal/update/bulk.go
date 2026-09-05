package update

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"sdio/internal/common"
	"sdio/internal/settings"
)

// DriverPackFilter reports whether a driver-pack filename (just the
// base name, e.g. "DP_LAN_Realtek-NT_26081.7z") belongs to a download
// category.
type DriverPackFilter func(filename string) bool

// AllDriverPacks matches every driver-pack file (the index half of a
// full download is collection.BootstrapIndexes).
func AllDriverPacks(string) bool { return true }

// NetworkDriverPacks matches Net/LAN/WLAN/WWAN driver-pack filenames -
// the "get this PC online quickly" category on the Welcome screen.
func NetworkDriverPacks(filename string) bool {
	fn := strings.ToLower(filename)
	for _, sub := range []string{"_net_", "_lan_", "_wlan-wifi_", "_wwan-4g_", "_wwan_"} {
		if strings.Contains(fn, sub) {
			return true
		}
	}
	return false
}

// DownloadDriverPacks downloads every .7z driver-pack file in
// s.TorrentFile matching filter and not already present in s.DrpDir.
// Progress and warnings are written to out. Once a pack's .7z lands
// in s.DrpDir, its pending index in s.IndexDir (if any) is promoted
// to its real DP_*.bin name - see PromotePendingIndex - so the very
// next scan matches against it instead of reporting it missing.
// s.UpdatesDir is a persistent staging directory for in-progress file
// data - not a temp directory, so an interrupted download resumes
// instead of restarting from zero next run. onProgress, if non-nil,
// is called with live byte-level progress across every selected file
// - see ProgressFunc. onAlert, if non-nil, is called for the torrent
// client's own Warning-or-higher events (see Config.OnAlert). Returns
// how many files were newly downloaded.
func DownloadDriverPacks(s *settings.Settings, onAlert func(level, message string), filter DriverPackFilter, out io.Writer, timeout time.Duration, onProgress ProgressFunc) (int, error) {
	if err := os.MkdirAll(s.DrpDir, 0o755); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(s.UpdatesDir, 0o755); err != nil {
		return 0, err
	}

	c, err := NewClient(Config{DataDir: s.UpdatesDir, Seed: s.Flags&settings.FlagKeepSeeding != 0, OnAlert: onAlert})
	if err != nil {
		return 0, fmt.Errorf("starting torrent client: %w", err)
	}
	defer c.Close()

	var t *Torrent
	if strings.HasPrefix(s.TorrentFile, "magnet:") {
		t, err = c.AddFromMagnet(s.TorrentFile)
	} else {
		t, err = c.AddFromFile(s.TorrentFile)
	}
	if err != nil {
		return 0, fmt.Errorf("adding torrent %s: %w", s.TorrentFile, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := t.WaitInfo(ctx); err != nil {
		return 0, fmt.Errorf("reading torrent metadata: %w", err)
	}

	var names []string
	for _, f := range t.Files() {
		base := path.Base(filepath.ToSlash(f.Path))
		if !strings.HasSuffix(strings.ToLower(base), ".7z") || !filter(base) {
			continue
		}
		if _, err := os.Stat(filepath.Join(s.DrpDir, base)); err == nil {
			continue // already have it
		}
		names = append(names, f.Path)
	}
	if len(names) == 0 {
		return 0, nil
	}

	selected := t.SelectFiles(names)
	fmt.Fprintf(out, "downloading %d driver pack(s)...\n", len(selected))

	dlCtx, dlCancel := context.WithTimeout(context.Background(), timeout)
	defer dlCancel()
	label := fmt.Sprintf("%d driver pack(s)", len(selected))
	if err := t.WaitDownload(dlCtx, selected, timeout, label, onProgress); err != nil {
		fmt.Fprintf(out, "warning: download did not finish: %v\n", err)
		// Which file(s), not just that something didn't finish -
		// otherwise a webseed outage or a peer-less file (both real,
		// seen against the actual public torrent) look identical to
		// "everything worked" once downloaded silently drops them.
		for _, fp := range t.Progress(selected).Files {
			if fp.Percent() < 100 {
				fmt.Fprintf(out, "  incomplete: %s (%d%%, %s of %s - no peers/webseed data for the rest)\n",
					path.Base(filepath.ToSlash(fp.Path)), fp.Percent(),
					common.BytesToStr(uint64(fp.Completed)), common.BytesToStr(uint64(fp.Total)))
			}
		}
	}

	downloaded := 0
	for _, f := range selected {
		base := path.Base(filepath.ToSlash(f.Path))
		src := filepath.Join(s.UpdatesDir, filepath.FromSlash(f.Path))
		if _, err := os.Stat(src); err != nil {
			continue // never completed, already reported above
		}
		dest := filepath.Join(s.DrpDir, base)
		if err := SaveFile(src, dest); err != nil {
			fmt.Fprintf(out, "warning: saving %s: %v\n", base, err)
			continue
		}
		if err := PromotePendingIndex(s.IndexDir, base); err != nil {
			fmt.Fprintf(out, "warning: promoting %s's index: %v\n", base, err)
		}
		fmt.Fprintf(out, "downloaded %s\n", base)
		downloaded++
	}
	return downloaded, nil
}
