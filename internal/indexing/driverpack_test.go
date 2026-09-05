package indexing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdio/internal/sdwfile"
)

// TestDriverpackRealIndexFile decodes a real index file (the same
// driver pack TestRealDriverPackFromArchive parses live from its .7z)
// and confirms Driverpack's getdrp_*-equivalent getters navigate the
// HWID -> Desc -> Manufacturer -> InfFile chain correctly, cross-
// checked against the known device from that other test.
func TestDriverpackRealIndexFile(t *testing.T) {
	const path = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI/DP_Ports_SDIO01_26083.bin"

	f, err := os.Open(path)
	if err != nil {
		t.Skipf("real index file not available at %s: %v", path, err)
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

	drp := &Driverpack{Path: filepath.Dir(path), Filename: filepath.Base(path), Index: idx}

	// The .inf's [Manufacturer] line ("%DT%=DtHw,NTamd64") declares two
	// model-section variants - bare "DtHw" and decorated "DtHw.NTamd64" -
	// each independently listing this same device with a different
	// install target, so genindex produces two separate HWID entries
	// for it (see dtportInf in realdriverpack_test.go).
	wantHWID := `DTBUS\COMPORT&VID_37DD&PID_6001`
	var found []int
	for i := range idx.HWIDs {
		if strings.EqualFold(drp.HWID(i), wantHWID) {
			found = append(found, i)
		}
	}
	if len(found) != 2 {
		t.Fatalf("HWID %q found %d times, want 2 (bare + .NTamd64 section variants)", wantHWID, len(found))
	}

	gotSections := map[string]int{}
	for _, i := range found {
		gotSections[strings.ToLower(drp.Section(i))] = i
	}
	for _, wantSection := range []string{"dthw", "dthw.ntamd64"} {
		i, ok := gotSections[wantSection]
		if !ok {
			t.Fatalf("no HWID entry with Section() == %q; got sections %v", wantSection, gotSections)
		}

		if !strings.HasSuffix(strings.ToLower(drp.InfName(i)), "dtport.inf") {
			t.Errorf("InfName(%d) = %q, want a name ending in dtport.inf", i, drp.InfName(i))
		}
		if !strings.Contains(strings.ToLower(drp.InfPath(i)), "dtport") {
			t.Errorf("InfPath(%d) = %q, want it to mention dtport", i, drp.InfPath(i))
		}
		if drp.Manufacturer(i) == "" {
			t.Errorf("Manufacturer(%d) is empty", i)
		}
		if drp.Desc(i) == "" {
			t.Errorf("Desc(%d) is empty", i)
		}
		v := drp.Version(i)
		if v.V1 != 1 || v.V4 != 6 {
			t.Errorf("Version(%d) = %+v, want 1.0.0.6", i, v)
		}
		if v.Day != 22 || v.Month != 12 || v.Year != 2025 {
			t.Errorf("Version(%d) date = %+v, want 2025-12-22", i, v)
		}
		if drp.InfCRC(i) == 0 {
			t.Errorf("InfCRC(%d) = 0, want a nonzero CRC", i)
		}
		if drp.InfSize(i) <= 0 {
			t.Errorf("InfSize(%d) = %d, want > 0", i, drp.InfSize(i))
		}

		t.Logf("HWID %d: infpath=%q infname=%q section=%q manufacturer=%q desc=%q version=%s install_picked=%q feature=%d",
			i, drp.InfPath(i), drp.InfName(i), drp.Section(i), drp.Manufacturer(i),
			drp.Desc(i), v, drp.InstallPicked(i), drp.Feature(i))
	}

	// The bare variant should resolve to DtPort.NT, the decorated
	// variant to DtPort.NTamd64, per the .inf's own [DtHw]/[DtHw.NTamd64]
	// bodies.
	if got := strings.ToLower(drp.InstallPicked(gotSections["dthw"])); got != "dtport.nt" {
		t.Errorf("InstallPicked(bare) = %q, want %q", got, "dtport.nt")
	}
	if got := strings.ToLower(drp.InstallPicked(gotSections["dthw.ntamd64"])); got != "dtport.ntamd64" {
		t.Errorf("InstallPicked(.ntamd64) = %q, want %q", got, "dtport.ntamd64")
	}
}

// TestDriverpackAllRealIndexFiles walks every real index file and
// exercises every HWID entry's full getter chain, checking for
// panics/out-of-range navigation and that string fields resolve to
// something plausible. This is the broad-coverage counterpart to
// TestDriverpackRealIndexFile's single-device deep check.
func TestDriverpackAllRealIndexFiles(t *testing.T) {
	root := "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI"
	candidates, _ := filepath.Glob(filepath.Join(root, "*.bin"))
	if len(candidates) == 0 {
		t.Skip("no real installation available; skipping")
	}

	checked := 0
	for _, path := range candidates {
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("opening %s: %v", path, err)
		}
		_, payload, err := sdwfile.Decode(f, true)
		f.Close()
		if err != nil {
			t.Fatalf("Decode(%s) error: %v", path, err)
		}
		idx, err := DecodeIndex(payload)
		if err != nil {
			t.Fatalf("DecodeIndex(%s) error: %v", path, err)
		}
		drp := &Driverpack{Path: root, Filename: filepath.Base(path), Index: idx}

		for i := range idx.HWIDs {
			if drp.HWID(i) == "" {
				t.Errorf("%s: HWID(%d) is empty", filepath.Base(path), i)
			}
			_ = drp.Section(i)
			_ = drp.InfPath(i)
			_ = drp.InfName(i)
			_ = drp.Manufacturer(i)
			_ = drp.Desc(i)
			_ = drp.Install(i)
			_ = drp.InstallPicked(i)
			_ = drp.Version(i)
			_ = drp.Feature(i)
			_ = drp.InfCRC(i)
			_ = drp.InfSize(i)
			_ = drp.InfPos(i)
			for n := 0; n < NumVerNames; n++ {
				_ = drp.Field(i, n)
				_ = drp.Cat(i, n)
			}
			checked++
		}
	}
	t.Logf("checked %d HWID entries across %d index files with no panics", checked, len(candidates))
}
