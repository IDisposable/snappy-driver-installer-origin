// Package collection finds and ranks driver-pack candidates for a
// device against a whole collection of indexed driver packs, ported
// from MatcherImp/Devicematch/Hwidmatch's orchestration in
// matcher.cpp. It is the top-level package that ties together
// internal/hardware (device enumeration), internal/indexing
// (driver-pack indexes), and internal/matcher (scoring primitives) -
// none of which can import each other in this direction (hardware
// already imports indexing for date/version parsing, and indexing
// imports matcher for OS-decoration data), so this orchestration lives
// one level up from all three.
//
// Comparing a candidate against the currently installed driver's own
// score (Hwidmatch::calc_status's STATUS_BETTER/WORSE/SAME/NEW/OLD/
// CURRENT bits) needs that driver's own score, computed via
// Driver::scaninf against its own .inf file (see
// indexing.ScanInstalledInf) - which needs file I/O this package
// doesn't otherwise do. Callers compute it (see internal/scan) and
// pass it into Match as an InstalledScore.
package collection

import (
	"strings"

	"sdio/internal/common"
	"sdio/internal/hardware"
	"sdio/internal/indexing"
	"sdio/internal/matcher"
)

// InstalledScore is the currently-installed driver's own score and
// version, computed the same way a candidate driver-pack entry is
// scored (matcher.Score) so indexing.CalcStatus can compare them
// fairly. See indexing.ScanInstalledInf for how to compute Score.
// CatalogFileBits/Feature/IsNTSection are the raw ingredients Score
// was built from (matcher.Score), kept alongside it so a caller can
// explain which specific factor makes one driver rank above another,
// not just the combined number.
type InstalledScore struct {
	Score   uint32
	Version common.Version

	CatalogFileBits int
	Feature         int
	IsNTSection     bool
}

// cmProbDisabled is CM_PROB_DISABLED, duplicated from the unexported
// hardware.deviceCMProbDisabled (see that constant's doc comment).
const cmProbDisabled = 0x16

// Candidate is one driver-pack HWID entry found for a device, ranked
// against the running system - the scoring half of Hwidmatch
// (matcher.cpp).
type Candidate struct {
	Driverpack   *indexing.Driverpack
	HWIDIndex    int
	DevPos       int
	IsHardwareID bool
	Result       matcher.Result
	Dup          bool
}

// DeviceMatch is one enumerated device's collected candidate drivers,
// ported from Devicematch (matcher.cpp). Candidates is sorted best
// first (see matcher.Result.Cmp) with duplicates marked, matching
// MatcherImp::sort.
type DeviceMatch struct {
	Device     hardware.Device
	Status     int // matcher.Status* bits; nonzero here means Candidates is empty
	Candidates []Candidate
}

// firstHWID returns a device's first hardware ID, or its first
// compatible ID if it has no hardware IDs, ported from
// Device::getHWIDby(0, ...).
func firstHWID(d hardware.Device) string {
	if len(d.HardwareIDs) > 0 {
		return d.HardwareIDs[0]
	}
	if len(d.CompatibleIDs) > 0 {
		return d.CompatibleIDs[0]
	}
	return ""
}

// isIgnored reports whether a device's primary hardware ID is on the
// user's ignore list, ported from Devicematch::isIgnored.
func isIgnored(d hardware.Device, ignoreList []string) bool {
	hwid := firstHWID(d)
	if hwid == "" {
		return false
	}
	for _, ignored := range ignoreList {
		if ignored == hwid {
			return true
		}
	}
	return false
}

// isMissing reports whether a device counts as "driver missing" for
// status-reporting purposes, ported from Devicematch::isMissing.
// installed is nil if the device has no installed driver.
func isMissing(d hardware.Device, installed *hardware.InstalledDriver) bool {
	if d.Problem == cmProbDisabled {
		return false
	}
	if d.Problem != 0 && len(d.HardwareIDs) > 0 {
		return true
	}
	if installed == nil {
		upper := strings.ToUpper(firstHWID(d))
		if strings.Contains(upper, "USBPRINT") || strings.Contains(upper, "DOT4PRT") || strings.Contains(upper, "BTHENUM") {
			return true
		}
	}
	if installed != nil && strings.EqualFold(installed.MatchingDeviceID, `PCI\CC_0300`) {
		return true
	}
	return false
}

// FindHWID searches every driver pack in packs for hwid (a device's
// hardware or compatible ID string), returning one Candidate per
// match found via each pack's on-disk hash table, ported from
// MatcherImp::findHWIDs. devPos/isHardwareID identify hwid's position
// in the device's own ID list (see matcher.IdentifierScore).
// deviceHardwareID is the device's own primary hardware ID, needed by
// CalcAltSectScore's vendor-specific checks. installedScore is nil if
// the device has no installed driver (see indexing.CalcStatus).
// suppressLowConfidence should be installed!=nil && device.Problem==0
// (see Match) - ported from a default-view display rule in
// Manager::filter, applied here instead since candidates are computed
// once rather than re-filtered on demand.
func FindHWID(packs []*indexing.Driverpack, hwid string, devPos int, isHardwareID bool, ctx indexing.MatchContext, deviceHardwareID string, installedScore *InstalledScore, suppressLowConfidence bool) []Candidate {
	key := int32(indexing.APHash([]byte(strings.ToUpper(hwid))))

	var out []Candidate
	for _, drp := range packs {
		val, found := drp.Index.Hashes.Find(key)
		for found {
			out = append(out, buildCandidate(drp, int(val), devPos, isHardwareID, ctx, deviceHardwareID, installedScore, suppressLowConfidence))
			val, found = drp.Index.Hashes.FindNext()
		}
	}
	return out
}

func buildCandidate(drp *indexing.Driverpack, hwidIndex, devPos int, isHardwareID bool, ctx indexing.MatchContext, deviceHardwareID string, installedScore *InstalledScore, suppressLowConfidence bool) Candidate {
	identifierScore := matcher.IdentifierScore(devPos, isHardwareID, int(drp.InfPos(hwidIndex)))

	section := drp.Section(hwidIndex)
	decorScore := matcher.DecorationScore(matcher.SectionDecorationIndex(section), ctx.Major, ctx.Minor, ctx.Build, ctx.ArchForDecoration())
	markerScore := matcher.MarkerScore(drp.InfPath(hwidIndex), ctx.Major, ctx.Minor, ctx.ArchForMarker())
	altSectScore := drp.CalcAltSectScore(hwidIndex, decorScore, ctx, deviceHardwareID)

	// Default-view display rule ported from Manager::filter: with
	// FILTER_SHOW_WORSE_RANK and FILTER_SHOW_INVALID both off (the
	// default), a candidate for a problem-free device that already has
	// an installed driver is hidden entirely unless it's at least
	// catalog-signed-valid (altsectscore==2) - an unsigned/uncertain
	// match (1) isn't worth surfacing when something already works.
	if suppressLowConfidence && altSectScore < 2 {
		altSectScore = 0
	}

	infPath := drp.InfPath(hwidIndex)
	isNTSection := strings.Contains(strings.ToLower(drp.InstallPicked(hwidIndex)), ".nt")
	score := matcher.Score(drp.CatalogFileBits(hwidIndex), drp.Feature(hwidIndex), identifierScore, ctx.Major, ctx.IsAMD64, isNTSection)
	candidateVersion := drp.Version(hwidIndex)

	var installedVersion common.Version
	var rawInstalledScore uint32
	if installedScore != nil {
		installedVersion = installedScore.Version
		rawInstalledScore = installedScore.Score
	}
	infPathHasFeaturePrefix := strings.Contains(strings.ToLower(infPath), "feature_")
	status := indexing.CalcStatus(installedScore != nil, installedVersion, candidateVersion, rawInstalledScore, score, infPathHasFeaturePrefix, altSectScore)

	return Candidate{
		Driverpack:   drp,
		HWIDIndex:    hwidIndex,
		DevPos:       devPos,
		IsHardwareID: isHardwareID,
		Result: matcher.Result{
			AltSectScore:  altSectScore,
			Score:         score,
			DriverVersion: candidateVersion,
			DecorScore:    decorScore,
			MarkerScore:   markerScore,
			Status:        status,
			InfCRC:        drp.InfCRC(hwidIndex),
			HWID:          drp.HWID(hwidIndex),
			Section:       section,
		},
	}
}

// Match builds a device's DeviceMatch by searching packs for each of
// its hardware and compatible IDs, ranking and dup-marking the results,
// ported from the Devicematch constructor plus MatcherImp::sort's
// per-device portion. installed is nil if the device has no installed
// driver; installedScore should be nil exactly when installed is (see
// InstalledScore).
func Match(d hardware.Device, installed *hardware.InstalledDriver, installedScore *InstalledScore, packs []*indexing.Driverpack, ctx indexing.MatchContext, ignoreList []string) DeviceMatch {
	if isIgnored(d, ignoreList) {
		return DeviceMatch{Device: d, Status: matcher.StatusIgnored}
	}

	// See FindHWID's doc comment: matches Manager::filter's
	// devicematch->device->problem==0 && devicematch->driver check.
	suppressLowConfidence := installed != nil && d.Problem == 0

	deviceHWID := firstHWID(d)
	var candidates []Candidate
	for pos, hwid := range d.HardwareIDs {
		candidates = append(candidates, FindHWID(packs, hwid, pos, true, ctx, deviceHWID, installedScore, suppressLowConfidence)...)
	}
	for pos, hwid := range d.CompatibleIDs {
		candidates = append(candidates, FindHWID(packs, hwid, pos, false, ctx, deviceHWID, installedScore, suppressLowConfidence)...)
	}

	if len(candidates) == 0 {
		status := matcher.StatusNFStandard
		switch {
		case isMissing(d, installed):
			status = matcher.StatusNFMissing
		case installed != nil && strings.Contains(strings.ToLower(installed.InfPath), "oem"):
			status = matcher.StatusNFUnknown
		}
		return DeviceMatch{Device: d, Status: status}
	}

	sortCandidates(candidates)
	markDups(candidates)
	return DeviceMatch{Device: d, Candidates: candidates}
}

// sortCandidates orders candidates best-first using matcher.Result.Cmp.
func sortCandidates(candidates []Candidate) {
	for i := 0; i < len(candidates); i++ {
		best := i
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].Result.Cmp(candidates[best].Result) > 0 {
				best = j
			}
		}
		candidates[i], candidates[best] = candidates[best], candidates[i]
	}
}

// markDups flags candidates that are the same underlying driver
// reached via a different device ID, ported from the dup-marking pass
// in MatcherImp::sort.
func markDups(candidates []Candidate) {
	for i := 0; i+1 < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[i].Result.IsDup(candidates[j].Result) {
				candidates[j].Dup = true
			}
		}
	}
}
