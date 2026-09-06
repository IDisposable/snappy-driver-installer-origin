package settings

// FilterShow is a bitmask of which driver-match categories to display.
// Bits use declaration order so the current sdigo.cfg format stays easy
// to inspect and maintain.
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

// DefaultFilters shows actionable missing, newer, and better matches,
// with one best candidate per device.
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
