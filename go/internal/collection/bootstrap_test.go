package collection

import (
	"os"
	"path/filepath"
	"testing"

	"sdio/internal/indexing"
	"sdio/internal/sdwfile"
)

// TestMain forces an explicit os.Exit after tests complete, matching
// the pattern in cmd/sdigo's and internal/update's test files - a real
// torrent download otherwise leaves the test process hanging well
// past when the test itself reports PASS (see
// TestBootstrapIndexesRealTorrent's doc comment).
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestPlaceholderIndexFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"DP_Ports_SDIO01_26083.bin", "_P_Ports_SDIO01_26083.bin"},
		{"DP_CardReader_26072.bin", "_P_CardReader_26072.bin"},
	}
	for _, c := range cases {
		if got := placeholderIndexFilename(c.in); got != c.want {
			t.Errorf("placeholderIndexFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBootstrapIndexesNoTorrentConfigured(t *testing.T) {
	if _, err := BootstrapIndexes("", t.TempDir(), t.TempDir(), nil); err == nil {
		t.Fatal("expected an error with no torrent source configured")
	}
}

// TestBootstrapIndexesRealTorrent downloads the ENTIRE index catalog
// from the real cached torrent into a fresh, empty directory (so every
// single index the torrent has counts as "missing"), confirming a
// substantial number download successfully and that a known real
// index (dtport's) is a valid, fully-decodable index afterward - not
// just "some bytes arrived". Index files are individually tiny (a few
// KB to ~1MB), so downloading the whole catalog (~200 files) is fast
// despite the file count.
//
// Skipped unless SDIO_TEST_REAL_TORRENT=1: see cmd/sdigo's
// TestDownloadPendingPacksRealTorrent for why real torrent tests are
// gated (a go-test-harness-specific shutdown hang, not a production
// issue).
func TestBootstrapIndexesRealTorrent(t *testing.T) {
	if os.Getenv("SDIO_TEST_REAL_TORRENT") != "1" {
		t.Skip("set SDIO_TEST_REAL_TORRENT=1 to run (real network download; see doc comment)")
	}
	const torrentFile = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/torrent/SDIO_Update.torrent"
	if _, err := os.Stat(torrentFile); err != nil {
		t.Skipf("real torrent file not available at %s: %v", torrentFile, err)
	}

	indexDir := t.TempDir()
	updatesDir := t.TempDir()
	count, err := BootstrapIndexes(torrentFile, indexDir, updatesDir, nil)
	if err != nil {
		t.Fatalf("BootstrapIndexes() error: %v", err)
	}
	if count < 100 {
		t.Errorf("downloaded %d index files, want at least 100 (the real torrent has ~200)", count)
	}
	t.Logf("downloaded %d index files", count)

	const dtportPlaceholder = "_P_Ports_SDIO01_26083.bin"
	f, err := os.Open(filepath.Join(indexDir, dtportPlaceholder))
	if err != nil {
		t.Fatalf("expected %s to have been downloaded: %v", dtportPlaceholder, err)
	}
	defer f.Close()

	_, payload, err := sdwfile.Decode(f, true)
	if err != nil {
		t.Fatalf("Decode(%s) error: %v", dtportPlaceholder, err)
	}
	idx, err := indexing.DecodeIndex(payload)
	if err != nil {
		t.Fatalf("DecodeIndex(%s) error: %v", dtportPlaceholder, err)
	}
	if len(idx.HWIDs) == 0 {
		t.Error("decoded index has 0 HWIDs, want a fully usable index")
	}

	// A second run against the same (now populated) directory should
	// download nothing new. Reuses the same updatesDir too, since that's
	// the real scenario this is meant to support (a resumed/rerun
	// download reusing its own staging directory).
	count2, err := BootstrapIndexes(torrentFile, indexDir, updatesDir, nil)
	if err != nil {
		t.Fatalf("second BootstrapIndexes() error: %v", err)
	}
	if count2 != 0 {
		t.Errorf("second run downloaded %d files, want 0 (everything already present)", count2)
	}
}
