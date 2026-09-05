package archive

import (
	"os"
	"path/filepath"
	"testing"
)

const realDriverPack = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/drivers/DP_Ports_SDIO01_26083.7z"

func TestOpenExtractRealDriverPack(t *testing.T) {
	r, err := Open(realDriverPack)
	if err != nil {
		t.Skipf("real driver pack not available at %s: %v", realDriverPack, err)
	}
	defer r.Close()

	files := r.Files()
	if len(files) == 0 {
		t.Fatal("expected at least one file in the archive")
	}

	var infPath string
	for _, f := range files {
		if HasSuffixFold(f.Name, ".inf") {
			infPath = f.Name
			break
		}
	}
	if infPath == "" {
		t.Fatal("expected at least one .inf file in the archive")
	}

	data, err := r.Extract(infPath)
	if err != nil {
		t.Fatalf("Extract(%s) error: %v", infPath, err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty .inf content")
	}
	if data[0] != '\r' && data[0] != '[' && data[0] != ';' {
		t.Errorf("extracted .inf content doesn't look like an .inf file, starts with %q", data[:min(20, len(data))])
	}
	t.Logf("extracted %s: %d bytes", infPath, len(data))
}

func TestHasSuffixFold(t *testing.T) {
	cases := []struct {
		name, suffix string
		want         bool
	}{
		{"foo.INF", ".inf", true},
		{"foo.inf", ".inf", true},
		{"foo.cat", ".inf", false},
		{"inf", ".inf", false},
		{".inf", ".inf", true},
	}
	for _, c := range cases {
		if got := HasSuffixFold(c.name, c.suffix); got != c.want {
			t.Errorf("HasSuffixFold(%q, %q) = %v, want %v", c.name, c.suffix, got, c.want)
		}
	}
}

// TestExtractPrefixRealDriverPack extracts dtport's whole driver
// folder from a real archive and confirms every expected file (the
// .inf plus its .cat and any others) lands on disk with matching
// content, ported use case: this is the step driver_install needs
// before calling UpdateDriverForPlugAndPlayDevices, since Windows
// needs the .inf's supporting files present alongside it, not just
// extracted individually.
func TestExtractPrefixRealDriverPack(t *testing.T) {
	r, err := Open(realDriverPack)
	if err != nil {
		t.Skipf("real driver pack not available at %s: %v", realDriverPack, err)
	}
	defer r.Close()

	const prefix = "dt/allx64/DtPort_1.0.0.6/"
	destDir := t.TempDir()

	n, err := r.ExtractPrefix(prefix, destDir)
	if err != nil {
		t.Fatalf("ExtractPrefix(%s) error: %v", prefix, err)
	}
	if n == 0 {
		t.Fatal("ExtractPrefix() extracted 0 files")
	}

	infPath := filepath.Join(destDir, "dtport.inf")
	extracted, err := os.ReadFile(infPath)
	if err != nil {
		t.Fatalf("reading extracted %s: %v", infPath, err)
	}

	want, err := r.Extract(prefix + "dtport.inf")
	if err != nil {
		t.Fatalf("Extract(%sdtport.inf) error: %v", prefix, err)
	}
	if string(extracted) != string(want) {
		t.Error("extracted file content doesn't match Extract()'s content")
	}

	if _, err := os.Stat(filepath.Join(destDir, "dtport.cat")); err != nil {
		t.Errorf("expected dtport.cat to also be extracted: %v", err)
	}
	t.Logf("extracted %d files to %s", n, destDir)
}

func TestExtractPrefixUnknownPrefixErrors(t *testing.T) {
	r, err := Open(realDriverPack)
	if err != nil {
		t.Skipf("real driver pack not available at %s: %v", realDriverPack, err)
	}
	defer r.Close()

	if _, err := r.ExtractPrefix("no/such/prefix/", t.TempDir()); err == nil {
		t.Fatal("expected an error for a prefix matching no files")
	}
}

func TestExtractMissingFileErrors(t *testing.T) {
	r, err := Open(realDriverPack)
	if err != nil {
		t.Skipf("real driver pack not available at %s: %v", realDriverPack, err)
	}
	defer r.Close()

	if _, err := r.Extract("does-not-exist.inf"); err == nil {
		t.Fatal("expected an error extracting a nonexistent file")
	}
}
