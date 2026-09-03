package update

import (
	"os"
	"testing"

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

func TestProgressPercent(t *testing.T) {
	cases := []struct {
		completed, total int64
		want              int
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
