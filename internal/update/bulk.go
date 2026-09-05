package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"sdio/internal/common"
	"sdio/internal/logging"
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
// client's own Warning-or-higher events (see Config.OnAlert). ctx
// cancels the operation early (e.g. a user-requested stop) - already-
// completed files are still saved and their indexes promoted rather
// than discarded, so a cancel behaves like a graceful partial run,
// not a lost one; a torrent client closed via the normal defer c.Close()
// path this way is far less likely to leave its on-disk piece state
// inconsistent than force-killing the whole process ever was. Returns
// how many files were newly downloaded. logger records a structured
// start/complete/failure entry for every individual file (not just an
// overall summary), the specific per-file visibility reported missing
// when diagnosing a stalled/crashed download.
func DownloadDriverPacks(ctx context.Context, s *settings.Settings, onAlert func(level, message string), filter DriverPackFilter, out io.Writer, timeout time.Duration, onProgress ProgressFunc, logger *logging.Logger) (int, error) {
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

	t, err := c.AddFromSpec(s.TorrentFile)
	if err != nil {
		return 0, fmt.Errorf("adding torrent %s: %w", s.TorrentFile, err)
	}

	infoCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := t.WaitInfo(infoCtx); err != nil {
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
	for _, f := range selected {
		logger.Info().Str("file", path.Base(filepath.ToSlash(f.Path))).Int64("bytes", f.Length).Msg("driver pack download starting")
	}

	dlCtx, dlCancel := context.WithTimeout(ctx, timeout)
	defer dlCancel()
	label := fmt.Sprintf("%d driver pack(s)", len(selected))
	if err := t.WaitDownload(dlCtx, selected, timeout, label, onProgress); err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintf(out, "download cancelled\n")
			logger.Info().Msg("driver pack download batch cancelled")
		} else {
			fmt.Fprintf(out, "warning: download did not finish: %v\n", err)
			logger.Warn().Err(err).Msg("driver pack download batch did not finish")
		}
		// Which file(s), not just that something didn't finish -
		// otherwise a webseed outage or a peer-less file (both real,
		// seen against the actual public torrent) look identical to
		// "everything worked" once downloaded silently drops them.
		for _, fp := range t.Progress(selected).Files {
			if fp.Percent() < 100 {
				base := path.Base(filepath.ToSlash(fp.Path))
				fmt.Fprintf(out, "  incomplete: %s (%d%%, %s of %s - no peers/webseed data for the rest)\n",
					base, fp.Percent(), common.BytesToStr(uint64(fp.Completed)), common.BytesToStr(uint64(fp.Total)))
				logger.Warn().Str("file", base).Int("percent", fp.Percent()).
					Int64("completed", fp.Completed).Int64("total", fp.Total).
					Msg("driver pack file incomplete")
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
			logger.Error().Err(err).Str("file", base).Msg("driver pack download failed: saving")
			continue
		}
		if err := PromotePendingIndex(s.IndexDir, base); err != nil {
			fmt.Fprintf(out, "warning: promoting %s's index: %v\n", base, err)
			logger.Error().Err(err).Str("file", base).Msg("driver pack download: promoting pending index failed")
		}
		fmt.Fprintf(out, "downloaded %s\n", base)
		logger.Info().Str("file", base).Msg("driver pack download complete")
		downloaded++
	}
	return downloaded, nil
}
