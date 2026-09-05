// Package update downloads driver packs via BitTorrent, using
// github.com/anacrolix/torrent instead of the original's bundled
// libtorrent glue.
//
// update.h declares Updater_t::torrent_url/torrent2_url (the .torrent
// metadata SDIO fetches to learn what's available) but never defines
// them anywhere in this codebase - they must be supplied at build time
// or by a private config not present in this source snapshot. This
// package therefore does not hardcode any tracker, webseed, or
// metadata-fetch URL: callers supply a torrent source (a local
// .torrent file, a magnet URI, or an http(s):// URL to fetch a
// .torrent file from) via AddFromSpec.
//
// The core behavior this package exists to preserve is the selective
// download model: SDIO's drivers, indexes, app binaries, and docs are
// all one shared multi-file torrent, and a normal run downloads only
// the specific driver-pack files a device match actually needs
// (Torrent.SelectFiles), not the whole multi-gigabyte collection.
// Confirmed against a real
// cached torrent/SDIO_Update.torrent from a production installation:
// a single-tracker-announce, multi-tracker-announce-list, HTTP-webseed
// multi-file torrent containing drivers/*.7z, indexes/**/*.bin, the
// app executables, and docs/tools.
package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	alog "github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"golang.org/x/time/rate"
)

// Config configures the torrent client. Outgoing port range and
// per-torrent connection limits aren't represented - anacrolix/torrent
// manages both internally.
type Config struct {
	DataDir      string // where downloaded file data lands (torrent_save_path's role)
	ListenPort   int    // 0 lets anacrolix/torrent pick a default
	DownloadKBps int    // 0 = unlimited
	UploadKBps   int    // 0 = unlimited
	Seed         bool   // keep serving pieces to other peers after completing (IsSeedingDrivers)

	// OnAlert, if non-nil, is called for every Warning-or-higher event
	// the torrent client itself logs (peer/tracker/webseed errors, not
	// this package's own operations) - ported from Updater_t's alert
	// handling (FLAG_TORRENTALERTS). Left unset, these are silently
	// discarded rather than left at anacrolix/torrent's own default of
	// writing straight to stderr, which would corrupt a caller that
	// owns the whole terminal (e.g. a TUI's alternate screen buffer).
	OnAlert func(level, message string)
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
	tc.Logger = alog.NewLogger()
	tc.Logger.SetHandlers(alertHandler{cfg.OnAlert})

	cl, err := torrent.NewClient(tc)
	if err != nil {
		return nil, fmt.Errorf("creating torrent client: %w", err)
	}
	return &Client{cl: cl}, nil
}

// alertHandler routes anacrolix/torrent's own log records to OnAlert
// instead of its default stderr handler, dropping anything below
// Warning (routine debug/info chatter isn't an "alert").
type alertHandler struct {
	onAlert func(level, message string)
}

func (h alertHandler) Handle(r alog.Record) {
	if h.onAlert == nil || r.Level.LessThan(alog.Warning) {
		return
	}
	h.onAlert(r.Level.LogString(), r.String())
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

// AddFromURL adds a torrent from a .torrent metadata file fetched over
// HTTP(S), such as one published on a GitHub Pages/raw.githubusercontent.com
// URL - the way this rewrite points a fresh machine at a torrent source
// without shipping the .torrent file alongside the binary or requiring
// a magnet link. Its file list is available immediately, same as
// AddFromFile, since the whole .torrent is fetched up front rather
// than resolved from peers/trackers.
func (c *Client) AddFromURL(url string) (*Torrent, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching torrent %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching torrent %s: %s", url, resp.Status)
	}

	mi, err := metainfo.Load(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing torrent %s: %w", url, err)
	}
	t, err := c.cl.AddTorrent(mi)
	if err != nil {
		return nil, fmt.Errorf("adding torrent %s: %w", url, err)
	}
	return &Torrent{t: t}, nil
}

// AddFromSpec adds a torrent from spec, dispatching on its form: a
// magnet: URI (AddFromMagnet), an http(s):// URL (AddFromURL), or
// otherwise a local .torrent file path (AddFromFile) - the single
// place every Settings.TorrentFile-driven call site should resolve a
// torrent source through, so the three forms stay in sync instead of
// each caller repeating its own prefix check.
func (c *Client) AddFromSpec(spec string) (*Torrent, error) {
	switch {
	case strings.HasPrefix(spec, "magnet:"):
		return c.AddFromMagnet(spec)
	case strings.HasPrefix(spec, "http://"), strings.HasPrefix(spec, "https://"):
		return c.AddFromURL(spec)
	default:
		return c.AddFromFile(spec)
	}
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
// download and explicitly skips every other file - the selective
// download model this package exists to preserve (see the package doc
// comment). Returns the FileInfo values selected, in torrent file
// order, for progress tracking.
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

// FileProgress is one file's own download progress, as opposed to
// Progress's aggregate across every selected file - needed once more
// than a handful of files download together (e.g. "Download All
// Driver Packs"), where an aggregate percent alone can sit unchanged
// for a long time while individual files actually finish and start.
type FileProgress struct {
	Path      string
	Completed int64
	Total     int64
}

// Percent returns Completed/Total as 0-100, or 100 if Total is 0.
func (f FileProgress) Percent() int {
	if f.Total == 0 {
		return 100
	}
	return int(f.Completed * 100 / f.Total)
}

// Progress sums BytesCompleted/Length across files. Label identifies
// what's being downloaded (e.g. a driver-pack
// filename), set by WaitDownload's caller - it carries no meaning on
// its own. Files carries the same breakdown per individual file, in
// the same order as the files argument Progress/WaitDownload were
// given.
type Progress struct {
	Label     string
	Completed int64
	Total     int64
	Files     []FileProgress
}

// Percent returns Completed/Total as 0-100, or 100 if Total is 0.
func (p Progress) Percent() int {
	if p.Total == 0 {
		return 100
	}
	return int(p.Completed * 100 / p.Total)
}

// Progress reports download progress across files (typically the
// slice SelectFiles returned), both as a Completed/Total aggregate and
// as Files, one entry per file.
func (t *Torrent) Progress(files []FileInfo) Progress {
	p := Progress{Files: make([]FileProgress, len(files))}
	for i, f := range files {
		c := f.BytesCompleted()
		p.Completed += c
		p.Total += f.Length
		p.Files[i] = FileProgress{Path: f.Path, Completed: c, Total: f.Length}
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

// saveFileRetries/saveFileRetryDelay bound how long SaveFile waits for
// a just-completed download's source file to become movable. The
// torrent client keeps a shared read-handle pool open past a piece's
// last write for verification/seeding (see anacrolix/torrent's
// storage/file-handle-cache.go), so a rename attempted the instant
// WaitDownload sees 100% can transiently fail on Windows with the
// data already fully correct on disk - confirmed against a real
// "Download All" run, where every affected file did succeed given a
// later retry.
const (
	saveFileRetries    = 10
	saveFileRetryDelay = 300 * time.Millisecond
)

// SaveFile relocates a downloaded file from a torrent client's data
// directory to its final destination, falling back to copy-then-
// remove if a direct rename fails (e.g. they're on different volumes -
// the torrent client's temporary data directory and a driver-pack/
// index directory need not be on the same drive). Both the rename and
// the copy's initial open retry past a transient "in use" failure -
// see saveFileRetries.
func SaveFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	var renameErr error
	for i := 0; i < saveFileRetries; i++ {
		if renameErr = os.Rename(src, dest); renameErr == nil {
			return nil
		}
		if i < saveFileRetries-1 {
			time.Sleep(saveFileRetryDelay)
		}
	}

	var in *os.File
	var openErr error
	for i := 0; i < saveFileRetries; i++ {
		if in, openErr = os.Open(src); openErr == nil {
			break
		}
		if i < saveFileRetries-1 {
			time.Sleep(saveFileRetryDelay)
		}
	}
	if openErr != nil {
		return fmt.Errorf("rename failed (%w) and source is still unreadable: %w", renameErr, openErr)
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
