package indexing

import (
	"testing"
	"unicode/utf16"

	"sdio/internal/archive"
)

// buildOSAttrBlock encodes one occurrence of the marker as found in a
// real .cat file: "OSAtt" (UTF-16LE) + "r" + 7 filler bytes + a length
// byte + a null-terminated UTF-16LE string, matching the exact 19-byte
// prefix confirmed against dtport.cat (see
// TestFindOSAttrRealCatalogFile) - the string starts at offset 19 from
// the marker, which is why FindOSAttr peeks at that offset to decide
// its alignment.
func buildOSAttrBlock(value string) []byte {
	buf := []byte{'O', 0, 'S', 0, 'A', 0, 't', 0, 't', 0, 'r', 0x02, 0x04, 0x10, 0x01, 0x00, 0x01, 0x04, 0x0e}
	units := utf16.Encode([]rune(value))
	units = append(units, 0)
	for _, u := range units {
		buf = append(buf, byte(u), byte(u>>8))
	}
	return buf
}

func TestFindOSAttrSyntheticSingleBlock(t *testing.T) {
	data := buildOSAttrBlock("2:6.1,2:10.0")
	got := FindOSAttr(data)
	if got != "2:6.1,2:10.0" {
		t.Errorf("FindOSAttr() = %q, want %q", got, "2:6.1,2:10.0")
	}
}

func TestFindOSAttrKeepsLongestOfMultipleBlocks(t *testing.T) {
	var data []byte
	data = append(data, buildOSAttrBlock("2:6.1")...)
	data = append(data, buildOSAttrBlock("2:6.1,2:10.0")...)
	data = append(data, buildOSAttrBlock("2:10.0")...)
	got := FindOSAttr(data)
	if got != "2:6.1,2:10.0" {
		t.Errorf("FindOSAttr() = %q, want the longest block %q", got, "2:6.1,2:10.0")
	}
}

func TestFindOSAttrNoMarker(t *testing.T) {
	if got := FindOSAttr([]byte("just some random bytes, no marker here")); got != "" {
		t.Errorf("FindOSAttr() = %q, want empty", got)
	}
}

func TestFindOSAttrTruncatedNearEnd(t *testing.T) {
	data := []byte{'O', 0, 'S', 0, 'A', 0, 't', 0, 't', 0}
	if got := FindOSAttr(data); got != "" {
		t.Errorf("FindOSAttr() on truncated marker = %q, want empty (must not panic or over-read)", got)
	}
}

func TestIsValidCat(t *testing.T) {
	cat := "2:6.1,2:10.0"
	cases := []struct {
		major, minor int
		want         bool
	}{
		{6, 1, true},
		{10, 0, true},
		{11, 0, true}, // Windows 11 normalizes to major 10 for cat matching
		{6, 2, false},
		{5, 1, false},
	}
	for _, c := range cases {
		if got := IsValidCat(cat, c.major, c.minor); got != c.want {
			t.Errorf("IsValidCat(%q, %d, %d) = %v, want %v", cat, c.major, c.minor, got, c.want)
		}
	}
}

func TestIsValidCatEmpty(t *testing.T) {
	if IsValidCat("", 10, 0) {
		t.Error("IsValidCat(\"\", ...) = true, want false")
	}
}

// TestFindOSAttrRealCatalogFile extracts a real .cat file from a real
// driver pack and confirms FindOSAttr finds its OSAttr string. This is
// the byte layout ("OSAtt" + "r" + 7 bytes + length byte + string)
// that buildOSAttrBlock above reproduces synthetically, confirmed via
// a hex dump of this exact file before writing FindOSAttr.
func TestFindOSAttrRealCatalogFile(t *testing.T) {
	const path = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/drivers/DP_Ports_SDIO01_26083.7z"
	const catName = "dt/allx64/DtPort_1.0.0.6/dtport.cat"

	r, err := archive.Open(path)
	if err != nil {
		t.Skipf("real driver pack not available at %s: %v", path, err)
	}
	defer r.Close()

	data, err := r.Extract(catName)
	if err != nil {
		t.Fatalf("Extract(%s) error: %v", catName, err)
	}

	got := FindOSAttr(data)
	if got == "" {
		t.Fatal("FindOSAttr() found no OSAttr string in a real .cat file")
	}
	t.Logf("FindOSAttr(%s) = %q", catName, got)

	if !IsValidCat(got, 10, 0) {
		t.Errorf("IsValidCat(%q, 10, 0) = false, want true (this driver targets Windows 10/11)", got)
	}
}
