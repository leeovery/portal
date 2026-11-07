package spawn

import (
	"path"
	"strings"
)

// Identity is a host terminal's macOS bundle id plus a friendly display name.
// The zero value is the NULL identity — no host-local terminal — which a
// remote/mosh client resolves to.
type Identity struct {
	BundleID string
	Name     string
	// Unsure marks a NULL identity detection couldn't settle: it can't tell
	// which terminal Portal is running in, where a plain NULL is one with no
	// app on this machine.
	Unsure bool
}

func (i Identity) IsNull() bool {
	return i.BundleID == ""
}

// NewIdentity yields the NULL identity for a blank bundle id, whatever appName
// is. Any other bundle id passes through even when Portal does not know it, with
// Name derived from the bundle id when appName is blank.
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

// MatchesFamily uses path.Match semantics: bundle ids carry no "/", so "*" spans
// the whole remainder including a channel suffix. A malformed pattern is a
// non-match rather than a failure.
func MatchesFamily(bundleID, pattern string) bool {
	ok, err := path.Match(pattern, bundleID)
	if err != nil {
		return false
	}
	return ok
}
