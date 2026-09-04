// Package update downloads driver packs via BitTorrent, ported from
// the torrent-handling parts of update.cpp/update.h, using
// github.com/anacrolix/torrent instead of porting the bundled
// libtorrent glue (see go/README.md).
//
// update.h declares Updater_t::torrent_url/torrent2_url (the .torrent
// metadata SDIO fetches to learn what's available) but never defines
// them anywhere in this codebase - they must be supplied at build time
// or by a private config not present in this source snapshot. This
// package therefore does not hardcode any tracker, webseed, or
// metadata-fetch URL: callers supply a torrent source (a local
// .torrent file or a magnet URI) via AddFromFile/AddFromMagnet.
//
// The core behavior this package exists to preserve is
// StartInstallDownload/EndInstallDownload's selective download model:
// SDIO's drivers, indexes, app binaries, and docs are all one shared
// multi-file torrent, and a normal run downloads only the specific
// driver-pack files a device match actually needs (Torrent.SelectFiles),
// not the whole multi-gigabyte collection. Confirmed against a real
// cached torrent/SDIO_Update.torrent from a production installation:
// a single-tracker-announce, multi-tracker-announce-list, HTTP-webseed
// multi-file torrent containing drivers/*.7z, indexes/**/*.bin, the
// app executables, and docs/tools.
package update

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/anacrolix/torrent"
	"golang.org/x/time/rate"
)

// Config configures the torrent client, ported from Updater_t's
// static settings (torrentport, downlimit, uplimit) in update.h.
// outgoingport_min/outgoingport_max and connections aren't
// represented - anacrolix/torrent manages outgoing ports and
// per-torrent connection limits internally.
type Config struct {
	DataDir      string // where downloaded file data lands (torrent_save_path's role)
	ListenPort   int    // 0 lets anacrolix/torrent pick a default
	DownloadKBps int    // 0 = unlimited
	UploadKBps   int    // 0 = unlimited
	Seed         bool   // keep serving pieces to other peers after completing (IsSeedingDrivers)
}

// Client wraps *torrent.Client.
type Client struct {
	cl *torrent.Client
}

// NewClient starts a torrent client with the given configuration.
func NewClient(cfg Config) (*Client, error) {
	tc := torrent.NewDefaultClientConfig()
	tc.DataDir = cfg.DataDir
	if cfg.ListenPort != 0 {
		tc.ListenPort = cfg.ListenPort
	}
	tc.Seed = cfg.Seed
	if cfg.DownloadKBps > 0 {
		tc.DownloadRateLimiter = rate.NewLimiter(rate.Limit(cfg.DownloadKBps*1024), 256<<10)
	}
	if cfg.UploadKBps > 0 {
		tc.UploadRateLimiter = rate.NewLimiter(rate.Limit(cfg.UploadKBps*1024), 256<<10)
	}

	cl, err := torrent.NewClient(tc)
	if err != nil {
		return nil, fmt.Errorf("creating torrent client: %w", err)
	}
	return &Client{cl: cl}, nil
}

// Close shuts down the client and every torrent it's managing.
func (c *Client) Close() []error {
	return c.cl.Close()
}

// AddFromFile adds a torrent from a local .torrent metadata file, such
// as a cached torrent/SDIO_Update.torrent.
func (c *Client) AddFromFile(path string) (*Torrent, error) {
	t, err := c.cl.AddTorrentFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("adding torrent %s: %w", path, err)
	}
	return &Torrent{t: t}, nil
}

// AddFromMagnet adds a torrent from a magnet URI. Its file list isn't
// available until WaitInfo returns (the magnet URI alone doesn't carry
// the file list - that comes from peers/trackers).
func (c *Client) AddFromMagnet(uri string) (*Torrent, error) {
	t, err := c.cl.AddMagnet(uri)
	if err != nil {
		return nil, fmt.Errorf("adding magnet: %w", err)
	}
	return &Torrent{t: t}, nil
}

// Torrent wraps *torrent.Torrent with SDIO's selective per-file
// download model.
type Torrent struct {
	t *torrent.Torrent
}

// WaitInfo blocks until the torrent's file list is available. Always
// returns immediately for a torrent added via AddFromFile (the
// .torrent file already carries it); needed after AddFromMagnet before
// calling Files/SelectFiles.
func (t *Torrent) WaitInfo(ctx context.Context) error {
	select {
	case <-t.t.GotInfo():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Name returns the torrent's display name (e.g. "SDIO_Update").
func (t *Torrent) Name() string {
	return t.t.Name()
}

// FileInfo describes one file inside a torrent and lets a caller
// select or deselect it for download.
type FileInfo struct {
	Path   string
	Length int64
	file   *torrent.File
}

// BytesCompleted returns how much of this file has downloaded so far.
func (f FileInfo) BytesCompleted() int64 {
	return f.file.BytesCompleted()
}

// Download marks this file for download.
func (f FileInfo) Download() {
	f.file.Download()
}

// Skip deprioritizes this file so it is not downloaded.
func (f FileInfo) Skip() {
	f.file.SetPriority(torrent.PiecePriorityNone)
}

// Files lists every file this torrent's metainfo describes. Every file
// starts deselected (PiecePriorityNone is the zero value) until
// Download or SelectFiles is called on it - nothing downloads by
// default, matching StartInstallDownload's selective model rather than
// fetching the whole shared torrent.
func (t *Torrent) Files() []FileInfo {
	files := t.t.Files()
	out := make([]FileInfo, len(files))
	for i, f := range files {
		out[i] = FileInfo{Path: f.Path(), Length: f.Length(), file: f}
	}
	return out
}

// SelectFiles marks exactly the files whose Path is in names for
// download and explicitly skips every other file, ported from
// StartInstallDownload(filenames): SDIO downloads only the specific
// driver-pack files a device match actually needs from the one shared
// torrent, not the whole collection. Returns the FileInfo values
// selected, in torrent file order, for progress tracking.
func (t *Torrent) SelectFiles(names []string) []FileInfo {
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}

	var selected []FileInfo
	for _, f := range t.t.Files() {
		info := FileInfo{Path: f.Path(), Length: f.Length(), file: f}
		if want[f.Path()] {
			f.Download()
			selected = append(selected, info)
		} else {
			f.SetPriority(torrent.PiecePriorityNone)
		}
	}
	return selected
}

// Progress sums BytesCompleted/Length across files, matching the
// percent-complete calculation ShowProgress displays in update.cpp.
// Label identifies what's being downloaded (e.g. a driver-pack
// filename), set by WaitDownload's caller - it carries no meaning on
// its own.
type Progress struct {
	Label     string
	Completed int64
	Total     int64
}

// Percent returns Completed/Total as 0-100, or 100 if Total is 0.
func (p Progress) Percent() int {
	if p.Total == 0 {
		return 100
	}
	return int(p.Completed * 100 / p.Total)
}

// Progress reports download progress across files (typically the
// slice SelectFiles returned).
func (t *Torrent) Progress(files []FileInfo) Progress {
	var p Progress
	for _, f := range files {
		p.Completed += f.BytesCompleted()
		p.Total += f.Length
	}
	return p
}

// ProgressFunc receives live download progress, invoked from
// WaitDownload's poll loop - it's how a caller renders the same kind
// of live percent/speed status update.cpp's ShowProgress builds from
// libtorrent's torrent_status, instead of blocking silently. May be
// nil.
type ProgressFunc func(Progress)

// WaitDownload blocks until every file in files has fully downloaded,
// ctx is done, or timeout elapses (whichever comes first). label is
// attached to every Progress reported to onProgress, which may be nil.
func (t *Torrent) WaitDownload(ctx context.Context, files []FileInfo, timeout time.Duration, label string, onProgress ProgressFunc) error {
	report := func() Progress {
		p := t.Progress(files)
		p.Label = label
		if onProgress != nil {
			onProgress(p)
		}
		return p
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if report().Percent() >= 100 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out waiting for the download to complete")
}

// SaveFile relocates a downloaded file from a torrent client's data
// directory to its final destination, falling back to copy-then-
// remove if a direct rename fails (e.g. they're on different volumes -
// the torrent client's temporary data directory and a driver-pack/
// index directory need not be on the same drive).
func SaveFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dest); err == nil {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

// Drop removes the torrent from the client, ported from
// Updater_t::StopTorrent.
func (t *Torrent) Drop() {
	t.t.Drop()
}
