package scan

import (
	"testing"

	"sdio/internal/collection"
	"sdio/internal/hardware"
	"sdio/internal/indexing"
	"sdio/internal/matcher"
	"sdio/internal/settings"
)

// TestDeviceResultBestRequiresDecorScore confirms Best() rejects a
// candidate with AltSectScore>0 but DecorScore==0, matching
// Hwidmatch::isdrivervalid's "altsectscore>0 && decorscore>0" (both
// required, not just AltSectScore) - a real, reachable state:
// CalcAltSectScore can pass a candidate whose own section failed the
// OS-decoration check as long as no other declared section of the
// same manufacturer scores higher.
func TestDeviceResultBestRequiresDecorScore(t *testing.T) {
	dr := DeviceResult{
		Candidates: []collection.Candidate{{
			Result: matcher.Result{AltSectScore: 2, DecorScore: 0, Status: matcher.StatusBetter},
		}},
	}
	if got := dr.Best(); got != nil {
		t.Errorf("Best() = %+v, want nil (DecorScore==0 should invalidate the candidate)", got)
	}
}

func TestDeviceResultBestAcceptsValidBetterCandidate(t *testing.T) {
	dr := DeviceResult{
		Candidates: []collection.Candidate{{
			Result: matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusBetter},
		}},
	}
	if got := dr.Best(); got == nil {
		t.Error("Best() = nil, want the valid, better candidate")
	}
}

func TestDeviceResultBestRejectsNonBetterCandidate(t *testing.T) {
	dr := DeviceResult{
		Candidates: []collection.Candidate{{
			Result: matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusSame},
		}},
	}
	if got := dr.Best(); got != nil {
		t.Errorf("Best() = %+v, want nil (StatusBetter not set)", got)
	}
}

func TestDeviceResultVisibleDefaultFiltersMatchesBetter(t *testing.T) {
	better := DeviceResult{Candidates: []collection.Candidate{{
		Result: matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusBetter},
	}}}
	if !better.Visible(settings.DefaultFilters) {
		t.Error("Visible(DefaultFilters) = false, want true for a StatusBetter candidate")
	}

	same := DeviceResult{Candidates: []collection.Candidate{{
		Result: matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusSame},
	}}}
	if same.Visible(settings.DefaultFilters) {
		t.Error("Visible(DefaultFilters) = true, want false for a StatusSame-only candidate (no FilterSame bit exists)")
	}
}

// TestDeviceResultVisibleNoCandidatesUsesDeviceStatus confirms every
// no-candidate-at-all status (NF_MISSING/NF_UNKNOWN/NF_STANDARD, the
// "absent in driver packs" bucket) is hidden under DefaultFilters -
// this rewrite has no way to find a driver for these regardless, and
// real-world sdio.cfg files converge on not showing them (see
// settings.DefaultFilters' doc comment). FilterNFMissing explicitly
// re-enables the "not installed" case if a caller wants it back.
func TestDeviceResultVisibleNoCandidatesUsesDeviceStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{"nf-missing hidden by default", matcher.StatusNFMissing},
		{"nf-standard hidden by default", matcher.StatusNFStandard},
		{"nf-unknown hidden by default", matcher.StatusNFUnknown},
	}
	for _, c := range cases {
		dr := DeviceResult{Status: c.status}
		if got := dr.Visible(settings.DefaultFilters); got {
			t.Errorf("%s: Visible(DefaultFilters) = true, want false", c.name)
		}
	}
	dr := DeviceResult{Status: matcher.StatusNFMissing}
	if !dr.Visible(settings.DefaultFilters | settings.FilterNFMissing) {
		t.Error("Visible(DefaultFilters|FilterNFMissing) = false, want true")
	}
}

func TestDeviceResultVisibleInvalidRequiresExplicitFilter(t *testing.T) {
	dr := DeviceResult{Candidates: []collection.Candidate{{
		Result: matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusBetter | matcher.StatusInvalid},
	}}}
	if dr.Visible(settings.DefaultFilters) {
		t.Error("Visible(DefaultFilters) = true, want false: StatusInvalid needs FilterInvalid even if otherwise StatusBetter")
	}
	if !dr.Visible(settings.DefaultFilters | settings.FilterInvalid) {
		t.Error("Visible(DefaultFilters|FilterInvalid) = false, want true")
	}
}

func TestMatchLabel(t *testing.T) {
	if got := MatchLabel(nil); got != "Missing" {
		t.Errorf("MatchLabel(nil) = %q, want %q", got, "Missing")
	}
	cases := []struct {
		status int
		want   string
	}{
		{matcher.StatusBetter | matcher.StatusNew, "Newer"},
		{matcher.StatusBetter | matcher.StatusOld, "Older"},
		{matcher.StatusBetter | matcher.StatusCurrent, "Better"},
		{matcher.StatusBetter, "Found"},
	}
	for _, c := range cases {
		best := &collection.Candidate{Result: matcher.Result{Status: c.status}}
		if got := MatchLabel(best); got != c.want {
			t.Errorf("MatchLabel(status=%#x) = %q, want %q", c.status, got, c.want)
		}
	}
}

func withPack(filename string) []collection.Candidate {
	return []collection.Candidate{{Driverpack: &indexing.Driverpack{Filename: filename}}}
}

// TestSortDevicesOrdersByPackNameWithinATier confirms devices tie
// (deviceSortsFirst equal on both) sort by their best candidate's
// driver-pack filename, ascending - Hwidmatch::cmpnames.
func TestSortDevicesOrdersByPackNameWithinATier(t *testing.T) {
	devices := []DeviceResult{
		{Device: hardware.Device{Description: "Z"}, Candidates: withPack("DP_Zebra.7z")},
		{Device: hardware.Device{Description: "A"}, Candidates: withPack("DP_Apple.7z")},
		{Device: hardware.Device{Description: "M"}, Candidates: withPack("DP_Mango.7z")},
	}
	sortDevices(devices)
	got := []string{devices[0].Device.Description, devices[1].Device.Description, devices[2].Device.Description}
	want := []string{"A", "M", "Z"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sortDevices() order = %v, want %v", got, want)
			break
		}
	}
}

// TestSortDevicesPutsNoCandidateDevicesAfterMatchedOnesInATier
// confirms a device with no candidate at all sorts after every device
// that has one, within the same deviceSortsFirst tier - the original
// swaps a device with no Hwidmatch to after one that has any.
func TestSortDevicesPutsNoCandidateDevicesAfterMatchedOnesInATier(t *testing.T) {
	devices := []DeviceResult{
		{Device: hardware.Device{Description: "NoMatch"}},
		{Device: hardware.Device{Description: "HasMatch"}, Candidates: withPack("DP_Zebra.7z")},
	}
	sortDevices(devices)
	if devices[0].Device.Description != "HasMatch" || devices[1].Device.Description != "NoMatch" {
		t.Errorf("sortDevices() = %v, want HasMatch before NoMatch", devices)
	}
}

// TestSortDevicesPutsProblemDevicesFirst confirms deviceSortsFirst
// wins over pack-name ordering entirely - a device with a problem
// code sorts before one without, regardless of driver-pack name.
func TestSortDevicesPutsProblemDevicesFirst(t *testing.T) {
	devices := []DeviceResult{
		{Device: hardware.Device{Description: "Normal", HardwareIDs: []string{"PCI\\VEN_1"}}, Candidates: withPack("DP_Apple.7z")},
		{
			Device:     hardware.Device{Description: "HasProblem", HardwareIDs: []string{"PCI\\VEN_2"}, Problem: 1},
			Candidates: withPack("DP_Zebra.7z"),
		},
	}
	sortDevices(devices)
	if devices[0].Device.Description != "HasProblem" {
		t.Errorf("sortDevices() = %v, want the problem device first regardless of pack name", devices)
	}
}

func TestDeviceSortsFirstDisabledDeviceIsNotSpecialCased(t *testing.T) {
	dr := DeviceResult{Device: hardware.Device{
		HardwareIDs: []string{"PCI\\VEN_1"}, Problem: 1, RawStatusFlags: 0x400,
	}}
	// Problem 0x16 (CM_PROB_DISABLED) with the has-problem status flag
	// set is exactly Device.Status()==DeviceDisabled - Devicematch::
	// isMissing explicitly excludes this case despite the device having
	// a nonzero problem code.
	dr.Device.Problem = 0x16
	if deviceSortsFirst(dr) {
		t.Error("deviceSortsFirst(disabled device) = true, want false (CM_PROB_DISABLED is excluded)")
	}
}

func TestDeviceSortsFirstNoDriverPrintClassDevice(t *testing.T) {
	dr := DeviceResult{Device: hardware.Device{HardwareIDs: []string{"USBPRINT\\VID_1"}}}
	if !deviceSortsFirst(dr) {
		t.Error("deviceSortsFirst(USBPRINT device with no installed driver) = false, want true")
	}
}

func TestDeviceSortsFirstPCI0300Placeholder(t *testing.T) {
	dr := DeviceResult{
		Device:    hardware.Device{HardwareIDs: []string{"PCI\\VEN_1"}},
		Installed: &hardware.InstalledDriver{MatchingDeviceID: `PCI\CC_0300`},
	}
	if !deviceSortsFirst(dr) {
		t.Error("deviceSortsFirst(installed PCI\\CC_0300 placeholder) = false, want true")
	}
}
