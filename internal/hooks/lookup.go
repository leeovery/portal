package hooks

import (
	"fmt"

	"github.com/leeovery/portal/internal/resumemode"
)

// OnResume is what a lookup found: the registered command and the mode it is
// pinned to, if any. Mode reports what is stored and nothing more — resolving
// it against the install-wide default belongs to the caller that knows both. A
// miss is the zero value, so Found discriminates it from a registration.
type OnResume struct {
	Command string
	Mode    resumemode.Mode
	Found   bool
}

// LookupOnResume returns the on-resume registration for hookKey. A missing or
// malformed hooks.json degrades silently to "no hook", as does a stored value
// carrying no command; only a genuine I/O error is returned, alongside the zero
// result. via names the calling surface for the degradation breadcrumb, as it
// does on the store's other reads.
//
// An empty hookKey is refused before the file is read: the map index is exact,
// so a stray "" entry would otherwise fire its command on every pane that
// carries no key. hookKey is otherwise used verbatim - never trimmed, so a
// whitespace-only key is looked up literally.
func (s *Store) LookupOnResume(hookKey string, via Via) (OnResume, error) {
	if hookKey == "" {
		return OnResume{}, nil
	}
	h, err := s.loadShared(via)
	if err != nil {
		return OnResume{}, fmt.Errorf("load hooks: %w", err)
	}
	registration, ok := h[hookKey][EventOnResume.String()]
	if !ok || registration.Command == "" {
		return OnResume{}, nil
	}
	return OnResume{Command: registration.Command, Mode: registration.Resume, Found: true}, nil
}
