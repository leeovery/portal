package log

import (
	"log/slog"
	"strings"
)

// Level-resolution source labels. They report HOW the resolved level was
// determined and are surfaced verbatim in the "log-level resolved" line wired
// up in a later phase.
const (
	// sourceDefault — PORTAL_LOG_LEVEL was unset; the level fell to the info
	// default.
	sourceDefault = "default"
	// sourceEnv — PORTAL_LOG_LEVEL was set to a value in the valid set.
	sourceEnv = "env"
	// sourceFallback — PORTAL_LOG_LEVEL was set to an invalid value; the level
	// fell back to info.
	sourceFallback = "fallback"
)

// resolveLevel maps a raw PORTAL_LOG_LEVEL value to an slog.Level, reporting how
// the level was resolved. Matching is performed on the trimmed, lowercased form
// against the closed valid set (debug/info/warn/error); the legacy "warning"
// alias is deliberately NOT accepted. The production default is info — an unset
// (empty) value resolves to (LevelInfo, "default"), and any non-empty value
// outside the valid set resolves to (LevelInfo, "fallback").
//
// raw is returned verbatim (not trimmed or lowercased) so the eventual raw= attr
// and the invalid-value WARN render the exact user input; only the match is
// normalized. The function is pure: the caller reads the env value
// (os.Getenv("PORTAL_LOG_LEVEL")) and passes it in, keeping resolution
// unit-testable without env mutation.
func resolveLevel(raw string) (lvl slog.Level, source string, observed string) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return slog.LevelInfo, sourceDefault, raw
	}

	switch normalized {
	case "debug":
		return slog.LevelDebug, sourceEnv, raw
	case "info":
		return slog.LevelInfo, sourceEnv, raw
	case "warn":
		return slog.LevelWarn, sourceEnv, raw
	case "error":
		return slog.LevelError, sourceEnv, raw
	default:
		return slog.LevelInfo, sourceFallback, raw
	}
}

// levelString maps an slog.Level to its lowercase token (debug/info/warn/error)
// for the resolved= attr. Any level that is not one of the four standard levels
// renders via slog's own lowercased String form (e.g. "info+2").
func levelString(lvl slog.Level) string {
	switch lvl {
	case slog.LevelDebug:
		return "debug"
	case slog.LevelInfo:
		return "info"
	case slog.LevelWarn:
		return "warn"
	case slog.LevelError:
		return "error"
	default:
		return strings.ToLower(lvl.String())
	}
}
