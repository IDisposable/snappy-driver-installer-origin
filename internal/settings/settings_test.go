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

// TestLoadDefaultCfgRealFile loads a real sdio.cfg (quoted legacy
// switches, unquoted bare switches, blank lines) from a real
// installation and confirms it applies, matching main()'s
// Settings.load(L"sdio.cfg") startup step.
func TestLoadDefaultCfgRealFile(t *testing.T) {
	dir := t.TempDir()
	cfg := "\"-drp_dir:drivers\"\n\"-index_dir:indexes\\SDI\"\n-filters:1062\n-expertmode -showconsole\n"
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
	if s.Filters != 1062 {
		t.Errorf("Filters = %d, want 1062", s.Filters)
	}
}

func TestFilterBitsMatchExistingCfgFiles(t *testing.T) {
	// From a real sdio.cfg in the wild: "-filters:1030" with the GUI
	// showing "Show Not Installed / Show Newer / Show Only Best" checked.
	const observed FilterShow = 1030
	want := FilterMissing | FilterNewer | FilterOne
	if observed != want {
		t.Fatalf("FilterMissing|FilterNewer|FilterOne = %d, want observed value %d", want, observed)
	}
	if observed&FilterBetter != 0 || observed&FilterNFMissing != 0 {
		t.Fatal("observed value should not have Better or NFMissing set")
	}
}

func TestDefaults(t *testing.T) {
	s := New()
	if s.DrpDir != "drivers" {
		t.Errorf("DrpDir = %q, want %q", s.DrpDir, "drivers")
	}
	if s.OutputDir != filepath.Join("indexes", "txt") {
		t.Errorf("OutputDir = %q", s.OutputDir)
	}
	if s.Flags != 0 {
		t.Errorf("Flags = %d, want 0 (no flag defaults to set)", s.Flags)
	}
	if s.Filters != DefaultFilters {
		t.Errorf("Filters = %d, want %d", s.Filters, DefaultFilters)
	}
	if s.StateMode != StateModeReal {
		t.Errorf("StateMode = %v, want StateModeReal", s.StateMode)
	}
}

func TestParseStringAndBoolFlags(t *testing.T) {
	s := New()
	err := s.Parse([]string{
		"-drp-dir", "D:\\drivers",
		"-checkupdates",
		"-nogui=true",
		"-filters", "5",
	})
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if s.DrpDir != "D:\\drivers" {
		t.Errorf("DrpDir = %q", s.DrpDir)
	}
	if s.Flags&FlagCheckUpdates == 0 {
		t.Error("expected FlagCheckUpdates set")
	}
	if s.Flags&FlagNoGUI == 0 {
		t.Error("expected FlagNoGUI set")
	}
	if s.Filters != 5 {
		t.Errorf("Filters = %d, want 5", s.Filters)
	}
}

func TestParseLsSwitchesStateMode(t *testing.T) {
	s := New()
	if err := s.Parse([]string{"-ls", "snapshot.snp"}); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if s.StateFile != "snapshot.snp" {
		t.Errorf("StateFile = %q", s.StateFile)
	}
	if s.StateMode != StateModeEmul {
		t.Errorf("StateMode = %v, want StateModeEmul", s.StateMode)
	}
}

func TestDefaultExtractDir(t *testing.T) {
	s := New()
	t.Setenv("TEMP", `C:\Users\test\AppData\Local\Temp`)
	if err := s.Parse(nil); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	want := `C:\Users\test\AppData\Local\Temp\SDIO`
	if s.ExtractDir != want {
		t.Errorf("ExtractDir = %q, want %q", s.ExtractDir, want)
	}
}

func TestParseExtractDirSwitchesExtractOnly(t *testing.T) {
	s := New()
	if s.Flags&FlagExtractOnly != 0 {
		t.Fatal("expected FlagExtractOnly unset by default")
	}
	if err := s.Parse([]string{"-extractdir", `D:\extract`}); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if s.ExtractDirRaw != `D:\extract` {
		t.Errorf("ExtractDirRaw = %q", s.ExtractDirRaw)
	}
	if s.Flags&FlagExtractOnly == 0 {
		t.Error("expected FlagExtractOnly set by -extractdir")
	}
}

func TestParseVirtualOSVersionResolvesName(t *testing.T) {
	s := New()
	if err := s.Parse([]string{"-virtual-os-version", "100"}); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if s.VirtualOSVersion != 100 {
		t.Errorf("VirtualOSVersion = %d, want 100", s.VirtualOSVersion)
	}
	if s.VirtualWindowsVersionName != "Windows 10" {
		t.Errorf("VirtualWindowsVersionName = %q, want %q", s.VirtualWindowsVersionName, "Windows 10")
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

func TestLegacyExtractDirSwitch(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sdigo.cfg")
	if err := os.WriteFile(cfgPath, []byte(`"-extractdir:D:\extract"`+"\n"), 0o644); err != nil {
		t.Fatalf("writing cfg fixture: %v", err)
	}

	s := New()
	if err := s.LoadFile(cfgPath); err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}
	if s.ExtractDirRaw != `D:\extract` {
		t.Errorf("ExtractDirRaw = %q", s.ExtractDirRaw)
	}
	if s.Flags&FlagExtractOnly == 0 {
		t.Error("expected FlagExtractOnly set by legacy -extractdir:")
	}
}

func TestLoadFileAcceptsLegacyCfgSyntax(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sdigo.cfg")
	legacy := `"-drp_dir:D:\driverpacks"
"-index_dir:D:\driverpacks\indexes"
"-output_dir:D:\driverpacks\indexes\txt"
"-lang:en-US"
"-theme:dark"
-scale:200
-verbose:31
-checkupdates
-index_hr
-a:64
-filters:9
`
	if err := os.WriteFile(cfgPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("writing legacy cfg fixture: %v", err)
	}

	s := New()
	if err := s.LoadFile(cfgPath); err != nil {
		t.Fatalf("LoadFile() error on legacy cfg: %v", err)
	}

	if s.DrpDir != `D:\driverpacks` {
		t.Errorf("DrpDir = %q", s.DrpDir)
	}
	if s.IndexDir != `D:\driverpacks\indexes` {
		t.Errorf("IndexDir = %q", s.IndexDir)
	}
	if s.OutputDir != `D:\driverpacks\indexes\txt` {
		t.Errorf("OutputDir = %q", s.OutputDir)
	}
	if s.Flags&FlagCheckUpdates == 0 {
		t.Error("expected FlagCheckUpdates set")
	}
	if s.Flags&FlagPrintIndex == 0 {
		t.Error("expected -index_hr to map to FlagPrintIndex")
	}
	if s.VirtualArchType != 64 {
		t.Errorf("VirtualArchType = %d, want 64", s.VirtualArchType)
	}
	if s.Filters != 9 {
		t.Errorf("Filters = %d, want 9", s.Filters)
	}
}

// TestLoadFileDropsRemovedFlags confirms a cfg file written by an
// earlier build of this rewrite (before autoinstall/novirusalerts/
// failsafe/keepunpackedindex/keeptempfiles/nostamp were removed for
// having no wired effect) still loads without error - every token
// from a cfg file passes through the same legacyDroppedExact table
// regardless of old vs. new syntax, so a removed flag's own name
// (identical in both) is silently dropped rather than failing Parse.
func TestLoadFileDropsRemovedFlags(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sdigo.cfg")
	cfg := "-checkupdates\n-autoinstall\n-novirusalerts\n-failsafe\n-keepunpackedindex\n-keeptempfiles\n-nostamp\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("writing cfg fixture: %v", err)
	}

	s := New()
	if err := s.LoadFile(cfgPath); err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}
	if s.Flags&FlagCheckUpdates == 0 {
		t.Error("expected FlagCheckUpdates set")
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
