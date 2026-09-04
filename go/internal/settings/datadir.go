package settings

import (
	"os"
	"path/filepath"
)

// ResolveDataDirs picks where DrpDir/IndexDir/OutputDir/LogDirRaw/
// UpdatesDir and
// sdio.cfg itself live, and returns the sdio.cfg path a caller should
// pass to LoadFile/Save. Three layouts, checked in order:
//
//   - Portable via the working directory: the current directory
//     already contains sdio.cfg, a "drivers" subdirectory, or an
//     "indexes" subdirectory. Directories are left at New()'s bare
//     relative defaults ("drivers", "indexes", ...), which every
//     other os.* call in this codebase already resolves against the
//     working directory - this is the original engine's own
//     behavior, unchanged.
//   - Portable via the executable's own directory: the working
//     directory has none of those markers, but the running
//     executable's directory does (e.g. launched via a shortcut whose
//     "Start in" folder differs from where the exe lives). Directories
//     are set to absolute, executable-relative paths.
//   - Installed: neither location has any of those markers - a bare
//     WinGet-installed exe with nothing alongside it. Data lives under
//     the current user's local application-data directory instead
//     (%LOCALAPPDATA%\SDIO on Windows).
//
// Command-line flags parsed after this call still override any path
// it sets, same as always - this only changes New()'s defaults, one
// level earlier than a config file already does.
func (s *Settings) ResolveDataDirs() string {
	if cwd, err := os.Getwd(); err == nil && looksPortable(cwd) {
		return DefaultCfgFilename
	}

	exeDir, exeErr := executableDir()
	if exeErr == nil && looksPortable(exeDir) {
		return filepath.Join(exeDir, DefaultCfgFilename)
	}

	base, err := os.UserCacheDir()
	if err != nil {
		if exeErr == nil {
			return filepath.Join(exeDir, DefaultCfgFilename)
		}
		return DefaultCfgFilename
	}
	base = filepath.Join(base, "SDIO")

	s.DrpDir = filepath.Join(base, "drivers")
	s.IndexDir = filepath.Join(base, "indexes")
	s.OutputDir = filepath.Join(base, "indexes", "txt")
	s.LogDirRaw = filepath.Join(base, "logs")
	s.UpdatesDir = filepath.Join(base, "updates")
	return filepath.Join(base, DefaultCfgFilename)
}

// LoadDefaultCfgResolved calls ResolveDataDirs, then loads sdio.cfg
// from wherever that resolved to, if present - a missing file is not
// an error, matching LoadDefaultCfg. Returns the resolved path so the
// caller can pass the same location to Save later.
func (s *Settings) LoadDefaultCfgResolved() (cfgPath string, err error) {
	cfgPath = s.ResolveDataDirs()
	err = s.LoadFile(cfgPath)
	if err != nil && os.IsNotExist(err) {
		err = nil
	}
	return cfgPath, err
}

func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe), nil
}

func looksPortable(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, DefaultCfgFilename)); err == nil {
		return true
	}
	for _, sub := range [...]string{"drivers", "indexes"} {
		if fi, err := os.Stat(filepath.Join(dir, sub)); err == nil && fi.IsDir() {
			return true
		}
	}
	return false
}
