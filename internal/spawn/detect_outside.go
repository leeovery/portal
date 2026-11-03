package spawn

import (
	"strings"
	"unicode"
)

const ghosttyBundleID = "com.mitchellh.ghostty"

// bundleIDVar is where macOS stamps the launching app's bundle id.
const bundleIDVar = "__CFBundleIdentifier"

// Checked explicitly rather than by scanning the environ, so the trusted signal
// set stays auditable and cannot silently widen.
var ghosttyEnvKeys = []string{
	"GHOSTTY_RESOURCES_DIR",
	"GHOSTTY_BIN_DIR",
}

// macOS stamps the launching app's bundle id into __CFBundleIdentifier and
// Ghostty stamps its own vars, so outside a session the env usually answers
// without a process walk. Inside one it is the environment of whoever made
// the session, so detection there never reads it.
func detectOutsideSession(getenv func(string) string, selfPID int, walker ProcessWalker, reader BundleReader) (Identity, error) {
	if bundleID := strings.TrimSpace(getenv(bundleIDVar)); plausibleBundleID(bundleID) {
		return NewIdentity(bundleID, ""), nil
	}

	if ghosttyEnvPresent(getenv) {
		return NewIdentity(ghosttyBundleID, "Ghostty"), nil
	}

	return walkToBundle(selfPID, walker, reader)
}

// IdentityVars names every environment variable detection reads a terminal's
// identity from.
func IdentityVars() []string {
	return append([]string{bundleIDVar}, ghosttyEnvKeys...)
}

func plausibleBundleID(bundleID string) bool {
	if !strings.Contains(bundleID, ".") {
		return false
	}
	if strings.IndexFunc(bundleID, unicode.IsSpace) >= 0 {
		return false
	}
	return true
}

func ghosttyEnvPresent(getenv func(string) string) bool {
	for _, key := range ghosttyEnvKeys {
		if strings.TrimSpace(getenv(key)) != "" {
			return true
		}
	}
	return false
}
