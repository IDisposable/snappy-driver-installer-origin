package scan

import (
	"testing"

	"sdio/internal/collection"
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
	if got := MatchLabel(nil); got != "MISSING" {
		t.Errorf("MatchLabel(nil) = %q, want %q", got, "MISSING")
	}
	cases := []struct {
		status int
		want   string
	}{
		{matcher.StatusBetter | matcher.StatusNew, "NEWER"},
		{matcher.StatusBetter | matcher.StatusOld, "OLDER"},
		{matcher.StatusBetter | matcher.StatusCurrent, "BETTER"},
		{matcher.StatusBetter, "FOUND"},
	}
	for _, c := range cases {
		best := &collection.Candidate{Result: matcher.Result{Status: c.status}}
		if got := MatchLabel(best); got != c.want {
			t.Errorf("MatchLabel(status=%#x) = %q, want %q", c.status, got, c.want)
		}
	}
}
