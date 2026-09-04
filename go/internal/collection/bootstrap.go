package collection

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sdio/internal/update"
)

// placeholderIndexFilename computes the underscore-prefixed pending
// placeholder name for a real index filename found in the torrent
// (e.g. "DP_APO_SDIO01_26083.bin" -> "_P_APO_SDIO01_26083.bin") -
// the inverse of expectedPackFilename, ported from the exact
// transformation in Updater_t::WelcomeDownloadIndexes
// ("DestFile=L"_"+DestFile.substr(1);").
func placeholderIndexFilename(properBinName string) string {
	return "_" + properBinName[1:]
}

// BootstrapIndexes downloads every driver-pack index (.bin) file the
// configured torrent has that isn't already present locally under its
// underscore-prefixed pending-placeholder name, saving each one with
// that name - ported from Updater_t::WelcomeDownloadIndexes (the
// Welcome dialog's first-run "get the index catalog" step). This is
// how a machine with no local index catalog at all gets one: a real
// SDIO installer ships the index catalog directly; this rewrite
// instead fetches it live from the torrent, since there's no
// installer step to bundle it in. Also usable to refresh an existing
// catalog (picking up newly-added driver-pack revisions, which get
// their own distinct filename and so are never mistaken for an
// already-known one) - see Settings.FlagCheckUpdates.
//
// Returns the number of index files downloaded. torrentFile is a
// local .torrent path or magnet URI (Settings.TorrentFile); an empty
// value is an error, matching the original's "Updates not
// initialised" guard on the equivalent scripted commands.
func BootstrapIndexes(torrentFile, indexDir string) (int, error) {
	if torrentFile == "" {
		return 0, fmt.Errorf("no torrent source configured")
	}
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return 0, fmt.Errorf("creating %s: %w", indexDir, err)
	}

	dataDir, err := os.MkdirTemp("", "sdio-bootstrap-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dataDir)

	c, err := update.NewClient(update.Config{DataDir: dataDir})
	if err != nil {
		return 0, fmt.Errorf("starting torrent client: %w", err)
	}
	defer c.Close()

	var t *update.Torrent
	if strings.HasPrefix(torrentFile, "magnet:") {
		t, err = c.AddFromMagnet(torrentFile)
	} else {
		t, err = c.AddFromFile(torrentFile)
	}
	if err != nil {
		return 0, fmt.Errorf("adding torrent %s: %w", torrentFile, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := t.WaitInfo(ctx); err != nil {
		return 0, fmt.Errorf("reading torrent metadata: %w", err)
	}

	var need []string
	nameByPath := map[string]string{}
	for _, f := range t.Files() {
		lower := strings.ToLower(f.Path)
		if !strings.Contains(lower, "/indexes/") && !strings.Contains(lower, `\indexes\`) {
			continue
		}
		if !strings.HasSuffix(lower, ".bin") {
			continue
		}
		base := filepath.Base(f.Path)
		if base == "" || base[0] == '_' {
			continue // already placeholder-shaped; shouldn't occur in a real torrent listing
		}
		placeholder := placeholderIndexFilename(base)
		if _, err := os.Stat(filepath.Join(indexDir, placeholder)); err == nil {
			continue // already downloaded under its placeholder name
		}
		need = append(need, f.Path)
		nameByPath[f.Path] = placeholder
	}
	if len(need) == 0 {
		return 0, nil
	}

	selected := t.SelectFiles(need)
	dlCtx, dlCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer dlCancel()
	if err := t.WaitDownload(dlCtx, selected, 10*time.Minute); err != nil {
		return 0, fmt.Errorf("downloading indexes: %w", err)
	}

	count := 0
	for _, fi := range selected {
		placeholder, ok := nameByPath[fi.Path]
		if !ok {
			continue
		}
		src := filepath.Join(dataDir, filepath.FromSlash(fi.Path))
		dest := filepath.Join(indexDir, placeholder)
		if err := update.SaveFile(src, dest); err != nil {
			continue // best-effort: one bad file shouldn't fail the whole bootstrap
		}
		count++
	}
	return count, nil
}
