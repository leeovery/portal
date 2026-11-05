package state

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"syscall"
)

// MigrationCutShort reports whether portal migrate was cut short beside the
// state directory dir between its two renames, or while it moved its staging
// dir's entries in: the tmux version's state is in TmuxStateDir, and
// MigrationStagingDir still holds what the migration built. Until a re-run
// finishes it, dir holds none of the migrated sessions, so nothing may write
// dir or judge hooks by it: a hook sweep would reap every migrated key. An
// empty staging dir is a move cut short as it removed the dir itself, with
// everything in place. With no state-tmux/ no migration has reached its
// renames, whatever is at the staging path, so that one stat answers every
// command on every install that never migrated. A staging dir it can't look
// in beside state-tmux/ is an error, as is a state-tmux/ it can't look at
// beside a staging dir holding anything.
func MigrationCutShort(dir string) (bool, error) {
	info, statErr := os.Stat(TmuxStateDir(dir))
	switch {
	case errors.Is(statErr, fs.ErrNotExist), statErr == nil && !info.IsDir():
		return false, nil
	}
	staged, err := holdsAnything(MigrationStagingDir(dir))
	switch {
	case err != nil:
		return false, err
	case !staged:
		return false, nil
	case statErr != nil:
		return false, fmt.Errorf("look for the tmux version's migrated state: %w", statErr)
	}
	return true, nil
}

// holdsAnything reports whether dir is there holding an entry. A path
// through a file names nothing, as one through a missing dir doesn't.
func holdsAnything(dir string) (bool, error) {
	f, err := os.Open(dir)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look in a migration's staging dir: %w", err)
	}
	defer func() { _ = f.Close() }()
	switch _, err := f.Readdirnames(1); {
	case errors.Is(err, io.EOF):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("look in a migration's staging dir: %w", err)
	}
	return true, nil
}
