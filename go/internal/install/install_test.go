package install

import (
	"os"
	"path/filepath"
	"testing"
)

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveExtraInfsKeepsTargetDeletesOthers(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dtport.inf")
	mustWriteFile(t, target)
	mustWriteFile(t, filepath.Join(dir, "other.inf"))
	mustWriteFile(t, filepath.Join(dir, "OTHER2.INF"))
	mustWriteFile(t, filepath.Join(dir, "readme.txt"))

	if err := RemoveExtraInfs(target); err != nil {
		t.Fatalf("RemoveExtraInfs() error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var remaining []string
	for _, e := range entries {
		remaining = append(remaining, e.Name())
	}
	if len(remaining) != 2 {
		t.Fatalf("remaining files = %v, want [dtport.inf readme.txt]", remaining)
	}
	want := map[string]bool{"dtport.inf": true, "readme.txt": true}
	for _, name := range remaining {
		if !want[name] {
			t.Errorf("unexpected surviving file %q", name)
		}
	}
}

func TestRemoveExtraInfsCaseInsensitiveMatch(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "DtPort.INF")
	mustWriteFile(t, filepath.Join(dir, "dtport.inf")) // same name, different case as target

	if err := RemoveExtraInfs(target); err != nil {
		t.Fatalf("RemoveExtraInfs() error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "dtport.inf")); err != nil {
		t.Error("expected the case-insensitively-matching file to survive")
	}
}

func TestRemoveExtraInfsNoOtherInfs(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dtport.inf")
	mustWriteFile(t, target)
	mustWriteFile(t, filepath.Join(dir, "readme.txt"))

	if err := RemoveExtraInfs(target); err != nil {
		t.Fatalf("RemoveExtraInfs() error: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Error("target .inf should still exist")
	}
	if _, err := os.Stat(filepath.Join(dir, "readme.txt")); err != nil {
		t.Error("unrelated non-.inf file should still exist")
	}
}
