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
var boolFlagDefs = []boolFlagDef{
	{"checkupdates", "check for driver pack updates", FlagCheckUpdates, true},
	{"onlyupdates", "only download updates, don't install", FlagOnlyUpdates, true},
	{"torrentalerts", "log torrent alert events", FlagTorrentAlerts, true},
	{"norestorepnt", "don't create a system restore point", FlagNoRestorePoint, true},
	{"novirusalerts", "don't warn about suspected virus-flagged files", FlagNoVirusAlerts, true},
	{"keepunpackedindex", "keep unpacked index files after use", FlagKeepUnpackIndex, true},

	{"preservecfg", "don't overwrite sdio.cfg on exit", FlagPreserveCfg, false},
	{"nogui", "run headless, without an interactive front end", FlagNoGUI, false},
	{"autoinstall", "install matched drivers without prompting", FlagAutoInstall, false},
	{"autoclose", "exit automatically once finished", FlagAutoClose, false},
	{"autoupdate", "update driver packs automatically", FlagAutoUpdate, false},
	{"nostop", "don't stop if creating a restore point fails", FlagNoStop, false},
	{"keeptempfiles", "don't delete temporary extraction files", FlagKeepTempFiles, false},
	{"disableinstall", "scan and match only, never install", FlagDisableInstall, false},
	{"failsafe", "run in fail-safe mode", FlagFailSafe, false},
	{"delextrainfs", "delete extra .inf files after install", FlagDelExtraInfs, false},
	{"nologfile", "don't write a log file", FlagNoLogFile, false},
	{"nosnapshot", "don't save a system snapshot", FlagNoSnapshot, false},
	{"nostamp", "don't timestamp log file names", FlagNoStamp, false},
	{"reindex", "force driver pack reindexing", FlagForceReindexing, false},
	{"index-hr", "write a human-readable index alongside the binary one", FlagPrintIndex, false},
}
