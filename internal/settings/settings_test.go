package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// chdir changes to dir for the duration of the test, restoring the
// original working directory in cleanup.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

func TestLoadDefaultCfgMissingFileIsNotAnError(t *testing.T) {
	chdir(t, t.TempDir())

	s := New()
	if err := s.LoadDefaultCfg(); err != nil {
		t.Fatalf("LoadDefaultCfg() error = %v, want nil when sdio.cfg doesn't exist", err)
	}
}

// TestLoadDefaultCfgRealFile loads a current-format sdio.cfg and confirms it applies.
func TestLoadDefaultCfgRealFile(t *testing.T) {
	dir := t.TempDir()
	cfg := "-drp-dir=drivers\n-index-dir=\"indexes\\SDI\"\n-filters=531\n"
	if err := os.WriteFile(filepath.Join(dir, DefaultCfgFilename), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	s := New()
	if err := s.LoadDefaultCfg(); err != nil {
		t.Fatalf("LoadDefaultCfg() error: %v", err)
	}
	if s.DrpDir != "drivers" {
		t.Errorf("DrpDir = %q, want %q", s.DrpDir, "drivers")
	}
	if s.IndexDir != `indexes\SDI` {
		t.Errorf("IndexDir = %q, want %q", s.IndexDir, `indexes\SDI`)
	}
	if s.Filters != 531 {
		t.Errorf("Filters = %d, want 531", s.Filters)
	}
}

func TestFilterBitsUseCurrentSequentialLayout(t *testing.T) {
	want := []FilterShow{
		FilterMissing, FilterNewer, FilterCurrent, FilterOld,
		FilterBetter, FilterWorseRank, FilterNFMissing, FilterNFUnknown,
		FilterNFStandard, FilterOne, FilterDup, FilterInvalid,
	}
	for i, bit := range want {
		if bit != FilterShow(1<<i) {
			t.Errorf("filter bit %d = %#x, want %#x", i, bit, FilterShow(1<<i))
		}
	}
}

func TestParseVirtualOSVersionUnknownCode(t *testing.T) {
	s := New()
	if err := s.Parse([]string{"-virtual-os-version", "999"}); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if s.VirtualWindowsVersionName != "Unknown OS" {
		t.Errorf("VirtualWindowsVersionName = %q, want %q", s.VirtualWindowsVersionName, "Unknown OS")
	}
}

func TestParseFiltersp(t *testing.T) {
	s := New()
	if err := s.Parse([]string{"-filtersp"}); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if s.Flags&FlagFilterSP == 0 {
		t.Error("expected FlagFilterSP set")
	}
}

func TestParseUnknownFlagErrors(t *testing.T) {
	s := New()
	if err := s.Parse([]string{"-does-not-exist"}); err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
}

func TestToolsDataDirFlagRemoved(t *testing.T) {
	s := New()
	if err := s.Parse([]string{"-data-dir", `tools\SDIO`}); err == nil {
		t.Fatal("expected -data-dir to be removed")
	}
}

func TestParseExpandsLogDir(t *testing.T) {
	s := New()
	t.Setenv("SDIO_TEST_LOGROOT", "C:\\logs")
	if err := s.Parse([]string{"-log-dir", "%SDIO_TEST_LOGROOT%\\sdio"}); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	want := "C:\\logs\\sdio"
	if s.LogDir != want {
		t.Errorf("LogDir = %q, want %q", s.LogDir, want)
	}
}

func TestSaveLoadRoundTripsPersistentFlagsOnly(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sdigo.cfg")

	s := New()
	if err := s.Parse([]string{
		"-drp-dir", "custom-drivers",
		"-checkupdates", // persistent
		"-autoclose",    // not persistent
		"-filters", "9",
	}); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if err := s.Save(cfgPath); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded := New()
	// Point the ignore-list lookup at a hostname-based file that won't
	// exist in the temp dir; LoadFile propagates that error, so drive
	// the two halves separately here.
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("reading saved cfg: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty cfg file")
	}

	if err := loaded.Parse(splitArgLine(string(data))); err != nil {
		t.Fatalf("Parse(saved cfg) error: %v", err)
	}

	if loaded.DrpDir != "custom-drivers" {
		t.Errorf("DrpDir = %q, want %q", loaded.DrpDir, "custom-drivers")
	}
	if loaded.Flags&FlagCheckUpdates == 0 {
		t.Error("expected persistent FlagCheckUpdates to round-trip")
	}
	if loaded.Flags&FlagAutoClose != 0 {
		t.Error("expected non-persistent FlagAutoClose to NOT round-trip")
	}
	if loaded.Filters != 9 {
		t.Errorf("Filters = %d, want 9", loaded.Filters)
	}
}

func TestSaveSkipsWhenPreserveCfgSet(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sdigo.cfg")

	s := New()
	s.Flags |= FlagPreserveCfg
	if err := s.Save(cfgPath); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatal("expected no cfg file to be written when FlagPreserveCfg is set")
	}
}

func TestSplitArgLineHandlesQuotedSpaces(t *testing.T) {
	got := splitArgLine(`-drp-dir="C:\Program Files\drivers" -checkupdates`)
	want := []string{`-drp-dir=C:\Program Files\drivers`, "-checkupdates"}
	if len(got) != len(want) {
		t.Fatalf("splitArgLine() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTorrentSourceDefaultsToEmbedded(t *testing.T) {
	s := New()
	if s.TorrentFile != "" {
		t.Fatalf("TorrentFile = %q, want empty for embedded source", s.TorrentFile)
	}
}

func TestTorrentSourceSelection(t *testing.T) {
	cases := []struct {
		name string
		file string
		want string
	}{
		{name: "embedded", want: ""},
		{name: "dynamic", file: "*", want: DynamicTorrentURL},
		{name: "user", file: `D:\custom\updates.torrent`, want: `D:\custom\updates.torrent`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			s.TorrentFile = tc.file
			if got := s.TorrentSource(); got != tc.want {
				t.Fatalf("TorrentSource() = %q, want %q", got, tc.want)
			}
		})
	}
}
