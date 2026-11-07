package cmd

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/leeovery/portal/internal/state"
	"golang.org/x/sys/unix"
)

// The waiting session's resume-pending flag, in the state directory as each
// process of the chain resolves it. Raised, it keeps the saver from capturing
// the panel over the session's transcript.

// raiseResumePending raises session's flag through set and returns its path,
// which the parked chain's shell tests and removes once no Portal binary is
// left to. A set that failed may still have made the file, which would keep
// the saver off a session that never waits, so the flag is lowered again,
// best effort: lowerErr is that lowering's failure, nil when it landed or
// was never owed.
func raiseResumePending(session string, set func(dir, name string) error) (path string, lowerErr, err error) {
	dir, err := state.Dir()
	if err != nil {
		return "", nil, err
	}
	path, err = state.ResumePending.Path(dir, session)
	if err != nil {
		return "", nil, err
	}
	if err := set(dir, session); err != nil {
		if !provedAbsent(path) {
			_, lowerErr = state.ResumePending.Clear(dir, session)
		}
		return "", lowerErr, err
	}
	return path, nil, nil
}

// provedAbsent reports whether nothing can stand at path: it, or a directory
// it would be in, is missing. A failed set that made nothing owes no lowering.
func provedAbsent(path string) bool {
	_, err := os.Lstat(path)
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// clearResumePending lowers session's flag. Lowering a flag already lowered
// succeeds: the session is as the answer wants it.
func clearResumePending(session string) error {
	dir, err := state.Dir()
	if err != nil {
		return err
	}
	_, err = state.ResumePending.Clear(dir, session)
	return err
}

// resumePendingRaised reports whether session's flag is raised, by the rule
// the parked chain's shell gates follow (flagProvedLowered), so the chain's
// Go and shell steps never disagree. A state directory that can't be
// resolved is an error, which the chain counts as raised too.
//
// state.SessionFlag's Exists reads a flag whose state directory is gone as
// lowered, which the saver wants; a waiting session must not close on it.
func resumePendingRaised(session string) (bool, error) {
	dir, err := state.Dir()
	if err != nil {
		return false, err
	}
	path, err := state.ResumePending.Path(dir, session)
	if err != nil {
		return false, err
	}
	return !flagLowered(path), nil
}

// flagLowered is flagProvedLowered's test in Go: nothing stands at path, and
// its directory can be searched for it, or, that directory gone too, the
// state directory above it can. Anything it can't see through leaves the flag
// raised.
func flagLowered(path string) bool {
	dir := filepath.Dir(path)
	return nothingAt(path) && (searchableDir(dir) || nothingAt(dir) && searchableDir(filepath.Dir(dir)))
}

// nothingAt is the shell's [ ! -e path ]: no file can be stat'd there.
func nothingAt(path string) bool {
	_, err := os.Stat(path)
	return err != nil
}

// searchableDir is the shell's [ -d dir ] && [ -x dir ].
func searchableDir(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir() && unix.Access(dir, unix.X_OK) == nil
}
