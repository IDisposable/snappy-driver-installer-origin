package settings

// Flags is a bitmask of engine behavior toggles, ported from the FLAG_*
// and COLLECTION_* enum in settings.h. Bit positions are not preserved
// from the original (they were tied to unrelated GUI menu-item IDs);
// nothing depends on their numeric value.
type Flags uint64

const (
	FlagForceReindexing Flags = 1 << iota
	FlagUseLZMA
	FlagPrintIndex
	FlagNoGUI
	FlagCheckUpdates
	FlagDisableInstall
	FlagAutoInstall
	FlagFailSafe
	FlagAutoClose
	FlagNoRestorePoint
	FlagNoLogFile
	FlagNoSnapshot
	FlagNoStamp
	FlagNoVirusAlerts
	FlagPreserveCfg
	FlagKeepUnpackIndex
	FlagKeepTempFiles
	FlagDPInstMode
	FlagDelExtraInfs
	FlagOnlyUpdates
	FlagAutoUpdate
	FlagFilterSP
	FlagTorrentAlerts
	FlagKeepSeeding
	FlagNoStop
	FlagExtractOnly
	FlagScriptMode
	FlagUpdatesOK
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
	{"torrentalerts", "log torrent alert events (not implemented - no alert-logging feature exists yet)", FlagTorrentAlerts, true},
	{"keepseeding", "keep seeding driver packs to other peers after download completes", FlagKeepSeeding, true},
	{"norestorepnt", "don't create a system restore point", FlagNoRestorePoint, true},
	{"novirusalerts", "don't warn about suspected virus-flagged files (not implemented - no virus-flagging feature exists yet)", FlagNoVirusAlerts, true},
	{"keepunpackedindex", "keep unpacked index files after use (not implemented - the index write path isn't ported yet)", FlagKeepUnpackIndex, true},

	{"preservecfg", "don't overwrite sdio.cfg on exit", FlagPreserveCfg, false},
	{"nogui", "run headless, without an interactive front end", FlagNoGUI, false},
	{"autoinstall", "install matched drivers without prompting (not implemented - the TUI always asks for explicit confirmation before installing)", FlagAutoInstall, false},
	{"autoclose", "exit automatically once finished (not implemented)", FlagAutoClose, false},
	{"autoupdate", "update driver packs automatically (not implemented)", FlagAutoUpdate, false},
	{"nostop", "don't stop if creating a restore point fails (not implemented - a restore-point failure never stops the install here regardless)", FlagNoStop, false},
	{"keeptempfiles", "don't delete temporary extraction files (not implemented - nothing deletes them yet either way)", FlagKeepTempFiles, false},
	{"disableinstall", "scan and match only: never install, and never create a restore point", FlagDisableInstall, false},
	{"failsafe", "run in fail-safe mode (not implemented - not needed by this rewrite's registry-scan port so far)", FlagFailSafe, false},
	{"delextrainfs", "delete extra .inf files after install", FlagDelExtraInfs, false},
	{"nologfile", "don't write a log file (not implemented - no log-file writer exists yet)", FlagNoLogFile, false},
	{"nosnapshot", "don't save a system snapshot (not implemented - no snapshot writer exists yet)", FlagNoSnapshot, false},
	{"nostamp", "don't timestamp log file names (not implemented - no log-file writer exists yet)", FlagNoStamp, false},
	{"reindex", "force driver pack reindexing (not implemented - the index write path isn't ported yet)", FlagForceReindexing, false},
	{"index-hr", "write a human-readable index alongside the binary one (not implemented - the index write path isn't ported yet)", FlagPrintIndex, false},
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
