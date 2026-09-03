package settings

import (
	"flag"
	"os"
	"regexp"
	"strconv"
)

// flagBit adapts a single Flags bit to flag.Value, so boolFlagDefs can
// drive flag.FlagSet.Var directly instead of a hand-rolled bool.
type flagBit struct {
	flags *Flags
	bit   Flags
}

func (f flagBit) String() string {
	if f.flags == nil {
		return "false"
	}
	return strconv.FormatBool(*f.flags&f.bit != 0)
}

func (f flagBit) Set(v string) error {
	b, err := strconv.ParseBool(v)
	if err != nil {
		return err
	}
	if b {
		*f.flags |= f.bit
	} else {
		*f.flags &^= f.bit
	}
	return nil
}

func (f flagBit) IsBoolFlag() bool { return true }

// filterShowValue adapts *FilterShow to flag.Value.
type filterShowValue struct{ v *FilterShow }

func (f filterShowValue) String() string {
	if f.v == nil {
		return "0"
	}
	return strconv.FormatUint(uint64(*f.v), 10)
}

func (f filterShowValue) Set(s string) error {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return err
	}
	*f.v = FilterShow(n)
	return nil
}

// stateFileValue sets StateFile and switches StateMode to emulation in
// one step, matching the original's "-ls:" handling.
type stateFileValue struct{ s *Settings }

func (f stateFileValue) String() string {
	if f.s == nil {
		return ""
	}
	return f.s.StateFile
}

func (f stateFileValue) Set(v string) error {
	f.s.StateFile = v
	f.s.StateMode = StateModeEmul
	return nil
}

// FlagSet builds a *flag.FlagSet bound to this Settings, for use with
// flag.FlagSet.Parse against os.Args[1:] or against tokens read from a
// config file (see LoadFile). One-shot action switches from the
// original (-PATH, -install, -7z, -?) are deliberately not registered
// here: they select a program mode rather than a setting, and belong to
// the CLI dispatch layer. "-?" becomes the standard "-h"/"-help".
func (s *Settings) FlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)

	fs.StringVar(&s.DrpDir, "drp-dir", s.DrpDir, "driver pack directory")
	fs.StringVar(&s.IndexDir, "index-dir", s.IndexDir, "index directory")
	fs.StringVar(&s.OutputDir, "output-dir", s.OutputDir, "text index output directory")
	fs.StringVar(&s.DataDir, "data-dir", s.DataDir, "SDIO tools data directory")
	fs.StringVar(&s.LogDirRaw, "log-dir", s.LogDirRaw, "log directory (may contain %VAR% references)")
	fs.StringVar(&s.FinishCmd, "finish-cmd", s.FinishCmd, "command to run when finished")
	fs.StringVar(&s.FinishRebootCmd, "finish-reboot-cmd", s.FinishRebootCmd, "command to run when finished and a reboot is needed")
	fs.StringVar(&s.FinishUpdateCmd, "finish-update-cmd", s.FinishUpdateCmd, "command to run when finished with updates available")
	fs.StringVar(&s.DeviceListFilename, "device-list", s.DeviceListFilename, "write a device list to this file")
	fs.Var(stateFileValue{s}, "ls", "replay a saved system snapshot (.snp) instead of scanning real hardware")

	fs.Var(filterShowValue{&s.Filters}, "filters", "bitmask of driver-match categories to display")
	fs.IntVar(&s.VirtualOSVersion, "virtual-os-version", s.VirtualOSVersion, "virtual OS version to match against")
	fs.IntVar(&s.VirtualArchType, "arch", s.VirtualArchType, "virtual architecture to match against: 32 or 64")

	fs.BoolFunc("filtersp", "restrict matches to service-pack validated dates", func(v string) error {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return err
		}
		if b {
			s.Flags |= FlagFilterSP
			s.Flags &^= FlagUseLZMA
		}
		return nil
	})

	for _, e := range boolFlagDefs {
		fs.Var(flagBit{&s.Flags, e.flag}, e.name, e.help)
	}

	return fs
}

// Parse parses args (typically os.Args[1:], or tokens from a config
// file) against a fresh FlagSet bound to s, then expands %VAR%
// references in LogDir.
func (s *Settings) Parse(args []string) error {
	if err := s.FlagSet("sdio").Parse(args); err != nil {
		return err
	}
	s.LogDir = expandWindowsEnv(s.LogDirRaw)
	return nil
}

var envVarPattern = regexp.MustCompile(`%([^%]+)%`)

// expandWindowsEnv expands %VAR% references, the convention used by
// sdio.cfg values (Windows' ExpandEnvironmentStrings), rather than
// os.ExpandEnv's $VAR/${VAR} syntax.
func expandWindowsEnv(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(m string) string {
		name := m[1 : len(m)-1]
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return m
	})
}
