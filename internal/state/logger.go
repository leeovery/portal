package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// Component constants identify the subsystem emitting a log entry. Call sites
// should pass these constants to Logger.{Debug,Info,Warn,Error} so the
// component column in portal.log stays consistent across the codebase.
const (
	ComponentDaemon    = "daemon"
	ComponentRestore   = "restore"
	ComponentHydrate   = "hydrate"
	ComponentNotify    = "notify"
	ComponentHooks     = "hooks"
	ComponentBootstrap = "bootstrap"
	ComponentPreview   = "preview"
)

// LogRotateThreshold is the file-size cap at which Logger rotates the current
// portal.log to portal.log.old. Both the open-time rotation in OpenLogger and
// the mid-write rotation in maybeRotate use this constant. 1 MiB per file
// (1,048,576 bytes). Exported so tests and sibling packages can reference it.
const LogRotateThreshold = 1 * 1024 * 1024 // 1 MiB

// openAppendLog opens path for appending with the canonical flag/mode used
// across logger.go (initial open in OpenLogger and post-rotate reopens in
// maybeRotate). Centralising the call ensures all sites stay in sync.
func openAppendLog(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
}

// Logger appends single-line, pipe-delimited entries to a log file.
// Format: "timestamp | level | component | message\n" where timestamp is
// RFC3339 UTC. Logger is safe for concurrent use from multiple goroutines:
// writes are serialised by an internal mutex so each entry lands on the file
// atomically with respect to other Logger callers in the same process.
//
// A nil *Logger is a valid no-op: all methods bail early. This lets callers
// proceed when log opening fails without sprinkling nil checks at call sites.
type Logger struct {
	mu       sync.Mutex
	f        *os.File
	minLevel Level
	// path is the on-disk path of the current log file. Empty for NopLogger
	// (where rotation has nothing to act on). Captured at OpenLogger so
	// maybeRotate can rename the file without re-deriving the path.
	path string
	// rotating indicates whether mid-write rotation is enabled for this
	// Logger. Set true only when OpenLogger is called with rotate=true (the
	// daemon process). Non-daemon callers leave it false so maybeRotate
	// short-circuits — only one writer rotates the file at a time.
	rotating bool
}

// OpenLogger opens path for appending and returns a Logger configured with
// the level read from PORTAL_LOG_LEVEL via parseLevel — case-insensitive
// "debug"/"info"/"warn"/"error"; any other value (including unset) defaults
// to LevelWarn so production runs emit warnings and errors only.
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

	f, err := openAppendLog(path)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", path, err)
	}

	return &Logger{
		f:        f,
		minLevel: parseLevel(os.Getenv("PORTAL_LOG_LEVEL")),
		path:     path,
		rotating: rotate,
	}, nil
}

// parseLevel maps a PORTAL_LOG_LEVEL string to a Level. Input is trimmed and
// lowercased; "debug"/"info"/"warn"/"warning"/"error" map to their respective
// levels. Any other value (including empty) returns LevelWarn so production
// runs default to warnings + above per the spec.
func parseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelWarn
	}
}

// rotateIfOversized renames path → path+".old" when path exists and is at
// least LogRotateThreshold bytes. A missing path is a no-op. Errors from
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

	if info.Size() < LogRotateThreshold {
		return nil
	}

	if err := os.Rename(path, path+".old"); err != nil {
		return fmt.Errorf("rotate log file %s: %w", path, err)
	}
	return nil
}

// maybeRotate performs daemon-only mid-write rotation. The caller must hold
// l.mu. pending is the byte count of the line about to be written; if the
// current file size plus pending would reach LogRotateThreshold, the current
// file is closed, renamed to path+".old" (replacing any existing one), and a
// fresh file is opened so the triggering write lands in the new portal.log.
//
// Failure handling is best-effort: rotation errors are logged once to stderr
// so the operator notices, but the Logger never panics or returns errors —
// logging must not fail the caller. If the rename or reopen fails, l.f is
// left in a state where the next write either retries the rotation (file
// still ≥ threshold) or no-ops (l.f == nil).
func (l *Logger) maybeRotate(pending int64) {
	if !l.rotating {
		return
	}
	if l.f == nil {
		return
	}
	st, err := l.f.Stat()
	if err != nil {
		return
	}
	if st.Size()+pending < LogRotateThreshold {
		return
	}

	_ = l.f.Close()

	oldPath := l.path + ".old"
	_ = os.Remove(oldPath) // ENOENT-tolerant; ensures Rename can replace.

	if err := os.Rename(l.path, oldPath); err != nil {
		fmt.Fprintf(os.Stderr, "portal: log rotation failed: %v\n", err)
		// Re-open the existing file so subsequent writes still land.
		f, reopenErr := openAppendLog(l.path)
		if reopenErr != nil {
			fmt.Fprintf(os.Stderr, "portal: log reopen failed: %v\n", reopenErr)
		}
		l.f = f // may be nil → next write no-ops
		return
	}

	f, err := openAppendLog(l.path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "portal: log reopen failed: %v\n", err)
		l.f = nil
		return
	}
	l.f = f
}

// NopLogger returns a non-nil *Logger whose internal file is nil so every
// write call short-circuits via the existing nil-file guard in write. It is a
// convenient sentinel for callers that want a non-nil pointer (e.g. struct
// fields whose nil-handling is awkward in tests) without having to open a real
// log file. The returned Logger holds no resources; Close is a no-op.
func NopLogger() *Logger {
	return &Logger{}
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
// Concurrent callers are serialised by l.mu so each entry lands atomically
// relative to other Logger calls in the same process. On nil receiver, level
// filtering, or write errors, the call is silently dropped — logging must
// never fail the caller.
func (l *Logger) write(level Level, levelLabel, component, format string, args ...any) {
	if l == nil {
		return
	}
	if level < l.minLevel {
		return
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s | %s | %s | %s\n", timestamp, levelLabel, component, msg)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Rotate before writing so the triggering line lands in the freshly
	// opened portal.log rather than pushing the rotated file over threshold.
	// maybeRotate is a no-op for non-daemon callers and for nil l.f.
	l.maybeRotate(int64(len(line)))

	if l.f == nil {
		return
	}
	_, _ = l.f.WriteString(line)
}
