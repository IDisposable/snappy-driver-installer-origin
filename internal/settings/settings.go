// Package settings holds directories, behavior flags, result filters, and
// current-format configuration parsing.
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
	// UpdatesDir is where a torrent download's file data lands while
	// in progress, before a completed file is moved into DrpDir/
	// IndexDir. Keeping this directory persistent lets interrupted downloads
	// resume after the process exits.
	UpdatesDir           string
	LogDirRaw            string // as configured, may contain %VAR% references
	LogDir               string // LogDirRaw with environment variables expanded
	ExtractDirRaw        string // as configured, may contain %VAR% references
	ExtractDir           string // ExtractDirRaw with environment variables expanded
	MaxExtractFileBytes  uint64
	MaxExtractTotalBytes uint64

	StateFile          string
	DeviceListFilename string

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

	// TorrentFile is a user-selected local .torrent path, magnet URI, or
	// HTTPS URL. An empty value selects the embedded torrent.
	TorrentFile string
}

// DynamicTorrentURL is the mutable torrent source selected by -torrent-file=*.
const DynamicTorrentURL = "https://github.com/IDisposable/snappy-driver-installer-origin/raw/refs/heads/main/seed/SDIO_Update.torrent"

// TorrentSource returns a user-selected source, the dynamic source for
// a literal "*", or an empty string for the embedded torrent.
func (s *Settings) TorrentSource() string {
	if s.TorrentFile == "*" {
		return DynamicTorrentURL
	}
	if s.TorrentFile != "" {
		return s.TorrentFile
	}
	return ""
}

// TorrentSourceKind identifies the selected torrent source for logs.
func (s *Settings) TorrentSourceKind() string {
	if s.TorrentFile == "*" {
		return "dynamic"
	}
	if s.TorrentFile != "" {
		return "user"
	}
	return "embedded"
}

// HasTorrentSource reports that the embedded torrent is always available.
func (s *Settings) HasTorrentSource() bool { return true }

// New returns Settings with the default application layout.
func New() *Settings {
	return &Settings{
		DrpDir:               "drivers",
		IndexDir:             "indexes",
		OutputDir:            filepath.Join("indexes", "txt"),
		UpdatesDir:           "updates",
		LogDirRaw:            "logs",
		ExtractDirRaw:        `%TEMP%\SDIO`,
		MaxExtractFileBytes:  512 << 20,
		MaxExtractTotalBytes: 4 << 30,
		StateFile:            "untitled.snp",
		StateMode:            StateModeReal,
		Filters:              DefaultFilters,
	}
}

// MarshalZerologObject renders the settings relevant to a run as
// structured log fields, replacing Settings_t::loginfo().
func (s *Settings) MarshalZerologObject(e *zerolog.Event) {
	e.Str("drp_dir", s.DrpDir).
		Str("index_dir", s.IndexDir).
		Str("output_dir", s.OutputDir).
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
