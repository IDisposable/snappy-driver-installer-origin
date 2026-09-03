package archive

import (
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
