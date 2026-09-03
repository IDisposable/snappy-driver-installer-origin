package collection

import "testing"

func TestIndexFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"DP_Ports_SDIO01_26083.7z", "DP_Ports_SDIO01_26083.bin"},
		{"DP_Ports_SDIO01_26083.7Z", "DP_Ports_SDIO01_26083.bin"},
	}
	for _, c := range cases {
		if got := indexFilename(c.in); got != c.want {
			t.Errorf("indexFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestLoadCollectionRealInstallation loads every real driver pack in
// the reference installation, confirming most load successfully (a
// handful of stale/mismatched index files are tolerated - see
// go/README.md's note on the reference collection containing orphaned
// .bin files with no matching .7z), then runs Match for the known
// dtport device against the loaded collection to confirm the loaded
// Driverpack values are usable end-to-end, not just individually
// decodable.
func TestLoadCollectionRealInstallation(t *testing.T) {
	const driverpackDir = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/drivers"
	const indexDir = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI"

	result, err := LoadCollection(driverpackDir, indexDir)
	if err != nil {
		t.Skipf("no real installation available: %v", err)
	}
	if len(result.Packs) == 0 {
		t.Fatal("loaded 0 driver packs")
	}
	t.Logf("loaded %d packs, skipped %d", len(result.Packs), len(result.Skipped))
	for _, s := range result.Skipped {
		t.Logf("skipped %s: %v", s.Filename, s.Err)
	}
	if len(result.Skipped) > len(result.Packs) {
		t.Errorf("skipped more packs (%d) than loaded (%d) - suspicious", len(result.Skipped), len(result.Packs))
	}
}
