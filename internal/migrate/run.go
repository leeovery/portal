package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
	"golang.org/x/sys/unix"
)

var logger = log.For("migrate")

// Course is the course a run took, by what it found.
type Course int

const (
	// NoState is a state dir holding no saved state, and no migration's.
	NoState Course = iota + 1
	// NotTmux is a state dir already the zmx version's that no migration
	// made so.
	NotTmux
	// Migrated is the tmux version's state migrated in full.
	Migrated
	// Finished is a run cut short between its renames finished: the state it
	// built put in place.
	Finished
	// AlreadyMigrated is a migration found landed, with only its bootstrap
	// left to run.
	AlreadyMigrated
)

// Bootstraps reports whether a run of this course leaves the zmx version's
// state in place, for portal migrate's bootstrap to restore.
func (c Course) Bootstraps() bool {
	return c == Migrated || c == Finished || c == AlreadyMigrated
}

// Config is what a run migrates, and how it reaches the tmux version and zmx.
type Config struct {
	// StateDir is the state dir, the tmux version's until a run swaps the zmx
	// version's in, and HooksFile is hooks.json.
	StateDir, HooksFile string
	Tmux                TmuxServer
	Daemon              Daemon
	// Live lists the names of the sessions zmx holds, which the plan's names
	// keep clear of beside Naming's Taken.
	Live   func(ctx context.Context) ([]string, error)
	Naming Naming
	// Waiting hears the tmux version's daemon's pid as a run starts to wait
	// for it to exit.
	Waiting func(pid int)
}

// Outcome is what a run did, for the report.
type Outcome struct {
	Course Course
	// StateDir is the state dir and TmuxDir where the tmux version's state is
	// kept, state-tmux/ beside it.
	StateDir, TmuxDir string
	// Backup is hooks.json's backup, kept in TmuxDir; empty when there is
	// none, as when there was no hooks.json.
	Backup string
	// Plan is what a Migrated run wrote. Nothing keeps a plan, so no other
	// course carries one.
	Plan Plan
}

// Run migrates the tmux version's state in StateDir to the zmx version's,
// one run at a time: another holding the state dir's parent is ErrLocked.
// What it does follows from what it finds there:
//   - A version 1 sessions.json with no state-tmux/ beside it migrates in
//     full: the tmux side first (Portal's tmux hooks unset and the saver
//     killed, when a tmux server answers, then the wait for the tmux
//     version's daemon to exit); hooks.json read and backed up; the plan;
//     the zmx version's state built in a fresh staging dir beside it and
//     hooks.json rewritten from the backup; then the commit point, the state
//     dir renamed to state-tmux/ and the staging dir to the state dir.
//   - state-tmux/ and a staging dir with no sessions.json in the state dir
//     finishes the second rename.
//   - state-tmux/ and no staging dir is a migration landed.
//   - No state-tmux/ and no version 1 sessions.json is nothing to migrate.
//   - A version 1 sessions.json beside state-tmux/, a staging dir beside
//     state-tmux/ and a version 2 sessions.json, and a sessions.json that
//     can't be read are refused, changing nothing.
//
// Until the commit point the state dir is still the tmux version's, so every
// other command refuses it and the zmx saver and hook sweep stand down: a run
// stopped before it is redone whole, with a fresh plan. Every refusal and
// failure is logged at ERROR and returned, worded for the user.
func Run(ctx context.Context, cfg Config) (Outcome, error) {
	start := time.Now()
	o := Outcome{StateDir: cfg.StateDir, TmuxDir: state.TmuxStateDir(cfg.StateDir)}
	release, err := lockParent(cfg.StateDir)
	if err != nil {
		return o, refuse(err)
	}
	defer release()
	found, err := inspect(cfg.StateDir)
	if err == nil {
		o.Course, err = found.course()
	}
	switch {
	case err != nil:
	case o.Course == Migrated:
		o.Plan, err = migrateState(ctx, cfg)
	case o.Course == Finished:
		err = install(found.staging, cfg.StateDir)
	}
	if err != nil {
		return o, refuse(err)
	}
	if o.Course.Bootstraps() {
		o.Backup, err = keptBackup(o.TmuxDir)
		if err != nil {
			return o, refuse(err)
		}
	}
	logOutcome(o, start)
	return o, nil
}

// refuse logs why a run stopped, worded for the user, and returns it.
func refuse(err error) error {
	err = worded(err)
	logger.Error("stopped", "error", err)
	return err
}

// logOutcome records what a run did, once it has done it.
func logOutcome(o Outcome, start time.Time) {
	switch o.Course {
	case NoState, NotTmux:
		logger.Info("nothing to migrate", "path", o.StateDir, log.Took(start))
	case Migrated:
		logger.Info("migrated", "sessions", len(o.Plan.Sessions), "path", o.TmuxDir, log.Took(start))
	case Finished:
		logger.Info("migration finished", "path", o.TmuxDir, log.Took(start))
	case AlreadyMigrated:
		logger.Info("already migrated", "path", o.TmuxDir, log.Took(start))
	}
}

// migrateState migrates the tmux version's state in full, and returns the
// plan it wrote.
func migrateState(ctx context.Context, cfg Config) (Plan, error) {
	if err := stopTmux(ctx, cfg.Tmux); err != nil {
		return Plan{}, err
	}
	if err := awaitDaemon(ctx, cfg); err != nil {
		return Plan{}, err
	}
	backup := HooksBackup(cfg.StateDir)
	if err := backUp(cfg.HooksFile, backup); err != nil {
		return Plan{}, err
	}
	plan, err := planFor(ctx, cfg, backup)
	if err != nil {
		return Plan{}, err
	}
	staging := state.MigrationStagingDir(cfg.StateDir)
	if err := build(plan, staging); err != nil {
		return Plan{}, err
	}
	if err := RewriteHooks(plan, backup, hooks.NewStore(cfg.HooksFile)); err != nil {
		return Plan{}, err
	}
	return plan, swap(cfg.StateDir, staging)
}

// stopTmux unsets Portal's tmux hooks and kills the tmux version's saver.
// With no tmux server answering there are none: they ended with it. Either
// failing stops the run, since a tmux hook left would run the tmux version
// against the state dir migrate rewrites.
func stopTmux(ctx context.Context, tmux TmuxServer) error {
	if !tmux.Answering(ctx) {
		logger.Info("no tmux server", "detail", "Portal's tmux hooks and saver ended with it")
		return nil
	}
	unset, err := tmux.RemoveHooks(ctx)
	if err != nil {
		return refused(nil, tmuxHooksLeftCopy, err)
	}
	logger.Info("tmux hooks removed", "unset", unset)
	killed, err := tmux.KillSaver(ctx)
	if err != nil {
		return refused(nil, tmuxSaverLeftCopy, err)
	}
	if killed {
		logger.Info("tmux saver killed")
	}
	return nil
}

// awaitDaemon waits for the tmux version's daemon to exit.
func awaitDaemon(ctx context.Context, cfg Config) error {
	start := time.Now()
	waiting := cfg.Waiting
	if waiting == nil {
		waiting = func(int) {}
	}
	pid, err := cfg.Daemon.AwaitExit(ctx, cfg.StateDir, waiting)
	if err != nil {
		return err
	}
	if pid != 0 {
		logger.Info("tmux daemon exited", "target_pid", pid, log.Took(start))
	}
	return nil
}

// backUp backs hooks.json up, logging where.
func backUp(live, backup string) error {
	wrote, err := backUpHooks(live, backup)
	if errors.Is(err, hooks.ErrMalformed) {
		return refused(nil, hooksMalformedCopy, live, err)
	}
	if err != nil {
		return err
	}
	if wrote {
		logger.Info("hooks backed up", "path", backup)
	}
	return nil
}

// planFor plans the migration of the tmux version's state, from its version
// 1 index and the keys of hooks.json's backup, its names clear of zmx's
// sessions.
func planFor(ctx context.Context, cfg Config, backup string) (Plan, error) {
	legacy, err := ReadLegacy(cfg.StateDir)
	if err != nil {
		return Plan{}, err
	}
	keys, err := backupKeys(backup)
	if err != nil {
		return Plan{}, err
	}
	live, err := cfg.Live(ctx)
	if err != nil {
		return Plan{}, refused(nil, unlistedCopy, err)
	}
	naming := cfg.Naming
	naming.Taken = slices.Concat(naming.Taken, live)
	return NewPlan(legacy, keys, naming)
}

// build builds the plan into staging, made afresh: a staging dir there is
// one a run cut short left, which only this run would finish.
func build(plan Plan, staging string) error {
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("remove the staging dir a migration cut short left at %s: %w", staging, err)
	}
	if err := Build(plan, staging); err != nil {
		return err
	}
	logger.Info("state built", "path", staging, "sessions", len(plan.Sessions))
	return nil
}

// swap is the commit point: the tmux version's state dir renamed to
// state-tmux/, then staging put in its place. Nothing logs between the two:
// a log line makes the state dir again, for its own file.
func swap(dir, staging string) error {
	tmuxDir := state.TmuxStateDir(dir)
	if err := os.Rename(dir, tmuxDir); err != nil {
		return fmt.Errorf("move the tmux version's state to %s: %w", tmuxDir, err)
	}
	unsynced := syncParent(dir)
	if err := stopAt(BetweenRenames); err != nil {
		return err
	}
	err := install(staging, dir)
	warnUnsynced(tmuxDir, unsynced)
	return err
}

// install puts staging in place as the state dir dir: by a rename, or, where
// a state dir holding no sessions.json is in the way, by moving staging's
// entries into it, sessions.json last, so dir becomes the zmx version's state
// only once everything it names is there. Every portal process logs into the
// state dir and makes it to do so, so a run cut short between its renames,
// and the re-run that finishes it, find one there holding their logs, and
// what a move cut short brought. Any other entry is in the way, and refused.
func install(staging, dir string) error {
	err := os.Rename(staging, dir)
	if errors.Is(err, unix.ENOTEMPTY) || errors.Is(err, fs.ErrExist) {
		if err := inTheWay(staging, dir); err != nil {
			return err
		}
		err = moveInto(staging, dir)
	}
	if err != nil {
		return fmt.Errorf("put the migrated state in place at %s: %w", dir, err)
	}
	warnUnsynced(dir, syncParent(dir))
	return nil
}

// inTheWay refuses a state dir dir holding anything but Portal's logs and
// what a move into it cut short brought from staging.
func inTheWay(staging, dir string) error {
	left, err := entryNames(staging)
	if err != nil {
		return err
	}
	held, err := entryNames(dir)
	if err != nil {
		return err
	}
	for _, name := range held {
		moved := slices.Contains(builtNames, name) && !slices.Contains(left, name)
		if !isLogFile(name) && !moved {
			return refused(ErrInTheWay, inTheWayCopy, dir, name, staging)
		}
	}
	return nil
}

// builtNames are the entries a build writes beside sessions.json, which a
// move into the state dir moves first: the scrollback dir and commit.lock,
// which sessions.json's write takes.
var builtNames = []string{filepath.Base(state.ScrollbackDir("")), filepath.Base(commitlock.Path(""))}

// moveInto moves each of staging's entries into dir, sessions.json last,
// then removes staging, empty.
func moveInto(staging, dir string) error {
	left, err := entryNames(staging)
	if err != nil {
		return err
	}
	index := filepath.Base(sessionindex.Path(dir))
	ordered := slices.DeleteFunc(slices.Clone(left), func(name string) bool { return name == index })
	if len(ordered) < len(left) {
		ordered = append(ordered, index)
	}
	for _, name := range ordered {
		if err := os.Rename(filepath.Join(staging, name), filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	warnUnsynced(dir, fileutil.SyncDir(dir))
	return os.Remove(staging)
}

// isLogFile reports whether name is one of the files Portal's log keeps in
// the state dir, every one of them named from portal.log.
func isLogFile(name string) bool {
	return strings.HasPrefix(name, logName)
}

var logName = filepath.Base(state.PortalLog(""))

func entryNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}

// syncParent orders the rename just made in dir's parent ahead of what
// follows, returning why it couldn't.
func syncParent(dir string) error { return fileutil.SyncDir(filepath.Dir(filepath.Clean(dir))) }

// warnUnsynced logs a sync that failed after a rename into dir: the rename
// landed all the same.
func warnUnsynced(dir string, err error) {
	if err != nil {
		logger.Warn("unsynced", "path", dir, "error", err, "error_class", "write-failed-fsync")
	}
}

// keptBackup is hooks.json's backup in tmuxDir, empty when there is none.
func keptBackup(tmuxDir string) (string, error) {
	backup := HooksBackup(tmuxDir)
	switch _, err := os.Lstat(backup); {
	case errors.Is(err, fs.ErrNotExist):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("look for hooks.json's backup: %w", err)
	}
	return backup, nil
}
