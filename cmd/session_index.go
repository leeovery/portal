package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

var indexLogger = log.For("sessions")

// legacyIndexError is a version 1 sessions.json, which belongs to the tmux
// version of Portal until portal migrate moves it aside.
type legacyIndexError struct {
	path string
}

func (e legacyIndexError) Error() string {
	return e.path + " is from the tmux version of Portal; run portal migrate to move it aside"
}

func (legacyIndexError) Unwrap() error { return sessionindex.ErrLegacyIndex }

// readSessionIndexIn reads sessions.json from the state directory dir; a
// missing file is an empty index.
func readSessionIndexIn(dir string) (sessionindex.Index, error) {
	idx, err := sessionindex.Read(dir)
	if errors.Is(err, sessionindex.ErrLegacyIndex) {
		return sessionindex.Index{}, legacyIndexError{path: sessionindex.Path(dir)}
	}
	if err != nil {
		return sessionindex.Index{}, fmt.Errorf("state directory %s: %w", dir, err)
	}
	return idx, nil
}

// updateSessionIndexLanding runs fn on the index in the state directory dir
// under the commit lock, writing it back when fn changed something, then runs
// landed under the same lock, once the update has landed: with what it
// dropped, empty when it changed nothing. landed runs for no update that
// failed, and an error it returns is the update's, whose write landed all the
// same. via is cli, tui or internal.
func updateSessionIndexLanding(dir, via string, fn func(*sessionindex.Index) error, landed func(sessionindex.Outcome) error) error {
	_, err := sessionindex.UpdateThen(dir, via, fn, landed)
	if errors.Is(err, sessionindex.ErrLegacyIndex) {
		return legacyIndexError{path: sessionindex.Path(dir)}
	}
	return err
}

// forgetting is a landing that forgets the entries picked chooses from what an
// update dropped.
func forgetting(dir, via string, picked func(sessionindex.Outcome) []sessionindex.Entry) func(sessionindex.Outcome) error {
	return func(outcome sessionindex.Outcome) error {
		forgetEntries(dir, picked(outcome), via)
		return nil
	}
}

// claimRule is the rule each claim judges by; tests fix the boot it reads.
var claimRule = state.ClaimRule

// claimed is a live session as its backend reports it, for a claim of its
// entry: Dir is where Portal minted it, empty for any other claimer.
type claimed struct {
	Name, Dir string
	Created   time.Time
	PID       int
}

// claimSession claims the live session's entry by the one rule every claim
// judges by, then runs then, when given, on the index in the same write, so
// what then changes is this creation's. Once the write has landed and what it
// dropped is forgotten, it runs afterward, when given, still under the commit
// lock, so no kill or claim lands between the claim and what afterward writes
// beside the entry.
func claimSession(via string, live claimed, then func(*sessionindex.Index) error, afterward func() error) error {
	dir, err := state.EnsureDir()
	if err != nil {
		return err
	}
	rule := claimRule(dir)
	return updateSessionIndexLanding(dir, via, func(idx *sessionindex.Index) error {
		idx.Claim(live.Name, live.Dir, live.Created, live.PID, rule)
		if then == nil {
			return nil
		}
		return then(idx)
	}, func(outcome sessionindex.Outcome) error {
		forgetEntries(dir, outcome.Dropped(), via)
		if afterward == nil {
			return nil
		}
		return afterward()
	})
}

// forget is state.Forget; tests pause it to hold the commit lock open.
var forget = state.Forget

// forgetEntries forgets each of entries, as the index in dir held them before
// a write dropped them.
func forgetEntries(dir string, entries []sessionindex.Entry, via string) {
	if len(entries) == 0 {
		return
	}
	store, err := loadHookStore()
	if err != nil {
		for _, entry := range entries {
			indexLogger.Warn("forget", "op", "forget", "session", entry.Name, "detail", "kept its resume hook", "error", err, "via", via)
		}
	}
	forget(dir, store, via, entries...)
}

// refuseUnusableState stops a command while the state directory holds an
// index this version must not touch, as refuseUnusableIndexIn judges it. A
// state directory that cannot be resolved passes too.
func refuseUnusableState() error {
	dir, err := state.Dir()
	if err != nil {
		return nil
	}
	return refuseUnusableIndexIn(dir)
}

// refuseUnusableIndexIn refuses the index in the state directory dir when this
// version must not touch it: a version 1 sessions.json, or one that is corrupt
// or from a newer version. A missing one passes. So does one that cannot be
// read, which shows nothing to protect; a command that needs it fails on its
// own.
func refuseUnusableIndexIn(dir string) error {
	_, err := readSessionIndexIn(dir)
	if _, unreadable := errors.AsType[*fs.PathError](err); unreadable {
		return nil
	}
	return err
}
