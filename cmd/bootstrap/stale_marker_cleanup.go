package bootstrap

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/leeovery/portal/internal/state"
)

// ErrZeroLivePanesWithMarkers is the sentinel returned by CleanStaleMarkers
// when ListAllPanesWithFormat returns no error but parses to zero live
// panes while at least one `@portal-skeleton-*` marker exists. Treating
// such a state as authoritative would mass-unset every marker — including
// markers protecting legitimate hydrate-in-progress panes — and
// destabilise a still-live tmux server. The orchestrator (task 2-5) wraps
// this sentinel as a soft warning rather than abort. See spec §Fix
// Component B (Mass-unset hazard guard).
var ErrZeroLivePanesWithMarkers = errors.New("stale-marker cleanup: zero live panes parsed; skipping to avoid mass-unset hazard")

// MarkerLister enumerates the live `@portal-skeleton-*` server-option markers
// keyed by canonical paneKey (no prefix). The production adapter delegates to
// state.ListSkeletonMarkers; tests inject lightweight fakes.
type MarkerLister interface {
	ListSkeletonMarkers() (map[string]struct{}, error)
}

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

// StaleMarkerCleaner is the orchestrator seam responsible for diffing
// canonical-paneKey markers against live-pane paneKeys and unsetting any
// marker whose paneKey is absent from the live-pane set. Each responsibility
// (marker enumeration, live-pane enumeration, marker unset) is a separate
// small interface so each can be mocked independently in tests, mirroring the
// dependency-shape pattern established by the FIFOSweeper / StaleCleaner
// adjacent seams.
//
// CleanStaleMarkers covers the happy path; later tasks layer on normalisation
// correctness, the mass-unset hazard guard, soft-warning posture, orchestrator
// wiring, adapter wiring, and end-to-end regression.
//
// Logger is optional. When non-nil, soft warnings (per-unset failure,
// malformed live-pane line) are emitted via Logger.Warn under
// ComponentBootstrap. A nil Logger is tolerated — *state.Logger is
// nil-safe and every Warn call is a no-op. This mirrors
// bootstrapadapter.FIFOSweeper's logger contract.
type StaleMarkerCleaner struct {
	Markers  MarkerLister
	Panes    LivePaneLister
	Unsetter MarkerUnsetter
	Logger   *state.Logger
}

// CleanStaleMarkers diffs the marker paneKey-set against the live-pane
// paneKey-set and unsets every marker whose paneKey is not present in the
// live-pane set.
//
// Algorithm:
//  1. Enumerate canonical-paneKey markers via Markers.ListSkeletonMarkers.
//  2. Enumerate live panes via Panes.ListAllPanesWithFormat using the literal
//     `#{session_name}:#{window_index}.#{pane_index}` format string. On
//     error, return without invoking any unset — the orchestrator surfaces
//     the error as a soft warning per spec §Fix Component B.
//  3. Parse each non-empty trimmed line into (session, window, pane) and
//     convert to canonical paneKey form via state.SanitizePaneKey.
//  4. Mass-unset hazard guard: if the parsed live-pane set is empty AND at
//     least one marker exists, return ErrZeroLivePanesWithMarkers without
//     invoking any unset. Treating an empty live set as authoritative would
//     destabilise a still-live tmux server by unsetting every marker —
//     including markers protecting legitimate hydrate-in-progress panes.
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
func (c *StaleMarkerCleaner) CleanStaleMarkers() error {
	markers, err := c.Markers.ListSkeletonMarkers()
	if err != nil {
		return err
	}

	raw, err := c.Panes.ListAllPanesWithFormat(liveFormat)
	if err != nil {
		return err
	}

	live := parseLivePaneSet(raw, c.Logger)

	// Mass-unset hazard guard. The guard MUST run before any unset so a
	// silently-empty live-pane result (whitespace-only output, all-malformed
	// lines, or genuinely zero live panes during tmux instability) cannot
	// fall through to "live set empty → unset every marker".
	if len(live) == 0 {
		if len(markers) == 0 {
			// Empty markers + empty live: nothing to do, no hazard.
			return nil
		}
		return ErrZeroLivePanesWithMarkers
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
			c.Logger.Warn(state.ComponentBootstrap, "stale-marker cleanup: unset %s: %v", name, err)
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
// Logger.Warn breadcrumb when a logger is wired. Malformed lines NEVER
// abort cleanup — including a malformed line in the live set would create a
// spurious "live" entry, while aborting would leave genuinely stale markers
// in place. Both failure modes are worse than skipping. logger may be nil;
// *state.Logger is nil-safe and Warn becomes a no-op.
func parseLivePaneSet(raw string, logger *state.Logger) map[string]struct{} {
	set := map[string]struct{}{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Split on rightmost ':' so session names containing ':' survive.
		colon := strings.LastIndex(line, ":")
		if colon < 0 {
			logger.Warn(state.ComponentBootstrap, "stale-marker cleanup: malformed live-pane line %q (missing colon)", line)
			continue
		}
		session := line[:colon]
		rest := line[colon+1:]
		dot := strings.Index(rest, ".")
		if dot < 0 {
			logger.Warn(state.ComponentBootstrap, "stale-marker cleanup: malformed live-pane line %q (missing dot)", line)
			continue
		}
		window, err := strconv.Atoi(rest[:dot])
		if err != nil {
			logger.Warn(state.ComponentBootstrap, "stale-marker cleanup: malformed live-pane line %q (window not int): %v", line, err)
			continue
		}
		pane, err := strconv.Atoi(rest[dot+1:])
		if err != nil {
			logger.Warn(state.ComponentBootstrap, "stale-marker cleanup: malformed live-pane line %q (pane not int): %v", line, err)
			continue
		}
		set[state.SanitizePaneKey(session, window, pane)] = struct{}{}
	}
	return set
}
