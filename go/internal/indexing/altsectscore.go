package indexing

import (
	"strings"

	"sdio/internal/common"
	"sdio/internal/matcher"
)

// MatchContext bundles the running system's state needed by
// CalcAltSectScore, ported from the subset of State (enum.cpp/
// matcher.cpp) that function reads.
type MatchContext struct {
	Major, Minor, Build int
	IsAMD64             bool // state->getArchitecture()==1
	IsLaptop            bool
	NotebookMarker      string // see matcher.NotebookOEMMarker; "" if not a laptop or no marker
	FilterSP            bool   // Settings.flags&FLAG_FILTERSP
}

// ArchForDecoration returns the architecture in matcher.DecorationScore's
// 1-based convention (1=x86, 2=amd64).
func (ctx MatchContext) ArchForDecoration() int {
	if ctx.IsAMD64 {
		return 2
	}
	return 1
}

// ArchForMarker returns the architecture in matcher.MarkerScore's
// 0-based convention (0=x86, 1=amd64).
func (ctx MatchContext) ArchForMarker() int {
	if ctx.IsAMD64 {
		return 1
	}
	return 0
}

// packVersionNumber extracts a driver pack's numeric release suffix
// from its filename (e.g. "..._16074.7z" -> 16074: the first
// underscore followed by a digit, parsed with atoi's leading-digits
// convention), ported from the packname-scanning block in
// Hwidmatch::calc_altsectscore. Returns 0 if no such suffix is found,
// matching the original's v==0 default.
func packVersionNumber(filename string) int {
	for i := 0; i+1 < len(filename); i++ {
		if filename[i] == '_' && filename[i+1] >= '0' && filename[i+1] <= '9' {
			return atoiPrefix(filename[i+1:])
		}
	}
	return 0
}

// PickCat returns which catalog-file field slot (see the Field*
// constants) holds the OS-attribute string to validate this HWID
// entry's driver against, ported from Hwidmatch::pickcat. The
// fallback-to-FieldCatalogFile return when nothing is found matches
// the original's return value collision (pickcat returns 0 both for
// "found CatalogFile" and "found nothing"); harmless there because the
// caller only acts on whether the resulting field is empty, which
// IsValidCatForDriver also does.
func (d *Driverpack) PickCat(hwidIndex int, isAMD64 bool) int {
	if isAMD64 && d.Cat(hwidIndex, FieldCatalogFileNTAMD64) != "" {
		return FieldCatalogFileNTAMD64
	}
	if d.Cat(hwidIndex, FieldCatalogFileNTx86) != "" {
		return FieldCatalogFileNTx86
	}
	if d.Cat(hwidIndex, FieldCatalogFileNT) != "" {
		return FieldCatalogFileNT
	}
	return FieldCatalogFile
}

// IsValidCatForDriver reports whether this HWID entry's driver has a
// catalog file whose signed OS-attribute string covers the running
// Windows version, ported from Hwidmatch::isvalidcat (combined with
// pickcat).
func (d *Driverpack) IsValidCatForDriver(hwidIndex, major, minor int, isAMD64 bool) bool {
	n := d.PickCat(hwidIndex, isAMD64)
	return IsValidCat(d.Cat(hwidIndex, n), major, minor)
}

// CalcAltSectScore reports whether a candidate driver survives every
// OS-decoration and vendor-specific validity check, ported from
// Hwidmatch::calc_altsectscore. curScore is the candidate's own
// DecorScore (matcher.DecorationScore) computed for the section it was
// actually found under; if any of the SAME manufacturer's other
// declared section variants would score higher against the running
// system, this candidate loses to that variant (returns 0).
// deviceHardwareID is the plug-and-play hardware ID of the device
// being matched (needed by the USB3 hub and Realtek blacklist checks).
// The return value is not a boolean: 0 means rejected, 1 means valid,
// 2 means valid with a confirmed catalog-file signature (or
// FilterSP-restricted matching, which trusts catalog-signed packs
// implicitly) - callers compare it numerically, matching the
// original's use as a tri-state score component.
func (d *Driverpack) CalcAltSectScore(hwidIndex, curScore int, ctx MatchContext, deviceHardwareID string) int {
	e := d.resolve(hwidIndex)
	manuf := d.Index.Manufacturers[e.manufIndex]
	for pos := 0; pos < int(manuf.SectionsN); pos++ {
		sect := d.SectionAtPos(int(e.manufIndex), pos)
		id := matcher.SectionDecorationIndex(sect)
		if matcher.DecorationScore(id, ctx.Major, ctx.Minor, ctx.Build, ctx.ArchForDecoration()) > curScore {
			return 0
		}
	}

	if !matcher.CalcNotebookValid(d.InfPath(hwidIndex), ctx.IsLaptop, ctx.NotebookMarker) {
		return 0
	}

	infPath := d.InfPath(hwidIndex)
	lowerPath := strings.ToLower(infPath)

	intel2, intel4 := `intel_2nd\`, `intel_4th\`
	if matcher.IntelPathUsesSDIPrefix(packVersionNumber(d.Filename)) {
		intel2, intel4 = `intel_sdi_2nd\`, `intel_sdi_4th\`
	}
	if strings.Contains(lowerPath, intel2) && !matcher.IsValidUSB3Hub(deviceHardwareID, matcher.IntelUSB3Gen2HubIDs) {
		return 0
	}
	if strings.Contains(lowerPath, intel4) && !matcher.IsValidUSB3Hub(deviceHardwareID, matcher.IntelUSB3Gen4HubIDs) {
		return 0
	}

	if strings.Contains(lowerPath, `matchver\`) || strings.Contains(lowerPath, `l\realtek\`) || strings.Contains(lowerPath, `l\r\`) {
		if !matcher.IsValidVer(d.Version(hwidIndex).V1, ctx.Major, ctx.Minor) {
			return 0
		}
	}

	if matcher.IsBlacklisted(deviceHardwareID, d.Section(hwidIndex), matcher.RealtekBlacklistHWID, matcher.RealtekBlacklistSection) {
		return 0
	}

	if strings.Contains(lowerPath, `matchmarker\`) {
		if matcher.MarkerScore(infPath, ctx.Major, ctx.Minor, ctx.ArchForMarker())&7 != 7 {
			return 0
		}
	}

	// The original returns 2 unconditionally here (Hwidmatch::
	// calc_altsectscore's `if(Settings.flags&FLAG_FILTERSP)return 2;`),
	// then separately, in Manager::filter (a display-layer pass that
	// can re-run without recomputing scores), downgrades any resulting
	// altsectscore==2 to 1 if isvalidcat actually fails. This rewrite
	// has no equivalent separate re-filter pass, so the two steps are
	// folded into one here - same net result (2 only when the catalog
	// genuinely validates, 1 otherwise), computed once.
	if ctx.FilterSP {
		if d.IsValidCatForDriver(hwidIndex, ctx.Major, ctx.Minor, ctx.IsAMD64) {
			return 2
		}
		return 1
	}

	if strings.Contains(lowerPath, "tweak") || strings.Contains(strings.ToLower(d.InfName(hwidIndex)), "tweak") {
		return 1
	}

	if d.IsValidCatForDriver(hwidIndex, ctx.Major, ctx.Minor, ctx.IsAMD64) {
		return 2
	}
	return 1
}

// CalcStatus combines a candidate driver's comparison against the
// currently installed driver into a status bitmask (see the Status*
// constants in internal/matcher), ported from the
// installed-driver-exists branch of Hwidmatch::calc_status.
// devicematch->isMissing and the STATUS_MISSING short-circuit aren't
// covered here - they need the not-yet-ported Devicematch/Driver
// types; callers should check that first and only call CalcStatus once
// they know whether an installed driver exists to compare against.
// installedVersion/installedScore are the installed driver's date and
// calc_score_h result; candidateVersion/candidateScore are this
// entry's. infPathHasFeaturePrefix is whether this candidate's .inf
// path contains "feature_", which narrows the score comparison to
// ignore bits 16-23 (0xFF00FFFF) of the packed score, where the
// feature-number component lives.
func CalcStatus(hasInstalledDriver bool, installedVersion, candidateVersion common.Version, installedScore, candidateScore uint32, infPathHasFeaturePrefix bool, altSectScore int) int {
	r := 0
	if hasInstalledDriver {
		switch d := common.CompareDate(installedVersion, candidateVersion); {
		case d < 0:
			r += matcher.StatusNew
		case d > 0:
			r += matcher.StatusOld
		default:
			r += matcher.StatusCurrent
		}

		res := matcher.CmpUnsigned(installedScore, candidateScore)
		if r&matcher.StatusCurrent != 0 && infPathHasFeaturePrefix {
			res = matcher.CmpUnsigned(installedScore&0xFF00FFFF, candidateScore&0xFF00FFFF)
		}
		switch {
		case res > 0:
			r += matcher.StatusBetter
		case res < 0:
			r += matcher.StatusWorse
		default:
			r += matcher.StatusSame
		}
	} else {
		r += matcher.StatusBetter
	}

	if altSectScore == 0 {
		r += matcher.StatusInvalid
	}
	return r
}
