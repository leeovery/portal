package cmd

import (
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tui"
)

// pendingSettleBound outlasts the time a restored session takes to raise its
// resume-pending flag, which it does while still hydrating: its transcript's
// replay, the replay's settle (hydrateSettleSleep) and the mark. The rest is
// for a large transcript on a machine under load; the settle's reads end
// sooner, once no session is hydrating.
const pendingSettleBound = 10 * time.Second

// pendingResumeFlags reads the sessions waiting on their resume panel from
// the state directory dir. A flag can outlive its session, so a caller counts
// only the sessions it lists, as the picker dots only the rows it shows.
type pendingResumeFlags struct{ dir string }

func (f pendingResumeFlags) ListPendingResumeSessions() (map[string]struct{}, error) {
	return raisedFlags(state.ResumePending, f.dir)
}

// hydratingFlags reads the sessions restore has made whose scrollback replay
// hasn't finished, from the state directory dir.
type hydratingFlags struct{ dir string }

func (f hydratingFlags) ListHydratingSessions() (map[string]struct{}, error) {
	return raisedFlags(state.Hydrating, f.dir)
}

func raisedFlags(flag state.SessionFlag, dir string) (map[string]struct{}, error) {
	names, err := flag.List(dir)
	if err != nil {
		return nil, err
	}
	raised := make(map[string]struct{}, len(names))
	for _, name := range names {
		raised[name] = struct{}{}
	}
	return raised, nil
}

// pickerFlagReaders are the picker's reads of the per-session flags: the
// waiting dots' and the settle's after a restore. A state directory that
// can't be resolved leaves both unwired, and the picker shows no dot.
func pickerFlagReaders() (tui.PendingResumeReader, tui.HydratingReader) {
	dir, err := state.Dir()
	if err != nil {
		return nil, nil
	}
	return pendingResumeFlags{dir: dir}, hydratingFlags{dir: dir}
}
