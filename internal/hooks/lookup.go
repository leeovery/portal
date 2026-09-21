package hooks

import "fmt"

// LookupOnResume returns the on-resume command registered for hookKey. A missing
// or malformed hooks.json degrades silently to "no hook", as does an empty
// command; only a genuine I/O error is returned. via names the calling surface
// for the degradation breadcrumb, as it does on the store's other reads.
//
// An empty hookKey is refused before the file is read: the map index is exact,
// so a stray "" entry would otherwise fire its command on every pane that
// carries no key. hookKey is otherwise used verbatim - never trimmed, so a
// whitespace-only key is looked up literally.
func (s *Store) LookupOnResume(hookKey string, via Via) (string, bool, error) {
	if hookKey == "" {
		return "", false, nil
	}
	h, err := s.loadShared(via)
	if err != nil {
		return "", false, fmt.Errorf("load hooks: %w", err)
	}
	events, ok := h[hookKey]
	if !ok {
		return "", false, nil
	}
	registration, ok := events[EventOnResume.String()]
	if !ok || registration.Command == "" {
		return "", false, nil
	}
	return registration.Command, true, nil
}
