package matcher

import "testing"

func TestSectionDecorationIndexBareNT(t *testing.T) {
	i := SectionDecorationIndex("root.nt")
	if i < 0 {
		t.Fatal("expected a match for bare '.nt'")
	}
	if OSDecorations[i] != "nt" {
		t.Errorf("matched %q, want %q", OSDecorations[i], "nt")
	}
}

func TestSectionDecorationIndexArchAndVersion(t *testing.T) {
	i := SectionDecorationIndex("Standard.NTamd64.10.0")
	if i < 0 {
		t.Fatal("expected a match")
	}
	if OSDecorations[i] != "ntamd64.10.0" {
		t.Errorf("matched %q, want %q", OSDecorations[i], "ntamd64.10.0")
	}
}

func TestSectionDecorationIndexWithBuildNumber(t *testing.T) {
	// Real .inf files (and OSDecorations itself) use the "..." skip
	// syntax to omit ProductType/SuiteMask when only Build matters -
	// this is the realistic case, unlike spelling all 7 dot-separated
	// fields out explicitly (see
	// TestSectionDecorationIndexExplicitFieldsExhaustsBudget below for
	// why that form is a genuine, faithfully-preserved limitation).
	i := SectionDecorationIndex("root.NTamd64.10.0...22000")
	if i < 0 {
		t.Fatal("expected a match")
	}
	if OSDecorations[i] != "ntamd64.10.0...22000" {
		t.Errorf("matched %q, want %q", OSDecorations[i], "ntamd64.10.0...22000")
	}
}

// TestSectionDecorationIndexExplicitFieldsExhaustsBudget documents a
// genuine limitation of the original algorithm, preserved as-is: the
// dot-splitting loop only fills sections[0..6] (7 slots), and each
// empty field (from repeated dots) still consumes one slot. A fully
// explicit "Arch.Major.Minor.ProductType.SuiteMask...Build" decoration
// (7 logical fields, 2 of them further splitting the "..." into two
// empty fields) exhausts the budget before reaching Build, so Build
// silently reads back as empty. Real .inf files avoid this by using
// the "..." skip syntax when they only care about Build (see the test
// above) - this case is not known to occur in practice, but the
// behavior must match the original either way.
func TestSectionDecorationIndexExplicitFieldsExhaustsBudget(t *testing.T) {
	i := SectionDecorationIndex("root.NTamd64.10.0.1.512...22000")
	if i < 0 {
		t.Fatal("expected a match (falls back to the no-build variant)")
	}
	if OSDecorations[i] != "ntamd64.10.0" {
		t.Errorf("matched %q, want %q (Build field lost, matching the original's behavior)", OSDecorations[i], "ntamd64.10.0")
	}
}

func TestSectionDecorationIndexNoMatch(t *testing.T) {
	if i := SectionDecorationIndex("root.nowhere"); i != -1 {
		t.Errorf("SectionDecorationIndex(no .nt) = %d, want -1", i)
	}
	if i := SectionDecorationIndex("root.nt.99.99"); i != -1 {
		t.Errorf("SectionDecorationIndex(unknown version) = %d, want -1", i)
	}
}

func TestDecorationScoreNoDecoration(t *testing.T) {
	if got := DecorationScore(-1, 10, 0, 26100, 2); got != 1 {
		t.Errorf("DecorationScore(-1, ...) = %d, want 1", got)
	}
}

func TestDecorationScoreArchMismatch(t *testing.T) {
	i := SectionDecorationIndex("root.ntx86.10.0")
	if i < 0 {
		t.Fatal("setup: expected a match for ntx86.10.0")
	}
	// arch=2 means amd64; the decoration wants x86 (arch 1).
	if got := DecorationScore(i, 10, 0, 26100, 2); got != 0 {
		t.Errorf("DecorationScore with mismatched arch = %d, want 0", got)
	}
}

func TestDecorationScoreVersionTooOld(t *testing.T) {
	i := SectionDecorationIndex("root.ntamd64.6.1") // Windows 7
	if i < 0 {
		t.Fatal("setup: expected a match for ntamd64.6.1")
	}
	// Running Windows XP (5.1): too old for a Windows-7-or-later decoration.
	if got := DecorationScore(i, 5, 1, 0, 2); got != 0 {
		t.Errorf("DecorationScore on an OS too old = %d, want 0", got)
	}
}

func TestDecorationScoreBuildTooLow(t *testing.T) {
	i := SectionDecorationIndex("root.ntamd64.10.0...22000") // Win 11 21H2
	if i < 0 {
		t.Fatal("setup: expected a match for the Windows 11 21H2 decoration")
	}
	// Same major.minor (10.0, since Win11 reports as 10 for this
	// comparison) but a build number below the decoration's minimum.
	if got := DecorationScore(i, 11, 0, 19045, 2); got != 0 {
		t.Errorf("DecorationScore with too-low a build = %d, want 0", got)
	}
	if got := DecorationScore(i, 11, 0, 22000, 2); got == 0 {
		t.Errorf("DecorationScore with a sufficient build = %d, want nonzero", got)
	}
}

func TestDecorationScoreArchSpecificBeatsGeneric(t *testing.T) {
	genericIdx := SectionDecorationIndex("root.nt.10.0")
	specificIdx := SectionDecorationIndex("root.ntamd64.10.0")
	generic := DecorationScore(genericIdx, 10, 0, 19045, 2)
	specific := DecorationScore(specificIdx, 10, 0, 19045, 2)
	if specific <= generic {
		t.Errorf("arch-specific score (%d) should exceed generic score (%d)", specific, generic)
	}
}

func TestMarkerScoreNoMatch(t *testing.T) {
	if got := MarkerScore("some/random/path", 10, 0, 1); got != 0 {
		t.Errorf("MarkerScore with no markers present = %d, want 0", got)
	}
}

func TestMarkerScoreExactVersionAndArch(t *testing.T) {
	// "6x64" implies major=6,minor=0,arch=1 (amd64) per the markers
	// table, and (unlike e.g. "10x64") no other marker name is a
	// substring of it, so this exercises a single, unambiguous match -
	// MarkerScore takes the max of major/minor/arch independently
	// across *every* matching marker, so overlapping matches would
	// otherwise combine into a value that doesn't correspond to any
	// single table entry.
	got := MarkerScore("driver/6x64/setup.inf", 6, 0, 1)
	if got&1 == 0 {
		t.Error("expected bit 1 (marker matched)")
	}
	if got&8 == 0 {
		t.Error("expected bit 8 (exact version match)")
	}
	if got&16 == 0 {
		t.Error("expected bit 16 (exact arch match)")
	}
}

func TestMarkerScoreArchOnly(t *testing.T) {
	// "ntx64" implies arch=1 only (major=minor=-1, "any version").
	got := MarkerScore("driver/ntx64/setup.inf", 10, 0, 1)
	if got&1 == 0 {
		t.Error("expected bit 1 (marker matched)")
	}
	if got&2 == 0 {
		t.Error("expected bit 2 (no version constraint => version allowed)")
	}
	if got&16 == 0 {
		t.Error("expected bit 16 (exact arch match)")
	}
}

func TestNotebookOEMMarkerDefault(t *testing.T) {
	if got := NotebookOEMMarker(""); got != "OEM_nb" {
		t.Errorf("NotebookOEMMarker(\"\") = %q, want %q", got, "OEM_nb")
	}
	if got := NotebookOEMMarker("Framework"); got != "OEM_nb" {
		t.Errorf("NotebookOEMMarker(%q) = %q, want %q (no filter matches an unlisted OEM)", "Framework", got, "OEM_nb")
	}
}

func TestNotebookOEMMarkerCaseInsensitive(t *testing.T) {
	// The original uses StrStrIW (case-insensitive); "packard" only
	// appears lowercase in the filter table, so this specifically
	// exercises that the Go port doesn't regress to case-sensitive
	// matching. (Deliberately no "NEC" in this string - oemFilters'
	// last-match-wins behavior, exercised separately below, would
	// otherwise make the NEC group win instead.)
	if got := NotebookOEMMarker("Packard Bell"); got != "Acer_nb" {
		t.Errorf("NotebookOEMMarker(%q) = %q, want %q", "Packard Bell", got, "Acer_nb")
	}
}

func TestNotebookOEMMarkerLastMatchWins(t *testing.T) {
	// "Packard"/"Bell" match the Acer group; "NEC" matches the NEC
	// group, which comes later in oemFilters - the original's loop
	// never breaks early, so whichever group matches *last* wins, not
	// the most specific or first match.
	if got := NotebookOEMMarker("Packard Bell NEC"); got != "NEC_nb" {
		t.Errorf("NotebookOEMMarker(%q) = %q, want %q (last-matching group)", "Packard Bell NEC", got, "NEC_nb")
	}
}

func TestNotebookOEMMarkerKnownBrand(t *testing.T) {
	if got := NotebookOEMMarker("Dell Inc."); got != "Dell_nb" {
		t.Errorf("NotebookOEMMarker(%q) = %q, want %q", "Dell Inc.", got, "Dell_nb")
	}
}

// TestSectionDecorationIndexRealDriverPackSection resolves the actual
// section name from a real driver pack's .inf file (dtport.inf's
// "[DtHw.NTamd64]", see internal/indexing's realdriverpack_test.go)
// against OSDecorations, tying this scoring logic back to production
// data one more time. "NTamd64" alone (no version numbers) should
// resolve to OSDecorations' bare architecture-only entry.
func TestSectionDecorationIndexRealDriverPackSection(t *testing.T) {
	i := SectionDecorationIndex("DtHw.NTamd64")
	if i < 0 {
		t.Fatal("expected a match for a bare architecture decoration")
	}
	if OSDecorations[i] != "ntamd64" {
		t.Errorf("matched %q, want %q", OSDecorations[i], "ntamd64")
	}
	// A bare architecture decoration should score with the
	// architecture-specific bonus but no version constraint.
	if got := DecorationScore(i, 11, 0, 26100, 2); got != osDecorationScore[i] {
		t.Errorf("DecorationScore = %d, want %d", got, osDecorationScore[i])
	}
}
