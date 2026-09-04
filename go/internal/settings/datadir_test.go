package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLooksPortableEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if looksPortable(dir) {
		t.Error("looksPortable() = true for an empty directory, want false")
	}
}

func TestLooksPortableWithCfgFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, DefaultCfgFilename), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if !looksPortable(dir) {
		t.Error("looksPortable() = false with sdio.cfg present, want true")
	}
}

func TestLooksPortableWithDriversDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "drivers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !looksPortable(dir) {
		t.Error("looksPortable() = false with a drivers/ subdirectory present, want true")
	}
}

func TestLooksPortableWithIndexesDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "indexes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !looksPortable(dir) {
		t.Error("looksPortable() = false with an indexes/ subdirectory present, want true")
	}
}

// TestLooksPortableIgnoresPlainFileNamedDrivers confirms the check
// requires drivers/indexes to actually be directories, not files that
// happen to share the name.
func TestLooksPortableIgnoresPlainFileNamedDrivers(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "drivers"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if looksPortable(dir) {
		t.Error("looksPortable() = true for a plain file named \"drivers\", want false")
	}
}

// TestResolveDataDirsPortableViaWorkingDirectory confirms a working
// directory with portable markers wins over any other check (in
// particular, over the test binary's own directory, which has none of
// them) and leaves DrpDir/IndexDir/OutputDir/LogDirRaw/UpdatesDir at
// New()'s bare relative defaults - the same paths every other os.*
// call in this codebase already resolves against the working
// directory.
func TestResolveDataDirsPortableViaWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "drivers"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	s := New()
	wantDrpDir, wantIndexDir, wantOutputDir, wantLogDir, wantUpdatesDir := s.DrpDir, s.IndexDir, s.OutputDir, s.LogDirRaw, s.UpdatesDir

	cfgPath := s.ResolveDataDirs()
	if cfgPath != DefaultCfgFilename {
		t.Errorf("ResolveDataDirs() = %q, want the bare relative %q", cfgPath, DefaultCfgFilename)
	}
	if s.DrpDir != wantDrpDir || s.IndexDir != wantIndexDir || s.OutputDir != wantOutputDir || s.LogDirRaw != wantLogDir || s.UpdatesDir != wantUpdatesDir {
		t.Errorf("ResolveDataDirs() changed directories for a portable working directory: DrpDir=%q IndexDir=%q OutputDir=%q LogDirRaw=%q UpdatesDir=%q",
			s.DrpDir, s.IndexDir, s.OutputDir, s.LogDirRaw, s.UpdatesDir)
	}
}

// TestResolveDataDirsInstalledUsesAppData confirms that with no
// portable markers anywhere (a bare installed exe, working directory
// elsewhere), ResolveDataDirs redirects DrpDir/IndexDir/UpdatesDir
// under the user's cache directory (%LOCALAPPDATA% on Windows) instead
// of leaving them at the bare "drivers"/"indexes"/"updates" that would
// otherwise resolve against whatever the working directory happens to
// be.
func TestResolveDataDirsInstalledUsesAppData(t *testing.T) {
	t.Chdir(t.TempDir()) // guaranteed no portable markers here

	s := New()
	cfgPath := s.ResolveDataDirs()

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("os.UserCacheDir() unavailable in this environment: %v", err)
	}
	wantBase := filepath.Join(cacheDir, "SDIO")
	if s.DrpDir != filepath.Join(wantBase, "drivers") {
		t.Errorf("DrpDir = %q, want under %q", s.DrpDir, wantBase)
	}
	if s.IndexDir != filepath.Join(wantBase, "indexes") {
		t.Errorf("IndexDir = %q, want under %q", s.IndexDir, wantBase)
	}
	if s.UpdatesDir != filepath.Join(wantBase, "updates") {
		t.Errorf("UpdatesDir = %q, want under %q", s.UpdatesDir, wantBase)
	}
	if cfgPath != filepath.Join(wantBase, DefaultCfgFilename) {
		t.Errorf("ResolveDataDirs() = %q, want %q", cfgPath, filepath.Join(wantBase, DefaultCfgFilename))
	}
}
