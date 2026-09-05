package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestErrorCount(t *testing.T) {
	var buf bytes.Buffer
	l := New(zerolog.DebugLevel, &buf)

	l.Info().Msg("fine")
	l.Error().Msg("boom")
	l.Error().Msg("boom again")

	if got := l.ErrorCount(); got != 2 {
		t.Fatalf("ErrorCount() = %d, want 2", got)
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := New(zerolog.InfoLevel, &buf)

	l.Debug().Msg("should not appear")
	l.Info().Msg("should appear")

	out := buf.String()
	if strings.Contains(out, "should not appear") {
		t.Fatalf("debug message leaked through at Info level: %q", out)
	}
	if !strings.Contains(out, "should appear") {
		t.Fatalf("expected info message in output: %q", out)
	}
}

func TestNilConsoleSuppressesOutput(t *testing.T) {
	l := New(zerolog.DebugLevel, nil)
	l.Info().Msg("into the void")
	// No assertion beyond "does not panic": there is nothing to read back from.
}

func TestStartWritesToFile(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	l := New(zerolog.DebugLevel, &buf)

	if err := l.Start(dir, "20260101_000000__host_"); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	l.Info().Msg("hello file")
	l.Save()
	l.Stop()

	data, err := os.ReadFile(filepath.Join(dir, "20260101_000000__host_log.txt"))
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if !strings.Contains(string(data), "hello file") {
		t.Fatalf("expected log file to contain message, got: %q", string(data))
	}
	if !strings.Contains(string(data), "start logging") || !strings.Contains(string(data), "stop logging") {
		t.Fatalf("expected start/stop markers in log file, got: %q", string(data))
	}
}

// TestStartAndStopMarkersSurviveWarnLevel is the regression case
// TestStartWritesToFile (DebugLevel) doesn't cover: cmd/sdigo's real
// logger runs at WarnLevel (see main.go - the file exists only for
// -torrentalerts), so a run that never logs a Warn-or-higher event
// must still produce a non-empty file with real start/stop markers,
// not a silently-filtered-to-nothing file - ported from log_start's
// unconditional print_file call (logging.cpp), which never gates the
// start/stop banner behind the console verbosity threshold.
func TestStartAndStopMarkersSurviveWarnLevel(t *testing.T) {
	dir := t.TempDir()
	l := New(zerolog.WarnLevel, nil)

	if err := l.Start(dir, "20260101_000000__host_"); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	l.Info().Msg("this should be filtered out, unlike the markers")
	l.Stop()

	data, err := os.ReadFile(filepath.Join(dir, "20260101_000000__host_log.txt"))
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("log file is empty - start/stop markers must not be filtered by the configured level")
	}
	if strings.Contains(string(data), "this should be filtered out") {
		t.Fatalf("Warn-level filtering should still apply to ordinary log calls, got: %q", data)
	}
	if !strings.Contains(string(data), "start logging") || !strings.Contains(string(data), "stop logging") {
		t.Fatalf("expected start/stop markers regardless of level, got: %q", data)
	}
}

// TestTimestampMatchesOriginalFormat pins Timestamp()'s output to the
// exact byte format Log_t::gen_timestamp produces (logging.cpp:
// wsprintf(..., L"%4d_%02d_%02d__%02d_%02d_%02d__%s_", ...)) - a real
// bug let a Go-idiomatic-but-wrong format ("20060102-150405", no
// separators, no hostname) reach cmd/sdigo instead of this function,
// producing filenames like "20260904-212226log.txt" instead of
// "2026_09_04__20_27_02__FRAMIUS_log.txt".
func TestTimestampMatchesOriginalFormat(t *testing.T) {
	ts := Timestamp()
	matched, err := regexp.MatchString(`^\d{4}_\d{2}_\d{2}__\d{2}_\d{2}_\d{2}__.+_$`, ts)
	if err != nil {
		t.Fatalf("regexp error: %v", err)
	}
	if !matched {
		t.Fatalf("Timestamp() = %q, want the form YYYY_MM_DD__HH_MM_SS__<hostname>_", ts)
	}
}

func TestStartFallsBackWhenDirUnwritable(t *testing.T) {
	// A path with a NUL byte can never be created as a directory, forcing
	// the temp-dir fallback path.
	var buf bytes.Buffer
	l := New(zerolog.DebugLevel, &buf)

	if err := l.Start("/nonexistent-root/\x00bad", "ts_"); err != nil {
		t.Fatalf("Start() should fall back rather than error, got: %v", err)
	}
	l.Stop()

	fallback := filepath.Join(os.TempDir(), "SDIO_logs", "ts_log.txt")
	if _, err := os.Stat(fallback); err != nil {
		t.Fatalf("expected fallback log file at %s: %v", fallback, err)
	}
	os.Remove(fallback)
}
