package update

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNetworkDriverPacksFilter(t *testing.T) {
	cases := []struct {
		filename string
		want     bool
	}{
		{"DP_LAN_Realtek-NT_26081.7z", true},
		{"DP_WLAN-WiFi_26082.7z", true},
		{"DP_Net_Intel_26080.7z", true},
		{"dp_wwan-4g_26080.7z", true},
		{"DP_Ports_SDIO01_26083.7z", false},
		{"DP_Biometric_26082.7z", false},
	}
	for _, c := range cases {
		if got := NetworkDriverPacks(c.filename); got != c.want {
			t.Errorf("NetworkDriverPacks(%q) = %v, want %v", c.filename, got, c.want)
		}
	}
}

func TestAllDriverPacksMatchesEverything(t *testing.T) {
	for _, name := range []string{"DP_Ports_SDIO01_26083.7z", "anything.7z", ""} {
		if !AllDriverPacks(name) {
			t.Errorf("AllDriverPacks(%q) = false, want true", name)
		}
	}
}

// realTorrentPathForBulk mirrors internal/installflow's real cached
// torrent fixture.
const realTorrentPathForBulk = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/torrent/SDIO_Update.torrent"

// TestDownloadDriverPacksRealTorrent downloads one genuinely small,
// known-pending real driver pack via a narrow filter (not a whole
// category - "network drivers" or "all" would mean hundreds of files/
// many GB, impractical for a test even gated behind the real-network
// opt-in), confirming the category-filter download path saves it
// under drpDir with its real filename.
//
// Skipped unless SDIO_TEST_REAL_TORRENT=1 - see internal/installflow's
// real-torrent test for why (a go-test-harness-specific shutdown
// quirk with real BitTorrent networking, not a production issue).
func TestDownloadDriverPacksRealTorrent(t *testing.T) {
	if os.Getenv("SDIO_TEST_REAL_TORRENT") != "1" {
		t.Skip("set SDIO_TEST_REAL_TORRENT=1 to run (real network download; see doc comment)")
	}
	if _, err := os.Stat(realTorrentPathForBulk); err != nil {
		t.Skipf("real torrent file not available at %s: %v", realTorrentPathForBulk, err)
	}

	const packFilename = "DP_CardReader_26072.7z"
	drpDir := t.TempDir()
	updatesDir := t.TempDir()

	onlyCardReader := func(filename string) bool {
		return strings.EqualFold(filename, packFilename)
	}

	var buf bytes.Buffer
	n, err := DownloadDriverPacks(realTorrentPathForBulk, drpDir, updatesDir, false, nil, onlyCardReader, &buf, 30*time.Minute, nil)
	if err != nil {
		t.Fatalf("DownloadDriverPacks() error: %v\noutput:\n%s", err, buf.String())
	}
	if n != 1 {
		t.Fatalf("DownloadDriverPacks() downloaded %d files, want 1\noutput:\n%s", n, buf.String())
	}

	dest := filepath.Join(drpDir, packFilename)
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("downloaded file not found at %s: %v", dest, err)
	}
	if info.Size() == 0 {
		t.Fatal("downloaded file is empty")
	}
	t.Logf("downloaded %s: %d bytes\noutput:\n%s", dest, info.Size(), buf.String())

	// A second run must find it already present and download nothing.
	buf.Reset()
	n, err = DownloadDriverPacks(realTorrentPathForBulk, drpDir, updatesDir, false, nil, onlyCardReader, &buf, 30*time.Minute, nil)
	if err != nil {
		t.Fatalf("second DownloadDriverPacks() error: %v", err)
	}
	if n != 0 {
		t.Errorf("second DownloadDriverPacks() downloaded %d files, want 0 (already present)", n)
	}
}
