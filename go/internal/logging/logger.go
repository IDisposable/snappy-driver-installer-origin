// Package logging provides structured logging and elapsed-time tracking
// for the SDI engine, replacing the printf-style channels and manual
// tick counters from logging.cpp.
package logging

import (
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// Timestamp returns a fresh timestamp string in the exact format the
// original's Log_t::gen_timestamp produces (logging.cpp: wsprintf(...,
// L"%4d_%02d_%02d__%02d_%02d_%02d__%s_", ...)) - the current local
// date/time plus the machine's hostname, trailing-underscore-
// terminated so a caller can directly append a bare filename like
// "log.txt" or "state.snp" (matching "%s\\%slog.txt"/"%s\\%sstate.snp"
// in logging.cpp/model.cpp) to get the real byte-for-byte filename
// convention, e.g. "2026_09_04__20_27_02__FRAMIUS_log.txt". Falls back
// to "unknown-host" if os.Hostname fails, rather than erroring the
// whole log/snapshot filename out over a missing hostname.
func Timestamp() string {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown-host"
	}
	return time.Now().Format("2006_01_02__15_04_05") + "__" + host + "_"
}

// Logger wraps a zerolog.Logger with file lifecycle management and an
// error counter. Use the embedded zerolog methods (Debug, Info, Warn,
// Error, ...) directly; call Start/Stop to add a log file sink.
type Logger struct {
	zerolog.Logger

	console    io.Writer
	level      zerolog.Level
	file       *os.File
	errorCount int64
}

// New creates a Logger at the given level. Pass nil for console to
// suppress console output entirely, e.g. when a TUI owns the terminal.
func New(level zerolog.Level, console io.Writer) *Logger {
	l := &Logger{console: console, level: level}
	l.rebuild()
	return l
}

func (l *Logger) rebuild() {
	var writers []io.Writer
	if l.console != nil {
		writers = append(writers, zerolog.ConsoleWriter{Out: l.console, TimeFormat: "15:04:05"})
	}
	if l.file != nil {
		writers = append(writers, l.file)
	}

	var w io.Writer = io.Discard
	switch len(writers) {
	case 1:
		w = writers[0]
	case 2:
		w = zerolog.MultiLevelWriter(writers...)
	}

	l.Logger = zerolog.New(w).
		Level(l.level).
		With().Timestamp().Logger().
		Hook(zerolog.HookFunc(l.countErrors))
}

func (l *Logger) countErrors(_ *zerolog.Event, level zerolog.Level, _ string) {
	if level == zerolog.ErrorLevel {
		atomic.AddInt64(&l.errorCount, 1)
	}
}

// ErrorCount returns the number of Error-level events logged so far.
func (l *Logger) ErrorCount() int64 {
	return atomic.LoadInt64(&l.errorCount)
}

// Start opens a log file named "<timestamp>log.txt" in logDir, falling
// back to a SDIO_logs folder under the OS temp dir if logDir can't be
// created, and adds it as a sink alongside the console writer.
func (l *Logger) Start(logDir, timestamp string) error {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		logDir = filepath.Join(os.TempDir(), "SDIO_logs")
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return err
		}
	}

	f, err := os.Create(filepath.Join(logDir, timestamp+"log.txt"))
	if err != nil {
		return err
	}

	l.file = f
	l.rebuild()
	// Log() (zerolog's no-level event) rather than Info() - ported from
	// log_start's unconditional print_file call (logging.cpp), which
	// always records the start marker in the file regardless of
	// console verbosity. l.level only ever gates what this rewrite's
	// callers explicitly log afterward (e.g. -torrentalerts warnings);
	// a leveled Info() call here would be filtered out at l.level's
	// current WarnLevel default, producing a genuinely empty file even
	// though the file itself opened and ran successfully.
	l.Logger.Log().Msg("start logging")
	return nil
}

// Save flushes the log file to disk.
func (l *Logger) Save() {
	if l.file != nil {
		l.file.Sync()
	}
}

// Stop writes a closing marker, closes the log file, and reverts to
// console-only output.
func (l *Logger) Stop() {
	if l.file == nil {
		return
	}
	l.Logger.Log().Msg("stop logging")
	l.file.Close()
	l.file = nil
	l.rebuild()
}
