package update

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
)

// realTorrentPath is a real cached SDIO update torrent from a
// production installation (multi-file: driver packs, indexes, app
// binaries, docs, tools), used to confirm this package's file-listing
// and selection logic against real, non-trivial metainfo rather than a
// synthetic single-file torrent.
const realTorrentPath = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/torrent/SDIO_Update.torrent"

// sharedTorrent is built once via TestMain and reused by every test in
// this file that only reads metadata (file list/lengths). Building one
// torrent.Client per test - each adding the same real, ~400-file
// torrent - was observed to make the process hang on exit (the test
// binary never returns after the last test completes, requiring a
// SIGTERM); a single shared instance avoids whatever cross-instance
// contention causes that, and building it once instead of per-test is
// the right call anyway (real metainfo parsing over ~400 files isn't
// free).
var sharedTorrent *Torrent

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sdio-update-test-")
	if err != nil {
		panic(err)
	}

	tc := torrent.NewDefaultClientConfig()
	tc.DataDir = dir
	tc.DisableTrackers = true
	tc.NoDHT = true
	tc.DisableWebseeds = true
	tc.DisableTCP = true
	tc.DisableUTP = true
	tc.NoDefaultPortForwarding = true
	tc.ListenPort = 0

	cl, err := torrent.NewClient(tc)
	if err != nil {
		panic(err)
	}
	c := &Client{cl: cl}

	tr, err := c.AddFromFile(realTorrentPath)
	if err == nil {
		sharedTorrent = tr
	}
	// If the real torrent file isn't present (no reference installation
	// on this machine), sharedTorrent stays nil and every test using it
	// skips - see requireSharedTorrent.

	code := m.Run()
	c.Close()
	os.RemoveAll(dir)
	os.Exit(code)
}

func requireSharedTorrent(t *testing.T) *Torrent {
	t.Helper()
	if sharedTorrent == nil {
		t.Skipf("real torrent file not available at %s", realTorrentPath)
	}
	return sharedTorrent
}

func TestAddFromFileListsRealFiles(t *testing.T) {
	tr := requireSharedTorrent(t)

	if tr.Name() != "SDIO_Update" {
		t.Errorf("Name() = %q, want %q", tr.Name(), "SDIO_Update")
	}

	files := tr.Files()
	if len(files) == 0 {
		t.Fatal("Files() returned no files")
	}

	const wantPath = "SDIO_Update/docs/changelog.txt"
	const wantLength = 24331
	found := false
	for _, f := range files {
		if f.Path == wantPath {
			found = true
			if f.Length != wantLength {
				t.Errorf("%s length = %d, want %d", wantPath, f.Length, wantLength)
			}
		}
	}
	if !found {
		t.Errorf("expected to find %q among %d files", wantPath, len(files))
	}
	t.Logf("torrent has %d files", len(files))
}

func TestSelectFilesReturnsOnlyRequested(t *testing.T) {
	tr := requireSharedTorrent(t)

	want := []string{"SDIO_Update/docs/changelog.txt", "SDIO_Update/SDIO_auto.bat"}
	selected := tr.SelectFiles(want)
	if len(selected) != len(want) {
		t.Fatalf("SelectFiles() returned %d files, want %d", len(selected), len(want))
	}

	gotPaths := map[string]bool{}
	for _, f := range selected {
		gotPaths[f.Path] = true
	}
	for _, w := range want {
		if !gotPaths[w] {
			t.Errorf("SelectFiles() result missing %q", w)
		}
	}
}

func TestSelectFilesIgnoresUnknownNames(t *testing.T) {
	tr := requireSharedTorrent(t)

	selected := tr.SelectFiles([]string{"this/file/does/not/exist.txt"})
	if len(selected) != 0 {
		t.Errorf("SelectFiles() with an unknown name returned %d files, want 0", len(selected))
	}
}

func TestSaveFileMovesContentAndCreatesDestDir(t *testing.T) {
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "source.bin")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "nested", "dest.bin")
	if err := SaveFile(src, dest); err != nil {
		t.Fatalf("SaveFile() error: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading dest: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("dest content = %q, want %q", got, "hello")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source file should no longer exist after SaveFile")
	}
}

// TestSaveFileRetriesUntilSourceBecomesAvailable exercises the retry
// path added for a real, empirically observed failure: the torrent
// client can still hold src open for a moment after a download
// reaches 100%, so the first rename attempt fails even though the
// data itself is already complete and correct. A held file lock can't
// be reproduced portably in a test, so this drives the same "not
// available yet, then available" shape via a source that doesn't
// exist until partway through the retry window - what matters here is
// that SaveFile keeps trying instead of failing on the first attempt.
func TestSaveFileRetriesUntilSourceBecomesAvailable(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.bin")
	dest := filepath.Join(t.TempDir(), "dest.bin")

	go func() {
		time.Sleep(saveFileRetryDelay + 50*time.Millisecond)
		os.WriteFile(src, []byte("hello"), 0o644)
	}()

	if err := SaveFile(src, dest); err != nil {
		t.Fatalf("SaveFile() error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "hello" {
		t.Errorf("dest content = %q (err=%v), want %q, nil", got, err, "hello")
	}
}

// TestSaveFileGivesUpAfterRetriesExhausted confirms a source that
// never appears still returns an error eventually, rather than
// retrying forever.
func TestSaveFileGivesUpAfterRetriesExhausted(t *testing.T) {
	src := filepath.Join(t.TempDir(), "never-created.bin")
	dest := filepath.Join(t.TempDir(), "dest.bin")

	start := time.Now()
	if err := SaveFile(src, dest); err == nil {
		t.Fatal("SaveFile() error = nil, want an error for a source that never appears")
	}
	if elapsed := time.Since(start); elapsed < saveFileRetryDelay {
		t.Errorf("SaveFile() returned after %v, want it to have retried for at least one delay interval", elapsed)
	}
}

func TestProgressPercent(t *testing.T) {
	cases := []struct {
		completed, total int64
		want             int
	}{
		{0, 100, 0},
		{50, 100, 50},
		{100, 100, 100},
		{0, 0, 100}, // no files selected counts as complete
	}
	for _, c := range cases {
		p := Progress{Completed: c.completed, Total: c.total}
		if got := p.Percent(); got != c.want {
			t.Errorf("Progress{%d,%d}.Percent() = %d, want %d", c.completed, c.total, got, c.want)
		}
	}
}

func TestFileProgressPercent(t *testing.T) {
	cases := []struct {
		completed, total int64
		want             int
	}{
		{0, 100, 0},
		{25, 100, 25},
		{100, 100, 100},
		{0, 0, 100},
	}
	for _, c := range cases {
		f := FileProgress{Completed: c.completed, Total: c.total}
		if got := f.Percent(); got != c.want {
			t.Errorf("FileProgress{%d,%d}.Percent() = %d, want %d", c.completed, c.total, got, c.want)
		}
	}
}

// TestProgressReportsPerFileBreakdown confirms Progress.Files carries
// one entry per selected file (not just the Completed/Total
// aggregate), in the same order the files were selected in - needed
// for a caller to show individual file progress when many files
// download together.
func TestProgressReportsPerFileBreakdown(t *testing.T) {
	tr := requireSharedTorrent(t)

	want := []string{"SDIO_Update/docs/changelog.txt", "SDIO_Update/SDIO_auto.bat"}
	selected := tr.SelectFiles(want)

	p := tr.Progress(selected)
	if len(p.Files) != len(want) {
		t.Fatalf("Progress().Files has %d entries, want %d", len(p.Files), len(want))
	}
	for i, f := range p.Files {
		if f.Path != selected[i].Path {
			t.Errorf("Files[%d].Path = %q, want %q", i, f.Path, selected[i].Path)
		}
		if f.Total != selected[i].Length {
			t.Errorf("Files[%d].Total = %d, want %d", i, f.Total, selected[i].Length)
		}
	}
}
