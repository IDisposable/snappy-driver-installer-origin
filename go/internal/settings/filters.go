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

// DefaultFilters deliberately does NOT match the original's
// compiled-in default (FilterMissing|FilterBetter|FilterNFMissing|
// FilterOne, from Settings_t's constructor in settings.cpp). That
// default shows every device with literally no candidate driver pack
// at all (FilterNFMissing) as "MISSING" - noisy for devices this
// rewrite has no way to find a driver for regardless (virtual buses,
// vendor-specific services, etc.), and not what real installations
// converge on: a real production sdio.cfg's persisted filters value
// (1062) drops FilterNFMissing and adds FilterNewer instead. This
// constant matches that real-world value, per explicit user direction
// ("the GUI doesn't show missing drivers for things that are not in
// the driver packs... I think that needs to be the default").
const DefaultFilters = FilterMissing | FilterNewer | FilterBetter | FilterOne

// FilterOption is one entry of FilterOptions, for a front end (e.g. a
// TUI options screen) to list and toggle every display filter without
// needing to know the bit values directly.
type FilterOption struct {
	Name string
	Help string
	Bit  FilterShow
}

// FilterOptions lists every FilterShow bit, in the same order as the
// original's "Show" menu, for building a settings UI.
func FilterOptions() []FilterOption {
	return []FilterOption{
		{"missing", "devices with no driver installed at all", FilterMissing},
		{"newer", "candidates dated more recently than the installed driver", FilterNewer},
		{"current", "candidates dated the same as the installed driver", FilterCurrent},
		{"old", "candidates dated older than the installed driver", FilterOld},
		{"better", "candidates that outrank the installed driver", FilterBetter},
		{"worse-rank", "candidates that rank below the installed driver", FilterWorseRank},
		{"nf-missing", "no candidate found, and the device has no driver", FilterNFMissing},
		{"nf-unknown", "no candidate found; an unrecognized (oem*.inf) driver is installed", FilterNFUnknown},
		{"nf-standard", "no candidate found; a standard/inbox driver is installed", FilterNFStandard},
		{"one", "show only the single best candidate per device", FilterOne},
		{"dup", "show duplicate candidates (same underlying driver via a different ID)", FilterDup},
		{"invalid", "show structurally invalid candidates (failed OS-decoration/vendor checks)", FilterInvalid},
	}
}
