// This file holds vendor-specific and OS-version-specific driver-
// candidate validity checks; indexing.CalcAltSectScore is the
// orchestration that calls them together with the scoring functions
// in score.go/scoring.go.
package matcher

import "strings"

// CmpUnsigned is a three-way compare. Driver scores are computed as
// bit-packed unsigned values (see Score) where plain subtraction would
// overflow/wrap, so callers must use this instead of a-b.
func CmpUnsigned(a, b uint32) int {
	switch {
	case a > b:
		return 1
	case a < b:
		return -1
	default:
		return 0
	}
}

// IsValidVer reports whether a driver's declared version (its first
// version component, e.g. 5, 6, or the "106" sentinel meaning 6.0) is
// compatible with the running Windows version. This is a coarse
// convention some driver packs use to hand-restrict a section to
// specific NT major versions beyond what the section's own ".ntNNN"
// decoration already encodes.
func IsValidVer(driverVersionV1, major, minor int) bool {
	switch driverVersionV1 {
	case 5:
		return major == 5
	case 6:
		return major != 5
	case 106:
		return major == 6 && minor == 0
	default:
		return true
	}
}

// IsBlacklisted reports whether a candidate driver should be rejected
// because it matches a known-bad (hardware ID substring, section name
// substring) pair. Both checks are case-insensitive substring
// matches, not exact matches - a known-bad ID could appear as a
// substring of a longer hardware ID string.
func IsBlacklisted(hardwareID, section, blacklistHWID, blacklistSection string) bool {
	if !strings.Contains(strings.ToLower(hardwareID), strings.ToLower(blacklistHWID)) {
		return false
	}
	return strings.Contains(strings.ToLower(section), strings.ToLower(blacklistSection))
}

// RealtekBlacklistHWID and RealtekBlacklistSection are the one
// hardcoded blacklist entry in calc_altsectscore: a specific
// SUBSYS-qualified device ID that must not be matched against any
// Realtek-named section, regardless of score.
const (
	RealtekBlacklistHWID    = "VEN_168C&DEV_002B&SUBSYS_30A117AA"
	RealtekBlacklistSection = "Realtek"
)

// IsValidUSB3Hub reports whether hardwareID matches one of pids (each
// a full hardware ID such as "IUSB3\ROOT_HUB30&VID_8086&PID_1E31").
// Used to restrict Intel USB 3.0 xHCI driver packs to the specific
// root-hub PIDs they are known to support - installing them against
// an unsupported hub can leave USB ports non-functional.
func IsValidUSB3Hub(hardwareID string, pids []string) bool {
	lower := strings.ToLower(hardwareID)
	for _, pid := range pids {
		if strings.Contains(lower, strings.ToLower(pid)) {
			return true
		}
	}
	return false
}

// IntelUSB3Gen2HubIDs and IntelUSB3Gen4HubIDs are the root-hub
// hardware IDs an "intel_2nd\"/"intel_sdi_2nd\" or
// "intel_4th\"/"intel_sdi_4th\" driver-pack path is restricted to,
// extracted verbatim from Hwidmatch::calc_altsectscore. Which pair of
// path prefixes applies (plain vs. "_sdi_") is decided by
// IntelPathUsesSDIPrefix.
var (
	IntelUSB3Gen2HubIDs = []string{
		`IUSB3\ROOT_HUB30&VID_8086&PID_1E31`,
	}
	IntelUSB3Gen4HubIDs = []string{
		`IUSB3\ROOT_HUB30&VID_8086&PID_8C31`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_8D31`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_8C7F`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_9C7F`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_9C31`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_9CB1`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_A12F`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_A22F`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_9D2F`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_A2AF`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_22B5`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_15B5`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_15B6`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_15C1`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_15DB`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_15D4`,
		`IUSB3\ROOT_HUB30&VID_8086&PID_0F35`,
	}
)

// IntelPathUsesSDIPrefix reports whether a driver pack's numeric
// suffix (e.g. "intel_usb3_16074" -> 16074) is recent enough to use
// the "intel_sdi_2nd\"/"intel_sdi_4th\" path prefixes instead of
// "intel_2nd\"/"intel_4th\". packVersion is the first
// underscore-followed-by-digits run found in the pack's filename; pass
// 0 if none was found, so the plain (non-SDI) prefixes apply.
func IntelPathUsesSDIPrefix(packVersion int) bool {
	return packVersion > 16073
}

// CalcNotebookValid reports whether a laptop-only driver-pack section
// is allowed to match the running system. infPath is the driver
// pack's on-disk path to its .inf file; marker is the running
// system's notebook OEM marker (see NotebookOEMMarker), empty if
// none/not a laptop.
func CalcNotebookValid(infPath string, isLaptop bool, marker string) bool {
	lower := strings.ToLower(infPath)
	if !strings.Contains(lower, `_nb\`) && !strings.Contains(lower, `touchpad_mouse\`) {
		return true
	}
	if !isLaptop || marker == "" {
		return false
	}
	return strings.Contains(lower, strings.ToLower(marker))
}
