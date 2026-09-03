package scan

import (
	"testing"

	"sdio/internal/collection"
	"sdio/internal/matcher"
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
