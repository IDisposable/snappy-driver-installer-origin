package matcher

import "strings"

// SectionDecorationIndex finds which OSDecorations entry the ".nt..."
// suffix of a decorated section name corresponds to, ported from
// calc_secttype in matcher.cpp. name is expected to contain ".nt"
// somewhere (case-insensitive) - e.g. "root.ntamd64.10.0" - and only
// the Architecture/OSMajorVersion/OSMinorVersion/BuildNumber fields of
// the decoration are considered (ProductType and SuiteMask, if
// present, are ignored), matching the DDK's decorated-section-name
// grammar:
// NT[Architecture][.[OSMajorVersion][.[OSMinorVersion][.[ProductType][.[SuiteMask][.[BuildNumber]]]]]
// (see https://learn.microsoft.com/windows-hardware/drivers/install/inf-manufacturer-section).
// Returns -1 if name has no ".nt" or doesn't match any known
// decoration.
func SectionDecorationIndex(name string) int {
	lower := strings.ToLower(name)
	at := strings.Index(lower, ".nt")
	if at < 0 {
		return -1
	}
	s := name[at:]

	var sections [7]string
	rest := s
	idx := 0
	for idx < 7 {
		dot := strings.IndexByte(rest, '.')
		if dot < 0 {
			break
		}
		sections[idx] = rest[:dot]
		rest = rest[dot+1:]
		idx++
	}
	if idx < 7 {
		sections[idx] = rest
	}

	var normalized strings.Builder
	if sections[1] != "" {
		normalized.WriteByte('.')
		normalized.WriteString(sections[1])
	}
	if sections[2] != "" {
		normalized.WriteByte('.')
		normalized.WriteString(sections[2])
	}
	if sections[3] != "" {
		normalized.WriteByte('.')
		normalized.WriteString(sections[3])
	}
	if sections[6] != "" {
		normalized.WriteString("...")
		normalized.WriteString(sections[6])
	}

	target := normalized.String()
	if len(target) < 3 {
		return -1
	}
	target = target[3:] // strip the leading ".nt", matching the original's buf+3

	for i, dec := range OSDecorations {
		if len(dec) < 2 {
			continue
		}
		if strings.EqualFold(target, dec[2:]) { // strip "nt", matching the original's nts[i]+2
			return i
		}
	}
	return -1
}

// DecorationScore computes the score a decorated section contributes
// toward a driver's match ranking against the running system, ported
// from Hwidmatch::calc_decorscore. id is an index into OSDecorations
// (e.g. from SectionDecorationIndex); id<0 (no decoration to check)
// always scores 1, meaning "not disqualified, no extra confidence
// either." major/minor/build describe the running Windows version
// (major*10+minor convention, matching hardware.WindowsVersionInfo).
// arch is 1=x86, 2=amd64, 3=ia64, 4=arm, 5=arm64 - one more than
// hardware's 0-based architecture index, matching the original's
// `arch=state->getArchitecture()+1`.
func DecorationScore(id, major, minor, build, arch int) int {
	if id < 0 {
		return 1
	}

	// Windows 11 counts as Windows 10 for this comparison - both
	// report major version 10 in the decoration scheme.
	if major == 11 {
		major = 10
	}

	if osDecorationVersion[id] != 0 && major*10+minor < osDecorationVersion[id] {
		return 0
	}
	if osDecorationArch[id] != 0 && arch != osDecorationArch[id] {
		return 0
	}

	// Starting with Windows 10 build 14310, an exact major.minor match
	// additionally requires the running build to be at least the
	// decoration's minimum build number.
	if osDecorationVersion[id] >= 100 && osDecorationBuild[id] >= 14310 {
		if major*10+minor == osDecorationVersion[id] && osDecorationBuild[id] > build {
			return 0
		}
	}

	return osDecorationScore[id]
}

// MarkerScore scores a driver-pack file path against a small set of
// "marker" substrings that imply an OS version and architecture
// (vendor-specific naming conventions, e.g. "78x86" meaning "Windows 7
// or 8, x86"), ported from Hwidmatch::calc_markerscore. major/minor
// describe the running Windows version. arch is 0-based (0=x86,
// 1=amd64, 2=ia64, 3=arm, 4=arm64) - unlike DecorationScore's 1-based
// convention, matching the original's differing conventions between
// the two functions exactly (calc_markerscore uses
// state->getArchitecture() directly, calc_decorscore adds 1).
//
// Returned bits: 1 = at least one marker matched, 2 = the OS version
// allows (matches or exceeds a matched marker, or no version marker
// matched at all), 4 = the architecture allows (matches, or no
// architecture marker matched at all), 8 = the OS version matches
// exactly, 16 = the architecture matches exactly.
func MarkerScore(path string, major, minor, arch int) int {
	lower := strings.ToLower(path)

	score := 0
	curMaj, curMin, curArch := -1, -1, -1

	for _, m := range osMarkers {
		if !strings.Contains(lower, m.Name) {
			continue
		}
		score = 1
		if m.Major == 0 {
			continue
		}
		if m.Major > curMaj {
			curMaj = m.Major
		}
		if m.Minor > curMin {
			curMin = m.Minor
		}
		if m.Arch > curArch {
			curArch = m.Arch
		}
	}

	if curMaj >= 0 && curMin >= 0 && major == curMaj && minor == curMin {
		score |= 8
	}
	if curMaj >= 0 && curMin >= 0 && major >= curMaj && minor >= curMin {
		score |= 2
	}
	if curMaj < 0 && score != 0 {
		score |= 2
	}
	if curArch >= 0 && curArch == arch {
		score |= 4 | 16
	}
	if curArch < 0 && score != 0 {
		score |= 4
	}
	return score
}

// NotebookOEMMarker identifies the OEM brand implied by a system's
// motherboard manufacturer string (e.g. "Dell Inc." -> "Dell_nb"),
// ported from State::genmarker. Returns "OEM_nb" if manuf doesn't
// match any known brand, or manuf is empty. If manuf matches more than
// one brand's filters, the last-matching brand (in oemFilters' order)
// wins, matching the original's unconditional overwrite in its loop
// (it never breaks early).
func NotebookOEMMarker(manuf string) string {
	marker := "OEM_nb"
	if manuf == "" {
		return marker
	}
	lower := strings.ToLower(manuf)

	for _, group := range oemFilters {
		for _, alt := range group[1:] {
			if strings.Contains(lower, strings.ToLower(alt)) {
				marker = group[0] + "_nb"
				break
			}
		}
	}
	return marker
}
