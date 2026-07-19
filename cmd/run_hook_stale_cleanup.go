package cmd

// runHookStaleCleanup is the single source of truth for the
// "prune hooks.json of entries whose paneKey no longer matches a live
// tmux pane" algorithm. It is the shared implementation behind two live
// callers:
//
//   - the daemon's throttled hook cleanup, maybeRunHookCleanup
//     (cmd/state_daemon.go), which runs it once per hookCleanupInterval on
//     the tick loop's idle branch — the sole AUTOMATIC caller; and
//   - the doctor --fix stale-hook prune, pruneDoctorStaleHooks
//     (cmd/doctor.go), the user-triggered manual repair.
//
// Its log format strings are declared exactly once here. Integration tests
// substring-assert against those format strings; collapsing the declarations
// to a single site eliminates the drift class where a reword at one site
// silently passes against an un-reworded sibling.
//
// Both callers treat a non-nil err from ListAllPaneHookKeys as Warn-and-continue:
// the helper emits the "stale-hook cleanup: list-panes failed" Warn to
// portal.log for post-hoc audit and returns nil, so a transient tmux read
// never fails the daemon tick or the user's command. There is no policy
// parameter — neither live caller wants a propagated ListAllPaneHookKeys error.
// hookStore.Load and store.CleanStale errors DO propagate (return err); each
// live caller handles that non-nil return itself (the daemon logs WARN and
// swallows; doctor --fix discards it after the canonical Warn is already in
// portal.log, leaving the entries for the re-diagnosis to report).
//
//   - onRemoved (nil-tolerant): per-removal callback invoked after a
//     successful store.CleanStale. The daemon passes nil (nothing to print
//     to the user). doctor --fix passes a stdout writer that prints
//     "Pruned stale hook: <key>" so each repair is surfaced to the user.
//
// Algorithm:
//   1. ListAllPaneHookKeys. On error emit Warn and return nil (Warn-and-continue).
//      The entry-point Debug is NOT emitted on this branch (terminal-Warn-only
//      branch).
//   2. store.Load. On error emit Warn, return err. The destructive
//      CleanStale call is NOT made on this branch.
//   3. Entry-point Debug breadcrumb with live + persisted counts.
//   4. Mass-deletion hazard guard: if live is empty AND persisted is
//      non-empty, emit hazard Warn and return nil (deferral, not an
//      error). Treating an empty live set as authoritative would
//      destabilise a still-live tmux server.
//   5. Both-empty: return nil (no Warn, no completion Debug — nothing
//      to do and no hazard to guard against).
//   6. store.CleanStale. On success, emit completion Debug and invoke
//      onRemoved once per removed key (when non-nil). Errors from
//      CleanStale propagate up verbatim.

import (
	"log/slog"

	"github.com/leeovery/portal/internal/hooks"
)

// AllPaneLister returns each live pane's hook key across all tmux sessions —
// the live set that feeds hooks.Store.CleanStale. Each key has the form
// <@portal-id or session_name>:window_index.pane_index, resolved per-session
// by tmux.HookKeyFormat (a stamped session yields "<id>:w.p", an un-stamped one
// "<name>:w.p"). This is the hook-key sibling of the name-based structural
// enumeration; the cleanup live set must derive keys the same way registration
// does so freshly-registered id-keyed entries are not mass-orphaned. Consumed by
// runHookStaleCleanup (below), the daemon's maybeRunHookCleanup
// (cmd/state_daemon.go), and doctor's stale-hooks check + --fix prune
// (cmd/doctor.go); production wiring supplies *tmux.Client, which satisfies it
// via ListAllPaneHookKeys.
type AllPaneLister interface {
	ListAllPaneHookKeys() ([]string, error)
}

// runHookStaleCleanup is the shared implementation of the daemon's throttled
// hook cleanup (maybeRunHookCleanup, cmd/state_daemon.go) and the doctor --fix
// stale-hook prune (pruneDoctorStaleHooks, cmd/doctor.go). See the
// package-doc-style block above for the full algorithm description and design
// rationale.
//
// A non-nil err from ListAllPaneHookKeys is Warn-and-continue: the helper logs the
// list-panes Warn and returns nil. hookStore.Load and store.CleanStale errors
// still propagate to the caller as a non-nil return.
//
// A nil logger is tolerated — substituted with the bootstrap package's
// discard logger so the call sites in this function can invoke logger.Warn /
// logger.Debug unconditionally. Production callers pass the bootstrap
// component's *slog.Logger.
//
// A nil onRemoved is tolerated — the per-removed-entry callback is
// simply skipped when nil (the daemon passes nil; doctor --fix passes a
// stdout writer).
func runHookStaleCleanup(
	lister AllPaneLister,
	store *hooks.Store,
	logger *slog.Logger,
	onRemoved func(string),
) error {
	if logger == nil {
		logger = bootstrapLogger
	}

	livePanes, err := lister.ListAllPaneHookKeys()
	if err != nil {
		logger.Warn("stale-hook cleanup: list-panes failed", "error", err)
		return nil
	}

	persisted, err := store.Load()
	if err != nil {
		logger.Warn("stale-hook cleanup: hookStore.Load failed", "error", err)
		return err
	}

	logger.Debug("stale-hook cleanup counts", "panes", len(livePanes), "entries", len(persisted))

	// Mass-deletion hazard guard — must run before any destructive
	// CleanStale invocation so a silently-empty live-pane result cannot
	// fall through to "live set empty → delete every hooks.json entry".
	// The deferral surfaces via Logger.Warn so the error channel
	// exclusively carries genuine dependency failures.
	if len(livePanes) == 0 {
		if len(persisted) == 0 {
			// Empty persisted + empty live: nothing to do, no hazard.
			return nil
		}
		logger.Warn("stale-hook cleanup: zero live panes parsed with hooks present; skipping to avoid mass-deletion hazard (next bootstrap retries)", "entries", len(persisted))
		return nil
	}

	removed, err := store.CleanStale(livePanes)
	if err != nil {
		return err
	}
	logger.Debug("stale-hook cleanup removed", "reaped", len(removed))

	if onRemoved != nil {
		for _, name := range removed {
			onRemoved(name)
		}
	}

	return nil
}
