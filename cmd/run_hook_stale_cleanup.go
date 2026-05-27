package cmd

// runHookStaleCleanup is the single source of truth for the
// "prune hooks.json of entries whose paneKey no longer matches a live
// tmux pane" algorithm. It collapses what used to be six branches of
// duplicated code across three sites (cleanStaleAdapter.CleanStale,
// portal-clean RunE, and the cleanStaleAdapterT test mirror) into one
// helper whose log format strings are declared exactly once. Integration
// tests substring-assert against those format strings; collapsing the
// declarations to a single site eliminates the drift class where a
// reword at one site silently passes against an un-reworded sibling.
//
// Policy axes:
//
//   - listErrorPolicy {returnError, swallow}: how to surface a non-nil
//     err from ListAllPanes. The bootstrap adapter uses returnError
//     (the orchestrator Warn-and-swallows at step 11, so the helper
//     escalates the err up through the StaleCleaner interface). The
//     portal-clean RunE uses swallow because the user-boundary contract
//     pre-fix already silenced this branch — the Warn breadcrumb lands
//     in portal.log for post-hoc audit while the command continues to
//     return nil. Both policies still emit the propagated-error Warn.
//
//   - onRemoved (nil-tolerant): per-removal callback invoked after a
//     successful store.CleanStale. The bootstrap adapter passes nil
//     (nothing to print to the user). portal clean passes a stdout
//     writer that prints "Removed stale hook: <key>" so the user-facing
//     output preserves the pre-extraction contract byte-for-byte.
//
// Algorithm (mirrors the pre-extraction six-branch shape verbatim):
//   1. ListAllPanes. On error apply policy: returnError → emit Warn,
//      return err. swallow → emit Warn, return nil. The entry-point
//      Debug is NOT emitted on this branch (terminal-Warn-only branch).
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
//      CleanStale propagate up verbatim regardless of policy — policy
//      governs ListAllPanes errors only.
//
// Note on duplicate Load: the portal-clean RunE loads the hooks store
// once upfront to check the persisted==0 early-exit, then delegates to
// this helper which loads again. The redundant ReadFile is intentional
// — keeps the helper self-contained (no pre-loaded-map parameter) and
// the second Load observes the same on-disk content. See the parent
// plan task for the explicit accept-the-double-Load decision (Option a).

import (
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/state"
)

// listErrorPolicy selects how runHookStaleCleanup surfaces a non-nil
// err from ListAllPanes. Both values still emit the propagated-error
// Warn breadcrumb under ComponentBootstrap — they differ only in the
// helper's return value.
type listErrorPolicy int

const (
	// returnError makes the helper return the ListAllPanes error to the
	// caller. Used by the bootstrap step-11 adapter so the orchestrator
	// observes the failure (and Warn-and-swallows at its own level).
	returnError listErrorPolicy = iota
	// swallow makes the helper log the Warn and return nil. Used by
	// portal clean's RunE so a transient tmux failure never fails the
	// user's command.
	swallow
)

// runHookStaleCleanup is the shared implementation of bootstrap step 11
// (cleanStaleAdapter.CleanStale) and the portal-clean hook-cleanup tail
// (cleanCmd.RunE). See the package-doc-style block above for the full
// algorithm description, policy axes, and design rationale.
//
// A nil logger is tolerated — substituted with cleanStaleNoopLogger so
// the call sites in this function can invoke logger.Warn / logger.Debug
// unconditionally.
//
// A nil onRemoved is tolerated — the per-removed-entry callback is
// simply skipped when nil (the bootstrap adapter passes nil; portal
// clean passes a stdout writer).
func runHookStaleCleanup(
	lister AllPaneLister,
	store *hooks.Store,
	logger bootstrap.Logger,
	policy listErrorPolicy,
	onRemoved func(string),
) error {
	if logger == nil {
		logger = cleanStaleNoopLogger{}
	}

	livePanes, err := lister.ListAllPanes()
	if err != nil {
		logger.Warn(state.ComponentBootstrap, "stale-hook cleanup: list-panes failed: %v", err)
		if policy == swallow {
			return nil
		}
		return err
	}

	persisted, err := store.Load()
	if err != nil {
		logger.Warn(state.ComponentBootstrap, "stale-hook cleanup: hookStore.Load failed: %v", err)
		return err
	}

	logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: live=%d persisted=%d", len(livePanes), len(persisted))

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
		logger.Warn(state.ComponentBootstrap,
			"stale-hook cleanup: zero live panes parsed with %d hook(s) present; skipping to avoid mass-deletion hazard (next bootstrap retries)",
			len(persisted))
		return nil
	}

	removed, err := store.CleanStale(livePanes)
	if err != nil {
		return err
	}
	logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: removed=%d", len(removed))

	if onRemoved != nil {
		for _, name := range removed {
			onRemoved(name)
		}
	}

	return nil
}
