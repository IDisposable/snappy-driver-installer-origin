package hardware

import (
	"strings"

	"sdio/internal/common"
)

// InstalledDriver describes a currently-installed driver, read from
// its registry key under
// SYSTEM\CurrentControlSet\Control\Class\<ClassGUID>\<Index>. Feature,
// CatalogFileBits, and an NT-section flag aren't fields here: getting
// them needs parsing the driver's own .inf file, which this package
// avoids (it only reads the registry) - see
// indexing.ScanInstalledInf and collection.InstalledScore, which scan
// package callers combine with this type's DevPos/IsHardwareID for
// the full picture.
type InstalledDriver struct {
	Desc             string
	ProviderName     string
	MatchingDeviceID string
	InfPath          string
	InfSection       string
	InfSectionExt    string
	Version          common.Version // from the DriverDate + DriverVersion registry strings

	// DevPos/IsHardwareID describe how MatchingDeviceID matched the
	// device's own hardware/compatible ID list. DevPos is -1 if no
	// match was found; combined with indexing.ScanInstalledInf's own
	// InfPos by matcher.IdentifierScore for the full identifier score
	// (see internal/scan).
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

// IsMicrosoftDriver reports whether inst's provider is Microsoft -
// almost always an inbox/generic Windows driver rather than a vendor
// one, which replacing is often unnecessary and can be riskier than
// leaving alone (Windows itself keeps it updated via Windows Update,
// and a vendor "upgrade" can be a worse fit than the inbox driver
// Microsoft ships for exactly this hardware class). Shared between
// cmd/sdigo's TUI ([MS] tag, select-all exclusion) and
// internal/report (excluding these from -install's automatic pending
// list) so the two surfaces can't drift apart on what counts.
func IsMicrosoftDriver(inst *InstalledDriver) bool {
	return inst != nil && strings.EqualFold(strings.TrimSpace(inst.ProviderName), "Microsoft")
}

func indexOfFold(list []string, s string) int {
	for i, v := range list {
		if strings.EqualFold(v, s) {
			return i
		}
	}
	return -1
}
