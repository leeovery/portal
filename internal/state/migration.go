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
// everything in place. Anything it can't look at is an error.
func MigrationCutShort(dir string) (bool, error) {
	if staged, err := holdsAnything(MigrationStagingDir(dir)); err != nil || !staged {
		return false, err
	}
	info, err := os.Stat(TmuxStateDir(dir))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("look for the tmux version's migrated state: %w", err)
	}
	return info.IsDir(), nil
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
