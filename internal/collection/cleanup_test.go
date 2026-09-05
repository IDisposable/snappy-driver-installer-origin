package collection

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindOldDriverPacksKeepsOnlyHighestRevisionPerGroup(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "DP_Ports_SDIO01_26040.7z")
	touch(t, dir, "DP_Ports_SDIO01_26072.7z")
	touch(t, dir, "DP_Ports_SDIO01_26083.7z") // highest, keep
	touch(t, dir, "DP_CardReader_26072.7z")   // only one in its group, keep

	old, err := FindOldDriverPacks(dir)
	if err != nil {
		t.Fatalf("FindOldDriverPacks() error: %v", err)
	}
	if len(old) != 2 {
		t.Fatalf("FindOldDriverPacks() = %+v, want exactly the 2 superseded Ports files", old)
	}
	names := map[string]bool{}
	for _, o := range old {
		names[filepath.Base(o.Path)] = true
		if o.KeptFile != "DP_Ports_SDIO01_26083.7z" {
			t.Errorf("KeptFile = %q, want DP_Ports_SDIO01_26083.7z", o.KeptFile)
		}
	}
	if !names["DP_Ports_SDIO01_26040.7z"] || !names["DP_Ports_SDIO01_26072.7z"] {
		t.Errorf("old = %v, want both lower-revision Ports files", names)
	}
}

func TestFindOldDriverPacksIgnoresFilesWithoutARevisionSegment(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "readme.txt")
	touch(t, dir, "DP_NoRevision.7z") // no "_?????" segment before .7z

	old, err := FindOldDriverPacks(dir)
	if err != nil {
		t.Fatalf("FindOldDriverPacks() error: %v", err)
	}
	if len(old) != 0 {
		t.Errorf("FindOldDriverPacks() = %+v, want nothing (no file has a 5-char revision segment)", old)
	}
}

func TestCleanOldDriverPacksDryRunDoesNotRemoveFiles(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "DP_Ports_SDIO01_26040.7z")
	touch(t, dir, "DP_Ports_SDIO01_26083.7z")

	var buf bytes.Buffer
	n, err := CleanOldDriverPacks(dir, true, &buf)
	if err != nil {
		t.Fatalf("CleanOldDriverPacks() error: %v", err)
	}
	if n != 1 {
		t.Errorf("CleanOldDriverPacks() = %d, want 1", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "DP_Ports_SDIO01_26040.7z")); err != nil {
		t.Error("dry run should not have removed the superseded file")
	}
}

func TestCleanOldDriverPacksRemovesSupersededFiles(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "DP_Ports_SDIO01_26040.7z")
	touch(t, dir, "DP_Ports_SDIO01_26083.7z")

	var buf bytes.Buffer
	n, err := CleanOldDriverPacks(dir, false, &buf)
	if err != nil {
		t.Fatalf("CleanOldDriverPacks() error: %v", err)
	}
	if n != 1 {
		t.Errorf("CleanOldDriverPacks() = %d, want 1", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "DP_Ports_SDIO01_26040.7z")); !os.IsNotExist(err) {
		t.Error("superseded file should have been removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "DP_Ports_SDIO01_26083.7z")); err != nil {
		t.Error("the kept (highest-revision) file should still exist")
	}
}
