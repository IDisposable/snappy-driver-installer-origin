package matcher

import "sdio/internal/common"

// Status bit values, ported verbatim from the STATUS_* enum in
// matcher.h. calc_status (Hwidmatch::calc_status), which combines
// these from a candidate driver's comparison against the currently
// installed one, is not yet ported - it needs the not-yet-built
// Devicematch/Driver object graph - but the bits themselves are
// needed by Result.Cmp below.
const (
	StatusBetter     = 0x001
	StatusSame       = 0x002
	StatusWorse      = 0x004
	StatusInvalid    = 0x008
	StatusMissing    = 0x010
	StatusNew        = 0x020
	StatusCurrent    = 0x040
	StatusOld        = 0x080
	StatusNFMissing  = 0x100
	StatusNFUnknown  = 0x200
	StatusNFStandard = 0x400
	StatusDup        = 0x800
	StatusIgnored    = 0x1000
)

// Result holds one candidate driver's computed match scores and
// identifying fields - the ranking half of Hwidmatch (matcher.cpp),
// decoupled from the not-yet-ported Driverpack/Devicematch object
// graph that computes these fields in the original.
type Result struct {
	AltSectScore  int
	Score         uint32
	DriverVersion common.Version
	DecorScore    int
	MarkerScore   int
	Status        int
	InfCRC        int32
	HWID          string
	Section       string
}

// Cmp orders two candidates best-first, breaking ties in order:
// validity tier (AltSectScore), then Score - higher wins, hence the
// negated CmpUnsigned - then release date, decoration score, marker
// score, and finally Status with StatusDup masked out (duplicate
// tracking lives in collection.Candidate.Dup instead, not this bit).
func (r Result) Cmp(other Result) int {
	if d := r.AltSectScore - other.AltSectScore; d != 0 {
		return d
	}
	if d := CmpUnsigned(r.Score, other.Score); d != 0 {
		return -d
	}
	if d := common.CompareDate(r.DriverVersion, other.DriverVersion); d != 0 {
		return d
	}
	if d := r.DecorScore - other.DecorScore; d != 0 {
		return d
	}
	if d := r.MarkerScore - other.MarkerScore; d != 0 {
		return d
	}
	return (r.Status &^ StatusDup) - (other.Status &^ StatusDup)
}

// IsDup reports whether r and other are the same underlying driver
// entry reached via two different candidate paths.
func (r Result) IsDup(other Result) bool {
	return r.InfCRC == other.InfCRC && r.HWID == other.HWID && r.Section == other.Section
}

// IsDriverValid reports whether r survived the OS-decoration and
// vendor-specific validity checks.
func (r Result) IsDriverValid() bool {
	return r.AltSectScore > 0 && r.DecorScore > 0
}
