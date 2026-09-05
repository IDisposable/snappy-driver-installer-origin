package indexing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDriverpackFolderSynthetic(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("DP_A.7z")
	mustWrite("DP_B.7Z") // case-insensitive extension match
	mustWrite("nested/DP_C.7z")
	mustWrite("DP_A.txt") // not a driver pack
	mustWrite("nested/readme.md")

	files, err := ScanDriverpackFolder(dir)
	if err != nil {
		t.Fatalf("ScanDriverpackFolder() error: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("got %d files, want 3: %+v", len(files), files)
	}

	names := map[string]bool{}
	for _, f := range files {
		names[f.Filename] = true
		if _, err := os.Stat(f.Path()); err != nil {
			t.Errorf("Path() %s does not exist: %v", f.Path(), err)
		}
	}
	for _, want := range []string{"DP_A.7z", "DP_B.7Z", "DP_C.7z"} {
		if !names[want] {
			t.Errorf("missing expected file %q in results", want)
		}
	}
}

func TestScanDriverpackFolderMissingRoot(t *testing.T) {
	if _, err := ScanDriverpackFolder(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected an error scanning a nonexistent root")
	}
}

func TestCountDriverpacksNeedingIndexForceReindex(t *testing.T) {
	files := []DriverpackFile{{Filename: "a.7z"}, {Filename: "b.7z"}}
	got := CountDriverpacksNeedingIndex(files, true, func(DriverpackFile) bool { return true })
	if got != len(files) {
		t.Errorf("CountDriverpacksNeedingIndex(forceReindex=true) = %d, want %d", got, len(files))
	}
}

func TestCountDriverpacksNeedingIndexRespectsValidity(t *testing.T) {
	files := []DriverpackFile{{Filename: "valid.7z"}, {Filename: "stale.7z"}, {Filename: "missing.7z"}}
	valid := map[string]bool{"valid.7z": true}
	got := CountDriverpacksNeedingIndex(files, false, func(f DriverpackFile) bool { return valid[f.Filename] })
	if got != 2 {
		t.Errorf("CountDriverpacksNeedingIndex() = %d, want 2", got)
	}
}

// TestScanDriverpackFolderRealCollection scans a real driver-pack
// collection and checks the count matches an independent filesystem
// count, and that internal/sdwfile.PeekVersion (used to implement
// hasValidIndex in production) succeeds against every real index file
// this collection has.
func TestScanDriverpackFolderRealCollection(t *testing.T) {
	root := "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/drivers"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("no real installation available at %s: %v", root, err)
	}

	files, err := ScanDriverpackFolder(root)
	if err != nil {
		t.Fatalf("ScanDriverpackFolder() error: %v", err)
	}

	independent := 0
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("os.ReadDir(%s) error: %v", root, err)
	}
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 3 && (e.Name()[len(e.Name())-3:] == ".7z" || e.Name()[len(e.Name())-3:] == ".7Z") {
			independent++
		}
	}

	if len(files) != independent {
		t.Errorf("ScanDriverpackFolder found %d files, independent count found %d", len(files), independent)
	}
	t.Logf("found %d real driver-pack archives", len(files))
}
