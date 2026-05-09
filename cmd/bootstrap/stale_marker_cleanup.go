package bootstrap

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/leeovery/portal/internal/state"
)

// stale_marker_cleanup.go's Logger field uses the package-local Logger
// interface (Debug/Warn/Error) — same shape every other orchestrator step
// seam depends on. *state.Logger satisfies the interface structurally,
// keeping production wiring (cmd/bootstrap_production.go's inline
// MarkerCleanupCore literal) untouched while letting tests inject a plain
// in-memory recording fake instead of opening a real on-disk log file.

// LivePaneLister enumerates live tmux panes via tmux's list-panes -a -F format
// call. The production adapter delegates to (*tmux.Client).ListAllPanesWithFormat
// — the error-propagating variant required by spec §Fix Component B (Adapter
// Wiring) so a tmux failure surfaces as a soft warning rather than a
// silently-empty result that would mass-unset every marker.
type LivePaneLister interface {
	ListAllPanesWithFormat(format string) (string, error)
}

// MarkerUnsetter clears a single tmux server-option by name. The production
// adapter delegates to (*tmux.Client).UnsetServerOption; tests inject
// recording fakes to assert which option names were unset.
type MarkerUnsetter interface {
	UnsetServerOption(name string) error
}

// liveFormat is the canonical tmux format string the cleanup step requests
// from list-panes -a. Each output line is `session:window.pane`, parsed via
// strconv.Atoi and converted to canonical paneKey form via
// state.SanitizePaneKey before set-difference computation.
const liveFormat = "#{session_name}:#{window_index}.#{pane_index}"

// MarkerCleanupCore is the orchestrator seam responsible for diffing
// canonical-paneKey markers against live-pane paneKeys and unsetting any
// marker whose paneKey is absent from the live-pane set. Each responsibility
// (marker enumeration, live-pane enumeration, marker unset) is a separate
// small interface so each can be mocked independently in tests, mirroring the
// dependency-shape pattern established by the FIFOSweeper / StaleCleaner
// adjacent seams.
//
// Logger is optional. When non-nil, soft warnings (per-unset failure,
// malformed live-pane line) are emitted via Logger.Warn under
// ComponentBootstrap. A nil Logger is tolerated — CleanStaleMarkers
// substitutes a no-op default at entry so call sites can dispatch
// unconditionally. This mirrors the Orchestrator's Logger contract;
// the field's interface type matches every other orchestrator step seam
// (HookRegistrar, FIFOSweeper et al.) so tests inject the same
// recordingLogger fake used elsewhere in the package.
type MarkerCleanupCore struct {
	// Markers mirrors FIFOSweeper.Client — *tmux.Client satisfies
	// state.ServerOptionLister directly via ShowAllServerOptions, so no
	// closure adapter glue is needed at the wiring site.
	Markers  state.ServerOptionLister
	Panes    LivePaneLister
	Unsetter MarkerUnsetter
	Logger   Logger
}

var _ MarkerCleaner = (*MarkerCleanupCore)(nil)

// CleanStaleMarkers diffs the marker paneKey-set against the live-pane
// paneKey-set and unsets every marker whose paneKey is not present in the
// live-pane set.
//
// Algorithm:
//  1. Enumerate canonical-paneKey markers via state.ListSkeletonMarkers(c.Markers).
//  2. Enumerate live panes via Panes.ListAllPanesWithFormat using the literal
//     `#{session_name}:#{window_index}.#{pane_index}` format string. On
//     error, return without invoking any unset — the orchestrator surfaces
//     the error as a soft warning per spec §Fix Component B.
//  3. Parse each non-empty trimmed line into (session, window, pane) and
//     convert to canonical paneKey form via state.SanitizePaneKey.
//  4. Mass-unset hazard guard: if the parsed live-pane set is empty AND at
//     least one marker exists, emit a Logger.Warn (component=bootstrap)
//     describing the deferral (including marker count) and return nil
//     without invoking any unset. Treating an empty live set as
//     authoritative would destabilise a still-live tmux server by
//     unsetting every marker — including markers protecting legitimate
//     hydrate-in-progress panes. The deferral is a successful soft
//     outcome ("skip this run; next bootstrap retries"), not a failure;
//     surfacing it as a return error would conflate it with genuine
//     dependency failures.
//  5. If the parsed live-pane set is empty AND no markers exist, return nil
//     — there is nothing to do and no hazard to guard against.
//  6. For each marker paneKey absent from the live set, invoke
//     Unsetter.UnsetServerOption(state.SkeletonMarkerPrefix + paneKey).
//
// CleanStaleMarkers never returns a *FatalError; every non-nil return is
// soft per spec §Fix Component B (Soft-Warning Posture). Per-marker unset
// failures are accumulated via errors.Join and the loop continues so a
// single transient tmux error never leaves genuinely-stale markers in
// place. Malformed live-pane lines are silently skipped inside
// parseLivePaneSet (with a Logger.Warn breadcrumb when a Logger is wired)
// rather than aborting cleanup, since aborting would also leave stale
// markers in place.
func (c *MarkerCleanupCore) CleanStaleMarkers() error {
	// Substitute a no-op Logger when none was injected so call sites can
	// invoke logger.Warn unconditionally, matching the Orchestrator's
	// Logger contract. Use a local var rather than mutating c.Logger so
	// the receiver's state is not silently rewritten across calls.
	logger := c.Logger
	if logger == nil {
		logger = noopLogger{}
	}

	markers, err := state.ListSkeletonMarkers(c.Markers)
	if err != nil {
		return err
	}

	raw, err := c.Panes.ListAllPanesWithFormat(liveFormat)
	if err != nil {
		return err
	}

	live := parseLivePaneSet(raw, logger)

	// Mass-unset hazard guard. The guard MUST run before any unset so a
	// silently-empty live-pane result (whitespace-only output, all-malformed
	// lines, or genuinely zero live panes during tmux instability) cannot
	// fall through to "live set empty → unset every marker". The deferral
	// surfaces via Logger.Warn so the error channel of CleanStaleMarkers
	// exclusively carries genuine dependency failures.
	if len(live) == 0 {
		if len(markers) == 0 {
			// Empty markers + empty live: nothing to do, no hazard.
			return nil
		}
		logger.Warn(state.ComponentBootstrap,
			"stale-marker cleanup: zero live panes parsed with %d marker(s) present; skipping to avoid mass-unset hazard (next bootstrap retries)",
			len(markers))
		return nil
	}

	var unsetErrs []error
	for paneKey := range markers {
		if _, alive := live[paneKey]; alive {
			continue
		}
		name := state.SkeletonMarkerPrefix + paneKey
		if err := c.Unsetter.UnsetServerOption(name); err != nil {
			// Record and continue: a single tmux transient must not
			// leave the remaining stale markers in place. The
			// orchestrator (task 2-5) Warn-and-swallows the aggregate.
			logger.Warn(state.ComponentBootstrap, "stale-marker cleanup: unset %s: %v", name, err)
			unsetErrs = append(unsetErrs, fmt.Errorf("unset %s: %w", name, err))
		}
	}
	if len(unsetErrs) > 0 {
		return errors.Join(unsetErrs...)
	}
	return nil
}

// parseLivePaneSet parses tmux's list-panes -a output (one
// `session:window.pane` entry per line) into a set of canonical paneKeys via
// state.SanitizePaneKey. Empty lines are silently skipped; lines that fail
// the parse contract (rightmost `:` for session/window.pane split, `.` for
// window/pane split, strconv.Atoi for indices) are skipped with a soft
// Logger.Warn breadcrumb. Malformed lines NEVER abort cleanup — including a
// malformed line in the live set would create a spurious "live" entry,
// while aborting would leave genuinely stale markers in place. Both failure
// modes are worse than skipping. logger must be non-nil; CleanStaleMarkers
// substitutes a no-op default before invoking parseLivePaneSet.
func parseLivePaneSet(raw string, logger Logger) map[string]struct{} {
	warn := func(line, reason string, args ...any) {
		logger.Warn(state.ComponentBootstrap,
			"stale-marker cleanup: malformed live-pane line %q ("+reason+")",
			append([]any{line}, args...)...)
	}
	set := map[string]struct{}{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Split on rightmost ':' so session names containing ':' survive.
		colon := strings.LastIndex(line, ":")
		if colon < 0 {
			warn(line, "missing colon")
			continue
		}
		session := line[:colon]
		rest := line[colon+1:]
		dot := strings.Index(rest, ".")
		if dot < 0 {
			warn(line, "missing dot")
			continue
		}
		window, err := strconv.Atoi(rest[:dot])
		if err != nil {
			warn(line, "window not int: %v", err)
			continue
		}
		pane, err := strconv.Atoi(rest[dot+1:])
		if err != nil {
			warn(line, "pane not int: %v", err)
			continue
		}
		set[state.SanitizePaneKey(session, window, pane)] = struct{}{}
	}
	return set
}
