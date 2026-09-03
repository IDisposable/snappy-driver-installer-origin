// Command sdi is the CLI entry point for the Go rewrite: scans the
// current hardware, loads a driver-pack collection, matches every
// device to the best available driver, and prints a plain-text
// report. This replaces gui.cpp's "scan and show results" screen for
// now; a TUI (see go/README.md) isn't built yet.
//
// By default this only scans and reports - nothing on the system is
// changed. Pass -install to actually extract and install matched
// drivers (internal/install), a real system-modifying action; see
// runInstall's doc comment.
package main

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
	"sdio/internal/scan"
	"sdio/internal/settings"
	"sdio/internal/update"
)

func main() {
	os.Exit(mainErr())
}

func mainErr() int {
	s := settings.New()
	if err := s.LoadDefaultCfg(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: loading sdio.cfg:", err)
	}

	fs := s.FlagSet("sdi")
	doInstall := fs.Bool("install", false, "install matched drivers (modifies the system; without this flag, only scan and report)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}
	s.ExpandDirs()

	// Ported from main()'s unconditional Settings.save() after a run
	// completes, so switches given on the command line persist into
	// sdio.cfg for next time (Settings.Save itself honors
	// -preservecfg and only ever writes the persistent subset of
	// fields - never GUI-only state, which this rewrite doesn't have).
	// Deferred so it still runs even if run() below returns an error.
	defer func() {
		if err := s.Save(settings.DefaultCfgFilename); err != nil {
			fmt.Fprintln(os.Stderr, "warning: saving sdio.cfg:", err)
		}
	}()

	if err := run(s, *doInstall); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func run(s *settings.Settings, doInstall bool) error {
	result, err := scan.Run(s)
	if err != nil {
		return err
	}
	bb, si := result.System.BaseBoard, result.System.SysInfo

	fmt.Printf("System: %s %s, Windows %d.%d build %d (%s), %d devices found\n",
		bb.SystemManufacturer, bb.SystemModel, si.Windows.Major, si.Windows.Minor, si.Windows.Build,
		archLabel(si.Is64Bit), len(result.Devices))
	fmt.Printf("Driver packs: %d loaded, %d skipped\n", len(result.Collection.Packs), len(result.Collection.Skipped))
	for _, skipped := range result.Collection.Skipped {
		fmt.Printf("  skipped %s: %v\n", skipped.Filename, skipped.Err)
	}

	var missing, matched int
	var pending []pendingInstall
	for _, dr := range result.Devices {
		best := dr.Best()
		if best == nil {
			missing++
			switch {
			case len(dr.Candidates) == 0:
				fmt.Printf("MISSING  %-50s (%s)\n", dr.Device.Description, scan.StatusLabel(dr.Status))
			case dr.Candidates[0].Result.AltSectScore == 0:
				fmt.Printf("MISSING  %-50s (no valid candidate found)\n", dr.Device.Description)
			default:
				fmt.Printf("MISSING  %-50s (already has an equal or better driver installed)\n", dr.Device.Description)
			}
			continue
		}
		matched++
		onTorrent := ""
		if best.Driverpack.Pending {
			onTorrent = " [needs download]"
		}
		fmt.Printf("FOUND    %-50s -> %s (%s, %s)%s\n",
			dr.Device.Description, best.Driverpack.Filename, best.Result.Section, best.Result.DriverVersion, onTorrent)
		pending = append(pending, pendingInstall{description: dr.Device.Description, candidate: *best})
	}

	fmt.Printf("\n%d devices matched, %d missing/no driver found\n", matched, missing)

	if doInstall && len(pending) > 0 {
		runInstall(s, pending)
	}
	return nil
}

// pendingInstall is a device matched to a candidate driver, queued for
// installation if -install was given.
type pendingInstall struct {
	description string
	candidate   collection.Candidate
}

// runInstall extracts and installs every pending candidate, ported
// from the per-device loop in Manager::thread_install: create one
// restore point up front (skipped if FlagDisableInstall is set,
// matching the original), then call internal/install.Driver for each
// device. This modifies the system - it is only reached when the
// caller passed -install explicitly.
func runInstall(s *settings.Settings, pending []pendingInstall) {
	if err := downloadPendingPacks(s, pending); err != nil {
		fmt.Fprintf(os.Stderr, "warning: downloading pending driver packs: %v\n", err)
	}

	if s.Flags&settings.FlagDisableInstall == 0 {
		// Windows throttles System Restore to about one automatic
		// checkpoint per day; without bypassing that, CreateRestorePoint
		// can silently do nothing if one was already made recently.
		// Ported from Manager::thread_install's
		// GetRestorePointCreationFrequency -> SetRestorePointCreation
		// Frequency(0) -> SRSetRestorePointW -> SetRestorePointCreation
		// Frequency(original) sequence.
		origFreq, freqErr := install.GetRestorePointCreationFrequency()
		if freqErr == nil {
			if err := install.SetRestorePointCreationFrequency(0); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not bypass the restore point frequency limit: %v\n", err)
			}
		}
		if err := install.CreateRestorePoint(install.RestorePointDescription); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not create a restore point: %v\n", err)
		}
		if freqErr == nil {
			if err := install.SetRestorePointCreationFrequency(origFreq); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not restore the original restore point frequency limit: %v\n", err)
			}
		}
	}

	for _, p := range pending {
		if err := installOne(s, p); err != nil {
			fmt.Fprintf(os.Stderr, "install %s: %v\n", p.description, err)
		}
	}
}

// downloadPendingPacks fetches the .7z for every pending (not-yet-
// downloaded) candidate driver pack via BitTorrent, ported from the
// role Collection::loadOnlineIndexes' DRIVERPACK_TYPE_UPDATE entries
// play together with Updater_t::StartInstallDownload's selective
// per-file download: a device can be matched against a pack whose
// index was downloaded ahead of its data, and installing it needs the
// data fetched first. Does nothing if no candidate is pending, so it
// is always safe to call.
func downloadPendingPacks(s *settings.Settings, pending []pendingInstall) error {
	var need []pendingInstall
	for _, p := range pending {
		if p.candidate.Driverpack.Pending {
			need = append(need, p)
		}
	}
	if len(need) == 0 {
		return nil
	}
	if s.TorrentFile == "" {
		return fmt.Errorf("%d driver pack(s) need downloading but no torrent source is configured (-torrent-file)", len(need))
	}

	dataDir, err := os.MkdirTemp("", "sdio-torrent-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dataDir)

	c, err := update.NewClient(update.Config{DataDir: dataDir})
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
		drp := p.candidate.Driverpack
		tf := findTorrentFile(files, drp.Filename)
		if tf == nil {
			fmt.Fprintf(os.Stderr, "warning: %s not found in the torrent, skipping\n", drp.Filename)
			continue
		}

		fmt.Printf("DOWNLOAD %-50s (%s, %d bytes)\n", p.description, drp.Filename, tf.Length)
		selected := t.SelectFiles([]string{tf.Path})
		if err := waitForDownload(t, selected); err != nil {
			fmt.Fprintf(os.Stderr, "warning: downloading %s: %v\n", drp.Filename, err)
			continue
		}

		dest := filepath.Join(drp.Path, drp.Filename)
		if err := moveFile(filepath.Join(dataDir, filepath.FromSlash(tf.Path)), dest); err != nil {
			fmt.Fprintf(os.Stderr, "warning: saving %s: %v\n", drp.Filename, err)
			continue
		}
		drp.Pending = false
		fmt.Printf("DOWNLOADED %-50s -> %s\n", p.description, dest)
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

// waitForDownload polls Progress until every file in files has fully
// downloaded, or the deadline passes. Real driver packs range from a
// few hundred KB to several GB, hence the generous timeout.
func waitForDownload(t *update.Torrent, files []update.FileInfo) error {
	deadline := time.Now().Add(30 * time.Minute)
	for time.Now().Before(deadline) {
		if t.Progress(files).Percent() >= 100 {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for the download to complete")
}

// moveFile relocates src to dest, falling back to copy-then-remove if
// a direct rename fails (e.g. src and dest are on different volumes -
// the torrent client's temporary data directory and the driver-pack
// directory need not be on the same drive).
func moveFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dest); err == nil {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

func installOne(s *settings.Settings, p pendingInstall) error {
	drp := p.candidate.Driverpack
	infPath := drp.InfPath(p.candidate.HWIDIndex)   // e.g. `dt\allx64\DtPort_1.0.0.6\`
	infName := drp.InfName(p.candidate.HWIDIndex)   // e.g. "dtport.inf"
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
				fmt.Fprintf(os.Stderr, "warning: removing extra .inf files for %s: %v\n", p.description, err)
			}
		}()
	}

	if s.Flags&settings.FlagDisableInstall != 0 {
		fmt.Printf("INSTALL  %-50s (-disableinstall set, not actually installing)\n", p.description)
		return nil
	}

	res, err := install.Driver(0, p.candidate.Result.HWID, extractedInf)
	if err != nil {
		return err
	}
	reboot := ""
	if res.NeedsReboot {
		reboot = " (reboot required)"
	}
	fmt.Printf("INSTALLED %-50s%s\n", p.description, reboot)
	return nil
}

func archLabel(is64Bit bool) string {
	if is64Bit {
		return "amd64"
	}
	return "x86"
}
