package usbdrive

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRequiredBytesSumsFilesRecursively(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 10)
	writeFile(t, filepath.Join(root, "sub", "b.txt"), 20)
	writeFile(t, filepath.Join(root, "sub", "deeper", "c.txt"), 5)

	got, err := RequiredBytes([]string{root})
	if err != nil {
		t.Fatalf("RequiredBytes() error: %v", err)
	}
	if got != 35 {
		t.Errorf("RequiredBytes() = %d, want 35", got)
	}
}

func TestCopyPortablePreservesStructureAndDoesNotTouchExisting(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "drivers", "DP_Test.7z"), 100)
	writeFile(t, filepath.Join(src, "sdio.cfg"), 10)

	dest := t.TempDir()
	// A pre-existing file that CopyPortable must not touch, since it
	// only ever adds/overwrites files it's explicitly given - never
	// deletes anything already on the "drive".
	preexisting := filepath.Join(dest, "keep-me.txt")
	writeFile(t, preexisting, 3)

	var buf bytes.Buffer
	err := CopyPortable(dest, []string{
		filepath.Join(src, "drivers"),
		filepath.Join(src, "sdio.cfg"),
	}, &buf)
	if err != nil {
		t.Fatalf("CopyPortable() error: %v\noutput:\n%s", err, buf.String())
	}

	for _, want := range []string{
		filepath.Join(dest, "drivers", "DP_Test.7z"),
		filepath.Join(dest, "sdio.cfg"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("expected %s to exist: %v", want, err)
		}
	}
	if _, err := os.Stat(preexisting); err != nil {
		t.Errorf("CopyPortable must not remove pre-existing files: %v", err)
	}
}
