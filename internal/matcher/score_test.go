package matcher

import "testing"

func TestIdentifierScoreOrdering(t *testing.T) {
	// Exact hardware-ID match at position 0 must beat everything else.
	hwExact := IdentifierScore(0, true, 0)
	hwCompat := IdentifierScore(0, true, 1)
	compatHW := IdentifierScore(0, false, 0)
	compatCompat := IdentifierScore(0, false, 1)

	if !(hwExact < hwCompat && hwCompat < compatHW && compatHW < compatCompat) {
		t.Errorf("expected strict ordering hwExact < hwCompat < compatHW < compatCompat, got %d, %d, %d, %d",
			hwExact, hwCompat, compatHW, compatCompat)
	}
}

func TestIdentifierScoreKnownValues(t *testing.T) {
	cases := []struct {
		devPos             int
		deviceIsHardwareID bool
		infPos             int
		want               int
	}{
		{5, true, 0, 5},
		{5, true, 1, 0x1000 + 5 + 0x100},
		{5, false, 0, 0x2000 + 5},
		{5, false, 2, 0x3000 + 5 + 0x200},
	}
	for _, c := range cases {
		if got := IdentifierScore(c.devPos, c.deviceIsHardwareID, c.infPos); got != c.want {
			t.Errorf("IdentifierScore(%d, %v, %d) = %#x, want %#x", c.devPos, c.deviceIsHardwareID, c.infPos, got, c.want)
		}
	}
}

func TestSignatureScore64BitWithGenericCatalog(t *testing.T) {
	// A driver with a plain/nt/ntamd64/ntia64 catalog reference on a
	// 64-bit system needs no extra signature bonus.
	if got := SignatureScore(CatalogFileNTAMD64Bit, true, false); got != 0 {
		t.Errorf("SignatureScore = %#x, want 0", got)
	}
}

func TestSignatureScore64BitWithOnlyX86Catalog(t *testing.T) {
	// Only an x86-specific catalog on a 64-bit system: architecture
	// mismatch, falls through to the isNTSection bonus.
	got := SignatureScore(CatalogFileNTx86Bit, true, false)
	if got != 0xC000 {
		t.Errorf("SignatureScore = %#x, want 0xC000", got)
	}
	got = SignatureScore(CatalogFileNTx86Bit, true, true)
	if got != 0x8000 {
		t.Errorf("SignatureScore (NT section) = %#x, want 0x8000", got)
	}
}

func TestSignatureScoreNoCatalogAtAll(t *testing.T) {
	if got := SignatureScore(0, false, false); got != 0xC000 {
		t.Errorf("SignatureScore(no catalog) = %#x, want 0xC000", got)
	}
}

func TestScoreModernWindowsFoldsInFeatureAndSignature(t *testing.T) {
	// major=10 (Windows 10/11) uses the richer (sig<<16)+(feature<<16)+id layout.
	got := Score(CatalogFileNTAMD64Bit, 0x02, 5, 10, true, false)
	want := uint32(0<<16) + uint32(0x02<<16) + 5 // signature contributes 0 (generic catalog present)
	if got != want {
		t.Errorf("Score = %#x, want %#x", got, want)
	}
}

func TestScoreLegacyWindowsIgnoresFeature(t *testing.T) {
	// major<6 (pre-Vista) uses the simpler sig+id layout; feature is ignored.
	got := Score(0, 0xFF, 5, 5, false, false)
	want := uint32(0xC000 + 5)
	if got != want {
		t.Errorf("Score (legacy) = %#x, want %#x", got, want)
	}
}
