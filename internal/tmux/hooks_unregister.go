package tmux

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// portalCommandSubstrings is the closed set of command-body fingerprints that
// identify a Portal-owned hook entry. Removal targets only entries whose
// command body contains one of these substrings; user and other-plugin
// entries on the same events are left untouched.
//
// The list mirrors every Portal-registered command (notify, signal-hydrate,
// migrate-rename) so that all Portal hooks — including the rename-key
// migration hook on session-renamed — are removable in one pass.
var portalCommandSubstrings = []string{
	"portal state notify",
	"portal state signal-hydrate",
	"portal state migrate-rename",
}

// portalEvents is the closed set of tmux events on which Portal registers
// hooks. UnregisterPortalHooks scopes its removals to these events; matching
// command substrings on any other event (e.g. window-renamed) are ignored.
//
// Order: save-trigger events first, then hydration-trigger events, then
// migrate-rename — mirrors the registration order in RegisterPortalHooks.
// Computed as the deduped union of all three category slices so future
// category additions in RegisterPortalHooks automatically flow through to
// the unregister side. Without this, the unregister side relied on the
// incidental overlap between saveTriggerEvents and migrateRenameEvents
// (both contain session-renamed) — a future event added only to the
// migrate-rename list would silently leak Portal-owned hooks on cleanup.
var portalEvents = dedupedEventList(saveTriggerEvents, hydrationTriggerEvents, migrateRenameEvents)

// dedupedEventList returns the concatenation of the input slices with
// duplicates filtered out, preserving first-seen order.
func dedupedEventList(slices ...[]string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, s := range slices {
		for _, e := range s {
			if _, ok := seen[e]; ok {
				continue
			}
			seen[e] = struct{}{}
			out = append(out, e)
		}
	}
	return out
}

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
