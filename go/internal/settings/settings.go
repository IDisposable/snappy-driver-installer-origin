// Package settings holds engine-level configuration: directories, the
// finish-command hooks, behavior flags, and result filters. Ported from
// settings.cpp/.h, with GUI presentation state (theme, window geometry,
// scale, hint delay, license, expert mode) dropped since this rewrite
// has no windowed GUI to configure.
package settings

import (
	"path/filepath"

	"github.com/rs/zerolog"
)

// StateMode selects whether the engine scans real hardware or replays a
// saved system snapshot.
type StateMode int

const (
	StateModeReal StateMode = iota
	StateModeEmul
	StateModeExit
)

// Settings holds the engine's configuration, normally populated by
// New() defaults, then Load or Parse.
type Settings struct {
	DrpDir    string
	IndexDir  string
	OutputDir string
	DrpExtDir string
	DataDir   string
	// UpdatesDir is where a torrent download's file data lands while
	// in progress, before a completed file is moved into DrpDir/
	// IndexDir - the original engine used a dedicated "updates"
	// staging directory the same way, rather than a throwaway temp
	// directory, so an interrupted download resumes instead of
	// restarting from zero next run (the torrent client verifies
	// already-written pieces against the torrent's own metainfo,
	// which a fresh directory can never have).
	UpdatesDir    string
	LogDirRaw     string // as configured, may contain %VAR% references
	LogDir        string // LogDirRaw with environment variables expanded
	ExtractDirRaw string // as configured, may contain %VAR% references
	ExtractDir    string // ExtractDirRaw with environment variables expanded

	StateFile          string
	DeviceListFilename string

	FinishCmd       string
	FinishRebootCmd string
	FinishUpdateCmd string

	Flags   Flags
	Filters FilterShow

	StateMode StateMode

	// VirtualOSVersion is the raw code from -v:<version> (e.g. 100 for
	// Windows 10); 0 means unset. VirtualWindowsVersionName is its
	// resolved display name via hardware.FindWindowsVersionName (always
	// a non-server lookup, matching the original's use of this switch),
	// or empty if VirtualOSVersion is unset.
	VirtualOSVersion          int
	VirtualWindowsVersionName string
	VirtualArchType           int

	IgnoreList []string

	// TorrentFile is a local .torrent file path or magnet URI to fetch
	// pending (not-yet-downloaded) driver packs from - see
	// collection.LoadOnlineIndexes and docs/PORTING_NOTES.md's
	// update.cpp entry. Empty means torrent downloads are disabled; no
	// tracker/webseed/metadata-fetch URL is hardcoded here, so this
	// must be supplied explicitly.
	TorrentFile string
}

// New returns Settings populated with the same defaults as the original
// Settings_t constructor.
func New() *Settings {
	return &Settings{
		DrpDir:        "drivers",
		IndexDir:      "indexes",
		OutputDir:     filepath.Join("indexes", "txt"),
		DataDir:       filepath.Join("tools", "SDIO"),
		UpdatesDir:    "updates",
		LogDirRaw:     "logs",
		ExtractDirRaw: `%TEMP%\SDIO`,
		StateFile:     "untitled.snp",
		Flags:         FlagUseLZMA,
		StateMode:     StateModeReal,
		Filters:       DefaultFilters,
	}
}

// MarshalZerologObject renders the settings relevant to a run as
// structured log fields, replacing Settings_t::loginfo().
func (s *Settings) MarshalZerologObject(e *zerolog.Event) {
	e.Str("drp_dir", s.DrpDir).
		Str("index_dir", s.IndexDir).
		Str("output_dir", s.OutputDir).
		Str("data_dir", s.DataDir).
		Str("updates_dir", s.UpdatesDir).
		Str("log_dir", s.LogDir).
		Uint64("filters", uint64(s.Filters)).
		Uint64("flags", uint64(s.Flags))

	if s.StateMode == StateModeEmul {
		e.Str("state_file", s.StateFile)
	}
	if s.VirtualArchType != 0 {
		e.Int("virtual_arch_type", s.VirtualArchType)
	}
	if s.VirtualOSVersion != 0 {
		e.Int("virtual_os_version", s.VirtualOSVersion).Str("virtual_windows_version_name", s.VirtualWindowsVersionName)
	}
}
