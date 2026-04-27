// Package state provides on-disk paths and identifiers for Portal's
// session-resurrection state directory.
package state

import (
	"fmt"
	"strings"

	"github.com/cespare/xxhash/v2"
)

// SanitizePaneKey returns the canonical paneKey for the given session name
// and live tmux indices. It produces a deterministic, filesystem-safe
// identifier suitable for use in scrollback filenames, hydration FIFO names,
// and tmux skeleton-marker option names.
//
// Algorithm:
//  1. Replace each filesystem-unsafe byte (forward slash, backslash, null) in
//     session with '_'.
//  2. Replace a leading '.' with '_'.
//  3. If the sanitized name differs from the original, append '-' followed by
//     the first 8 hex characters of xxhash.Sum64String(session) to disambiguate
//     distinct names that collapse to the same sanitized stem.
//  4. Append '__<window>.<pane>'.
//
// The result is stable across processes and platforms.
func SanitizePaneKey(session string, window, pane int) string {
	sanitized := sanitizeSessionName(session)

	stem := sanitized
	if sanitized != session {
		stem = sanitized + "-" + collisionSuffix(session)
	}

	return fmt.Sprintf("%s__%d.%d", stem, window, pane)
}

// sanitizeSessionName replaces filesystem-unsafe bytes and a leading '.' with
// '_'. The substitutions are byte-wise so behavior is identical regardless of
// whether the input contains valid UTF-8.
func sanitizeSessionName(session string) string {
	var b strings.Builder
	b.Grow(len(session))
	for i := 0; i < len(session); i++ {
		c := session[i]
		if isUnsafeByte(c) {
			b.WriteByte('_')
			continue
		}
		b.WriteByte(c)
	}

	out := b.String()
	if len(out) > 0 && out[0] == '.' {
		out = "_" + out[1:]
	}
	return out
}

// isUnsafeByte reports whether b conflicts with filesystem path conventions.
// Forward slash and backslash are path separators on Unix and Windows
// respectively; null bytes terminate C strings used by most filesystem APIs.
func isUnsafeByte(b byte) bool {
	switch b {
	case '/', '\\', 0x00:
		return true
	}
	return false
}

// collisionSuffix returns the first 8 lowercase hex characters of
// xxhash.Sum64String(session). Used to disambiguate two distinct session
// names that sanitize to the same stem.
func collisionSuffix(session string) string {
	return fmt.Sprintf("%016x", xxhash.Sum64String(session))[:8]
}
