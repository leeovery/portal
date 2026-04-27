package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Level controls which log entries Logger writes to disk.
// Lower levels are more verbose.
type Level int

// Log levels in ascending order of severity. Logger writes an entry only when
// its level is greater than or equal to Logger.minLevel.
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// logRotateThreshold is the file-size cap at which OpenLogger rotates the
// current portal.log to portal.log.old. Matches the spec's "1 MB per file"
// (interpreted as 1 MiB to match the binary growth pattern of log files).
const logRotateThreshold = 1 << 20 // 1 MiB

// Logger appends single-line, pipe-delimited entries to a log file.
// Format: "timestamp | level | component | message\n" where timestamp is
// RFC3339 UTC. Logger is safe for concurrent calls only insofar as POSIX
// guarantees atomic writes for entries below PIPE_BUF — adequate for one-line
// entries written by a single process.
//
// A nil *Logger is a valid no-op: all methods bail early. This lets callers
// proceed when log opening fails without sprinkling nil checks at call sites.
type Logger struct {
	f        *os.File
	minLevel Level
}

// OpenLogger opens path for appending and returns a Logger configured with
// the level read from PORTAL_LOG_LEVEL ("debug" → LevelDebug, anything else
// → LevelInfo).
//
// When rotate is true and path exists with size ≥ 1 MiB, OpenLogger renames
// path to path+".old" before opening. Any existing path+".old" is overwritten.
// When rotate is false, the existing file is opened as-is regardless of size.
//
// The parent directory is created with mode 0700 if missing. The log file is
// opened with mode 0600.
func OpenLogger(path string, rotate bool) (*Logger, error) {
	if rotate {
		if err := rotateIfOversized(path); err != nil {
			return nil, err
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create log parent dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", path, err)
	}

	return &Logger{f: f, minLevel: levelFromEnv()}, nil
}

// levelFromEnv reads PORTAL_LOG_LEVEL and returns the corresponding Level.
// Default is LevelInfo. Only "debug" lowers the threshold; other values
// (including unset and unrecognised) fall back to LevelInfo so that
// production runs default to warnings + above per the spec.
func levelFromEnv() Level {
	if os.Getenv("PORTAL_LOG_LEVEL") == "debug" {
		return LevelDebug
	}
	return LevelInfo
}

// rotateIfOversized renames path → path+".old" when path exists and is at
// least logRotateThreshold bytes. A missing path is a no-op. Errors from
// stat (other than ErrNotExist) and rename are returned wrapped so callers
// can surface a specific failure.
func rotateIfOversized(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat log file %s: %w", path, err)
	}

	if info.Size() < logRotateThreshold {
		return nil
	}

	if err := os.Rename(path, path+".old"); err != nil {
		return fmt.Errorf("rotate log file %s: %w", path, err)
	}
	return nil
}

// Close releases the underlying file. Safe to call on a nil Logger.
func (l *Logger) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	return l.f.Close()
}

// Debug writes a DEBUG-level entry. No-op on nil receiver or when minLevel
// is above LevelDebug.
func (l *Logger) Debug(component, format string, args ...any) {
	l.write(LevelDebug, "DEBUG", component, format, args...)
}

// Info writes an INFO-level entry.
func (l *Logger) Info(component, format string, args ...any) {
	l.write(LevelInfo, "INFO", component, format, args...)
}

// Warn writes a WARN-level entry.
func (l *Logger) Warn(component, format string, args ...any) {
	l.write(LevelWarn, "WARN", component, format, args...)
}

// Error writes an ERROR-level entry.
func (l *Logger) Error(component, format string, args ...any) {
	l.write(LevelError, "ERROR", component, format, args...)
}

// write formats a single line and appends it to the underlying file.
// On nil receiver, level filtering, or write errors, the call is silently
// dropped — logging must never fail the caller.
func (l *Logger) write(level Level, levelLabel, component, format string, args ...any) {
	if l == nil || l.f == nil {
		return
	}
	if level < l.minLevel {
		return
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s | %s | %s | %s\n", timestamp, levelLabel, component, msg)
	_, _ = l.f.WriteString(line)
}
