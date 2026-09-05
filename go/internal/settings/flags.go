package settings

// Flags is a bitmask of engine behavior toggles, ported from the FLAG_*
// and COLLECTION_* enum in settings.h. Bit positions are not preserved
// from the original (they were tied to unrelated GUI menu-item IDs);
// nothing depends on their numeric value.
type Flags uint64

const (
	FlagForceReindexing Flags = 1 << iota
	FlagPrintIndex
	FlagNoGUI
	FlagCheckUpdates
	FlagDisableInstall
	FlagAutoClose
	FlagNoRestorePoint
	FlagNoLogFile
	FlagNoSnapshot
	FlagPreserveCfg
	FlagDelExtraInfs
	FlagOnlyUpdates
	FlagAutoUpdate
	FlagFilterSP
	FlagTorrentAlerts
	FlagKeepSeeding
	FlagNoStop
	FlagExtractOnly
)

// boolFlagDef ties a command-line flag name to a Flags bit, and records
// whether the flag is a persistent preference (round-tripped through
// sdio.cfg) or a one-shot switch that only applies to a single run.
type boolFlagDef struct {
	name    string
	help    string
	flag    Flags
	persist bool
}

// boolFlagDefs is the single source of truth for both flag registration
// and saving, replacing the original's two independently-maintained
// if-chains in parse() and save() (which had already drifted apart).
// Several of these are registered (and so round-trip through sdio.cfg
// and show up as options-screen checkboxes) but have no wired effect
// yet, each for its own reason - see the "(not implemented...)" note
// on each. They're kept registered rather than removed so an existing
// cfg file's setting isn't silently dropped, and so the gap is visible
// instead of the option quietly vanishing. Found via a forensic pass
// over every flag against the original's FLAG_* semantics
// (source/settings.cpp/update.cpp/manager.cpp); see the go-rewrite
// branch history for the full per-flag audit.
var boolFlagDefs = []boolFlagDef{
	{"checkupdates", "check for driver pack updates", FlagCheckUpdates, true},
	{"onlyupdates", "only fetch driver packs newer than what's on disk (not implemented - the original's revision-comparison filter isn't ported)", FlagOnlyUpdates, true},
	{"torrentalerts", "log torrent alert events", FlagTorrentAlerts, true},
	{"keepseeding", "keep seeding driver packs to other peers after download completes", FlagKeepSeeding, true},
	{"norestorepnt", "don't create a system restore point", FlagNoRestorePoint, true},

	{"preservecfg", "don't overwrite sdio.cfg on exit", FlagPreserveCfg, false},
	{"nogui", "run headless, without an interactive front end", FlagNoGUI, false},
	{"autoclose", "exit automatically once an install or download finishes", FlagAutoClose, false},
	{"autoupdate", "download every driver pack automatically, once, right after the first scan", FlagAutoUpdate, false},
	{"nostop", "install anyway if creating a restore point fails (default: abort the install)", FlagNoStop, false},
	{"disableinstall", "scan and match only: never install, and never create a restore point", FlagDisableInstall, false},
	{"delextrainfs", "delete extra .inf files after install", FlagDelExtraInfs, false},
	{"nologfile", "don't write a log file (not implemented - no log-file writer exists yet)", FlagNoLogFile, false},
	{"nosnapshot", "don't save a system snapshot (logs/*.snp) after scanning", FlagNoSnapshot, false},
	{"reindex", "rebuild every driver pack's index from its own .7z, even if a valid one already exists", FlagForceReindexing, false},
	{"index-hr", "also write a human-readable text index alongside any index (re)built this run", FlagPrintIndex, false},
}

// FlagOption is one entry of FlagOptions, for a front end (e.g. a TUI
// options screen) to list and toggle every registered engine flag
// without needing to know boolFlagDefs' internal layout.
type FlagOption struct {
	Name    string
	Help    string
	Bit     Flags
	Persist bool // whether this flag round-trips through sdio.cfg
}

// FlagOptions lists every registered boolean engine flag (the same
// set FlagSet/Save use), for building a settings UI.
func FlagOptions() []FlagOption {
	out := make([]FlagOption, len(boolFlagDefs))
	for i, d := range boolFlagDefs {
		out[i] = FlagOption{Name: d.name, Help: d.help, Bit: d.flag, Persist: d.persist}
	}
	return out
}
