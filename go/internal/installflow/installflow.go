// Package installflow extracts and installs matched driver packs. It
// is shared by cmd/sdigo's -nogui -install flag and its interactive
// install screen so the one real system-modifying action in this
// rewrite has exactly one implementation, not two that can quietly
// drift apart.
package installflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sdio/internal/archive"
	"sdio/internal/collection"
	"sdio/internal/install"
	"sdio/internal/settings"
	"sdio/internal/update"
)

// Pending is a device matched to a candidate driver, queued for
// installation.
type Pending struct {
	Description string
	Candidate   collection.Candidate
}

// Run extracts and installs every pending candidate: creates one
// restore point up front (skipped if FlagDisableInstall or
// FlagNoRestorePoint is set, matching the original), then calls
// InstallOne for each device. Progress and warnings are written to out
// as human-readable lines (one Fprintf call each) rather than directly
// to os.Stdout/Stderr, so a caller that isn't a plain terminal (e.g. a
// TUI screen that owns the whole terminal via the alternate screen
// buffer) can capture them into a buffer instead of corrupting its own
// display. onProgress, if non-nil, is called with live byte-level
// progress while a pending driver pack downloads. onAlert, if
// non-nil, is called for the torrent client's own Warning-or-higher
// events (see update.Config.OnAlert). This modifies the system - it
// is only reached when a caller has explicitly requested an install.
func Run(s *settings.Settings, pending []Pending, out io.Writer, onAlert func(level, message string), onProgress update.ProgressFunc) {
	if err := DownloadPending(s, pending, out, onAlert, onProgress); err != nil {
		fmt.Fprintf(out, "warning: downloading pending driver packs: %v\n", err)
	}

	if s.Flags&(settings.FlagDisableInstall|settings.FlagNoRestorePoint) == 0 {
		// Windows throttles System Restore to about one automatic
		// checkpoint per day; without bypassing that,
		// install.CreateRestorePoint can silently do nothing if one was
		// already made recently. Ported from Manager::thread_install's
		// GetRestorePointCreationFrequency -> SetRestorePointCreation
		// Frequency(0) -> SRSetRestorePointW -> SetRestorePointCreation
		// Frequency(original) sequence.
		origFreq, freqErr := install.GetRestorePointCreationFrequency()
		if freqErr == nil {
			if err := install.SetRestorePointCreationFrequency(0); err != nil {
				fmt.Fprintf(out, "warning: could not bypass the restore point frequency limit: %v\n", err)
			}
		}
		if err := install.CreateRestorePoint(install.RestorePointDescription); err != nil {
			fmt.Fprintf(out, "warning: could not create a restore point: %v\n", err)
		}
		if freqErr == nil {
			if err := install.SetRestorePointCreationFrequency(origFreq); err != nil {
				fmt.Fprintf(out, "warning: could not restore the original restore point frequency limit: %v\n", err)
			}
		}
	}

	for _, p := range pending {
		if err := InstallOne(s, p, out); err != nil {
			fmt.Fprintf(out, "install %s: %v\n", p.Description, err)
		}
	}
}

// DownloadPending fetches the .7z for every pending (not-yet-
// downloaded) candidate driver pack via BitTorrent, ported from the
// role Collection::loadOnlineIndexes' DRIVERPACK_TYPE_UPDATE entries
// play together with Updater_t::StartInstallDownload's selective
// per-file download: a device can be matched against a pack whose
// index was downloaded ahead of its data, and installing it needs the
// data fetched first. Does nothing if no candidate is pending, so it
// is always safe to call. onProgress, if non-nil, is called with live
// byte-level progress for whichever pack is currently downloading.
// onAlert, if non-nil, is called for the torrent client's own
// Warning-or-higher events (see update.Config.OnAlert).
func DownloadPending(s *settings.Settings, pending []Pending, out io.Writer, onAlert func(level, message string), onProgress update.ProgressFunc) error {
	var need []Pending
	for _, p := range pending {
		if p.Candidate.Driverpack.Pending {
			need = append(need, p)
		}
	}
	if len(need) == 0 {
		return nil
	}
	if s.TorrentFile == "" {
		return fmt.Errorf("%d driver pack(s) need downloading but no torrent source is configured (-torrent-file)", len(need))
	}

	// UpdatesDir persists across runs (unlike a temp directory that
	// would be removed here) so a download interrupted mid-file
	// resumes instead of restarting from zero next time - the torrent
	// client verifies already-written pieces against the torrent's own
	// metainfo, which a fresh directory can never have.
	if err := os.MkdirAll(s.UpdatesDir, 0o755); err != nil {
		return err
	}

	c, err := update.NewClient(update.Config{DataDir: s.UpdatesDir, Seed: s.Flags&settings.FlagKeepSeeding != 0, OnAlert: onAlert})
	if err != nil {
		return fmt.Errorf("starting torrent client: %w", err)
	}
	defer c.Close()

	var t *update.Torrent
	if strings.HasPrefix(s.TorrentFile, "magnet:") {
		t, err = c.AddFromMagnet(s.TorrentFile)
	} else {
		t, err = c.AddFromFile(s.TorrentFile)
	}
	if err != nil {
		return fmt.Errorf("adding torrent %s: %w", s.TorrentFile, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := t.WaitInfo(ctx); err != nil {
		return fmt.Errorf("reading torrent metadata: %w", err)
	}
	files := t.Files()

	for _, p := range need {
		drp := p.Candidate.Driverpack
		tf := findTorrentFile(files, drp.Filename)
		if tf == nil {
			fmt.Fprintf(out, "warning: %s not found in the torrent, skipping\n", drp.Filename)
			continue
		}

		fmt.Fprintf(out, "DOWNLOAD %-50s (%s, %d bytes)\n", p.Description, drp.Filename, tf.Length)
		selected := t.SelectFiles([]string{tf.Path})
		dlCtx, dlCancel := context.WithTimeout(context.Background(), 30*time.Minute)
		err := t.WaitDownload(dlCtx, selected, 30*time.Minute, drp.Filename, onProgress)
		dlCancel()
		if err != nil {
			fmt.Fprintf(out, "warning: downloading %s: %v\n", drp.Filename, err)
			continue
		}

		dest := filepath.Join(drp.Path, drp.Filename)
		if err := update.SaveFile(filepath.Join(s.UpdatesDir, filepath.FromSlash(tf.Path)), dest); err != nil {
			fmt.Fprintf(out, "warning: saving %s: %v\n", drp.Filename, err)
			continue
		}
		drp.Pending = false
		fmt.Fprintf(out, "DOWNLOADED %-50s -> %s\n", p.Description, dest)
	}
	return nil
}

// findTorrentFile finds the entry in files whose path ends with
// packFilename as a path component (the torrent's own root-folder
// name varies and isn't assumed).
func findTorrentFile(files []update.FileInfo, packFilename string) *update.FileInfo {
	lower := strings.ToLower(packFilename)
	for i, f := range files {
		p := strings.ToLower(f.Path)
		if strings.HasSuffix(p, "/"+lower) || strings.HasSuffix(p, `\`+lower) {
			return &files[i]
		}
	}
	return nil
}

// InstallOne extracts one candidate's driver folder from its pack and
// installs it.
func InstallOne(s *settings.Settings, p Pending, out io.Writer) error {
	drp := p.Candidate.Driverpack
	infPath := drp.InfPath(p.Candidate.HWIDIndex)   // e.g. `dt\allx64\DtPort_1.0.0.6\`
	infName := drp.InfName(p.Candidate.HWIDIndex)   // e.g. "dtport.inf"
	prefix := strings.ReplaceAll(infPath, `\`, "/") // archive entries use "/"

	packPath := filepath.Join(drp.Path, drp.Filename)
	r, err := archive.Open(packPath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", packPath, err)
	}
	defer r.Close()

	destDir := filepath.Join(s.ExtractDir, filepath.FromSlash(strings.TrimSuffix(prefix, "/")))
	if _, err := r.ExtractPrefix(prefix, destDir); err != nil {
		return fmt.Errorf("extracting %s: %w", prefix, err)
	}
	extractedInf := filepath.Join(destDir, infName)

	// Ported from the unconditional removeextrainfs(inf) call after
	// driver_install in Manager::thread_install: runs regardless of
	// whether the install below succeeds, fails, or is skipped.
	if s.Flags&settings.FlagDelExtraInfs != 0 {
		defer func() {
			if err := install.RemoveExtraInfs(extractedInf); err != nil {
				fmt.Fprintf(out, "warning: removing extra .inf files for %s: %v\n", p.Description, err)
			}
		}()
	}

	if s.Flags&settings.FlagDisableInstall != 0 {
		fmt.Fprintf(out, "INSTALL  %-50s (-disableinstall set, not actually installing)\n", p.Description)
		return nil
	}
	// -extractdir's own help text documents it as "also switches to
	// extract-only mode (no install)" (see extractDirValue.Set, which
	// sets this flag as a side effect); honor that here.
	if s.Flags&settings.FlagExtractOnly != 0 {
		fmt.Fprintf(out, "EXTRACTED %-50s -> %s (extract-only mode, not installing)\n", p.Description, destDir)
		return nil
	}

	res, err := install.Driver(0, p.Candidate.Result.HWID, extractedInf)
	if err != nil {
		return err
	}
	reboot := ""
	if res.NeedsReboot {
		reboot = " (reboot required)"
	}
	fmt.Fprintf(out, "INSTALLED %-50s%s\n", p.Description, reboot)
	return nil
}
