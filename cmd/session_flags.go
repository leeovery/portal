package cmd

import (
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tui"
)

// pendingSettleBound outlasts the longest a hydrate helper keeps its session
// hydrating once its replay is done, about 7 s: the read of the session's
// creation (hydrateSessionBound) and its claim's wait for commit.lock (5 s).
// So a restored session left waiting on its panel has its dot by then.
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

// waitingSessions counts the sessions listed in sessions whose resume panel
// waits, as the state directory dir's flags say.
func waitingSessions(dir string, sessions []backend.Session) (int, error) {
	pending, err := pendingResumeFlags{dir: dir}.ListPendingResumeSessions()
	if err != nil {
		return 0, err
	}
	waiting := 0
	for _, s := range sessions {
		if _, ok := pending[s.Name]; ok {
			waiting++
		}
	}
	return waiting, nil
}
