package log

import (
	"strconv"
	"strings"
)

// defaultRotateSize is the size-cap safety valve applied when
// PORTAL_LOG_ROTATE_SIZE is unset or invalid: 500 MiB. The size cap is a
// disk-fill valve, not a correctness boundary — it is chosen so it never fires
// in normal use even at DEBUG steady-state yet catches a runaway within ~1 day.
const defaultRotateSize int64 = 500 * 1024 * 1024

// Binary size suffix multipliers. K/M/G are powers of 1024 (KiB/MiB/GiB), not
// the decimal 1000-base SI units — so 500M = 500 * 1024 * 1024 = 524288000.
const (
	suffixK int64 = 1024
	suffixM int64 = 1024 * 1024
	suffixG int64 = 1024 * 1024 * 1024
)

// resolveRotateSize maps a raw PORTAL_LOG_ROTATE_SIZE value to a byte count,
// reporting how it was resolved with the same (value, source) shape as the
// level resolver. The grammar is a base-10 integer with an optional
// case-insensitive K/M/G binary suffix; a bare integer is a literal byte count.
//
// An unset (empty/whitespace) value resolves to (defaultRotateSize, "default").
// Any malformed value — non-numeric prefix, unknown/multiple suffix, fractional,
// zero or negative — resolves to (defaultRotateSize, "fallback"). Zero is
// rejected because a 0-byte cap would rotate on every write.
//
// The function is pure: the caller reads os.Getenv("PORTAL_LOG_ROTATE_SIZE")
// and passes it in, keeping resolution unit-testable without env mutation.
func resolveRotateSize(raw string) (int64, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultRotateSize, sourceDefault
	}

	digits, multiplier := splitRotateSize(trimmed)
	if digits == "" {
		return defaultRotateSize, sourceFallback
	}

	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || n <= 0 {
		return defaultRotateSize, sourceFallback
	}

	// Guard against overflow before multiplying.
	if multiplier > 1 && n > (1<<63-1)/multiplier {
		return defaultRotateSize, sourceFallback
	}

	return n * multiplier, sourceEnv
}

// splitRotateSize separates the numeric prefix from an optional single trailing
// K/M/G suffix (case-insensitive). It returns the digit string and the byte
// multiplier; a malformed input (empty digits, unknown suffix, or trailing
// characters after a suffix) returns ("", 0) so the caller falls back.
func splitRotateSize(s string) (digits string, multiplier int64) {
	last := s[len(s)-1]
	switch last {
	case 'K', 'k':
		return s[:len(s)-1], suffixK
	case 'M', 'm':
		return s[:len(s)-1], suffixM
	case 'G', 'g':
		return s[:len(s)-1], suffixG
	default:
		// No recognised suffix: the whole string must be the byte count. A
		// stray non-digit trailing char (e.g. "5X") is caught when ParseInt
		// fails on the digit string below.
		return s, 1
	}
}

// defaultRetentionDays is the retention window applied when
// PORTAL_LOG_RETENTION_DAYS is unset or invalid: 30 days of rotated history.
const defaultRetentionDays = 30

// maxRetentionDays is the upper bound of the valid retention range; values above
// it are rejected as fallbacks.
const maxRetentionDays = 365

// resolveRetentionDays maps a raw PORTAL_LOG_RETENTION_DAYS value to a day count,
// reporting how it was resolved and the verbatim raw for the eventual
// invalid-value startup WARN (emitted by a later task, not here).
//
// An unset (empty/whitespace) value resolves to (defaultRetentionDays, "default",
// ""). A valid integer in [0, 365] resolves to (n, "env", raw) — note 0 is
// VALID (delete everything older than today); only negative is rejected. A
// non-integer, negative, or > 365 value resolves to (defaultRetentionDays,
// "fallback", raw) with raw preserved verbatim.
//
// The function is pure: the caller reads os.Getenv("PORTAL_LOG_RETENTION_DAYS")
// and passes it in, keeping resolution unit-testable without env mutation.
func resolveRetentionDays(raw string) (int, string, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultRetentionDays, sourceDefault, raw
	}

	n, err := strconv.Atoi(trimmed)
	if err != nil || n < 0 || n > maxRetentionDays {
		return defaultRetentionDays, sourceFallback, raw
	}

	return n, sourceEnv, raw
}
