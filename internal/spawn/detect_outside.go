package spawn

import (
	"strings"
	"unicode"
)

// ghosttyBundleID is Ghostty's macOS bundle id. This is detection-layer
// knowledge — the GHOSTTY_* env fast-path resolves to this id directly; mapping
// it to a window-spawning driver is a later phase's concern.
const ghosttyBundleID = "com.mitchellh.ghostty"

// ghosttyEnvKeys is the finite, named set of Ghostty-specific environment
// variables treated as a fast-path signal that the host terminal is Ghostty.
// Ghostty stamps these into every shell it spawns and they are accurate outside
// tmux. Checked explicitly (never via a full-environ scan) so the trusted
// signal set stays auditable and cannot silently widen.
var ghosttyEnvKeys = []string{
	"GHOSTTY_RESOURCES_DIR",
	"GHOSTTY_BIN_DIR",
}

// detectOutsideTmux resolves the host-terminal Identity for a Portal process
// running OUTSIDE tmux, where the environment reflects the launching terminal.
// It prefers a cheap env fast-path and walks the process tree only when the env
// yields nothing usable, in this precedence:
//
//  1. __CFBundleIdentifier — macOS stamps the launching app's bundle id here. A
//     plausible value resolves the identity directly, with no walk.
//  2. GHOSTTY_* — Ghostty stamps its own env vars; their presence (absent a
//     usable __CFBundleIdentifier) resolves to Ghostty's known bundle id, with
//     no walk.
//  3. Otherwise walkToBundle from selfPID (the picker's own pid), propagating
//     its resolved / clean-NULL / transient-error outcome verbatim.
//
// getenv is an injectable seam (production passes os.Getenv) so the resolution
// is unit-testable without touching the real environment.
func detectOutsideTmux(getenv func(string) string, selfPID int, walker ProcessWalker, reader BundleReader) (Identity, error) {
	if bundleID := strings.TrimSpace(getenv("__CFBundleIdentifier")); plausibleBundleID(bundleID) {
		return NewIdentity(bundleID, ""), nil
	}

	if ghosttyEnvPresent(getenv) {
		return NewIdentity(ghosttyBundleID, "Ghostty"), nil
	}

	return walkToBundle(selfPID, walker, reader)
}

// plausibleBundleID applies a minimal sanity check to an env-supplied bundle id
// before it is trusted without a walk: it must contain a dot and carry no
// internal whitespace. An empty or whitespace-only value (the caller trims
// first) fails the dot check, so it cleanly triggers the walk fallback rather
// than yielding a bogus identity.
func plausibleBundleID(bundleID string) bool {
	if !strings.Contains(bundleID, ".") {
		return false
	}
	if strings.IndexFunc(bundleID, unicode.IsSpace) >= 0 {
		return false
	}
	return true
}

// ghosttyEnvPresent reports whether any of the named Ghostty env vars carries a
// non-empty (post-trim) value.
func ghosttyEnvPresent(getenv func(string) string) bool {
	for _, key := range ghosttyEnvKeys {
		if strings.TrimSpace(getenv(key)) != "" {
			return true
		}
	}
	return false
}
