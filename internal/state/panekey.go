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
//  1. Replace each byte not in [A-Za-z0-9._-] with '_'.
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

// sanitizeSessionName applies an allowlist substitution: each byte in
// [A-Za-z0-9._-] is preserved; every other byte becomes '_'. A leading '.' is
// then also replaced with '_' so the sanitized stem is not filesystem-hidden
// by default on Unix. Substitutions are byte-wise so behaviour is identical
// regardless of whether the input contains valid UTF-8.
func sanitizeSessionName(session string) string {
	var b strings.Builder
	b.Grow(len(session))
	for i := 0; i < len(session); i++ {
		c := session[i]
		if isAllowedByte(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('_')
	}

	out := b.String()
	if len(out) > 0 && out[0] == '.' {
		out = "_" + out[1:]
	}
	return out
}

// isAllowedByte reports whether b is in the allowlist [A-Za-z0-9._-]. All
// other bytes (whitespace, shell-meta, path separators, NUL, high bytes from
// multi-byte UTF-8 sequences, etc.) are replaced with '_' by
// sanitizeSessionName.
func isAllowedByte(b byte) bool {
	switch {
	case b >= 'A' && b <= 'Z':
		return true
	case b >= 'a' && b <= 'z':
		return true
	case b >= '0' && b <= '9':
		return true
	case b == '.' || b == '_' || b == '-':
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
