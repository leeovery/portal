package migrate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
	"golang.org/x/sys/unix"
)

// What a run refuses, each worded for the user by its copy.
var (
	// ErrLocked is another run under way.
	ErrLocked = errors.New("another portal migrate is running")
	// ErrBothStates is the tmux version's state beside an earlier
	// migration's state-tmux/: migrate never overwrites state-tmux/.
	ErrBothStates = errors.New("the tmux version's state is beside an earlier migration's")
	// ErrStagingBesideState is a staging dir beside state-tmux/ and a version
	// 2 state dir: something wrote the state dir between a run's renames.
	ErrStagingBesideState = errors.New("a migration's staging dir is beside a state dir written since")
	// ErrInTheWay is a state dir holding what no run put there, where a run
	// finishing its renames would put the staging dir.
	ErrInTheWay = errors.New("the state dir holds what no migration put there")
	// ErrMigratedStateGone is state-tmux/ beside a state dir holding no
	// index, with no staging dir: no run leaves that, since every build
	// writes an index, so migrate can't tell what became of the state it
	// migrated.
	ErrMigratedStateGone = errors.New("an earlier migration's state is gone from the state dir")
	// ErrDaemonRunning is the tmux version's daemon still running at the
	// bound: it would write the tmux version's state over migrate's.
	ErrDaemonRunning = errors.New("the tmux version's daemon is still running")
)

// refusal is an error worded for the user, matching the errors it wraps.
type refusal struct {
	msg   string
	wraps []error
}

func (r refusal) Error() string   { return r.msg }
func (r refusal) Unwrap() []error { return r.wraps }

// refused is an error worded by format and args, matching kind, when there
// is one, and every error among args.
func refused(kind error, format string, args ...any) error {
	var wraps []error
	if kind != nil {
		wraps = append(wraps, kind)
	}
	for _, arg := range args {
		if err, ok := arg.(error); ok {
			wraps = append(wraps, err)
		}
	}
	return refusal{msg: fmt.Sprintf(format, args...), wraps: wraps}
}

// worded is err as the user reads it: a refusal as it is, and any other
// failure as a run stopped by it.
func worded(err error) error {
	if _, ok := errors.AsType[refusal](err); ok {
		return err
	}
	return refused(nil, stoppedCopy, err)
}

// indexKind is what the state dir's sessions.json is.
type indexKind int

const (
	noIndex indexKind = iota
	tmuxIndex
	zmxIndex
)

// found is what a run finds before it changes anything: the state dir's
// index, whether state-tmux/ is beside it, and whether a run was cut short
// between its renames (state.MigrationCutShort).
type found struct {
	dir, tmuxDir, staging string
	index                 indexKind
	hasTmuxDir, cutShort  bool
}

// inspect reads what is in and beside the state dir dir, removing first an
// empty staging dir. A sessions.json there that can't be read, or is of
// another version than 1 or 2, is an error, as is anything it can't look
// for.
func inspect(dir string) (found, error) {
	f := found{dir: dir, tmuxDir: state.TmuxStateDir(dir), staging: state.MigrationStagingDir(dir)}
	var err error
	if f.index, err = readIndexKind(dir); err != nil {
		return f, err
	}
	if f.hasTmuxDir, err = exists(f.tmuxDir); err != nil {
		return f, err
	}
	if err := removeEmptyStaging(f.staging); err != nil {
		return f, err
	}
	f.cutShort, err = state.MigrationCutShort(dir)
	return f, err
}

// removeEmptyStaging removes the staging dir when it is there holding
// nothing: what a move into the state dir cut short as it ended leaves,
// everything moved but the dir itself.
func removeEmptyStaging(staging string) error {
	names, err := entryNames(staging)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("look for %s: %w", staging, err)
	case len(names) > 0:
		return nil
	}
	return os.Remove(staging)
}

func readIndexKind(dir string) (indexKind, error) {
	path := sessionindex.Path(dir)
	if present, err := exists(path); err != nil || !present {
		return noIndex, err
	}
	_, err := sessionindex.Read(dir)
	switch {
	case err == nil:
		return zmxIndex, nil
	case errors.Is(err, sessionindex.ErrLegacyIndex):
		return tmuxIndex, nil
	}
	return noIndex, refused(nil, unusableIndexCopy, path, err)
}

// course is the course a run takes on what it found, or why it takes none.
func (f found) course() (Course, error) {
	switch {
	case f.index == tmuxIndex && f.hasTmuxDir:
		return 0, refused(ErrBothStates, bothStatesCopy, f.dir, f.tmuxDir)
	case f.index == tmuxIndex:
		return Migrated, nil
	case f.cutShort && f.index == zmxIndex:
		return 0, refused(ErrStagingBesideState, stagingBesideStateCopy, f.staging, f.dir)
	case f.cutShort:
		return Finished, nil
	case f.hasTmuxDir && f.index == zmxIndex:
		return AlreadyMigrated, nil
	case f.hasTmuxDir:
		return 0, refused(ErrMigratedStateGone, migratedStateGoneCopy, f.dir, f.tmuxDir)
	case f.index == zmxIndex:
		return NotTmux, nil
	}
	return NoState, nil
}

// exists reports whether something is at path, an error when it can't
// tell.
func exists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look for %s: %w", path, err)
	}
	return true, nil
}

// lockParent takes an exclusive lock on the state dir dir's parent, where
// every rename of a run lands, and returns its release: ErrLocked while
// another run holds it. Locking the directory itself leaves no file behind.
// With no parent there is nothing to migrate, and nothing to lock.
func lockParent(dir string) (func(), error) {
	parent := filepath.Dir(filepath.Clean(dir))
	f, err := os.Open(parent)
	if errors.Is(err, fs.ErrNotExist) {
		return func() {}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %s to lock it: %w", parent, err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, refused(ErrLocked, lockedCopy)
		}
		return nil, fmt.Errorf("lock %s: %w", parent, err)
	}
	return func() { _ = f.Close() }, nil
}
