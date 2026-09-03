package indexing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdio/internal/common"
	"sdio/internal/matcher"
	"sdio/internal/sdwfile"
)

func TestPackVersionNumber(t *testing.T) {
	cases := []struct {
		filename string
		want     int
	}{
		{"DP_USB3_intel_16074.7z", 16074},
		{"DP_USB3_intel_16073.7z", 16073},
		{"DP_Ports_SDIO01_26083.bin", 26083},
		{"no_underscore_digit_here.7z", 0},
		{"DP_a_b_c.7z", 0},
	}
	for _, c := range cases {
		if got := packVersionNumber(c.filename); got != c.want {
			t.Errorf("packVersionNumber(%q) = %d, want %d", c.filename, got, c.want)
		}
	}
}

func realDtPortDriverpack(t *testing.T) (*Driverpack, int) {
	t.Helper()
	const path = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI/DP_Ports_SDIO01_26083.bin"
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("real index file not available at %s: %v", path, err)
	}
	defer f.Close()

	_, payload, err := sdwfile.Decode(f, true)
	if err != nil {
		t.Fatalf("Decode(%s) error: %v", path, err)
	}
	idx, err := DecodeIndex(payload)
	if err != nil {
		t.Fatalf("DecodeIndex(%s) error: %v", path, err)
	}
	drp := &Driverpack{Path: filepath.Dir(path), Filename: "DP_Ports_SDIO01_26083.7z", Index: idx}

	wantHWID := `DTBUS\COMPORT&VID_37DD&PID_6001`
	for i := range idx.HWIDs {
		if strings.EqualFold(drp.HWID(i), wantHWID) && strings.EqualFold(drp.Section(i), "dthw.ntamd64") {
			return drp, i
		}
	}
	t.Fatalf("HWID %q with section dthw.ntamd64 not found", wantHWID)
	return nil, 0
}

// TestCalcAltSectScoreRealDeviceValid confirms a plain, unrestricted
// driver pack (no intel_2nd/matchver/matchmarker path segments) scores
// as valid against a plausible running-system context.
func TestCalcAltSectScoreRealDeviceValid(t *testing.T) {
	drp, i := realDtPortDriverpack(t)
	ctx := MatchContext{Major: 10, Minor: 0, Build: 19045, IsAMD64: true}

	decorScore := matcher.DecorationScore(matcher.SectionDecorationIndex(drp.Section(i)), ctx.Major, ctx.Minor, ctx.Build, ctx.ArchForDecoration())
	got := drp.CalcAltSectScore(i, decorScore, ctx, `USB\VID_37DD&PID_6001`)
	if got == 0 {
		t.Fatalf("CalcAltSectScore() = 0, want a nonzero (valid) score for an unrestricted driver pack")
	}
	t.Logf("CalcAltSectScore() = %d (decorScore=%d)", got, decorScore)
}

// TestCalcAltSectScoreRejectsWhenAltSectionScoresHigher confirms the
// "another declared section of this manufacturer would score better"
// rejection path fires when curScore is set below what the decorated
// section actually scores.
func TestCalcAltSectScoreRejectsWhenAltSectionScoresHigher(t *testing.T) {
	drp, i := realDtPortDriverpack(t)
	ctx := MatchContext{Major: 10, Minor: 0, Build: 19045, IsAMD64: true}

	if got := drp.CalcAltSectScore(i, -1, ctx, `USB\VID_37DD&PID_6001`); got != 0 {
		t.Errorf("CalcAltSectScore(curScore=-1) = %d, want 0 (some section should outscore an impossibly low curScore)", got)
	}
}

func TestCalcAltSectScoreRealtekBlacklist(t *testing.T) {
	drp, i := realDtPortDriverpack(t)
	ctx := MatchContext{Major: 10, Minor: 0, Build: 19045, IsAMD64: true}
	decorScore := matcher.DecorationScore(matcher.SectionDecorationIndex(drp.Section(i)), ctx.Major, ctx.Minor, ctx.Build, ctx.ArchForDecoration())

	blacklistedHWID := `PCI\VEN_168C&DEV_002B&SUBSYS_30A117AA`
	// This only exercises the blacklist path if the driver's own
	// section happens to contain "Realtek" - it doesn't for dtport, so
	// this just confirms IsBlacklisted's hwid-only match doesn't false-
	// positive when the section name doesn't mention Realtek.
	got := drp.CalcAltSectScore(i, decorScore, ctx, blacklistedHWID)
	if got == 0 {
		t.Error("expected the Realtek-HWID-but-non-Realtek-section case not to be blacklisted")
	}
}

func TestCalcAltSectScoreFilterSPShortCircuits(t *testing.T) {
	drp, i := realDtPortDriverpack(t)
	ctx := MatchContext{Major: 10, Minor: 0, Build: 19045, IsAMD64: true, FilterSP: true}
	decorScore := matcher.DecorationScore(matcher.SectionDecorationIndex(drp.Section(i)), ctx.Major, ctx.Minor, ctx.Build, ctx.ArchForDecoration())

	if got := drp.CalcAltSectScore(i, decorScore, ctx, `USB\VID_37DD&PID_6001`); got != 2 {
		t.Errorf("CalcAltSectScore(FilterSP=true) = %d, want 2", got)
	}
}

// TestCalcAltSectScoreAllRealHWIDEntries walks every real index file's
// every HWID entry, computing CalcAltSectScore with a plausible
// context, checking only that it never panics and always returns 0,
// 1, or 2.
func TestCalcAltSectScoreAllRealHWIDEntries(t *testing.T) {
	root := "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI"
	candidates, _ := filepath.Glob(filepath.Join(root, "*.bin"))
	if len(candidates) == 0 {
		t.Skip("no real installation available; skipping")
	}
	ctx := MatchContext{Major: 10, Minor: 0, Build: 19045, IsAMD64: true, IsLaptop: true, NotebookMarker: "Dell"}

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
		drp := &Driverpack{Path: root, Filename: strings.TrimSuffix(filepath.Base(path), ".bin") + ".7z", Index: idx}

		for i := range idx.HWIDs {
			decorScore := matcher.DecorationScore(matcher.SectionDecorationIndex(drp.Section(i)), ctx.Major, ctx.Minor, ctx.Build, ctx.ArchForDecoration())
			got := drp.CalcAltSectScore(i, decorScore, ctx, drp.HWID(i))
			if got < 0 || got > 2 {
				t.Fatalf("%s HWID %d: CalcAltSectScore() = %d, want 0/1/2", filepath.Base(path), i, got)
			}
			checked++
		}
	}
	t.Logf("checked CalcAltSectScore on %d real HWID entries across %d index files with no panics", checked, len(candidates))
}

func TestCalcStatusNoInstalledDriver(t *testing.T) {
	got := CalcStatus(false, common.Version{}, common.Version{}, 0, 0, false, 1)
	if got&matcher.StatusBetter == 0 {
		t.Errorf("CalcStatus(no installed driver) = %#x, want StatusBetter set", got)
	}
	if got&matcher.StatusInvalid != 0 {
		t.Errorf("CalcStatus(altSectScore=1) = %#x, want StatusInvalid unset", got)
	}
}

func TestCalcStatusInvalidWhenAltSectScoreZero(t *testing.T) {
	got := CalcStatus(false, common.Version{}, common.Version{}, 0, 0, false, 0)
	if got&matcher.StatusInvalid == 0 {
		t.Errorf("CalcStatus(altSectScore=0) = %#x, want StatusInvalid set", got)
	}
}

func TestCalcStatusNewerCandidateWins(t *testing.T) {
	installed := common.Version{Year: 2020, Month: 1, Day: 1}
	candidate := common.Version{Year: 2024, Month: 1, Day: 1}
	got := CalcStatus(true, installed, candidate, 5, 5, false, 1)
	if got&matcher.StatusNew == 0 {
		t.Errorf("CalcStatus() = %#x, want StatusNew set", got)
	}
}

func TestCalcStatusFeaturePrefixNarrowsScoreComparison(t *testing.T) {
	installed := common.Version{Year: 2024, Month: 1, Day: 1}
	candidate := common.Version{Year: 2024, Month: 1, Day: 1}
	// 0xFF00FFFF clears bits 16-23 only (a single middle byte, not the
	// whole low word) - scores differ only there.
	installedScore := uint32(0x00050000)
	candidateScore := uint32(0x00070000)

	withoutPrefix := CalcStatus(true, installed, candidate, installedScore, candidateScore, false, 1)
	if withoutPrefix&matcher.StatusSame != 0 {
		t.Error("without the feature_ prefix, differing bits 16-23 should not compare equal")
	}

	withPrefix := CalcStatus(true, installed, candidate, installedScore, candidateScore, true, 1)
	if withPrefix&matcher.StatusSame == 0 {
		t.Errorf("CalcStatus(feature_ prefix) = %#x, want StatusSame set (bits 16-23 masked out)", withPrefix)
	}
}
