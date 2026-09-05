package indexing

import "strings"

// ResolvedDevice is one fully-resolved device entry from a
// manufacturer's model section: a description, the install section
// SDIO picked (or its best guess), a "featurescore" flag, and the
// hardware IDs it matches. Ported from the per-item body of the "Find
// [manufacturer]" pass in Driverpack::indexinf_ansi - the part
// ParseManufacturers itself stops short of (see its doc comment).
type ResolvedDevice struct {
	Description   string
	Install       string // the raw, unresolved install-section field as written on the device line (e.g. "DtPort.NT")
	InstallPicked string // the resolved section name, "$name1,name2,..." if resolved via the decoration-suffix fallback, or "{missing}"
	Feature       int    // 0xFF if no "featurescore" key was found in the resolved section
	HWIDs         []string
}

// defaultFeature is the "no featurescore key found" sentinel, ported
// from Driverpack::indexinf_ansi's `int feature_c=0xFF;`.
const defaultFeature = 0xFF

// sectionResolution is the resolved install section for one device
// line: what to display/persist (DisplayName) and which section body
// (if any) to search for a "featurescore" key.
type sectionResolution struct {
	DisplayName string
	Matched     []SectionRange
}

// resolveInstallSection finds the .inf section that documents how to
// install a device whose [Manufacturer]-section line names rawInstall.
//
// The chain, tried in order, stopping at the first match:
//  1. "<rawInstall>.nt"
//  2. "<rawInstall>" bare
//  3. "<rawInstall>.<lastDecoration>" (the manufacturer's own
//     last-declared decoration, e.g. "ntamd64"), then repeatedly
//     dropping the trailing character and retrying until the
//     remaining string is no longer than rawInstall itself. This is a
//     crude character-truncation search, not a clean tokenized one -
//     ported as-is from the original rather than "cleaned up", since a
//     different (even if more sensible-looking) fallback would resolve
//     different real .inf files differently.
//  4. Every suffix in osDecorationSuffixes (matcher.cpp's nts[] table),
//     tried exhaustively rather than stopping at the first match: every
//     match found is accumulated into the display name ("$a,b,c,"), but
//     only the LAST match's section body is kept for the "featurescore"
//     lookup - this asymmetry is a quirk of the original, preserved for
//     fidelity rather than "fixed."
//
// If nothing matches, DisplayName is "{missing}" and Matched is nil.
func resolveInstallSection(sections InfSections, rawInstall, lastDecoration string, osDecorationSuffixes []string) sectionResolution {
	root := strings.ToLower(rawInstall)

	if r, ok := sections[root+".nt"]; ok {
		return sectionResolution{DisplayName: root + ".nt", Matched: r}
	}
	if r, ok := sections[root]; ok {
		return sectionResolution{DisplayName: root, Matched: r}
	}

	candidate := root
	if lastDecoration != "" {
		candidate = root + "." + lastDecoration
	}
	for len(candidate) >= len(root) {
		if r, ok := sections[candidate]; ok {
			return sectionResolution{DisplayName: candidate, Matched: r}
		}
		candidate = candidate[:len(candidate)-1]
	}

	var accumulated strings.Builder
	var lastMatch []SectionRange
	for _, suffix := range osDecorationSuffixes {
		name := root + "." + suffix
		if r, ok := sections[name]; ok {
			accumulated.WriteString(name)
			accumulated.WriteByte(',')
			lastMatch = r
		}
	}
	if accumulated.Len() > 0 {
		return sectionResolution{DisplayName: "$" + accumulated.String(), Matched: lastMatch}
	}

	return sectionResolution{DisplayName: "{missing}"}
}

// findFeatureScore searches resolution's matched section(s) for a
// "featurescore" key. callerSection is the manufacturer model section
// currently being walked (e.g. "root.ntamd64"); if resolution resolved
// right back to that same section, the lookup is skipped rather than
// re-parsing the section ResolveManufacturerSection is already
// mid-walk of.
func findFeatureScore(data []byte, resolution sectionResolution, callerSection string, stringList map[string]string) int {
	if resolution.DisplayName == callerSection {
		return defaultFeature
	}

	feature := defaultFeature
	for _, r := range resolution.Matched {
		p := NewInfParser(data, r.Begin, r.End, stringList)
		for {
			key, ok := p.ParseItem()
			if !ok {
				break
			}
			if strings.ToLower(key) == "featurescore" {
				if val, ok := p.ParseField(); ok {
					feature, _ = ParseHexByte(val)
				}
			}
			for {
				if _, ok := p.ParseField(); !ok {
					break
				}
			}
		}
	}
	return feature
}

// ResolveManufacturerSection walks one of a manufacturer's candidate
// model sections (sectionName - one of ManufacturerEntry.Sections) and
// resolves each device line's install section and hardware IDs.
// lastDecoration should be the decoration text with the section-name
// root stripped (e.g. "ntamd64" for candidate "root.ntamd64", "" for
// the bare "root" candidate) - it feeds resolveInstallSection's third
// fallback tier. osDecorationSuffixes should be
// internal/matcher.OSDecorations[:].
func ResolveManufacturerSection(data []byte, sections InfSections, sectionName, lastDecoration string, stringList map[string]string, osDecorationSuffixes []string) []ResolvedDevice {
	var devices []ResolvedDevice

	for _, r := range sections[sectionName] {
		p := NewInfParser(data, r.Begin, r.End, stringList)
		for {
			desc, ok := p.ParseItem()
			if !ok {
				break
			}
			rawInstall, ok := p.ParseField()
			if !ok {
				continue
			}

			resolution := resolveInstallSection(sections, rawInstall, lastDecoration, osDecorationSuffixes)
			feature := defaultFeature
			if resolution.Matched != nil {
				feature = findFeatureScore(data, resolution, sectionName, stringList)
			}

			var hwids []string
			for {
				hwid, ok := p.ParseField()
				if !ok {
					break
				}
				if hwid == "" {
					continue
				}
				hwids = append(hwids, strings.ToUpper(hwid))
			}

			devices = append(devices, ResolvedDevice{
				Description:   desc,
				Install:       rawInstall,
				InstallPicked: resolution.DisplayName,
				Feature:       feature,
				HWIDs:         hwids,
			})
		}
	}
	return devices
}
