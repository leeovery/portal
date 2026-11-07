package migrate

import (
	"context"
	"slices"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/sessionindex"
)

// Waits reports whether a restore of the sessions cfg's state dir owes brings
// any up on its resume panel: an entry portal migrate made, its restore not
// yet finished, with a resume hook in cfg's hooks.json. It is read before the
// bootstrap, since a session's helper finishing its restore clears the mark.
// An index or hooks.json it can't read tells of none.
func Waits(cfg Config) bool {
	idx, err := sessionindex.Read(cfg.StateDir)
	if err != nil {
		return false
	}
	held, err := readHooks(cfg.HooksFile)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(idx.Entries(), func(e sessionindex.Entry) bool {
		return e.ResumeLazy && held[e.Name][hooks.EventOnResume.String()].Command != ""
	})
}

// Left names, in the index's order, each session cfg's state dir still owes
// a restore whose session zmx doesn't hold: one zmx holds is restored, its
// helper still finishing. Every restore brings such a session back, as does
// x <name>.
func Left(ctx context.Context, cfg Config) ([]string, error) {
	idx, err := sessionindex.Read(cfg.StateDir)
	if err != nil {
		return nil, err
	}
	live, err := cfg.Live(ctx)
	if err != nil {
		return nil, err
	}
	var left []string
	for _, e := range idx.Entries() {
		if e.Owed && !slices.Contains(live, e.Name) {
			left = append(left, e.Name)
		}
	}
	return left, nil
}
