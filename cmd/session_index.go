package cmd

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// legacyIndexError is a version 1 sessions.json, which belongs to the tmux
// version of Portal until portal migrate moves it aside.
type legacyIndexError struct {
	path string
}

func (e legacyIndexError) Error() string {
	return e.path + " is from the tmux version of Portal; run portal migrate to move it aside"
}

func (legacyIndexError) Unwrap() error { return sessionindex.ErrLegacyIndex }

// readSessionIndex reads sessions.json from the state directory; a missing
// file is an empty index.
func readSessionIndex() (sessionindex.Index, error) {
	dir, err := state.Dir()
	if err != nil {
		return sessionindex.Index{}, err
	}
	return readSessionIndexIn(dir)
}

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
// lock, writing it back when fn changed something. via is cli, tui or
// internal.
func updateSessionIndex(via string, fn func(*sessionindex.Index) error) error {
	dir, err := state.Dir()
	if err != nil {
		return err
	}
	err = sessionindex.Update(dir, via, fn)
	if errors.Is(err, sessionindex.ErrLegacyIndex) {
		return legacyIndexError{path: sessionindex.Path(dir)}
	}
	return err
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
