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

func TestDeviceResultVisibleNoCandidatesUsesDeviceStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{"nf-missing shown by default", matcher.StatusNFMissing, true},
		{"nf-standard hidden by default", matcher.StatusNFStandard, false},
		{"nf-unknown hidden by default", matcher.StatusNFUnknown, false},
	}
	for _, c := range cases {
		dr := DeviceResult{Status: c.status}
		if got := dr.Visible(settings.DefaultFilters); got != c.want {
			t.Errorf("%s: Visible(DefaultFilters) = %v, want %v", c.name, got, c.want)
		}
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
