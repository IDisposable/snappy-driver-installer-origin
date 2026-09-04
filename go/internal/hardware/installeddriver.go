package hardware

import (
	"strings"

	"sdio/internal/common"
)

// InstalledDriver describes a currently-installed driver, read from
// its registry key under
// SYSTEM\CurrentControlSet\Control\Class\<ClassGUID>\<Index>. Ported
// from Driver in enum.cpp - the parts independent of
// Driverpack::scaninf's .inf-cache lookup (Feature, CatalogFileBits,
// Cat, and a corrected InfPos), which isn't ported yet: it needs the
// Driverpack/inf-cache orchestration from indexing.cpp's genindex,
// which this rewrite hasn't built (see go/README.md).
// Without it, DevPos reflects only the device-ID match
// (Driver::calc_dev_pos), not scaninf's refinement of InfPos.
type InstalledDriver struct {
	Desc             string
	ProviderName     string
	MatchingDeviceID string
	InfPath          string
	InfSection       string
	InfSectionExt    string
	Version          common.Version // from the DriverDate + DriverVersion registry strings

	// DevPos/IsHardwareID describe how MatchingDeviceID matched the
	// device's own hardware/compatible ID list, ported from
	// Driver::calc_dev_pos. DevPos is -1 if no match was found.
	DevPos       int
	IsHardwareID bool
}

// MatchDeviceID finds matchingDeviceID within hardwareIDs first,
// falling back to compatibleIDs only if hardwareIDs has no match and
// compatibleIDs is non-empty, ported from Driver::calc_dev_pos and
// Driver::findHWID_in_list. Returns the matched position and whether
// it came from hardwareIDs (true) or compatibleIDs (false); pos is -1
// if matchingDeviceID appears in neither list actually searched.
func MatchDeviceID(hardwareIDs, compatibleIDs []string, matchingDeviceID string) (pos int, isHardwareID bool) {
	isHardwareID = true
	pos = indexOfFold(hardwareIDs, matchingDeviceID)

	if pos < 0 && len(compatibleIDs) > 0 {
		isHardwareID = false
		pos = indexOfFold(compatibleIDs, matchingDeviceID)
	}
	return pos, isHardwareID
}

func indexOfFold(list []string, s string) int {
	for i, v := range list {
		if strings.EqualFold(v, s) {
			return i
		}
	}
	return -1
}
