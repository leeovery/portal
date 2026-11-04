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

// updateSessionIndex runs fn on the state directory's index under the commit
// lock, writing it back when fn changed something, and makes the directory
// first: the lock lives in it. Once the write has landed, and before the lock
// is released, it forgets each entry the write removed or replaced. via is
// cli, tui or internal.
func updateSessionIndex(via string, fn func(*sessionindex.Index) error) error {
	dir, err := state.EnsureDir()
	if err != nil {
		return err
	}
	return updateSessionIndexIn(dir, via, fn)
}

func updateSessionIndexIn(dir, via string, fn func(*sessionindex.Index) error) error {
	return updateSessionIndexForgetting(dir, via, fn, sessionindex.Outcome.Dropped)
}

// updateSessionIndexForgetting is updateSessionIndexIn forgetting, under the
// same lock, the entries forgotten picks from what the update did, which is
// empty when it changed nothing. Nothing is forgotten for an update that
// failed.
func updateSessionIndexForgetting(dir, via string, fn func(*sessionindex.Index) error, forgotten func(sessionindex.Outcome) []sessionindex.Entry) error {
	_, err := sessionindex.UpdateThen(dir, via, fn, func(outcome sessionindex.Outcome) {
		forgetEntries(dir, forgotten(outcome), via)
	})
	if errors.Is(err, sessionindex.ErrLegacyIndex) {
		return legacyIndexError{path: sessionindex.Path(dir)}
	}
	return err
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
// what then changes is this creation's.
func claimSession(via string, live claimed, then func(*sessionindex.Index) error) error {
	dir, err := state.EnsureDir()
	if err != nil {
		return err
	}
	rule := claimRule(dir)
	return updateSessionIndexIn(dir, via, func(idx *sessionindex.Index) error {
		idx.Claim(live.Name, live.Dir, live.Created, live.PID, rule)
		if then == nil {
			return nil
		}
		return then(idx)
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
// index this version must not touch: a version 1 sessions.json, or one that is
// corrupt or from a newer version. A missing one passes. So does one that
// cannot be resolved or read, which shows nothing to protect; a command that
// needs it fails on its own.
func refuseUnusableState() error {
	dir, err := state.Dir()
	if err != nil {
		return nil
	}
	_, err = readSessionIndexIn(dir)
	if _, unreadable := errors.AsType[*fs.PathError](err); unreadable {
		return nil
	}
	return err
}
