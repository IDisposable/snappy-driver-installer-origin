package settings

// FilterShow is a bitmask of which driver-match categories to display,
// ported from fileter_show in settings.h. Bit positions intentionally
// match the original exactly (1<<ID_SHOW_MISSING, etc.), even though
// those GUI menu-item IDs don't exist in this rewrite: -filters:N is
// persisted as a raw integer in users' existing sdio.cfg files, so the
// numbering must round-trip unchanged rather than being cleaned up.
type FilterShow uint32

const (
	FilterMissing    FilterShow = 1 << 1  // ID_SHOW_MISSING
	FilterNewer      FilterShow = 1 << 2  // ID_SHOW_NEWER
	FilterCurrent    FilterShow = 1 << 3  // ID_SHOW_CURRENT
	FilterOld        FilterShow = 1 << 4  // ID_SHOW_OLD
	FilterBetter     FilterShow = 1 << 5  // ID_SHOW_BETTER
	FilterWorseRank  FilterShow = 1 << 6  // ID_SHOW_WORSE_RANK
	FilterNFMissing  FilterShow = 1 << 7  // ID_SHOW_NF_MISSING
	FilterNFUnknown  FilterShow = 1 << 8  // ID_SHOW_NF_UNKNOWN
	FilterNFStandard FilterShow = 1 << 9  // ID_SHOW_NF_STANDARD
	FilterOne        FilterShow = 1 << 10 // ID_SHOW_ONE
	FilterDup        FilterShow = 1 << 11 // ID_SHOW_DUP
	FilterInvalid    FilterShow = 1 << 12 // ID_SHOW_INVALID
)

// DefaultFilters matches the original's default: not-installed, better
// matches, missing-and-not-found devices, and only the best match per
// device. "Newer" is left out by default, as in the original.
const DefaultFilters = FilterMissing | FilterBetter | FilterNFMissing | FilterOne
