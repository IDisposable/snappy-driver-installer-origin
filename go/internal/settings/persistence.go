package settings

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
)

// LoadFile reads switches from a config file (one or more per line, in
// either this rewrite's "-flag=value" syntax or the original engine's
// "-flag:value" syntax, so an existing sdio.cfg keeps working) and
// applies them, then loads the per-host ignore list. filename may
// contain %VAR% references.
func (s *Settings) LoadFile(filename string) error {
	data, err := os.ReadFile(expandWindowsEnv(filename))
	if err != nil {
		return err
	}

	var sb strings.Builder
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimLeft(sc.Text(), " \t")
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		sb.WriteString(line)
		sb.WriteByte(' ')
	}
	if err := sc.Err(); err != nil {
		return err
	}

	var args []string
	for _, tok := range splitArgLine(sb.String()) {
		if translated, keep := translateLegacyArg(tok); keep {
			args = append(args, translated)
		}
	}

	if err := s.Parse(args); err != nil {
		return err
	}

	// A missing per-host ignore file is the common case, not a failure:
	// match the original, which logs and continues with an empty list.
	_ = s.loadIgnoreList()
	return nil
}

// Save writes the persistent subset of settings (see boolFlagDefs) to
// filename, in the same form LoadFile reads. It does nothing if
// FlagPreserveCfg is set.
func (s *Settings) Save(filename string) error {
	if s.Flags&FlagPreserveCfg != 0 {
		return nil
	}

	var sb strings.Builder
	writeStr := func(flagName, value string) {
		fmt.Fprintf(&sb, "-%s=%s\n", flagName, quoteArg(value))
	}

	writeStr("drp-dir", s.DrpDir)
	writeStr("index-dir", s.IndexDir)
	writeStr("output-dir", s.OutputDir)
	writeStr("data-dir", s.DataDir)
	writeStr("log-dir", s.LogDirRaw)
	sb.WriteByte('\n')

	writeStr("finish-cmd", s.FinishCmd)
	writeStr("finish-reboot-cmd", s.FinishRebootCmd)
	writeStr("finish-update-cmd", s.FinishUpdateCmd)
	sb.WriteByte('\n')

	fmt.Fprintf(&sb, "-filters=%d\n\n", s.Filters)

	for _, e := range boolFlagDefs {
		if e.persist && s.Flags&e.flag != 0 {
			fmt.Fprintf(&sb, "-%s\n", e.name)
		}
	}

	return os.WriteFile(filename, []byte(sb.String()), 0o644)
}

func quoteArg(s string) string {
	return `"` + s + `"`
}

// splitArgLine tokenizes a line of switches on whitespace, treating
// double-quoted spans as a single token. This is a deliberately
// simplified stand-in for full shell-style quoting: it only needs to
// parse cfg files this package itself writes (via Save), not arbitrary
// shell-quoted input.
func splitArgLine(s string) []string {
	var args []string
	var cur strings.Builder
	inQuotes := false

	flush := func() {
		if cur.Len() > 0 {
			args = append(args, cur.String())
			cur.Reset()
		}
	}

	for _, r := range s {
		switch {
		case r == '"':
			inQuotes = !inQuotes
		case isSpace(r) && !inQuotes:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return args
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func ignoreListFilename() (string, error) {
	host, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("hwid-ignore_%s.txt", host), nil
}

func (s *Settings) loadIgnoreList() error {
	filename, err := ignoreListFilename()
	if err != nil {
		return err
	}

	s.IgnoreList = nil
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimLeft(sc.Text(), " \t")
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		s.IgnoreList = append(s.IgnoreList, line)
	}
	return sc.Err()
}

// AddIgnoreList appends hwid to the ignore list and persists it to the
// per-host ignore file.
func (s *Settings) AddIgnoreList(hwid string) error {
	s.IgnoreList = append(s.IgnoreList, hwid)

	filename, err := ignoreListFilename()
	if err != nil {
		return err
	}

	var sb strings.Builder
	for _, id := range s.IgnoreList {
		sb.WriteString(id)
		sb.WriteByte('\n')
	}
	return os.WriteFile(filename, []byte(sb.String()), 0o644)
}
