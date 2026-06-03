package tmux

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/leeovery/portal/internal/log"
)

// bootstrapLogger is the component-bound WARN sink for the per-event read
// failure UnregisterPortalHooks now emits. Bound once at package init so the
// exported UnregisterPortalHooks signature (consumed as a function value by
// cmd/state_cleanup.go) stays unchanged. Mirrors the per-package
// log.For(...) binding pattern used elsewhere in this package.
var bootstrapLogger = log.For("bootstrap")

// portalCommandSubstrings is the closed set of command-body fingerprints that
// identify a Portal-owned hook entry. Removal targets only entries whose
// command body contains one of these substrings; user and other-plugin
// entries on the same events are left untouched.
//
// DERIVED, not hand-authored: it is the union of every managedEvents entry's
// fingerprints (via teardownFingerprints in hooks_register.go) plus the legacy
// migrate-rename substring. Because the registration-side fingerprints flow in
// from managedEvents, adding a future hook category to managedEvents
// automatically widens teardown coverage — a registered category can never
// again become un-reapable. (The session-closed `portal state commit-now`
// fingerprint was missing from the previous hand-authored literal, so the
// converged commit-now hook survived `portal hooks reset`; deriving the set
// closes that seam.) TestPortalTeardownFingerprintParity guards the derivation.
//
// `portal state migrate-rename` is intentionally retained even though the
// register side no longer installs it (it appears in NO managedEvents entry, so
// teardownFingerprints explicitly adds it): older Portal binaries (pre-Task
// 7-2) shipped the inert migrate-rename hook on `session-renamed`, and any
// installation upgrading from one of those builds will still have a stale entry
// sitting in the global hook table. Keeping the substring here means a single
// `portal hooks reset` (or any other path that calls UnregisterPortalHooks)
// cleans those legacy entries up. Drop only once the migration window for those
// binaries is considered closed.
var portalCommandSubstrings = teardownFingerprints()

// portalEvents is the closed set of tmux events on which Portal registers
// hooks. UnregisterPortalHooks scopes its removals to these events; matching
// command substrings on any other event (e.g. window-renamed) are ignored.
//
// Derived — not independently authored — from managedEvents (the convergence
// engine's per-event table in hooks_register.go) by projecting each entry's
// event field. managedEvents is the single source of truth for the
// Portal-managed event-set, so registration and teardown provably operate
// over the identical events and adding a future event to managedEvents
// automatically widens teardown coverage. TestPortalManagedEventSetParity
// guards against accidental re-divergence.
//
// Order mirrors managedEvents declaration order (save-trigger events first,
// then the two hydration-trigger events) — preserved because
// hooks_unregister_test.go's cross-event removal-order assertion follows it.
//
// Legacy `migrate-rename` entries from older binaries land on
// `session-renamed`, which is one of the projected events, so they remain
// reachable through portalCommandSubstrings without a dedicated event entry.
var portalEvents = managedEventNames()

// UnregisterPortalHooks removes every Portal-owned hook entry from the global
// tmux hook table.
//
// Algorithm:
//  1. For each event in portalEvents, read that event's entries via the
//     per-event seam ShowGlobalHooksForEvent(event). The no-arg show-hooks -g
//     enumeration is deliberately NOT used: tmux 3.6b omits an entire class of
//     events (pane-* and the geometry/rename window-* events) from the global
//     read, so a single global enumeration sees zero Portal entries on those
//     blind events and would remove nothing. Reading per-event sidesteps the
//     blind spot.
//  2. Collect that event's entries whose command body contains any
//     portalCommandSubstrings.
//  3. Remove those entries via set-hook -gu in descending index order — so a
//     removal never shifts a not-yet-processed index.
//
// Non-Portal entries on the same events are left untouched. Matching
// substrings on events outside portalEvents are ignored (each event is read in
// isolation). Per-removal failures do not short-circuit the loop — every
// removal is attempted and errors are aggregated via errors.Join, with each
// leaf error naming the failing event[index] and wrapping the underlying tmux
// error.
//
// Per-event read failure is fold-and-continue (not all-or-nothing): a failing
// ShowGlobalHooksForEvent emits the canonical show-hooks-failed WARN
// (error_class=unexpected) under the bootstrap component, folds a
// "show-hooks failed on <event>: %w" leaf into the aggregate, and the loop
// proceeds so every other event is still torn down.
//
// The exported signature is func(*Client) error — it is consumed as a function
// value by cmd/state_cleanup.go — so this stays a thin wrapper that binds the
// package-level bootstrap WARN sink and delegates to unregisterPortalHooks.
func UnregisterPortalHooks(c *Client) error {
	return unregisterPortalHooks(c, bootstrapLogger)
}

// unregisterPortalHooks is the injected-logger teardown variant. The logger is
// the sink for the per-event read-failure WARN (production passes the
// package-level bootstrapLogger via the UnregisterPortalHooks wrapper; tests
// pass a recording logger to assert the WARN shape through the same seam the
// register side uses). It routes the per-event read+WARN and the descending-
// index unset loop through the same shared helpers convergeEvent uses
// (warnShowHooksFailure + evictPortalEntries), so the two paths cannot drift.
//
// A nil logger is tolerated and falls through to the shared internal/log
// discard sink via log.OrDiscard.
func unregisterPortalHooks(c *Client, logger *slog.Logger) error {
	logger = log.OrDiscard(logger)

	var errs []error
	for _, event := range portalEvents {
		raw, err := c.ShowGlobalHooksForEvent(event)
		if err != nil {
			// Canonical show-hooks-failed WARN (one production definition in
			// warnShowHooksFailure) followed by the teardown-specific
			// event-named wrap. Fold and continue so the remaining events are
			// still torn down.
			warnShowHooksFailure(logger, err)
			errs = append(errs, fmt.Errorf("show-hooks failed on %s: %w", event, err))
			continue
		}

		// Filter with teardown's fingerprint set (portalCommandSubstrings,
		// which retains the legacy migrate-rename substring registration never
		// installs), then route through the shared descending-index unset loop.
		portal := portalEntriesFor(parseEventEntries(raw, event))
		_, failures := evictPortalEntries(c, event, portal)
		for _, f := range failures {
			// Teardown's contract folds each per-index failure into the
			// errors.Join aggregate naming the failing event[index].
			errs = append(errs, fmt.Errorf("unset hook on %s[%d]: %w", event, f.index, f.err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// portalEntriesFor filters entries down to those whose command body contains
// any of portalCommandSubstrings. Order is preserved.
func portalEntriesFor(entries []HookEntry) []HookEntry {
	var out []HookEntry
	for _, entry := range entries {
		if containsAny(entry.Command, portalCommandSubstrings) {
			out = append(out, entry)
		}
	}
	return out
}

// containsAny reports whether s contains any of the supplied substrings.
func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
