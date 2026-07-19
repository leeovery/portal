package tmux

import "fmt"

// PortalHookCountsByEvent reports, for every Portal-managed tmux event, how
// many global hook entries are Portal-authored. It reads each event
// independently via ShowGlobalHooksForEvent — never the no-arg global
// show-hooks -g, which tmux 3.6b renders blind to the pane-* / geometry
// window-* events (the exact events on which duplicate hooks stack) — and
// classifies each entry with the SAME predicate convergeEvent uses:
// containsAny(entry.Command, me.fingerprints). The count therefore reflects
// exactly the entries registration treats as Portal-owned.
//
// The returned map is keyed by event name and always carries every managed
// event, a zero-count event included, so a caller can distinguish "registered
// once" (1), "duplicated" (>=2) and "not registered" (0). A per-event read
// failure returns a nil map and the wrapped error (the transient path a caller
// reports as not-evaluable rather than a false health verdict).
//
// This lives in internal/tmux so managedEvents / fingerprint knowledge never
// leaks into the cmd layer. It is strictly read-only — show-hooks only, no
// set-hook, no mutation.
func PortalHookCountsByEvent(c *Client) (map[string]int, error) {
	counts := make(map[string]int, len(managedEvents))
	for _, me := range managedEvents {
		raw, err := c.ShowGlobalHooksForEvent(me.event)
		if err != nil {
			return nil, fmt.Errorf("show-hooks failed on %s: %w", me.event, err)
		}
		n := 0
		for _, entry := range parseEventEntries(raw, me.event) {
			if containsAny(entry.Command, me.fingerprints) {
				n++
			}
		}
		counts[me.event] = n
	}
	return counts, nil
}
