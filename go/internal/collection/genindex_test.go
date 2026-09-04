package collection

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"sdio/internal/indexing"
	"sdio/internal/sdwfile"
)

// realPortsArchive/realPortsIndex are the same real driver pack used
// throughout internal/indexing's tests (see realdriverpack_test.go) -
// a pack whose real, shipped index gives this test a genuine ground
// truth to compare a freshly built one against, not just "did it not
// crash".
const (
	realPortsArchive = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/drivers/DP_Ports_SDIO01_26083.7z"
	realPortsIndex   = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI/DP_Ports_SDIO01_26083.bin"
)

// hwidSet collects every hardware ID an index declares, uppercased -
// the comparison unit this test uses, since a freshly built index's
// own record ordering/interning offsets can legitimately differ from
// the original's (see indexing.BuildIndex's doc comment) while still
// being a fully correct index.
func hwidSet(t *testing.T, idx *indexing.Index) map[string]bool {
	t.Helper()
	set := make(map[string]bool, len(idx.HWIDs))
	for _, h := range idx.HWIDs {
		set[strings.ToUpper(idx.Text.GetString(h.HWID))] = true
	}
	return set
}

func loadRealIndex(t *testing.T, path string) *indexing.Index {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("real index not available at %s: %v", path, err)
	}
	defer f.Close()
	_, payload, err := sdwfile.Decode(f, true)
	if err != nil {
		t.Fatalf("Decode(%s) error: %v", path, err)
	}
	idx, err := indexing.DecodeIndex(payload)
	if err != nil {
		t.Fatalf("DecodeIndex(%s) error: %v", path, err)
	}
	return idx
}

// TestBuildIndexFromArchiveMatchesRealShippedIndex is the strongest
// check this package can offer without a second real SDIO client to
// cross-validate against: build a brand new index from the real
// DP_Ports_SDIO01_26083.7z, decode the REAL shipped index for that
// same pack, and confirm they declare the same hardware IDs, the same
// section for each, and the same install-picked value - the fields
// collection.Match actually depends on to find and rank a candidate.
// Byte-for-byte record layout/ordering is deliberately not compared -
// see indexing.BuildIndex's doc comment for why that's not required.
func TestBuildIndexFromArchiveMatchesRealShippedIndex(t *testing.T) {
	if _, err := os.Stat(realPortsArchive); err != nil {
		t.Skipf("real driver pack not available at %s: %v", realPortsArchive, err)
	}

	built, err := BuildIndexFromArchive(realPortsArchive)
	if err != nil {
		t.Fatalf("BuildIndexFromArchive() error: %v", err)
	}
	real := loadRealIndex(t, realPortsIndex)

	gotHWIDs, wantHWIDs := hwidSet(t, built), hwidSet(t, real)
	if len(gotHWIDs) == 0 {
		t.Fatal("BuildIndexFromArchive() found 0 hardware IDs, want at least the real dtport device")
	}
	for hwid := range wantHWIDs {
		if !gotHWIDs[hwid] {
			t.Errorf("built index is missing real HWID %q", hwid)
		}
	}
	for hwid := range gotHWIDs {
		if !wantHWIDs[hwid] {
			t.Errorf("built index has extra HWID %q not in the real shipped index", hwid)
		}
	}

	// Known real device from this pack (see internal/indexing's
	// realDtPortCandidate-style fixtures): confirm the built index
	// resolves the exact same section/install string a real client
	// reading the shipped index would.
	const wantHWID = `DTBUS\COMPORT&VID_37DD&PID_6001`
	builtDrp := &indexing.Driverpack{Filename: "DP_Ports_SDIO01_26083.7z", Index: built}
	realDrp := &indexing.Driverpack{Filename: "DP_Ports_SDIO01_26083.7z", Index: real}

	builtPos := findHWIDPos(t, built, wantHWID)
	realPos := findHWIDPos(t, real, wantHWID)
	if got, want := strings.ToLower(builtDrp.Section(builtPos)), strings.ToLower(realDrp.Section(realPos)); got != want {
		t.Errorf("Section() = %q, want %q", got, want)
	}
	if got, want := strings.ToLower(builtDrp.InstallPicked(builtPos)), strings.ToLower(realDrp.InstallPicked(realPos)); got != want {
		t.Errorf("InstallPicked() = %q, want %q", got, want)
	}
}

func findHWIDPos(t *testing.T, idx *indexing.Index, hwid string) int {
	t.Helper()
	for i, h := range idx.HWIDs {
		if strings.EqualFold(idx.Text.GetString(h.HWID), hwid) {
			return i
		}
	}
	t.Fatalf("HWID %q not found in index", hwid)
	return -1
}

// TestSaveIndexRoundTripsThroughRealArchive confirms SaveIndex's
// output loads back through the exact same sdwfile.Decode/DecodeIndex
// path every real indexes/**/*.bin file goes through - not just that
// BuildIndex's in-memory result looks right.
func TestSaveIndexRoundTripsThroughRealArchive(t *testing.T) {
	if _, err := os.Stat(realPortsArchive); err != nil {
		t.Skipf("real driver pack not available at %s: %v", realPortsArchive, err)
	}

	built, err := BuildIndexFromArchive(realPortsArchive)
	if err != nil {
		t.Fatalf("BuildIndexFromArchive() error: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "DP_Ports_SDIO01_26083.bin")
	if err := SaveIndex(built, dest); err != nil {
		t.Fatalf("SaveIndex() error: %v", err)
	}

	reloaded, err := loadIndex(dest)
	if err != nil {
		t.Fatalf("loadIndex(SaveIndex() output) error: %v", err)
	}

	got, want := hwidSet(t, reloaded), hwidSet(t, built)
	if len(got) != len(want) {
		t.Fatalf("reloaded index has %d HWIDs, want %d (round trip lost data)", len(got), len(want))
	}
	for hwid := range want {
		if !got[hwid] {
			t.Errorf("reloaded index is missing HWID %q present before saving", hwid)
		}
	}
}

// syntheticInf is a small, hand-built .inf covering one manufacturer,
// one undecorated section, and one decorated (.NTamd64) variant of the
// same section - enough to exercise BuildIndex's manufacturer/section-
// position/HWID walk without depending on real driver-pack data.
const syntheticInf = `[Version]
Signature="$Windows NT$"
Class=Ports
Provider=%MFG%
CatalogFile=synth.cat
DriverVer=01/02/2024,3.4.5.6

[Manufacturer]
%MFG%=SynthMfg,NTamd64

[SynthMfg]
%DEV1.DeviceDesc%=Synth.Install,SYNTH\VID_0001&PID_0002

[SynthMfg.NTamd64]
%DEV1.DeviceDesc%=Synth.Install.amd64,SYNTH\VID_0001&PID_0002&REV_01

[Strings]
MFG="Synth Corp"
DEV1.DeviceDesc="Synth Widget"
`

// TestBuildIndexSyntheticFixture is a fast, non-real-machine-dependent
// check of BuildIndex's core walk: one manufacturer, root + decorated
// section variants, one HWID per variant.
func TestBuildIndexSyntheticFixture(t *testing.T) {
	infFiles := map[string][]byte{
		"synth/synth.inf": []byte(syntheticInf),
	}
	idx := indexing.BuildIndex(infFiles, []string{"NTamd64"})

	if len(idx.InfFiles) != 1 {
		t.Fatalf("InfFiles = %d, want 1", len(idx.InfFiles))
	}
	if len(idx.Manufacturers) != 1 {
		t.Fatalf("Manufacturers = %d, want 1", len(idx.Manufacturers))
	}
	if got := idx.Text.GetString(idx.Manufacturers[0].Manufacturer); got != "Synth Corp" {
		t.Errorf("manufacturer name = %q, want %q", got, "Synth Corp")
	}

	var gotHWIDs []string
	for _, h := range idx.HWIDs {
		gotHWIDs = append(gotHWIDs, idx.Text.GetString(h.HWID))
	}
	sort.Strings(gotHWIDs)
	wantHWIDs := []string{`SYNTH\VID_0001&PID_0002`, `SYNTH\VID_0001&PID_0002&REV_01`}
	sort.Strings(wantHWIDs)
	if len(gotHWIDs) != len(wantHWIDs) {
		t.Fatalf("HWIDs = %v, want %v", gotHWIDs, wantHWIDs)
	}
	for i := range wantHWIDs {
		if gotHWIDs[i] != wantHWIDs[i] {
			t.Errorf("HWIDs = %v, want %v", gotHWIDs, wantHWIDs)
		}
	}

	// The hashtable must resolve a real lookup the same way
	// collection.Match performs one (see buildHashtable's doc comment
	// on why the key is uppercased).
	key := int32(indexing.APHash([]byte(strings.ToUpper(`SYNTH\VID_0001&PID_0002`))))
	if _, found := idx.Hashes.Find(key); !found {
		t.Error("Hashes.Find() didn't find the synthetic HWID's key")
	}
}
