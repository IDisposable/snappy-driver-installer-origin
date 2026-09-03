package collection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdio/internal/hardware"
	"sdio/internal/indexing"
	"sdio/internal/matcher"
	"sdio/internal/sdwfile"
)

func loadRealPack(t *testing.T, path, filename string) *indexing.Driverpack {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("real index file not available at %s: %v", path, err)
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
	return &indexing.Driverpack{Path: filepath.Dir(path), Filename: filename, Index: idx}
}

// TestMatchRealDtPortDevice runs the full Match pipeline against a
// synthetic device carrying the real DTBUS\COMPORT&VID_37DD&PID_6001
// hardware ID and a real driver pack's index, confirming it finds both
// section-variant candidates (see TestDriverpackRealIndexFile in
// internal/indexing), ranks the decorated (.NTamd64) one first since
// it scores higher on a Windows 10 amd64 system, and marks neither as
// a dup of the other (different sections).
func TestMatchRealDtPortDevice(t *testing.T) {
	const path = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI/DP_Ports_SDIO01_26083.bin"
	drp := loadRealPack(t, path, "DP_Ports_SDIO01_26083.7z")

	device := hardware.Device{
		HardwareIDs: []string{`DTBUS\COMPORT&VID_37DD&PID_6001`},
	}
	ctx := indexing.MatchContext{Major: 10, Minor: 0, Build: 19045, IsAMD64: true}

	dm := Match(device, nil, []*indexing.Driverpack{drp}, ctx, nil)

	if dm.Status != 0 {
		t.Fatalf("DeviceMatch.Status = %#x, want 0 (candidates found)", dm.Status)
	}
	if len(dm.Candidates) != 2 {
		t.Fatalf("got %d candidates, want 2 (bare + .NTamd64 sections)", len(dm.Candidates))
	}

	best := dm.Candidates[0]
	if !strings.EqualFold(best.Result.Section, "dthw.ntamd64") {
		t.Errorf("best candidate section = %q, want the decorated .NTamd64 section to rank first", best.Result.Section)
	}
	if best.Result.AltSectScore == 0 {
		t.Error("best candidate has AltSectScore == 0 (should have been valid)")
	}
	for _, c := range dm.Candidates {
		if c.Dup {
			t.Errorf("candidate (section=%q) marked as dup, want neither marked (different sections)", c.Result.Section)
		}
	}
}

func TestMatchNoCandidatesFallsBackToNFStandard(t *testing.T) {
	device := hardware.Device{HardwareIDs: []string{`ACPI\NONEXISTENT_DEVICE_ID_1234`}}
	ctx := indexing.MatchContext{Major: 10, Minor: 0, Build: 19045, IsAMD64: true}

	dm := Match(device, nil, nil, ctx, nil)
	if dm.Status != matcher.StatusNFStandard {
		t.Errorf("Status = %#x, want StatusNFStandard", dm.Status)
	}
	if dm.Candidates != nil {
		t.Errorf("Candidates = %v, want nil", dm.Candidates)
	}
}

func TestMatchIgnoredDevice(t *testing.T) {
	device := hardware.Device{HardwareIDs: []string{`ACPI\IGNOREME`}}
	ctx := indexing.MatchContext{Major: 10, Minor: 0, Build: 19045, IsAMD64: true}

	dm := Match(device, nil, nil, ctx, []string{`ACPI\IGNOREME`})
	if dm.Status != matcher.StatusIgnored {
		t.Errorf("Status = %#x, want StatusIgnored", dm.Status)
	}
}

func TestMatchNoHardwareIDNoCrash(t *testing.T) {
	device := hardware.Device{}
	ctx := indexing.MatchContext{Major: 10, Minor: 0, Build: 19045, IsAMD64: true}
	dm := Match(device, nil, nil, ctx, nil)
	if dm.Status != matcher.StatusNFStandard {
		t.Errorf("Status = %#x, want StatusNFStandard", dm.Status)
	}
}

func TestIsMissingDisabledDeviceIsNotMissing(t *testing.T) {
	d := hardware.Device{Problem: cmProbDisabled, HardwareIDs: []string{"X"}}
	if isMissing(d, nil) {
		t.Error("a disabled device (CM_PROB_DISABLED) should not count as missing")
	}
}

func TestIsMissingProblemWithHardwareID(t *testing.T) {
	d := hardware.Device{Problem: 1, HardwareIDs: []string{"X"}}
	if !isMissing(d, nil) {
		t.Error("a device with a problem code and a hardware ID should be missing")
	}
}

func TestIsMissingNoDriverKnownMissingBuses(t *testing.T) {
	for _, hwid := range []string{`USB\USBPRINT_FOO`, `LPTENUM\DOT4PRT_FOO`, `BTHENUM\SOMETHING`} {
		d := hardware.Device{HardwareIDs: []string{hwid}}
		if !isMissing(d, nil) {
			t.Errorf("device with hwid %q and no installed driver should be missing", hwid)
		}
	}
}

func TestIsMissingInstalledDriverSuppressesKnownMissingBusCheck(t *testing.T) {
	d := hardware.Device{HardwareIDs: []string{`USB\USBPRINT_FOO`}}
	installed := &hardware.InstalledDriver{MatchingDeviceID: `USB\USBPRINT_FOO`}
	if isMissing(d, installed) {
		t.Error("the USBPRINT/DOT4PRT/BTHENUM check should only apply when there is no installed driver")
	}
}

func TestIsMissingPCIBaseClassAlwaysMissing(t *testing.T) {
	d := hardware.Device{HardwareIDs: []string{`PCI\VEN_0000`}}
	installed := &hardware.InstalledDriver{MatchingDeviceID: `PCI\CC_0300`}
	if !isMissing(d, installed) {
		t.Error("an installed driver matched via the PCI\\CC_0300 base class should count as missing")
	}
}

func TestFirstHWIDPrefersHardwareOverCompatible(t *testing.T) {
	d := hardware.Device{HardwareIDs: []string{"H1", "H2"}, CompatibleIDs: []string{"C1"}}
	if got := firstHWID(d); got != "H1" {
		t.Errorf("firstHWID() = %q, want %q", got, "H1")
	}
}

func TestFirstHWIDFallsBackToCompatible(t *testing.T) {
	d := hardware.Device{CompatibleIDs: []string{"C1"}}
	if got := firstHWID(d); got != "C1" {
		t.Errorf("firstHWID() = %q, want %q", got, "C1")
	}
}
