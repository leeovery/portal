package cmd

import (
	"context"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/tui"
)

// shownSession joins a live session with the index: what every surface that
// names a session shows of it. The entry counts only for the session's own
// creation, so one recreated under the name, or one that did not answer and
// so reports no creation, shows its name and no recorded dir.
func shownSession(s backend.Session, idx *sessionindex.Index) tui.Session {
	entry, _ := idx.LookupLive(s.Name, s.Created)
	return tui.Session{
		Name:         s.Name,
		Label:        entry.Label,
		Dir:          entry.Dir,
		Cwd:          s.Dir,
		Attached:     s.Attached(),
		NotAnswering: !s.Answered,
	}
}

func shownSessions(sessions []backend.Session, idx *sessionindex.Index) []tui.Session {
	out := make([]tui.Session, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, shownSession(s, idx))
	}
	return out
}

// listShownSessions is the user's sessions as shown. An index that cannot be
// read costs only what it records: each session still shows, by its name.
func listShownSessions(ctx context.Context, b backend.Backend) ([]tui.Session, error) {
	sessions, err := userSessions(ctx, b)
	if err != nil {
		return nil, err
	}
	idx, _ := readSessionIndex()
	return shownSessions(sessions, &idx), nil
}

// pickerLister serves the picker its rows.
type pickerLister struct {
	ctx context.Context
	b   backend.Backend
}

func (l pickerLister) ListSessions() ([]tui.Session, error) {
	return listShownSessions(l.ctx, l.b)
}
