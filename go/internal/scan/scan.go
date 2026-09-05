// Package scan runs the full "detect hardware, load a driver-pack
// collection, match every device" pipeline cmd/sdigo's report and TUI
// both need, so this orchestration lives in one place.
package scan

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"sdio/internal/collection"
	"sdio/internal/hardware"
	"sdio/internal/indexing"
	"sdio/internal/logging"
	"sdio/internal/matcher"
	"sdio/internal/settings"
)

// System summarizes the machine, the parts a front end needs to
// display and to build a MatchContext from.
type System struct {
	BaseBoard hardware.BaseBoard
	SysInfo   hardware.SysInfo
	IsLaptop  bool
}

// DeviceResult is one enumerated device plus its match outcome.
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
	// can render a per-field installed-vs-candidate comparison (see
	// cmd/sdigo's compareInstalledVsCandidate) without recomputing it
	// from scratch.
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

// Visible reports whether this device should be shown under filters -
// this rewrite always keeps one best candidate per device (there's no
// "show all candidates" toggle to honor; the original's own
// equivalent flag read was dead code, unconditionally overwritten
// before use). Unlike Best, which always applies the fixed default
// filter set regardless of the caller's actual Settings.Filters,
// Visible lets a front end honor the user's configured filters (see
// cmd/sdigo's options screen).
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

	// FirstRun is true if the index directory was missing or empty
	// when Run started - a front end can use this to show a
	// first-run/Welcome screen. True regardless of whether a torrent
	// file is configured (unlike the bootstrap it also gates), since
	// it's answering "does this machine have any local data yet", not
	// "did a bootstrap actually happen".
	FirstRun bool
}

// Prepared carries Prepare's hardware-detection results into
// MatchWithCollection - split out from Run so a caller (cmd/sdigo's
// TUI) can show a match against whatever collection is already on
// disk right away, then call MatchWithCollection again alone once a
// background bootstrap/download finishes, without repeating hardware
// detection (SetupAPI device enumeration over hundreds of devices
// isn't free).
type Prepared struct {
	System   System
	FirstRun bool

	devices  []hardware.Device
	si       hardware.SysInfo
	isLaptop bool
	marker   string
}

// Prepare detects hardware and creates DrpDir/IndexDir if missing -
// the fast, no-network part of Run.
func Prepare(s *settings.Settings) (Prepared, error) {
	var p Prepared

	bb, err := hardware.GetBaseBoard()
	if err != nil {
		return p, fmt.Errorf("reading base board info: %w", err)
	}
	si, err := hardware.GetSysInfoFast()
	if err != nil {
		return p, fmt.Errorf("reading system info: %w", err)
	}
	devices, err := hardware.ScanDevices()
	if err != nil {
		return p, fmt.Errorf("scanning devices: %w", err)
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

	p.System = System{BaseBoard: bb, SysInfo: si, IsLaptop: isLaptop}
	p.devices, p.si, p.isLaptop, p.marker = devices, si, isLaptop, marker

	// Captured before this function's own side effects below (the
	// MkdirAll calls) could change the answer: a front end uses this to
	// decide whether to show a first-run/Welcome screen.
	p.FirstRun = indexDirNeedsBootstrap(s.IndexDir)

	// LoadCollection's directory scan errors out entirely on a missing
	// directory rather than treating it as "0 packs found" - a real
	// possibility on a fresh install (no drivers/indexes yet, no
	// -torrent-file configured to create them). Creating both
	// unconditionally keeps a brand new data directory usable.
	if err := os.MkdirAll(s.DrpDir, 0o755); err != nil {
		return p, fmt.Errorf("creating %s: %w", s.DrpDir, err)
	}
	if err := os.MkdirAll(s.IndexDir, 0o755); err != nil {
		return p, fmt.Errorf("creating %s: %w", s.IndexDir, err)
	}
	return p, nil
}

// MatchWithCollection loads whatever driver-pack collection is
// currently on disk and matches every device Prepare found against
// it. Call again with the same Prepared after a bootstrap/download to
// refresh with the new data, without repeating hardware detection.
// onProgress, if non-nil, is called before each driver pack is loaded
// (see collection.LoadCollection).
func MatchWithCollection(s *settings.Settings, p Prepared, onProgress func(current, total int, filename string)) (Result, error) {
	res := Result{System: p.System, FirstRun: p.FirstRun}

	var err error
	res.Collection, err = collection.LoadCollection(s.DrpDir, s.IndexDir,
		s.Flags&settings.FlagForceReindexing != 0, s.Flags&settings.FlagPrintIndex != 0, onProgress)
	if err != nil {
		return res, fmt.Errorf("loading driver-pack collection: %w", err)
	}

	major, minor, isAMD64 := virtualEnvironment(s, p.si)
	ctx := indexing.MatchContext{
		Major: major, Minor: minor, Build: p.si.Windows.Build,
		IsAMD64: isAMD64, IsLaptop: p.isLaptop, NotebookMarker: p.marker,
		FilterSP: s.Flags&settings.FlagFilterSP != 0,
	}

	res.Devices = make([]DeviceResult, 0, len(p.devices))
	for _, d := range p.devices {
		var installed *hardware.InstalledDriver
		if d.DriverKeyName != "" {
			if drv, err := hardware.OpenInstalledDriver(d.DriverKeyName, d); err == nil {
				installed = &drv
			}
		}
		installedScore := scoreInstalledDriver(p.si, installed)

		dm := collection.Match(d, installed, installedScore, res.Collection.Packs, ctx, s.IgnoreList)
		if dm.Status == matcher.StatusIgnored {
			continue
		}
		res.Devices = append(res.Devices, DeviceResult{
			Device: d, Status: dm.Status, Candidates: dm.Candidates,
			Installed: installed, InstalledScore: installedScore,
		})
	}
	sortDevices(res.Devices)

	return res, nil
}

// Run performs the full pipeline: Prepare, an inline bootstrap if
// warranted, then MatchWithCollection - the synchronous, one-shot
// entry point -nogui and other non-interactive callers use. cmd/sdigo's
// TUI instead calls Prepare/MatchWithCollection directly, so a
// bootstrap can run as its own background step after an initial match
// against whatever's already on disk, rather than blocking the first
// render on a network operation. logger records a structured entry
// for every file BootstrapIndexes downloads.
func Run(s *settings.Settings, logger *logging.Logger) (Result, error) {
	p, err := Prepare(s)
	if err != nil {
		return Result{}, err
	}

	// Bootstrap/refresh the index catalog from the configured torrent
	// (see collection.BootstrapIndexes). Always attempted when the index
	// directory is empty, since otherwise a machine with no local
	// catalog at all can never do anything; also attempted on request
	// via -checkupdates, matching that flag's documented purpose
	// ("check for driver pack updates") - re-running the same
	// bootstrap picks up any index the torrent has that isn't already
	// present locally, including newly-added pack revisions (which get
	// their own distinct filename, so are never mistaken for an
	// already-known one). A failure here is not fatal: Run proceeds
	// with whatever collection is already present locally.
	var indexesDownloaded int
	var bootstrapErr error
	if s.TorrentFile != "" && (p.FirstRun || s.Flags&settings.FlagCheckUpdates != 0) {
		indexesDownloaded, bootstrapErr = collection.BootstrapIndexes(context.Background(), s.TorrentFile, s.IndexDir, s.UpdatesDir, s.Flags&settings.FlagKeepSeeding != 0, nil, nil, logger)
	}

	res, err := MatchWithCollection(s, p, nil)
	res.IndexesDownloaded, res.BootstrapError = indexesDownloaded, bootstrapErr
	return res, err
}

// sortDevices orders res.Devices the same way every front end (TUI
// table, -nogui report, -json report) should show them, ported from
// MatcherImp::sorta: a narrow set of device classes always sort
// first regardless of match status (deviceSortsFirst), then ties are
// broken by the best candidate's driver-pack filename
// (Hwidmatch::cmpnames - the original's other branch, comparing .inf
// paths for a pack literally named "unpacked.7z", doesn't apply here
// since this rewrite only loads real archived .7z packs). Devices
// with no candidate at all sort after every device that has one,
// within the same tier - stable otherwise, matching the original
// leaving same-tier no-candidate devices in their enumeration order.
func sortDevices(devices []DeviceResult) {
	sort.SliceStable(devices, func(i, j int) bool {
		fi, fj := deviceSortsFirst(devices[i]), deviceSortsFirst(devices[j])
		if fi != fj {
			return fi
		}
		pi, hasI := bestPackName(devices[i])
		pj, hasJ := bestPackName(devices[j])
		if hasI != hasJ {
			return hasI
		}
		if !hasI {
			return false
		}
		return pi < pj
	})
}

// deviceSortsFirst mirrors Devicematch::isMissing(state) - despite the
// name, not "no driver found" (that's Status/Best), but a fixed set of
// display-ordering exceptions: a device with any problem code except
// CM_PROB_DISABLED, a USB/Dot4/Bluetooth print-class device with no
// driver object read at all, or an installed driver whose matching ID
// is the "PCI\CC_0300" placeholder class.
func deviceSortsFirst(dr DeviceResult) bool {
	if dr.Device.Status() == hardware.DeviceDisabled {
		return false
	}
	if dr.Device.Problem != 0 && len(dr.Device.HardwareIDs) > 0 {
		return true
	}
	if dr.Installed == nil {
		hwid0 := ""
		if len(dr.Device.HardwareIDs) > 0 {
			hwid0 = strings.ToUpper(dr.Device.HardwareIDs[0])
		}
		for _, cls := range [...]string{"USBPRINT", "DOT4PRT", "BTHENUM"} {
			if strings.Contains(hwid0, cls) {
				return true
			}
		}
		return false
	}
	return strings.EqualFold(dr.Installed.MatchingDeviceID, `PCI\CC_0300`)
}

// bestPackName returns the driver-pack filename of dr's top-ranked
// candidate (which cmpnames compares two devices by, breaking a
// deviceSortsFirst tie), and whether dr has one at all.
func bestPackName(dr DeviceResult) (name string, ok bool) {
	if len(dr.Candidates) == 0 {
		return "", false
	}
	return dr.Candidates[0].Driverpack.Filename, true
}

// virtualEnvironment resolves the Windows major/minor version and
// architecture to match against - the real detected values, unless
// -v/-arch ask to match against a different one instead (checking
// what a device would match on a target other than the machine
// actually running, ported from virtual_os_version/virtual_arch_type's
// original purpose). VirtualOSVersion decodes as major*10+minor, the
// same convention hardware.FindWindowsVersionName's table already
// uses (e.g. 100 -> 10.0, 61 -> 6.1) - Build has no virtual equivalent
// in the original either, so it's always the real detected value.
func virtualEnvironment(s *settings.Settings, si hardware.SysInfo) (major, minor int, isAMD64 bool) {
	major, minor, isAMD64 = si.Windows.Major, si.Windows.Minor, si.Is64Bit
	if s.VirtualOSVersion != 0 {
		major, minor = s.VirtualOSVersion/10, s.VirtualOSVersion%10
	}
	if s.VirtualArchType != 0 {
		isAMD64 = s.VirtualArchType == 64
	}
	return major, minor, isAMD64
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
// score by parsing its own .inf file (indexing.ScanInstalledInf) into
// the same kind of score matcher.Score gives a candidate. Returns nil
// if installed is nil or its .inf file can't be read (e.g. already
// removed, or a permission issue) - collection.Match treats a nil
// InstalledScore as "no installed driver to compare against" and
// reports every candidate as StatusBetter unconditionally
// (indexing.CalcStatus's hasInstalledDriver=false branch), rather than
// failing the whole comparison.
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

// MatchLabel renders a short label for a device's best candidate. Only
// the New/Old/Current bits are checked here - the other six BETTER/
// SAME/WORSE combinations don't apply, since best is nil unless
// StatusBetter is already set (see DeviceResult.Best). New/Old/Current
// is the date-vs-installed axis, independent of the score axis a
// plain "Found" collapses - sized for a table column, not a sentence.
// A device with no installed driver to compare dates against at all
// (first-time install) has neither bit set, so falls through to
// "Found".
func MatchLabel(best *collection.Candidate) string {
	if best == nil {
		return "Missing"
	}
	switch {
	case best.Result.Status&matcher.StatusNew != 0:
		return "Newer"
	case best.Result.Status&matcher.StatusOld != 0:
		return "Older"
	case best.Result.Status&matcher.StatusCurrent != 0:
		return "Better"
	default:
		return "Found"
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
