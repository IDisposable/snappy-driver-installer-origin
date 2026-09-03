package settings

// FilterShow is a bitmask of which driver-match categories to display,
// ported from fileter_show in settings.h. Bit positions are renumbered
// sequentially; the original tied them to GUI menu-item IDs that don't
// exist in this rewrite.
type FilterShow uint32

const (
	FilterMissing FilterShow = 1 << iota
	FilterNewer
	FilterCurrent
	FilterOld
	FilterBetter
	FilterWorseRank
	FilterNFMissing
	FilterNFUnknown
	FilterNFStandard
	FilterOne
	FilterDup
	FilterInvalid
)

// DefaultFilters matches the original's default: not-installed, better
// matches, missing-and-not-found devices, and only the best match per
// device. "Newer" is left out by default, as in the original.
const DefaultFilters = FilterMissing | FilterBetter | FilterNFMissing | FilterOne
