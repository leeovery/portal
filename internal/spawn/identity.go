package spawn

import (
	"path"
	"strings"
)

// Identity is a detected host terminal's identity: its macOS bundle id and a
// friendly display name for reading. BundleID is the exact bundle id read from
// the terminal's `.app` (e.g. "dev.warp.Warp-Stable", "com.mitchellh.ghostty",
// "com.apple.Terminal"); Name is the human-readable app name (e.g. "Warp",
// "Ghostty", "Apple Terminal") shown by the picker banner and the `portal doctor`
// host-terminal line.
//
// The zero value is the NULL identity — no host-local terminal — which a
// remote/mosh client or an unsupported/transient detection outcome resolves to.
type Identity struct {
	BundleID string
	Name     string
}

// IsNull reports whether the identity is the NULL state — no host-local
// terminal. NULL is defined solely by an empty bundle id.
func (i Identity) IsNull() bool {
	return i.BundleID == ""
}

// NewIdentity builds a host-terminal identity from a bundle id and an optional
// app name.
//
// The bundle id is trimmed first: an empty (or whitespace-only) bundle id
// yields the zero Identity — the NULL state — regardless of appName. Any
// non-empty bundle id always yields a passthrough identity (never NULL), even
// when the bundle id is unknown to Portal.
//
// Name is set to appName when a non-empty one is supplied; otherwise it is
// derived from the bundle id (see deriveName). Name is never left empty for a
// non-empty bundle id.
func NewIdentity(bundleID, appName string) Identity {
	bundleID = strings.TrimSpace(bundleID)
	if bundleID == "" {
		return Identity{}
	}

	name := strings.TrimSpace(appName)
	if name == "" {
		name = deriveName(bundleID)
	}

	return Identity{BundleID: bundleID, Name: name}
}

// deriveName derives a friendly display name from a (trimmed, non-empty) bundle
// id: it takes the last dot-delimited segment, then trims any channel suffix at
// the first hyphen (e.g. "dev.warp.Warp-Stable" -> "Warp", "com.apple.Terminal"
// -> "Terminal"). If that leaves an empty segment it falls back to the full
// bundle id so the name is never empty. This is a fallback used only when no
// app name is supplied; the process-tree walk provides the true `.app` display
// name when available.
func deriveName(bundleID string) string {
	segment := bundleID
	if i := strings.LastIndex(segment, "."); i >= 0 {
		segment = segment[i+1:]
	}
	if i := strings.IndexByte(segment, '-'); i >= 0 {
		segment = segment[:i]
	}
	if segment == "" {
		return bundleID
	}
	return segment
}

// MatchesFamily reports whether bundleID belongs to the bundle-id family
// described by pattern. A "*" in pattern matches any run of characters,
// including a channel suffix, so "dev.warp.Warp-Stable" matches the family glob
// "dev.warp.Warp-*"; an exact (no-"*") pattern matches only its literal; a bare
// "*" matches any bundle id; a non-matching pattern returns false.
//
// It uses path.Match semantics. Bundle ids contain no "/" separator, so
// path.Match's "*" (which does not cross "/") matches the entire remainder of
// the bundle id — including the channel suffix — as required. A malformed
// pattern (path.ErrBadPattern) is treated as a non-match rather than a failure.
func MatchesFamily(bundleID, pattern string) bool {
	ok, err := path.Match(pattern, bundleID)
	if err != nil {
		return false
	}
	return ok
}
