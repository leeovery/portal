package tmux

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

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
//  1. Read the global hook table via show-hooks -g.
//  2. For each event in portalEvents, collect entries whose command body
//     contains any portalCommandSubstrings.
//  3. Remove those entries via set-hook -gu in reverse index order — defensive
//     against any edge case where index renumbering could shift surviving
//     entries (tmux 3.0+ does not renumber, but reverse order is cheap insurance).
//
// Non-Portal entries on the same events are left untouched. Matching
// substrings on events outside portalEvents are ignored. Per-removal failures
// do not short-circuit the loop — every removal is attempted and errors are
// aggregated via errors.Join, with each leaf error naming the failing
// event[index] and wrapping the underlying tmux error.
//
// On show-hooks failure the error is wrapped with "show-hooks failed: %w" and
// no removal is attempted.
func UnregisterPortalHooks(c *Client) error {
	raw, err := c.ShowGlobalHooks()
	if err != nil {
		return fmt.Errorf("show-hooks failed: %w", err)
	}

	entries := ParseShowHooks(raw)

	var errs []error
	for _, event := range portalEvents {
		portal := portalEntriesFor(entries[event])
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
