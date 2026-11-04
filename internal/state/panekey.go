package state

import (
	"fmt"
	"strings"

	"github.com/cespare/xxhash/v2"
)

// SanitizePaneKey returns the canonical, filesystem-safe paneKey, shaped
// <stem>__<window>.<pane>. A name that survives sanitisation unchanged is its
// own stem; one that had bytes replaced gains a hash suffix, so two distinct
// names cannot collapse onto one stem. The result is stable across platforms.
func SanitizePaneKey(session string, window, pane int) string {
	sanitized := sanitizeSessionName(session)

	stem := sanitized
	if sanitized != session {
		stem = sanitized + "-" + collisionSuffix(session)
	}

	return fmt.Sprintf("%s__%d.%d", stem, window, pane)
}

// A leading '.' also becomes '_' so the stem is not filesystem-hidden on Unix.
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

func collisionSuffix(session string) string {
	return fmt.Sprintf("%016x", xxhash.Sum64String(session))[:8]
}
