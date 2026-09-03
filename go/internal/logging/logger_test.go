package logging

import (
	"bytes"
	"os"
	"path/filepath"
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
