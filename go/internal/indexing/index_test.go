package indexing

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sdio/internal/sdwfile"
)

func TestEncodeDecodeIndexRoundTrip(t *testing.T) {
	idx := &Index{
		InfFiles: []InfFile{
			{InfPath: 1, InfFilename: 2, InfSize: 100, InfCRC: 200, Version: rawVersion{Day: 1, Month: 2, Year: 2024, V1: 1, V2: 0, V3: 0, V4: 0}},
		},
		Manufacturers: []Manufacturer{
			{InffileIndex: 0, Manufacturer: 3, Sections: 4, SectionsN: 1},
		},
		Descs: []Desc{
			{ManufacturerIndex: 0, SectPos: 1, Desc: 5, Install: 6, InstallPicked: 0, Feature: 0},
		},
		HWIDs: []HWID{
			{DescIndex: 0, InfPos: 0, HWID: 7},
		},
		Text:   Txt{Data: []byte("hello\x00world\x00")},
		Hashes: Hashtable{Size: 4, Items: []HashItem{{Key: 1, Value: 2, Next: -1, ValueLen: 0}}},
	}

	encoded, err := EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex() error: %v", err)
	}

	decoded, err := DecodeIndex(encoded)
	if err != nil {
		t.Fatalf("DecodeIndex() error: %v", err)
	}

	if !reflect.DeepEqual(idx, decoded) {
		t.Errorf("round-tripped index does not match original.\ngot:  %+v\nwant: %+v", decoded, idx)
	}
}

func TestDecodeIndexRejectsTrailingBytes(t *testing.T) {
	idx := &Index{Text: Txt{Data: []byte{}}}
	encoded, err := EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex() error: %v", err)
	}

	if _, err := DecodeIndex(append(encoded, 0xff, 0xff)); err == nil {
		t.Fatal("expected an error for trailing bytes after a well-formed index")
	}
}

func TestTxtGetAndGetW(t *testing.T) {
	txt := Txt{Data: []byte("abc\x00")}
	if got := string(txt.Get(0)); got != "abc" {
		t.Errorf("Get(0) = %q, want %q", got, "abc")
	}
	if got := txt.Get(100); got != nil {
		t.Errorf("Get(out of range) = %v, want nil", got)
	}

	// "Hi" as UTF-16LE, null-terminated.
	wide := Txt{Data: []byte{'H', 0, 'i', 0, 0, 0}}
	if got := wide.GetW(0); got != "Hi" {
		t.Errorf("GetW(0) = %q, want %q", got, "Hi")
	}
}

func TestAPHashIsDeterministic(t *testing.T) {
	a := APHash([]byte("PCI\\VEN_8086&DEV_1234"))
	b := APHash([]byte("PCI\\VEN_8086&DEV_1234"))
	if a != b {
		t.Fatal("APHash should be deterministic for the same input")
	}
	c := APHash([]byte("PCI\\VEN_8086&DEV_5678"))
	if a == c {
		t.Fatal("APHash should (almost certainly) differ for different input")
	}
}

// TestDecodeRealIndexes decodes every real driver-pack index file's
// full record structure (not just the raw decompressed bytes, which
// internal/sdwfile's own tests already cover), if a production
// installation is available. This is the strongest evidence the record
// layouts (data_inffile_t/data_manufacturer_t/data_desc_t/data_HWID_t/
// Hashitem field order and sizes) are correct: DecodeIndex rejects any
// trailing bytes, so a wrong struct size or field order would
// desynchronize the read sequence and fail almost immediately.
func TestDecodeRealIndexes(t *testing.T) {
	candidates, _ := filepath.Glob("/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI/*.bin")
	if len(candidates) == 0 {
		t.Skip("no real installation available at the expected path; skipping")
	}

	for _, path := range candidates {
		t.Run(filepath.Base(path), func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatalf("opening %s: %v", path, err)
			}
			defer f.Close()

			_, payload, err := sdwfile.Decode(f, true)
			if err != nil {
				t.Fatalf("sdwfile.Decode(%s) error: %v", path, err)
			}

			idx, err := DecodeIndex(payload)
			if err != nil {
				t.Fatalf("DecodeIndex(%s) error: %v", path, err)
			}

			t.Logf("%s: %d inf files, %d manufacturers, %d descs, %d hwids, %d text bytes, %d hash buckets",
				filepath.Base(path), len(idx.InfFiles), len(idx.Manufacturers), len(idx.Descs), len(idx.HWIDs),
				len(idx.Text.Data), idx.Hashes.Size)

			if len(idx.HWIDs) == 0 {
				t.Error("expected at least one hardware ID entry")
			}

			// Spot-check that at least some HWID strings look like real
			// PCI/USB hardware IDs, not garbage - a wrong field layout
			// would point offsets at the wrong place in the text blob
			// and produce nonsense here. Sampled across the whole file
			// (not just the first few entries): some driver packs lead
			// with dozens of legitimate non-bus service-binding IDs
			// (e.g. Intel's ANS teaming driver uses "IANSMINIPORT"/
			// "IANSPROTOCOL" before any PCI\VEN_ id appears), so a
			// small fixed-size prefix sample isn't reliable evidence
			// either way.
			sane := 0
			for _, h := range idx.HWIDs {
				s := idx.Text.GetString(h.HWID)
				if len(s) > 0 && (strings.Contains(s, "VEN_") || strings.Contains(s, "VID_") || strings.Contains(s, `\`)) {
					sane++
				}
			}
			if sane == 0 && len(idx.HWIDs) > 0 {
				t.Errorf("none of the first HWID strings looked like hardware IDs; got e.g. %q", idx.Text.GetString(idx.HWIDs[0].HWID))
			}
		})
	}
}
