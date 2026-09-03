package indexing

import (
	"os"
	"testing"
)

func TestResolveInstallSectionPhaseA_DotNT(t *testing.T) {
	data := []byte("[root.nt]\nfoo=1\n")
	sections, _ := DiscoverSections(data)
	res := resolveInstallSection(sections, "Root", "", nil)
	if res.DisplayName != "root.nt" {
		t.Errorf("DisplayName = %q, want %q", res.DisplayName, "root.nt")
	}
	if len(res.Matched) == 0 {
		t.Error("expected a matched section range")
	}
}

func TestResolveInstallSectionPhaseB_Bare(t *testing.T) {
	data := []byte("[root]\nfoo=1\n")
	sections, _ := DiscoverSections(data)
	res := resolveInstallSection(sections, "Root", "", nil)
	if res.DisplayName != "root" {
		t.Errorf("DisplayName = %q, want %q", res.DisplayName, "root")
	}
}

func TestResolveInstallSectionPhaseC_LastDecoration(t *testing.T) {
	data := []byte("[root.ntamd64]\nfoo=1\n")
	sections, _ := DiscoverSections(data)
	res := resolveInstallSection(sections, "Root", "ntamd64", nil)
	if res.DisplayName != "root.ntamd64" {
		t.Errorf("DisplayName = %q, want %q", res.DisplayName, "root.ntamd64")
	}
}

func TestResolveInstallSectionPhaseC_CharacterTruncation(t *testing.T) {
	// "root.ntamd64" doesn't exist, but "root.ntamd6" does (a
	// deliberately odd fixture exercising the character-by-character
	// truncation fallback, not a realistic .inf).
	data := []byte("[root.ntamd6]\nfoo=1\n")
	sections, _ := DiscoverSections(data)
	res := resolveInstallSection(sections, "Root", "ntamd64", nil)
	if res.DisplayName != "root.ntamd6" {
		t.Errorf("DisplayName = %q, want %q (truncated match)", res.DisplayName, "root.ntamd6")
	}
}

func TestResolveInstallSectionPhaseD_DecorationFallback(t *testing.T) {
	data := []byte("[root.ntamd64.10.0]\nfirst=1\n[root.ntx86.6.1]\nsecond=1\n")
	sections, _ := DiscoverSections(data)
	// No ".nt", no bare "root", no lastDecoration match: falls through
	// to trying every osDecorationSuffix, in *suffixes* order (not file
	// declaration order) - "ntx86.6.1" is checked, and so accumulated
	// and become the kept Matched range, after "ntamd64.10.0", even
	// though "ntamd64.10.0" is declared first in the file.
	suffixes := []string{"ntamd64.10.0", "ntx86.6.1", "ntia64.5.1"}
	res := resolveInstallSection(sections, "Root", "unrelated", suffixes)

	want := "$root.ntamd64.10.0,root.ntx86.6.1,"
	if res.DisplayName != want {
		t.Errorf("DisplayName = %q, want %q", res.DisplayName, want)
	}
	// Matched should be the LAST-in-suffixes-order accumulated match
	// (root.ntx86.6.1), per the original's quirk of not breaking early
	// and keeping whichever match it saw most recently.
	got := string(data[res.Matched[0].Begin:res.Matched[0].End])
	if got != "\nsecond=1\n" {
		t.Errorf("Matched section content = %q, want the ntx86.6.1 section's body", got)
	}
}

func TestResolveInstallSectionMissing(t *testing.T) {
	data := []byte("[unrelated]\nfoo=1\n")
	sections, _ := DiscoverSections(data)
	res := resolveInstallSection(sections, "Root", "", nil)
	if res.DisplayName != "{missing}" {
		t.Errorf("DisplayName = %q, want %q", res.DisplayName, "{missing}")
	}
	if res.Matched != nil {
		t.Error("expected no matched sections")
	}
}

func TestFindFeatureScore(t *testing.T) {
	data := []byte("[root.nt]\nFeatureScore=0x02\nfoo=1\n")
	sections, _ := DiscoverSections(data)
	res := resolveInstallSection(sections, "Root", "", nil)
	feature := findFeatureScore(data, res, "caller.section", nil)
	if feature != 2 {
		t.Errorf("feature = %d, want 2", feature)
	}
}

func TestFindFeatureScoreDefaultWhenAbsent(t *testing.T) {
	data := []byte("[root.nt]\nfoo=1\n")
	sections, _ := DiscoverSections(data)
	res := resolveInstallSection(sections, "Root", "", nil)
	feature := findFeatureScore(data, res, "caller.section", nil)
	if feature != defaultFeature {
		t.Errorf("feature = %d, want defaultFeature (%d)", feature, defaultFeature)
	}
}

func TestFindFeatureScoreSelfReferenceGuard(t *testing.T) {
	data := []byte("[loop.nt]\nFeatureScore=0xFF\n")
	sections, _ := DiscoverSections(data)
	res := resolveInstallSection(sections, "Loop", "", nil)
	// callerSection equals the resolved DisplayName: must not recurse
	// or read the featurescore from itself.
	feature := findFeatureScore(data, res, res.DisplayName, nil)
	if feature != defaultFeature {
		t.Errorf("feature = %d, want defaultFeature (self-reference guard)", feature)
	}
}

func TestResolveManufacturerSectionEndToEnd(t *testing.T) {
	data := []byte(
		"[Root.ntamd64]\n" +
			"Desc1=Install1,PCI\\VEN_1234&DEV_0001\n" +
			"Desc2=Install2,PCI\\VEN_1234&DEV_0002,PCI\\VEN_1234&DEV_0003\n" +
			"\n" +
			"[Install1.nt]\n" +
			"FeatureScore=0x01\n" +
			"\n" +
			"[Install2.nt]\n",
	)
	sections, _ := DiscoverSections(data)

	devices := ResolveManufacturerSection(data, sections, "root.ntamd64", "ntamd64", nil, nil)
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}

	if devices[0].Description != "Desc1" || devices[0].InstallPicked != "install1.nt" || devices[0].Feature != 1 {
		t.Errorf("device[0] = %+v", devices[0])
	}
	if len(devices[0].HWIDs) != 1 || devices[0].HWIDs[0] != `PCI\VEN_1234&DEV_0001` {
		t.Errorf("device[0].HWIDs = %v", devices[0].HWIDs)
	}

	if devices[1].Description != "Desc2" || devices[1].InstallPicked != "install2.nt" || devices[1].Feature != defaultFeature {
		t.Errorf("device[1] = %+v", devices[1])
	}
	wantHWIDs := []string{`PCI\VEN_1234&DEV_0002`, `PCI\VEN_1234&DEV_0003`}
	if len(devices[1].HWIDs) != 2 || devices[1].HWIDs[0] != wantHWIDs[0] || devices[1].HWIDs[1] != wantHWIDs[1] {
		t.Errorf("device[1].HWIDs = %v, want %v", devices[1].HWIDs, wantHWIDs)
	}
}

// TestResolveRealWdmaUsbManufacturer resolves a real manufacturer
// section (Altec, from wdma_usb.inf) end-to-end and checks the results
// against the file's actual content (verified by manual inspection,
// see infsections_test.go's TestParseWholeSection for the same
// section's raw device lines).
func TestResolveRealWdmaUsbManufacturer(t *testing.T) {
	path := "/mnt/c/Windows/inf/wdma_usb.inf"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("real .inf file not available at %s: %v", path, err)
	}
	data := readRealInfAsASCII(t, path)

	sections, _ := DiscoverSections(data)
	strs := ParseStrings(data, sections)
	entries := ParseManufacturers(data, sections, strs)

	var altec *ManufacturerEntry
	for i := range entries {
		if entries[i].SectionRoot == "altec.section" {
			altec = &entries[i]
			break
		}
	}
	if altec == nil {
		t.Fatal(`expected a manufacturer entry with SectionRoot "altec.section"`)
	}

	// altec.Sections[0] is the bare root; altec.Sections[1] should be
	// "altec.section.ntamd64" per the file's "%Altec.Mfg%=Altec.Section,
	// ntamd64" line.
	if len(altec.Sections) < 2 {
		t.Fatalf("altec.Sections = %v, want at least 2 entries", altec.Sections)
	}
	sectionName := altec.Sections[1]
	if sectionName != "altec.section.ntamd64" {
		t.Fatalf("altec.Sections[1] = %q, want %q", sectionName, "altec.section.ntamd64")
	}

	devices := ResolveManufacturerSection(data, sections, sectionName, "ntamd64", strs, nil)
	if len(devices) != 6 {
		t.Fatalf("got %d devices, want 6 (matches TestParseWholeSection's fixture)", len(devices))
	}

	wantFirstHWID := `USB\VID_04D2&PID_FF05`
	if len(devices[0].HWIDs) != 1 || devices[0].HWIDs[0] != wantFirstHWID {
		t.Errorf("devices[0].HWIDs = %v, want [%q]", devices[0].HWIDs, wantFirstHWID)
	}
	// USBAudio.NonCompliantAltec has no [.nt]/[.ntamd64]-decorated
	// counterpart in this file's Altec section (verified by
	// inspection), so bare-name resolution (phase b) should apply.
	if devices[0].InstallPicked != "usbaudio.noncompliantaltec" {
		t.Errorf("devices[0].InstallPicked = %q", devices[0].InstallPicked)
	}
}
