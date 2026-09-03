package main

import (
	"os"
	"path/filepath"
	"testing"

	"sdio/internal/archive"
	"sdio/internal/collection"
	"sdio/internal/indexing"
	"sdio/internal/settings"
)

// TestMain forces an explicit os.Exit after tests complete. A real
// torrent download (unlike internal/update's offline-only tests)
// leaves the test process hanging well past when tests report PASS -
// same class of shutdown quirk noted in internal/update's test file,
// but this time not resolved by it (this file only ever creates one
// client for one test, so it isn't the "multiple clients" cause
// suspected there). Production cmd/sdi is unaffected either way: its
// real main() already calls os.Exit(mainErr()) unconditionally.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// realTorrentPath is a real cached SDIO update torrent from a
// production installation (see internal/update's tests) - reused here
// to validate the actual download-and-move path downloadPendingPacks
// implements, not just its unit pieces.
const realTorrentPath = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/torrent/SDIO_Update.torrent"

// TestDownloadPendingPacksRealTorrent downloads a real driver pack
// (DP_CardReader_26072.7z, ~60MB - independently confirmed absent
// from the reference installation's drivers folder, i.e. actually
// pending, not already downloaded) from the real cached torrent, and
// confirms the result lands where installOne expects it and is a
// valid, openable .7z archive - not just "some bytes arrived".
//
// Skipped unless SDIO_TEST_REAL_TORRENT=1: real BitTorrent networking
// (unlike internal/update's offline-metadata-only tests) leaves the
// test process hanging 30-90s past when the test itself reports PASS,
// even with an explicit TestMain/os.Exit - suspected to be a
// go-test-harness-specific interaction with anacrolix/torrent's
// shutdown path (real `go run`/binary invocations of cmd/torrenttest
// earlier did not exhibit this), not a production issue, since real
// cmd/sdi's main() calls os.Exit(mainErr()) as its literal last
// statement with no framework code after it to hang in. Not resolved;
// gated instead of fixed so routine `go test ./...` runs don't pay a
// ~60s tax for one real-network test.
func TestDownloadPendingPacksRealTorrent(t *testing.T) {
	if os.Getenv("SDIO_TEST_REAL_TORRENT") != "1" {
		t.Skip("set SDIO_TEST_REAL_TORRENT=1 to run (real network download; see doc comment)")
	}
	if _, err := os.Stat(realTorrentPath); err != nil {
		t.Skipf("real torrent file not available at %s: %v", realTorrentPath, err)
	}

	destDir := t.TempDir()
	const packFilename = "DP_CardReader_26072.7z"

	if _, err := os.Stat(filepath.Join(destDir, packFilename)); err == nil {
		t.Fatal("test setup bug: destination file already exists before download")
	}

	s := settings.New()
	s.TorrentFile = realTorrentPath

	pending := []pendingInstall{{
		description: "test: " + packFilename,
		candidate: collection.Candidate{
			Driverpack: &indexing.Driverpack{
				Path:     destDir,
				Filename: packFilename,
				Pending:  true,
			},
		},
	}}

	if err := downloadPendingPacks(s, pending); err != nil {
		t.Fatalf("downloadPendingPacks() error: %v", err)
	}

	if pending[0].candidate.Driverpack.Pending {
		t.Error("Driverpack.Pending should be cleared after a successful download")
	}

	destPath := filepath.Join(destDir, packFilename)
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("downloaded file not found at %s: %v", destPath, err)
	}
	if info.Size() == 0 {
		t.Fatal("downloaded file is empty")
	}
	t.Logf("downloaded %s: %d bytes", destPath, info.Size())

	r, err := archive.Open(destPath)
	if err != nil {
		t.Fatalf("downloaded file is not a valid .7z archive: %v", err)
	}
	defer r.Close()
	if len(r.Files()) == 0 {
		t.Error("downloaded archive has 0 files")
	}
}
