package installflow

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"sdio/internal/archive"
	"sdio/internal/collection"
	"sdio/internal/indexing"
	"sdio/internal/logging"
	"sdio/internal/sdwfile"
	"sdio/internal/settings"
)

// testLogger is a discard-everything Logger for tests that need to
// pass one but aren't exercising logging itself.
var testLogger = logging.New(zerolog.Disabled, nil)

// TestMain forces an explicit os.Exit after tests complete. A real
// torrent download (unlike internal/update's offline-only tests)
// leaves the test process hanging well past when tests report PASS -
// same class of shutdown quirk noted in internal/update's test file,
// but this time not resolved by it (this file only ever creates one
// client for one test, so it isn't the "multiple clients" cause
// suspected there). Production cmd/sdigo is unaffected either way: its
// real main() already calls os.Exit(mainErr()) unconditionally.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// realDtPortInstall builds a real, fully-usable Pending from the
// reference installation's real dtport index/archive, for exercising
// InstallOne's flag-gating branches without needing a synthetic
// index.
func realDtPortInstall(t *testing.T) Pending {
	t.Helper()
	const indexPath = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI/DP_Ports_SDIO01_26083.bin"
	const packDir = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/drivers"
	const packFilename = "DP_Ports_SDIO01_26083.7z"

	f, err := os.Open(indexPath)
	if err != nil {
		t.Skipf("real index file not available at %s: %v", indexPath, err)
	}
	defer f.Close()
	_, payload, err := sdwfile.Decode(f, true)
	if err != nil {
		t.Fatalf("Decode() error: %v", err)
	}
	idx, err := indexing.DecodeIndex(payload)
	if err != nil {
		t.Fatalf("DecodeIndex() error: %v", err)
	}
	drp := &indexing.Driverpack{Path: packDir, Filename: packFilename, Index: idx}

	wantHWID := `DTBUS\COMPORT&VID_37DD&PID_6001`
	for i := range idx.HWIDs {
		if strings.EqualFold(drp.HWID(i), wantHWID) {
			return Pending{
				Description: "test: dtport",
				Candidate:   collection.Candidate{Driverpack: drp, HWIDIndex: i},
			}
		}
	}
	t.Fatalf("HWID %q not found in %s", wantHWID, indexPath)
	return Pending{}
}

// TestInstallOneExtractOnlyModeSkipsInstall confirms -extractdir's
// documented "also switches to extract-only mode (no install)"
// behavior (FlagExtractOnly) is actually honored: InstallOne extracts
// but must return nil without ever reaching install.Driver. On this
// (non-Windows) test machine, install.Driver always returns an
// "unsupported platform" error (see install_other.go), so reaching it
// would make this test fail - a real, cross-platform way to prove the
// call was skipped without mocking.
func TestInstallOneExtractOnlyModeSkipsInstall(t *testing.T) {
	p := realDtPortInstall(t)
	s := settings.New()
	s.ExtractDir = t.TempDir()
	s.Flags |= settings.FlagExtractOnly

	if err := InstallOne(s, p, io.Discard); err != nil {
		t.Fatalf("InstallOne() with FlagExtractOnly error: %v, want nil (install.Driver must not be reached)", err)
	}
}

// TestInstallOneDisableInstallSkipsInstall is the same proof for
// FlagDisableInstall (already wired before this test existed;
// included for symmetry with the extract-only case above).
func TestInstallOneDisableInstallSkipsInstall(t *testing.T) {
	p := realDtPortInstall(t)
	s := settings.New()
	s.ExtractDir = t.TempDir()
	s.Flags |= settings.FlagDisableInstall

	if err := InstallOne(s, p, io.Discard); err != nil {
		t.Fatalf("InstallOne() with FlagDisableInstall error: %v, want nil", err)
	}
}

// TestInstallOneNormalModeReachesInstallDriver confirms that, absent
// both skip flags, InstallOne actually attempts the install (and thus
// fails with the platform-unsupported error here, proving it got that
// far) - a regression guard against accidentally adding another early
// return that silently skips installing.
func TestInstallOneNormalModeReachesInstallDriver(t *testing.T) {
	p := realDtPortInstall(t)
	s := settings.New()
	s.ExtractDir = t.TempDir()

	err := InstallOne(s, p, io.Discard)
	if err == nil {
		t.Fatal("InstallOne() with no skip flags returned nil, want the platform-unsupported error from install.Driver")
	}
}

// TestRunAbortsWhenRestorePointFailsAndNotNoStop confirms Run refuses
// to install anything if the restore point it attempted failed,
// ported from install.cpp's "if restore point was selected and failed
// then abort" guard - install.CreateRestorePoint always fails with
// the platform-unsupported error on this dev machine (see
// internal/install's !windows stub), which doubles as a real,
// non-mocked way to force that failure path here.
func TestRunAbortsWhenRestorePointFailsAndNotNoStop(t *testing.T) {
	p := realDtPortInstall(t)
	s := settings.New()
	s.ExtractDir = t.TempDir()

	var buf strings.Builder
	if ok := Run(s, []Pending{p}, &buf, nil, nil, testLogger); ok {
		t.Error("Run() = true, want false for an aborted install")
	}

	out := buf.String()
	if !strings.Contains(out, "install aborted") {
		t.Errorf("Run() output = %q, want it to report the install was aborted", out)
	}
	if strings.Contains(out, "INSTALLED") {
		t.Error("Run() should not have reached InstallOne's success path")
	}
}

// TestRunProceedsWhenNoStopSet confirms -nostop overrides the abort
// above - InstallOne is reached (and fails on its own platform-
// unsupported error, proving it got that far, not that it succeeded).
func TestRunProceedsWhenNoStopSet(t *testing.T) {
	p := realDtPortInstall(t)
	s := settings.New()
	s.ExtractDir = t.TempDir()
	s.Flags |= settings.FlagNoStop

	var buf strings.Builder
	if ok := Run(s, []Pending{p}, &buf, nil, nil, testLogger); ok {
		t.Error("Run() = true, want false - InstallOne still fails on this dev machine's platform-unsupported error")
	}

	out := buf.String()
	if strings.Contains(out, "install aborted") {
		t.Errorf("Run() output = %q, want -nostop to skip the abort", out)
	}
	if !strings.Contains(out, "install "+p.Description) {
		t.Errorf("Run() output = %q, want it to have reached InstallOne", out)
	}
}

// TestRunReturnsTrueWhenEverythingSucceeds confirms Run's return value
// is a real success signal, not just always-false because this dev
// machine can't create restore points or install drivers -
// -disableinstall skips both, leaving nothing to fail.
func TestRunReturnsTrueWhenEverythingSucceeds(t *testing.T) {
	p := realDtPortInstall(t)
	s := settings.New()
	s.ExtractDir = t.TempDir()
	s.Flags |= settings.FlagDisableInstall

	var buf strings.Builder
	if ok := Run(s, []Pending{p}, &buf, nil, nil, testLogger); !ok {
		t.Errorf("Run() = false with -disableinstall (skips the restore point and the real install), want true: %s", buf.String())
	}
}

// realTorrentPath is a real cached SDIO update torrent from a
// production installation (see internal/update's tests) - reused here
// to validate the actual download-and-move path DownloadPending
// implements, not just its unit pieces.
const realTorrentPath = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/torrent/SDIO_Update.torrent"

// TestDownloadPendingRealTorrent downloads a real driver pack
// (DP_CardReader_26072.7z, ~60MB - independently confirmed absent
// from the reference installation's drivers folder, i.e. actually
// pending, not already downloaded) from the real cached torrent, and
// confirms the result lands where InstallOne expects it and is a
// valid, openable .7z archive - not just "some bytes arrived".
//
// Skipped unless SDIO_TEST_REAL_TORRENT=1: real BitTorrent networking
// (unlike internal/update's offline-metadata-only tests) leaves the
// test process hanging 30-90s past when the test itself reports PASS,
// even with an explicit TestMain/os.Exit - suspected to be a
// go-test-harness-specific interaction with anacrolix/torrent's
// shutdown path (real `go run`/binary invocations of cmd/torrenttest
// earlier did not exhibit this), not a production issue, since real
// cmd/sdigo's main() calls os.Exit(mainErr()) as its literal last
// statement with no framework code after it to hang in. Not resolved;
// gated instead of fixed so routine `go test ./...` runs don't pay a
// ~60s tax for one real-network test.
func TestDownloadPendingRealTorrent(t *testing.T) {
	if os.Getenv("SDIO_TEST_REAL_TORRENT") != "1" {
		t.Skip("set SDIO_TEST_REAL_TORRENT=1 to run (real network download; see doc comment)")
	}
	if _, err := os.Stat(realTorrentPath); err != nil {
		t.Skipf("real torrent file not available at %s: %v", realTorrentPath, err)
	}

	destDir := t.TempDir()
	const packFilename = "DP_CardReader_26072.7z"

	if _, err := os.Stat(filepath.Join(destDir, packFilename)); err == nil {
		t.Fatal("test setup bug: destination file already exists before download")
	}

	s := settings.New()
	s.TorrentFile = realTorrentPath

	pending := []Pending{{
		Description: "test: " + packFilename,
		Candidate: collection.Candidate{
			Driverpack: &indexing.Driverpack{
				Path:     destDir,
				Filename: packFilename,
				Pending:  true,
			},
		},
	}}

	if err := DownloadPending(s, pending, io.Discard, nil, nil, testLogger); err != nil {
		t.Fatalf("DownloadPending() error: %v", err)
	}

	if pending[0].Candidate.Driverpack.Pending {
		t.Error("Driverpack.Pending should be cleared after a successful download")
	}

	destPath := filepath.Join(destDir, packFilename)
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("downloaded file not found at %s: %v", destPath, err)
	}
	if info.Size() == 0 {
		t.Fatal("downloaded file is empty")
	}
	t.Logf("downloaded %s: %d bytes", destPath, info.Size())

	r, err := archive.Open(destPath)
	if err != nil {
		t.Fatalf("downloaded file is not a valid .7z archive: %v", err)
	}
	defer r.Close()
	if len(r.Files()) == 0 {
		t.Error("downloaded archive has 0 files")
	}
}
