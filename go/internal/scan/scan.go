// Package scan runs the full "detect hardware, load a driver-pack
// collection, match every device" pipeline that both cmd/sdi's
// plain-text report and any future TUI front end need, so that
// orchestration lives in one place rather than being duplicated across
// front ends.
package scan

import (
	"fmt"
	"os"
	"strings"

	"sdio/internal/collection"
	"sdio/internal/hardware"
	"sdio/internal/indexing"
	"sdio/internal/matcher"
	"sdio/internal/settings"
)

// System summarizes the machine scan.cpp/enum.cpp gathers, the parts a
// front end needs to display and to build a MatchContext from.
type System struct {
	BaseBoard hardware.BaseBoard
	SysInfo   hardware.SysInfo
	IsLaptop  bool
}

// DeviceResult is one enumerated device plus its match outcome, ported
// from what a Devicematch (matcher.cpp) exposes to the UI.
type DeviceResult struct {
	Device     hardware.Device
	Status     int // matcher.Status* bits
	Candidates []collection.Candidate

	// Installed is the currently installed driver's own registry-read
	// details (provider, inf path/section, version, matched ID), or
	// nil if the device has no driver key or it couldn't be read - the
	// same *hardware.InstalledDriver Run() already computes a score
	// from, kept around for a front end that wants to display it (see
	// cmd/sdigo's detail screen and wide-terminal "Installed" column).
	Installed *hardware.InstalledDriver

	// InstalledScore is the installed driver's own computed score
	// (see scoreInstalledDriver), or nil if Installed is nil or its
	// .inf couldn't be read - kept alongside Installed so a front end
	// can reproduce the original's per-field installed-vs-candidate
	// comparison (Manager::draw_hint's cm_score) without recomputing
	// it from scratch.
	InstalledScore *collection.InstalledScore
}

// Best returns the top-ranked candidate that represents a genuine
// improvement over any currently installed driver, or nil if there
// isn't one. This mirrors the original's default filter view
// (settings.DefaultFilters: missing/better/nf-missing, one candidate
// per device) rather than "any structurally valid candidate" - a
// device whose best candidate is merely equal to (matcher.StatusSame)
// or worse than (matcher.StatusWorse) what's already installed is not
// "found" in the actionable sense, even though it has a nonzero
// AltSectScore. IsDriverValid additionally requires DecorScore>0
// (Hwidmatch::isdrivervalid checks both), matching
// FILTER_SHOW_INVALID's default-off behavior.
func (r DeviceResult) Best() *collection.Candidate {
	if len(r.Candidates) == 0 {
		return nil
	}
	best := r.Candidates[0]
	if !best.Result.IsDriverValid() || best.Result.Status&matcher.StatusBetter == 0 {
		return nil
	}
	return &r.Candidates[0]
}

// Visible reports whether this device should be shown under filters,
// ported from the per-status-bit OR logic in Manager::filter
// (`options&statustnl[k].filter && status&statustnl[k].status`),
// adapted to this rewrite's single-best-candidate-per-device model
// (FILTER_SHOW_ONE is always effectively active in the original too -
// its own flag read is dead code, unconditionally overwritten with 1).
// Unlike Best, which always applies the original's default filter set
// regardless of the caller's actual Settings.Filters, Visible lets a
// front end honor the user's configured filters (see cmd/sdigo's
// options screen).
func (r DeviceResult) Visible(filters settings.FilterShow) bool {
	if len(r.Candidates) == 0 {
		switch r.Status {
		case matcher.StatusNFMissing:
			return filters&settings.FilterNFMissing != 0
		case matcher.StatusNFUnknown:
			return filters&settings.FilterNFUnknown != 0
		case matcher.StatusNFStandard:
			return filters&settings.FilterNFStandard != 0
		}
		return false
	}

	st := r.Candidates[0].Result.Status
	visible := false
	if filters&settings.FilterBetter != 0 && st&matcher.StatusBetter != 0 {
		visible = true
	}
	if filters&settings.FilterWorseRank != 0 && st&matcher.StatusWorse != 0 {
		visible = true
	}
	if filters&settings.FilterNewer != 0 && st&matcher.StatusNew != 0 {
		visible = true
	}
	if filters&settings.FilterCurrent != 0 && st&matcher.StatusCurrent != 0 {
		visible = true
	}
	if filters&settings.FilterOld != 0 && st&matcher.StatusOld != 0 {
		visible = true
	}
	if !visible {
		return false
	}
	if st&matcher.StatusInvalid != 0 && filters&settings.FilterInvalid == 0 {
		return false
	}
	return true
}

// Result is the outcome of a full scan.
type Result struct {
	System     System
	Collection collection.LoadResult
	Devices    []DeviceResult

	// IndexesDownloaded is how many index files Run fetched via
	// collection.BootstrapIndexes before loading the collection (0 if
	// no bootstrap/update-check was attempted, or none were needed).
	IndexesDownloaded int
	// BootstrapError is non-nil if a bootstrap/update-check attempt
	// failed (e.g. no network) - non-fatal: Run still proceeds with
	// whatever collection is already present locally.
	BootstrapError error
}

// Run performs the full pipeline: hardware detection, driver-pack
// collection loading, and per-device matching, ported from the
// combination of State::getsysinfo_fast/Device enumeration
// (enum.cpp), Collection::scanfolder (indexing.cpp), and
// MatcherImp::populate (matcher.cpp) that a full SDIO run performs
// before showing its device list.
func Run(s *settings.Settings) (Result, error) {
	var res Result

	bb, err := hardware.GetBaseBoard()
	if err != nil {
		return res, fmt.Errorf("reading base board info: %w", err)
	}
	si, err := hardware.GetSysInfoFast()
	if err != nil {
		return res, fmt.Errorf("reading system info: %w", err)
	}
	devices, err := hardware.ScanDevices()
	if err != nil {
		return res, fmt.Errorf("scanning devices: %w", err)
	}

	hasACPIBattery := false
	for _, d := range devices {
		for _, id := range d.HardwareIDs {
			if strings.Contains(id, "*ACPI0003") {
				hasACPIBattery = true
			}
		}
	}
	isLaptop := hardware.IsLaptop(bb.ChassisType, si.Monitors, si.Battery, hasACPIBattery)
	marker := ""
	if isLaptop {
		marker = matcher.NotebookOEMMarker(bb.SystemManufacturer)
	}
	res.System = System{BaseBoard: bb, SysInfo: si, IsLaptop: isLaptop}

	// LoadCollection's directory scan errors out entirely on a missing
	// directory rather than treating it as "0 packs found" - a real
	// possibility on a fresh install (no drivers/indexes yet, no
	// -torrent-file configured to create them). Creating both
	// unconditionally keeps a brand new data directory usable.
	if err := os.MkdirAll(s.DrpDir, 0o755); err != nil {
		return res, fmt.Errorf("creating %s: %w", s.DrpDir, err)
	}
	if err := os.MkdirAll(s.IndexDir, 0o755); err != nil {
		return res, fmt.Errorf("creating %s: %w", s.IndexDir, err)
	}

	// Bootstrap/refresh the index catalog from the configured torrent,
	// ported from Updater_t::WelcomeDownloadIndexes (see
	// collection.BootstrapIndexes). Always attempted when the index
	// directory is empty, since otherwise a machine with no local
	// catalog at all can never do anything; also attempted on request
	// via -checkupdates, matching that flag's documented purpose
	// ("check for driver pack updates") - re-running the same
	// bootstrap picks up any index the torrent has that isn't already
	// present locally, including newly-added pack revisions (which get
	// their own distinct filename, so are never mistaken for an
	// already-known one). A failure here is not fatal: Run proceeds
	// with whatever collection is already present locally.
	if s.TorrentFile != "" && (indexDirNeedsBootstrap(s.IndexDir) || s.Flags&settings.FlagCheckUpdates != 0) {
		res.IndexesDownloaded, res.BootstrapError = collection.BootstrapIndexes(s.TorrentFile, s.IndexDir)
	}

	res.Collection, err = collection.LoadCollection(s.DrpDir, s.IndexDir)
	if err != nil {
		return res, fmt.Errorf("loading driver-pack collection: %w", err)
	}

	ctx := indexing.MatchContext{
		Major: si.Windows.Major, Minor: si.Windows.Minor, Build: si.Windows.Build,
		IsAMD64: si.Is64Bit, IsLaptop: isLaptop, NotebookMarker: marker,
		FilterSP: s.Flags&settings.FlagFilterSP != 0,
	}

	res.Devices = make([]DeviceResult, 0, len(devices))
	for _, d := range devices {
		var installed *hardware.InstalledDriver
		if d.DriverKeyName != "" {
			if drv, err := hardware.OpenInstalledDriver(d.DriverKeyName, d); err == nil {
				installed = &drv
			}
		}
		installedScore := scoreInstalledDriver(si, installed)

		dm := collection.Match(d, installed, installedScore, res.Collection.Packs, ctx, s.IgnoreList)
		if dm.Status == matcher.StatusIgnored {
			continue
		}
		res.Devices = append(res.Devices, DeviceResult{
			Device: d, Status: dm.Status, Candidates: dm.Candidates,
			Installed: installed, InstalledScore: installedScore,
		})
	}

	return res, nil
}

// indexDirNeedsBootstrap reports whether indexDir doesn't exist, isn't
// readable, or has no entries - the "nothing to scan yet" case that
// forces a bootstrap attempt regardless of -checkupdates.
func indexDirNeedsBootstrap(indexDir string) bool {
	entries, err := os.ReadDir(indexDir)
	if err != nil {
		return true
	}
	return len(entries) == 0
}

// scoreInstalledDriver computes the currently-installed driver's own
// score by parsing its own .inf file, ported from Driver::scaninf
// (which recovers the feature/catalog-file/HWID-position data)
// combined with calc_score_h (which turns that into the same kind of
// score matcher.Score gives a candidate). Returns nil if installed is
// nil or its .inf file can't be read (e.g. already removed, or a
// permission issue) - Match treats a nil InstalledScore as "no
// installed driver to compare against", matching the original's
// "STATUS_BETTER unconditionally" fallback for that case.
func scoreInstalledDriver(si hardware.SysInfo, installed *hardware.InstalledDriver) *collection.InstalledScore {
	if installed == nil {
		return nil
	}
	data, err := os.ReadFile(si.WinDir + installed.InfPath)
	if err != nil {
		return nil
	}

	sect := installed.InfSection + installed.InfSectionExt
	info := indexing.ScanInstalledInf(data, sect, installed.MatchingDeviceID, matcher.OSDecorations[:])

	identifierScore := matcher.IdentifierScore(installed.DevPos, installed.IsHardwareID, info.InfPos)
	score := matcher.Score(info.CatalogFileBits, info.Feature, identifierScore, si.Windows.Major, si.Is64Bit, info.IsNTSection)
	return &collection.InstalledScore{
		Score: score, Version: installed.Version,
		CatalogFileBits: info.CatalogFileBits, Feature: info.Feature, IsNTSection: info.IsNTSection,
	}
}

// MatchLabel renders a short label for a device's best candidate,
// ported from the STATUS_BETTER_NEW/_CUR/_OLD branches of
// itembar_t::str_status (the other six BETTER/SAME/WORSE combinations
// don't apply here: best is nil unless StatusBetter is already set,
// per DeviceResult.Best). NEW/OLD/CURRENT is the date-vs-installed
// axis, independent of the BETTER/WORSE/SAME score axis a plain
// "FOUND" collapses - the original shows a full sentence per case
// ("More optimal driver available, though it's older"); this returns
// a short word instead, sized for a table column rather than a
// sentence. A device with no installed driver to compare dates
// against at all (first-time install) has neither bit set, so falls
// through to "FOUND".
func MatchLabel(best *collection.Candidate) string {
	if best == nil {
		return "MISSING"
	}
	switch {
	case best.Result.Status&matcher.StatusNew != 0:
		return "NEWER"
	case best.Result.Status&matcher.StatusOld != 0:
		return "OLDER"
	case best.Result.Status&matcher.StatusCurrent != 0:
		return "BETTER"
	default:
		return "FOUND"
	}
}

// StatusLabel renders a device's status as a short human-readable
// string, for status values that carry meaning with no candidates
// (see DeviceResult.Best for the has-a-candidate case).
func StatusLabel(status int) string {
	switch status {
	case matcher.StatusNFMissing:
		return "driver missing"
	case matcher.StatusNFUnknown:
		return "unknown driver installed"
	case matcher.StatusNFStandard:
		return "standard driver installed, nothing better found"
	default:
		return fmt.Sprintf("status %#x", status)
	}
}
