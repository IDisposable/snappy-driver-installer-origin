package indexing

import "strings"

// InstalledInfInfo is what Driver::scaninf recovers about the .inf
// file that produced a currently-installed driver: everything
// matcher.Score needs to score that driver the same way a driver-pack
// candidate is scored, so it can be compared fairly against one (see
// indexing.CalcStatus).
type InstalledInfInfo struct {
	CatalogFileBits int
	Feature         int
	InfPos          int  // position of matchingDeviceID within its device line's own HWID list
	IsNTSection     bool // whether sect (InfSection+InfSectionExt) is ".nt"-decorated
	Found           bool // false if matchingDeviceID/sect weren't found anywhere in the .inf
}

// ScanInstalledInf parses an installed driver's own .inf content and
// recovers the feature score, catalog-file field bitmask, and
// HWID-list position for the specific (section, hardware ID) pair the
// driver was installed under, ported from Driver::scaninf combined
// with Driverpack::fillinfo. data is the raw .inf file content (read
// from %windir%\inf\<InfPath>); sect is InfSection+InfSectionExt from
// the installed driver's registry data (hardware.InstalledDriver);
// matchingDeviceID is the specific hardware ID Windows used to select
// this driver (hardware.InstalledDriver.MatchingDeviceID).
// osDecorationSuffixes should be matcher.OSDecorations[:].
//
// Unlike the original - which caches parsed .inf files across many
// device scans in a shared Driverpack instance keyed by file path, and
// searches only the HWID_list entries a given .inf contributed via a
// start_index offset - this parses fresh on every call and searches
// every manufacturer/section/device it finds. Functionally identical;
// callers scanning many devices that share one underlying .inf file
// (common - one oem*.inf often covers several devices) should cache
// the result by .inf path themselves if this shows up as a hot path.
func ScanInstalledInf(data []byte, sect, matchingDeviceID string, osDecorationSuffixes []string) InstalledInfInfo {
	info := InstalledInfInfo{Feature: defaultFeature, IsNTSection: strings.Contains(strings.ToLower(sect), ".nt")}

	sections, _ := DiscoverSections(data)
	strs := ParseStrings(data, sections)
	verInfo := ParseVersionSection(data, sections, strs)

	for i := FieldCatalogFile; i <= FieldCatalogFileNTAMD64; i++ {
		if verInfo.Fields[i] != "" {
			info.CatalogFileBits |= 1 << i
		}
	}

	bestInfPos := -1
	for _, me := range ParseManufacturers(data, sections, strs) {
		for _, secName := range me.Sections {
			lastDecoration := strings.TrimPrefix(strings.TrimPrefix(secName, me.SectionRoot), ".")
			for _, dev := range ResolveManufacturerSection(data, sections, secName, lastDecoration, strs, osDecorationSuffixes) {
				if !strings.EqualFold(dev.InstallPicked, sect) && !strings.Contains(strings.ToLower(dev.Install), strings.ToLower(sect)) {
					continue
				}
				for pos, hwid := range dev.HWIDs {
					if strings.EqualFold(hwid, matchingDeviceID) && (bestInfPos < 0 || pos < bestInfPos) {
						bestInfPos = pos
						info.Feature = dev.Feature
					}
				}
			}
		}
	}

	if bestInfPos < 0 {
		info.Feature = defaultFeature
		return info
	}
	info.InfPos = bestInfPos
	info.Found = true
	return info
}
