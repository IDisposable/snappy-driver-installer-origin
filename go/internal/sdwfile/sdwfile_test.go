package sdwfile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPeekVersion(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, 0x205, []byte("payload"), true); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
	version, ok := PeekVersion(bytes.NewReader(buf.Bytes()))
	if !ok || version != 0x205 {
		t.Errorf("PeekVersion() = %#x, %v; want 0x205, true", version, ok)
	}
}

func TestPeekVersionRejectsBadMagic(t *testing.T) {
	if _, ok := PeekVersion(bytes.NewReader([]byte("NOTSDW!"))); ok {
		t.Error("expected PeekVersion to reject a non-SDW header")
	}
}

func TestPeekVersionAgainstRealIndexFiles(t *testing.T) {
	candidates, _ := filepath.Glob("/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI/*.bin")
	if len(candidates) == 0 {
		t.Skip("no real installation available at the expected path; skipping")
	}
	for _, path := range candidates[:min(10, len(candidates))] {
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("opening %s: %v", path, err)
		}
		version, ok := PeekVersion(f)
		f.Close()
		if !ok {
			t.Errorf("PeekVersion(%s) failed", filepath.Base(path))
		}
		if version == 0 {
			t.Errorf("PeekVersion(%s) = 0, want a nonzero format version", filepath.Base(path))
		}
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	for _, compressed := range []bool{true, false} {
		payload := bytes.Repeat([]byte("hello driver pack index data "), 1000)

		var buf bytes.Buffer
		if err := Encode(&buf, 0x205, payload, compressed); err != nil {
			t.Fatalf("Encode(compressed=%v) error: %v", compressed, err)
		}

		version, got, err := Decode(&buf, compressed)
		if err != nil {
			t.Fatalf("Decode(compressed=%v) error: %v", compressed, err)
		}
		if version != 0x205 {
			t.Errorf("version = %#x, want 0x205", version)
		}
		if !bytes.Equal(got, payload) {
			t.Errorf("round-tripped payload does not match original (compressed=%v)", compressed)
		}
	}
}

func TestDecodeRejectsBadMagic(t *testing.T) {
	_, _, err := Decode(bytes.NewReader([]byte("NOTSDW!!")), true)
	if err == nil {
		t.Fatal("expected an error for a non-SDW header")
	}
}

// TestDecodeRealIndexFiles decodes every driver-pack index file from a
// production installation, if available, and checks each decompressed
// payload's size is sane. This is the strongest evidence this
// package's understanding of the on-disk format is correct - see
// go/README.md's "SDW container format" section for how it was
// reverse-engineered - and that it holds across files, not just one
// sample.
func TestDecodeRealIndexFiles(t *testing.T) {
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

			version, payload, err := Decode(f, true)
			if err != nil {
				t.Fatalf("Decode(%s) error: %v", path, err)
			}
			if version == 0 {
				t.Error("expected a non-zero format version")
			}
			if len(payload) == 0 {
				t.Error("expected a non-empty decompressed payload")
			}
			t.Logf("decoded %s: version=%#x payload=%d bytes", filepath.Base(path), version, len(payload))
		})
	}
}

// TestDecodeRealStateSnapshots decodes every state snapshot (.snp)
// under the reference installation's logs directory, from more than
// one machine, if available. Byte-compatibility for .snp files isn't
// required (see go/README.md), but they share the SDW container
// format, so successfully decoding samples from a second, independent
// machine is further evidence the format understanding generalizes.
func TestDecodeRealStateSnapshots(t *testing.T) {
	candidates, _ := filepath.Glob("/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/logs/*.snp")
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

			version, payload, err := Decode(f, true)
			if err != nil {
				t.Fatalf("Decode(%s) error: %v", path, err)
			}
			if len(payload) == 0 {
				t.Error("expected a non-empty decompressed payload")
			}
			t.Logf("decoded %s: version=%#x payload=%d bytes", filepath.Base(path), version, len(payload))
		})
	}
}
