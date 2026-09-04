package collection

import (
	"os"
	"path/filepath"
	"testing"

	"sdio/internal/indexing"
)

// realIndexDirCopy copies the reference installation's real index
// directory into a fresh temp dir and returns its path - LoadCollection
// can now write a freshly-built index for any pack it finds missing/
// stale one for (see BuildIndexFromArchive), and these tests must not
// let that land in the real, OneDrive-synced production directory just
// from running `go test`. driverpackDir itself is never written to
// (only read to discover/extract .7z files), so it's fine to point
// straight at the real one.
func realIndexDirCopy(t *testing.T, realIndexDir string) string {
	t.Helper()
	entries, err := os.ReadDir(realIndexDir)
	if err != nil {
		t.Skipf("no real installation available: %v", err)
	}
	dst := t.TempDir()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(realIndexDir, e.Name()))
		if err != nil {
			t.Fatalf("copying %s: %v", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0o644); err != nil {
			t.Fatalf("copying %s: %v", e.Name(), err)
		}
	}
	return dst
}

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

func TestExpectedPackFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"_P_Ports_SDIO01_26083.bin", "DP_Ports_SDIO01_26083.7z"},
		{"_P_CardReader_26072.bin", "DP_CardReader_26072.7z"},
	}
	for _, c := range cases {
		if got := expectedPackFilename(c.in); got != c.want {
			t.Errorf("expectedPackFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestLoadOnlineIndexesRealInstallation confirms LoadOnlineIndexes
// against a real installation known (by direct inspection) to have
// 104 underscore-prefixed placeholder index files, of which 101
// already have a locally-downloaded .7z (stale placeholders, skipped)
// and 3 genuinely don't (DP_CardReader_26072, DP_Video_nVIDIA_
// Server_26040, DP_Videos_AMD_Server_26040 - all Server-only/legacy
// variants, plausibly excluded from the pre-downloaded set).
func TestLoadOnlineIndexesRealInstallation(t *testing.T) {
	const driverpackDir = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/drivers"
	const indexDir = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI"

	// Deliberately re-derive the locally-scanned (non-pending) pack
	// list ourselves, rather than reusing LoadCollection's result:
	// LoadCollection's return value already has pending packs merged
	// in, and passing that back in as "already loaded" would make
	// every pending pack look already-downloaded.
	files, err := indexing.ScanDriverpackFolder(driverpackDir)
	if err != nil {
		t.Skipf("no real installation available: %v", err)
	}
	locallyLoaded := make([]*indexing.Driverpack, len(files))
	for i, f := range files {
		locallyLoaded[i] = &indexing.Driverpack{Filename: f.Filename}
	}

	pending, err := LoadOnlineIndexes(indexDir, locallyLoaded)
	if err != nil {
		t.Fatalf("LoadOnlineIndexes() error: %v", err)
	}

	wantPending := map[string]bool{
		"DP_CardReader_26072.7z":          true,
		"DP_Video_nVIDIA_Server_26040.7z": true,
		"DP_Videos_AMD_Server_26040.7z":   true,
	}
	if len(pending) != len(wantPending) {
		var got []string
		for _, p := range pending {
			got = append(got, p.Filename)
		}
		t.Fatalf("got %d pending packs %v, want %d: %v", len(pending), got, len(wantPending), wantPending)
	}
	for _, p := range pending {
		if !wantPending[p.Filename] {
			t.Errorf("unexpected pending pack %q", p.Filename)
		}
		if !p.Pending {
			t.Errorf("%s: Pending = false, want true", p.Filename)
		}
		if len(p.Index.HWIDs) == 0 {
			t.Errorf("%s: index has 0 HWIDs, want a fully usable index", p.Filename)
		}
	}
}

// TestLoadCollectionIncludesPendingPacks confirms LoadCollection
// itself (not just LoadOnlineIndexes directly) surfaces the same 3
// pending packs, with Path set so a caller could locate where the
// download should land.
func TestLoadCollectionIncludesPendingPacks(t *testing.T) {
	const driverpackDir = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/drivers"
	indexDir := realIndexDirCopy(t, "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI")

	result, err := LoadCollection(driverpackDir, indexDir, false, false)
	if err != nil {
		t.Skipf("no real installation available: %v", err)
	}

	pendingCount := 0
	for _, p := range result.Packs {
		if !p.Pending {
			continue
		}
		pendingCount++
		if p.Path != driverpackDir {
			t.Errorf("%s: Path = %q, want %q", p.Filename, p.Path, driverpackDir)
		}
		if filepath.Ext(p.Filename) != ".7z" {
			t.Errorf("%s: expected a .7z filename", p.Filename)
		}
	}
	if pendingCount != 3 {
		t.Errorf("got %d pending packs via LoadCollection, want 3", pendingCount)
	}
}

// TestLoadCollectionRealInstallation loads every real driver pack in
// the reference installation (a pack with a missing/stale index now
// gets a fresh one built from its own .7z instead of being skipped -
// see BuildIndexFromArchive), confirming very few packs are genuinely
// unusable (an unreadable .7z, not just a missing index).
func TestLoadCollectionRealInstallation(t *testing.T) {
	const driverpackDir = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/drivers"
	indexDir := realIndexDirCopy(t, "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI")

	result, err := LoadCollection(driverpackDir, indexDir, false, false)
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
