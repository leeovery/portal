package tmux

import (
	"errors"
	"fmt"
	"slices"
	"sort"
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
// `portal state migrate-rename` is intentionally retained even though the
// register side no longer installs it: older Portal binaries (pre-Task 7-2)
// shipped the inert migrate-rename hook on `session-renamed`, and any
// installation upgrading from one of those builds will still have a stale
// entry sitting in the global hook table. Keeping the substring here means
// a single `portal hooks reset` (or any other path that calls
// UnregisterPortalHooks) cleans those legacy entries up. Drop only once the
// migration window for those binaries is considered closed.
var portalCommandSubstrings = []string{
	"portal state notify",
	"portal state signal-hydrate",
	"portal state migrate-rename",
}

// portalEvents is the closed set of tmux events on which Portal registers
// hooks. UnregisterPortalHooks scopes its removals to these events; matching
// command substrings on any other event (e.g. window-renamed) are ignored.
//
// Order: save-trigger events first, then hydration-trigger events — mirrors
// the registration order in RegisterPortalHooks. The two source slices are
// disjoint by construction (save events fire on the server/window side;
// hydration events fire on the client side), so a plain concatenation is
// sufficient — no deduping needed.
//
// Legacy `migrate-rename` entries from older binaries land on
// `session-renamed`, which is part of saveTriggerEvents, so they remain
// reachable through portalCommandSubstrings without a dedicated event entry.
var portalEvents = slices.Concat(saveTriggerEvents, HydrationTriggerEvents)

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
// "show-hooks failed: %w" leaf naming the event into the aggregate, and the
// loop proceeds so every other event is still torn down.
func UnregisterPortalHooks(c *Client) error {
	var errs []error
	for _, event := range portalEvents {
		raw, err := c.ShowGlobalHooksForEvent(event)
		if err != nil {
			// Canonical show-hooks-failed WARN+wrap: message "show-hooks failed",
			// error_class=unexpected, underlying err wrapped verbatim. Fold and
			// continue so the remaining events are still torn down.
			bootstrapLogger.Warn("show-hooks failed", "error", err, "error_class", "unexpected")
			errs = append(errs, fmt.Errorf("show-hooks failed on %s: %w", event, err))
			continue
		}

		portal := portalEntriesFor(ParseShowHooks(raw)[event])
		sort.Slice(portal, func(i, j int) bool {
			return portal[i].Index > portal[j].Index
		})
		for _, entry := range portal {
			if err := c.UnsetGlobalHookAt(event, entry.Index); err != nil {
				errs = append(errs, fmt.Errorf("unset hook on %s[%d]: %w", event, entry.Index, err))
			}
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
