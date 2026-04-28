package state

import (
	"strings"

	"github.com/leeovery/portal/internal/tmuxout"
)

// SkeletonMarkerPrefix is the tmux server-option name prefix used to mark
// panes that have been skeleton-restored and are awaiting hydration. The
// daemon's save loop uses these markers to skip capturing scrollback for
// such panes — the on-disk pre-boot scrollback file is the authoritative
// state until hydration completes.
const SkeletonMarkerPrefix = "@portal-skeleton-"

// RestoringMarkerName is the tmux server-option name set by bootstrap during
// the skeleton-restore phase. While set, the daemon's tick loop must skip
// captures entirely so it does not record half-built session structure.
const RestoringMarkerName = "@portal-restoring"

// ServerOptionLister is the seam used by ListSkeletonMarkers. It is satisfied
// implicitly by *tmux.Client via its ShowAllServerOptions method. Defining the
// interface here keeps internal/state free of an internal/tmux import, which
// would close a cycle (internal/tmux imports internal/state for daemon-state
// plumbing).
type ServerOptionLister interface {
	ShowAllServerOptions() (string, error)
}

// RestoringChecker is the seam used by IsRestoringSet. Like ServerOptionLister
// it is satisfied implicitly by *tmux.Client (via TryGetServerOption) and
// avoids an import cycle.
//
// TryGetServerOption returns (value, found, err) so absence vs. real failure
// is distinguishable without importing tmux.ErrOptionNotFound here.
type RestoringChecker interface {
	TryGetServerOption(name string) (string, bool, error)
}

// ListSkeletonMarkers enumerates the set of paneKeys whose skeleton markers
// are currently set as tmux server options. A single ShowAllServerOptions
// call replaces N per-pane GetServerOption calls during each capture cycle —
// see specification → Marker Coordination → Enumeration mechanism.
//
// Output lines are tmux's `show-options -sv` format: `@name "value"` (quoted)
// or `@name value` (unquoted). Lines without a value, with an empty value, or
// whose name does not begin with SkeletonMarkerPrefix are silently skipped.
//
// On a ShowAllServerOptions failure the function returns (nil, err) — never a
// partial set. Callers can rely on a non-nil return value indicating success.
func ListSkeletonMarkers(c ServerOptionLister) (map[string]struct{}, error) {
	out, err := c.ShowAllServerOptions()
	if err != nil {
		return nil, err
	}
	set := map[string]struct{}{}
	if out == "" {
		return set, nil
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.IndexAny(line, " \t")
		if idx < 0 {
			// No value separator — treat as absent.
			continue
		}
		name := line[:idx]
		if !strings.HasPrefix(name, SkeletonMarkerPrefix) {
			continue
		}
		value := tmuxout.StripMatchedOuterQuotes(strings.TrimSpace(line[idx+1:]))
		if value == "" {
			// Empty value — treat as absent.
			continue
		}
		paneKey := strings.TrimPrefix(name, SkeletonMarkerPrefix)
		set[paneKey] = struct{}{}
	}
	return set, nil
}

// IsRestoringSet reports whether the @portal-restoring marker is currently
// set to a non-empty value. The daemon's tick checks this at entry and skips
// the capture cycle while bootstrap is mid-skeleton-build (see specification
// → Restoration guard).
//
// Absent and empty-value markers both report false. Any underlying tmux error
// is propagated so a real failure does not silently masquerade as "not
// restoring".
func IsRestoringSet(c RestoringChecker) (bool, error) {
	val, found, err := c.TryGetServerOption(RestoringMarkerName)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	return val != "", nil
}
