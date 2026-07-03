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

// BootstrappedMarkerName is the tmux server-option name of the version-stamped
// bootstrap latch. It uses the same server-option mechanism as
// RestoringMarkerName — it dies with the tmux server, so a server restart
// auto-clears it and the next command re-runs a full bootstrap. It differs in
// that its VALUE is load-bearing (the running binary version), not
// presence-only: satisfaction is a plain equality against the running version
// (see BootstrappedLatchSatisfied), so a post-upgrade binary re-bootstraps and
// re-stamps on its first command.
const BootstrappedMarkerName = "@portal-bootstrapped"

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
// is distinguishable: absence is reported as (value, false, nil); real
// failures surface as ("", false, non-nil-err). Callers can use
// errors.Is(err, tmux.ErrOptionNotFound) on the underlying GetServerOption
// path to identify genuine absence (TryGetServerOption already does this
// internally on this package's behalf, so internal/state does not need to
// import internal/tmux).
type RestoringChecker interface {
	TryGetServerOption(name string) (string, bool, error)
}

// ServerOptionWriter is the seam used by SetSkeletonMarker / UnsetSkeletonMarker.
// It is satisfied implicitly by *tmux.Client via its SetServerOption /
// UnsetServerOption methods. Defining the interface here keeps internal/state
// free of an internal/tmux import — see ServerOptionLister for the full
// rationale.
type ServerOptionWriter interface {
	SetServerOption(name, value string) error
	UnsetServerOption(name string) error
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
	for line := range strings.SplitSeq(out, "\n") {
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

// SetSkeletonMarker writes the `@portal-skeleton-<paneKey>` server option to
// "1", marking the pane as skeleton-restored and awaiting hydration. The save
// loop consumes these markers (via ListSkeletonMarkers) to skip capturing
// scrollback for marked panes — the on-disk pre-boot scrollback file is the
// authoritative state until hydration completes.
//
// The companion to UnsetSkeletonMarker; both are the canonical write-side
// helpers, colocated with ListSkeletonMarkers (the read side) and the prefix
// constant.
func SetSkeletonMarker(w ServerOptionWriter, paneKey string) error {
	return w.SetServerOption(SkeletonMarkerPrefix+paneKey, "1")
}

// UnsetSkeletonMarker clears the `@portal-skeleton-<paneKey>` server option,
// signalling that the pane has been hydrated and the save loop may resume
// capturing its scrollback.
//
// The companion to SetSkeletonMarker; both are the canonical write-side
// helpers, colocated with ListSkeletonMarkers (the read side) and the prefix
// constant.
func UnsetSkeletonMarker(w ServerOptionWriter, paneKey string) error {
	return w.UnsetServerOption(SkeletonMarkerPrefix + paneKey)
}

// UnsetSkeletonMarkerForFIFO clears the skeleton marker for the pane whose
// hydration FIFO is fifoPath. It composes PaneKeyFromFIFOPath (recovering the
// canonical paneKey embedded in the FIFO basename) with UnsetSkeletonMarker,
// encoding the FIFOPath ⇄ paneKey invariant in a single helper so callers
// holding only the FIFO path do not have to derive the paneKey themselves.
//
// See PaneKeyFromFIFOPath for the inverse mapping (basename → paneKey) and
// FIFOPath for the forward mapping (paneKey → basename).
func UnsetSkeletonMarkerForFIFO(w ServerOptionWriter, fifoPath string) error {
	return UnsetSkeletonMarker(w, PaneKeyFromFIFOPath(fifoPath))
}

// IsRestoringSet reports whether the @portal-restoring marker is currently
// set to a non-empty value. The daemon's tick checks this at entry and skips
// the capture cycle while bootstrap is mid-skeleton-build (see specification
// → Restoration guard).
//
// Absent and empty-value markers both report false. Any underlying tmux error
// is propagated so a real failure does not silently masquerade as "not
// restoring"; the propagated error wraps a *tmux.CommandError recoverable via
// errors.As, carrying the captured stderr for diagnosis.
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

// BootstrappedLatchSatisfied reports whether the @portal-bootstrapped latch is
// satisfied for runningVersion — i.e. the latch is present AND its stored value
// exactly equals runningVersion. A single TryGetServerOption read drives a
// four-outcome verdict, all folded into one boolean:
//
//	absent (found == false)      -> false (cold/fresh server → full bootstrap)
//	present + value matches      -> true  (already bootstrapped this binary)
//	present + value mismatches   -> false (post-upgrade → full bootstrap)
//	read error / down server     -> false (unreadable → full bootstrap)
//
// The comparison is a naive parse-free string equality (stored ==
// runningVersion) because the stored value format is exactly cmd.version in v1,
// with no forensic extras. An empty stored value (found == true, val == "") is
// therefore not satisfied unless runningVersion is itself empty; production
// always passes a non-empty version, so this falls out of plain equality rather
// than a special case.
//
// Both "value mismatch" and "unreadable/error" deliberately fold into
// not-satisfied: a down server makes the read fail gracefully, so no separate
// ServerRunning() probe is needed. Unlike IsRestoringSet — which propagates its
// error so a real failure cannot masquerade as "not restoring" — this helper
// intentionally swallows the read error into a bare bool: the Phase 2 consumer
// wants a single verdict, and "unreadable" correctly maps to "full bootstrap."
// Do not "fix" this into a (bool, error) signature.
//
// runningVersion is a plain string parameter (not read from cmd.version) so
// internal/state stays a leaf — importing cmd would close a cycle, since
// internal/tmux imports internal/state — and so the version-mismatch branch is
// unit-testable without rebuilding the binary.
func BootstrappedLatchSatisfied(c RestoringChecker, runningVersion string) bool {
	val, found, err := c.TryGetServerOption(BootstrappedMarkerName)
	if err != nil {
		return false
	}
	if !found {
		return false
	}
	return val == runningVersion
}
