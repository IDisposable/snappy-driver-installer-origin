package settings

import (
	"os"
	"path/filepath"
	"testing"
)

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
	if s.Flags&FlagUseLZMA == 0 {
		t.Error("expected FlagUseLZMA set by default")
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

func TestParseFiltersp(t *testing.T) {
	s := New()
	if s.Flags&FlagUseLZMA == 0 {
		t.Fatal("expected FlagUseLZMA set before parse")
	}
	if err := s.Parse([]string{"-filtersp"}); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if s.Flags&FlagFilterSP == 0 {
		t.Error("expected FlagFilterSP set")
	}
	if s.Flags&FlagUseLZMA != 0 {
		t.Error("expected FlagUseLZMA cleared by -filtersp")
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
	cfgPath := filepath.Join(dir, "sdio.cfg")

	s := New()
	if err := s.Parse([]string{
		"-drp-dir", "custom-drivers",
		"-checkupdates",  // persistent
		"-autoinstall",   // not persistent
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
	if loaded.Flags&FlagAutoInstall != 0 {
		t.Error("expected non-persistent FlagAutoInstall to NOT round-trip")
	}
	if loaded.Filters != 9 {
		t.Errorf("Filters = %d, want 9", loaded.Filters)
	}
}

func TestSaveSkipsWhenPreserveCfgSet(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sdio.cfg")

	s := New()
	s.Flags |= FlagPreserveCfg
	if err := s.Save(cfgPath); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatal("expected no cfg file to be written when FlagPreserveCfg is set")
	}
}

func TestLoadFileAcceptsLegacyCfgSyntax(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sdio.cfg")
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
