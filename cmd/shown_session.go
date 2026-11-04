package cmd

import (
	"context"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tui"
)

// shownIndex is the index as the surfaces that name a session read it, with
// the rule a claim judges each entry by, so a surface shows what a claim of
// the session would keep.
type shownIndex struct {
	idx  sessionindex.Index
	rule sessionindex.Rule
}

// readShownIndex reads the state directory's index for showing. Any error
// returns the empty shownIndex, which shows every session by its name.
func readShownIndex() (shownIndex, error) {
	dir, err := state.Dir()
	if err != nil {
		return shownIndex{}, err
	}
	idx, err := readSessionIndexIn(dir)
	if err != nil {
		return shownIndex{}, err
	}
	return shownIndex{idx: idx, rule: claimRule(dir)}, nil
}

// show joins a live session with the index: what every surface that names a
// session shows of it. The entry counts for the session's own creation, and
// for one a claim would take over; a session recreated under the name, or one
// that did not answer and so reports no creation, shows its name and no
// recorded dir.
func (s shownIndex) show(live backend.Session) tui.Session {
	entry, _ := s.idx.LookupLive(live.Name, live.Created, live.PID, s.rule)
	return tui.Session{
		Name:         live.Name,
		Label:        entry.Label,
		Dir:          entry.Dir,
		Cwd:          live.Dir,
		Attached:     live.Attached(),
		NotAnswering: !live.Answered,
	}
}

// listShownSessions is the user's sessions as shown. An index that cannot be
// read costs only what it records: each session still shows, by its name.
func listShownSessions(ctx context.Context, b backend.Backend) ([]tui.Session, error) {
	sessions, err := userSessions(ctx, b)
	if err != nil {
		return nil, err
	}
	shown, _ := readShownIndex()
	out := make([]tui.Session, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, shown.show(s))
	}
	return out, nil
}

// pickerLister serves the picker its rows.
type pickerLister struct {
	ctx context.Context
	b   backend.Backend
}

func (l pickerLister) ListSessions() ([]tui.Session, error) {
	return listShownSessions(l.ctx, l.b)
}
