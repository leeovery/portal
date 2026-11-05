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
//     full: first every check it can make up front (the hooks the plan reads,
//     the version 1 index and zmx's sessions); then the tmux side (Portal's
//     tmux hooks unset and the saver killed, when a tmux server answers,
//     then the wait for the tmux version's daemon to exit); hooks.json
//     backed up; the plan, from the index read again; the zmx version's
//     state built in a fresh staging dir beside it and hooks.json rewritten
//     from the backup; then the commit point, the state dir renamed to
//     state-tmux/ and the staging dir to the state dir.
//   - A migration cut short between its renames (state.MigrationCutShort),
//     with no sessions.json in the state dir, finishes the second rename.
//   - state-tmux/, no staging dir and a version 2 sessions.json is a
//     migration landed.
//   - No state-tmux/ and no version 1 sessions.json is nothing to migrate.
//   - A version 1 sessions.json beside state-tmux/, a migration cut short
//     beside a version 2 sessions.json, state-tmux/ beside a state dir
//     holding no sessions.json and no migration cut short, and a
//     sessions.json that can't be read are refused, changing nothing.
//
// Until the commit point the state dir is still the tmux version's, so every
// other command refuses it and the zmx saver and hook sweep stand down: a run
// stopped before it is redone whole, with a fresh plan. Between the renames
// they refuse and stand down on the migration cut short, until a run
// finishes it. Every refusal and failure is logged at ERROR and returned,
// worded for the user.
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
		err = stage{rerun: finishesCopy}.stopped(install(found.staging, cfg.StateDir))
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

// Stopped is err, a failure that stops portal migrate before a run starts,
// worded and logged as a run's own.
func Stopped(err error) error { return refuse(err) }

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
// plan it wrote. A failure that stops it is worded by how far it got.
func migrateState(ctx context.Context, cfg Config) (Plan, error) {
	s := &stage{rerun: againCopy}
	plan, err := migrateFrom(ctx, cfg, s)
	return plan, s.stopped(err)
}

// migrateFrom is migrateState's work, s noting how far it gets.
func migrateFrom(ctx context.Context, cfg Config, s *stage) (Plan, error) {
	backup := HooksBackup(cfg.StateDir)
	live, err := precheck(ctx, cfg, backup)
	if err != nil {
		return Plan{}, err
	}
	stop, err := stopTmux(ctx, cfg.Tmux)
	if err != nil {
		return Plan{}, err
	}
	s.tmuxStopped, s.rerun = stop.answered, startsOverCopy
	if err := awaitDaemon(ctx, cfg, stop.killed); err != nil {
		return Plan{}, err
	}
	if err := backUp(cfg.HooksFile, backup); err != nil {
		return Plan{}, err
	}
	plan, err := planFor(cfg, backup, live)
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
	return plan, swap(cfg.StateDir, staging, s)
}

// stage is how far a run has got, which words a failure that stops it:
// whether it stopped Portal's tmux side, which then saves nothing, and what
// a re-run does: from the stop on it starts over, and past the commit
// point's first rename it finishes the migration.
type stage struct {
	tmuxStopped bool
	rerun       string
}

// stopped words err, nil or a failure that stops a run at s. A refusal keeps
// its own words, but for a hooks file that doesn't parse, which s words too.
func (s stage) stopped(err error) error {
	if err == nil {
		return nil
	}
	tmux := ""
	if s.tmuxStopped {
		tmux = tmuxOffCopy
	}
	if m, ok := errors.AsType[malformedHooks](err); ok {
		return refused(hooks.ErrMalformed, hooksMalformedCopy, m.path, m.err, tmux, s.rerun)
	}
	if _, ok := errors.AsType[refusal](err); ok {
		return err
	}
	return refused(nil, stoppedCopy, err, tmux, s.rerun)
}

// precheck makes, before the tmux side is stopped, every check that can be
// made up front, so a refusal among them leaves the tmux version saving as
// it did: the hooks the plan reads, as strictly as it reads them; the
// version 1 index, as the plan reads it; and zmx's sessions, which it
// returns for the plan's names. Neither file can pass now and fail later:
// the tmux version's writers refuse a hooks.json that doesn't parse, and its
// daemon's flush writes the index it reads itself, which the plan reads
// afresh.
func precheck(ctx context.Context, cfg Config, backup string) ([]string, error) {
	source, err := hooksSource(cfg.HooksFile, backup)
	if err != nil {
		return nil, err
	}
	if _, err := readHooks(source); err != nil {
		return nil, err
	}
	if _, err := ReadLegacy(cfg.StateDir); err != nil {
		return nil, err
	}
	live, err := cfg.Live(ctx)
	if err != nil {
		return nil, refused(nil, unlistedCopy, err)
	}
	return live, nil
}

// tmuxStop is what stopping the tmux side did: whether a tmux server
// answered, so Portal's hooks on it were unset, and whether a saver was
// killed.
type tmuxStop struct {
	answered, killed bool
}

// stopTmux unsets Portal's tmux hooks and kills the tmux version's saver.
// With no tmux server answering there are none: they ended with it. Either
// failing stops the run, since a tmux hook left would run the tmux version
// against the state dir migrate rewrites.
func stopTmux(ctx context.Context, tmux TmuxServer) (tmuxStop, error) {
	if !tmux.Answering(ctx) {
		logger.Info("no tmux server", "detail", "Portal's tmux hooks and saver ended with it")
		return tmuxStop{}, nil
	}
	unset, err := tmux.RemoveHooks(ctx)
	if err != nil {
		return tmuxStop{}, refused(nil, tmuxHooksLeftCopy, err)
	}
	logger.Info("tmux hooks removed", "unset", unset)
	killed, err := tmux.KillSaver(ctx)
	if err != nil {
		return tmuxStop{}, refused(nil, tmuxSaverLeftCopy, err)
	}
	if killed {
		logger.Info("tmux saver killed")
	}
	return tmuxStop{answered: true, killed: killed}, nil
}

// awaitDaemon waits for the tmux version's daemon to exit, its saver killed
// when stopped says so.
func awaitDaemon(ctx context.Context, cfg Config, stopped bool) error {
	start := time.Now()
	waiting := cfg.Waiting
	if waiting == nil {
		waiting = func(int) {}
	}
	pid, err := cfg.Daemon.AwaitExit(ctx, cfg.StateDir, stopped, waiting)
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
	if err != nil {
		return err
	}
	if wrote {
		logger.Info("hooks backed up", "path", backup)
	}
	return nil
}

// planFor plans the migration of the tmux version's state, from its version
// 1 index, read again now its daemon has flushed, and the keys of
// hooks.json's backup, its names clear of live, the sessions zmx holds.
func planFor(cfg Config, backup string, live []string) (Plan, error) {
	legacy, err := ReadLegacy(cfg.StateDir)
	if err != nil {
		return Plan{}, err
	}
	keys, err := backupKeys(backup)
	if err != nil {
		return Plan{}, err
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

// renameDir makes the commit point's two renames: os.Rename, but in tests,
// which watch each one land, or fail it.
var renameDir = os.Rename

// swap is the commit point: the tmux version's state dir renamed to
// state-tmux/, then staging put in its place, past which a re-run finishes
// the migration rather than starting over, as s comes to say. Nothing logs
// between the two: a log line makes the state dir again, for its own file.
func swap(dir, staging string, s *stage) error {
	tmuxDir := state.TmuxStateDir(dir)
	if err := renameDir(dir, tmuxDir); err != nil {
		return fmt.Errorf("move the tmux version's state to %s: %w", tmuxDir, err)
	}
	s.rerun = finishesCopy
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
	err := renameDir(staging, dir)
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
