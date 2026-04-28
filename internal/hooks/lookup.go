package hooks

import "fmt"

// LookupOnResume returns the on-resume command registered for hookKey.
//
// Return contract:
//   - ("", false, nil) — no on-resume hook registered for hookKey, OR
//     hooks.json is missing, OR malformed JSON. Matches Store.Load
//     contract: file-level corruption degrades silently to "no hook".
//   - ("", false, err) — genuine I/O error (e.g. EISDIR, permission denied).
//     Wrapped with "load hooks" prefix.
//   - (cmd, true, nil) — non-empty on-resume command registered for hookKey.
//
// hookKey is the raw saved structural identifier (session:window.pane);
// un-sanitized so colons in session names round-trip verbatim.
//
// Empty-string commands are treated as "no hook" — avoids spawning an
// empty `sh -c ”` from the helper's exec chain.
func LookupOnResume(store *Store, hookKey string) (string, bool, error) {
	h, err := store.Load()
	if err != nil {
		return "", false, fmt.Errorf("load hooks: %w", err)
	}
	events, ok := h[hookKey]
	if !ok {
		return "", false, nil
	}
	cmd, ok := events["on-resume"]
	if !ok || cmd == "" {
		return "", false, nil
	}
	return cmd, true, nil
}
