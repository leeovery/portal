// Package restore brings back, once a bootstrap, the saved sessions the
// backend no longer holds. Each is made again under its own name, running the
// hydrate helper, which replays the session's saved transcript and hands the
// session to its resume hook, its resume panel or its shell.
package restore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// via is the surface restore's writes to the index record.
const via = "internal"

// Config is what a Restorer works over.
type Config struct {
	// Backend lists the live sessions and makes the restored ones.
	Backend backend.Backend
	// StateDir holds sessions.json, the sessions' flags and their transcripts.
	StateDir string
	// Exe is the portal binary every restored session runs as its hydrate
	// helper, as an absolute path. It has no default: under go test the
	// running binary is the test binary, which a restored session would run as
	// the whole suite.
	Exe string
	// Logger records the run under its caller's component; nil discards.
	Logger *slog.Logger
}

// Result is what a restore did, for its caller to warn of.
type Result struct {
	// IndexErr is why sessions.json couldn't be read: it is unreadable,
	// malformed, a version 1 index or a newer one. Nothing was restored.
	IndexErr error
	// ListErr is why the backend's sessions couldn't be listed. Nothing was
	// restored.
	ListErr error
	// Restored counts the sessions made again. Failed counts those that
	// couldn't be, whose entries stay for the next bootstrap to retry.
	Restored, Failed int
}

// Restorer restores the saved sessions. It is not safe for concurrent use.
type Restorer struct {
	backend  backend.Backend
	dir      string
	exe      string
	logger   *slog.Logger
	progress func(n, m int)
}

// testBinarySuffix ends the name of every binary go test builds.
const testBinarySuffix = ".test"

// New returns a Restorer over cfg. It refuses a Config with no Backend or no
// StateDir, and an Exe that is not an absolute path or is a test binary.
func New(cfg Config) (*Restorer, error) {
	switch {
	case cfg.Backend == nil:
		return nil, errors.New("restore: no backend to restore sessions in")
	case cfg.StateDir == "":
		return nil, errors.New("restore: no state directory to restore sessions from")
	case !filepath.IsAbs(cfg.Exe):
		return nil, fmt.Errorf("restore: the hydrate helper's executable must be an absolute path, not %q", cfg.Exe)
	case strings.HasSuffix(cfg.Exe, testBinarySuffix):
		return nil, fmt.Errorf("restore: the hydrate helper's executable %q is a test binary, which a restored session would run as the whole suite", cfg.Exe)
	}
	return &Restorer{backend: cfg.Backend, dir: cfg.StateDir, exe: cfg.Exe, logger: log.OrDiscard(cfg.Logger)}, nil
}

// SetProgress has Restore report each session it takes up: the nth of the m
// it restores.
func (r *Restorer) SetProgress(fn func(n, m int)) { r.progress = fn }

// Restore makes again, in index order, the session of every saved entry that
// may predate this boot and that the backend doesn't list, Portal's own aside.
// An entry from this boot whose session is gone ended this boot, and is the
// saver's to drop; with no boot time to judge by, every entry may predate it.
// A session it can't make keeps its entry for the next bootstrap. It asks the
// backend nothing when sessions.json can't be read, makes nothing when the
// backend can't be listed, and makes no further session once ctx ends.
func (r *Restorer) Restore(ctx context.Context) Result {
	start := time.Now()
	pass := &run{Restorer: r}
	pass.restore(ctx)
	r.logger.Info("restore complete", "sessions", pass.res.Restored, "anomalous", pass.anomalous, log.Took(start))
	return pass.res
}

// run is one Restore: what it did, and how many warnings its sessions drew.
type run struct {
	*Restorer
	res       Result
	anomalous int
}

func (r *run) restore(ctx context.Context) {
	idx, err := sessionindex.Read(r.dir)
	if err != nil {
		r.logger.Warn("sessions.json unreadable", "error", err)
		r.res.IndexErr = err
		return
	}
	if idx.Len() == 0 {
		return
	}
	live, err := r.backend.List(ctx)
	if err != nil {
		r.logger.Warn("list sessions failed", "error", err)
		r.res.ListErr = err
		return
	}
	due := r.due(idx.Entries(), live)
	for i, e := range due {
		if ctx.Err() != nil {
			r.logger.Warn("restore cancelled", "detail", fmt.Sprintf("%d of %d sessions left for the next bootstrap", len(due)-i, len(due)), "error", context.Cause(ctx))
			return
		}
		if r.progress != nil {
			r.progress(i+1, len(due))
		}
		r.restoreOne(ctx, e)
	}
}

// due is the entries to restore.
func (r *run) due(entries []sessionindex.Entry, live []backend.Session) []sessionindex.Entry {
	listed := make(map[string]bool, len(live))
	for _, s := range live {
		listed[s.Name] = true
	}
	rule := state.ClaimRule(r.dir)
	var due []sessionindex.Entry
	for _, e := range entries {
		if why := skipped(e, listed, rule); why != "" {
			r.logger.Debug("session skipped", "session", e.Name, "reason", why)
			continue
		}
		due = append(due, e)
	}
	return due
}

// skipped says why the entry's session is not restored, and is empty when it
// is: its session is listed, answering or not, it is one of Portal's own, or
// it can't predate this boot.
func skipped(e sessionindex.Entry, listed map[string]bool, rule sessionindex.Rule) string {
	switch {
	case listed[e.Name]:
		return "it is live"
	case backend.IsPortalOwned(e.Name):
		return "it is one of Portal's own"
	case !rule.MayPredateBoot(e):
		return "it ended this boot"
	}
	return ""
}

// restoreOne makes the entry's session again. A name Create refuses is never
// tried. The session's hydrating flag goes up first, keeping the saver off it
// until the helper has replayed its transcript, and a session whose flag can't
// be looked for or raised is not made.
func (r *run) restoreOne(ctx context.Context, e sessionindex.Entry) {
	if err := r.backend.ValidateName(e.Name); err != nil {
		r.failed(e.Name, fmt.Errorf("its name is one the backend can never create: %w", err))
		return
	}
	raisedBefore, err := state.Hydrating.Exists(r.dir, e.Name)
	if err == nil {
		err = state.Hydrating.Set(r.dir, e.Name)
	}
	if err != nil {
		r.failed(e.Name, fmt.Errorf("raise its hydrating flag: %w", err))
		return
	}
	dir, argv := startDir(e), r.command(e)
	s, err := r.backend.Create(ctx, e.Name, dir, argv)
	if err != nil {
		if s, err = r.madeAnyway(ctx, e.Name, argv, raisedBefore, err); err != nil {
			r.failed(e.Name, err)
			return
		}
	}
	r.restored(e, s, dir)
}

// madeAnyway looks up the session a Create that failed may have made all the
// same, as one that timed out waiting for it to answer has. A session running
// restore's helper is restored, its flag the helper's to lower. Any other
// session holding the name is not restore's, so its flag comes down whoever
// raised it: left up, it would keep the saver off that session for good. With
// no session under the name, only a flag this run raised comes down, as one
// raised before is another restore's. A session that can't be looked up may be
// the helper's, so its flag stays.
func (r *run) madeAnyway(ctx context.Context, name string, argv []string, raisedBefore bool, createErr error) (backend.Session, error) {
	live, err := r.backend.Session(context.WithoutCancel(ctx), name)
	_, heldAs := errors.AsType[*backend.HeldAsError](err)
	switch {
	case err == nil && live.Command == r.backend.DisplayCommand(argv):
		return live, nil
	case err == nil:
		r.lowerHydrating(name)
		return backend.Session{}, fmt.Errorf("%w; the session holding its name runs %q, not restore's helper", createErr, live.Command)
	case heldAs:
		r.lowerHydrating(name)
		return backend.Session{}, fmt.Errorf("%w; %w", createErr, err)
	case errors.Is(err, backend.ErrNoSuchSession):
		if !raisedBefore {
			r.lowerHydrating(name)
		}
		return backend.Session{}, createErr
	}
	return backend.Session{}, fmt.Errorf("%w; can't tell whether it was made: %w", createErr, err)
}

// restored records the session's creation as the entry's own, under the
// commit lock every writer of the index holds. A session reporting no
// creation records none: the hydrate helper's claim records it.
func (r *run) restored(e sessionindex.Entry, s backend.Session, dir string) {
	r.res.Restored++
	if _, err := sessionindex.Update(r.dir, via, func(idx *sessionindex.Index) error {
		idx.Restored(e, s.Created, s.PID)
		return nil
	}); err != nil {
		r.warn("record session creation failed", e.Name, "error", err)
	}
	r.logger.Debug("session restored", "session", e.Name, "path", dir)
}

func (r *run) failed(name string, err error) {
	r.res.Failed++
	r.warn("restore session failed", name, "error", err)
}

func (r *run) lowerHydrating(name string) {
	if _, err := state.Hydrating.Clear(r.dir, name); err != nil {
		r.warn("clear hydrating flag failed", name, "error", err)
	}
}

// warn logs a warning about the named session, which the run's summary
// counts as anomalous.
func (r *run) warn(msg, name string, attrs ...any) {
	r.anomalous++
	r.logger.Warn(msg, append([]any{"session", name}, attrs...)...)
}

// command is the hydrate helper's argv for the entry's session, naming its
// transcript only when it has one to replay.
func (r *run) command(e sessionindex.Entry) []string {
	argv := []string{r.exe, "state", "hydrate", "--session", e.Name}
	if file, ok := r.transcript(e); ok {
		argv = append(argv, "--file", file)
	}
	return argv
}

// transcript is the file holding the entry's saved transcript, and whether
// the helper should replay it: it exists, or can't be looked for, a failure
// the helper records when it opens it. A file the entry names that is no
// transcript of its session is never read.
func (r *run) transcript(e sessionindex.Entry) (string, bool) {
	if e.ScrollbackFile == "" {
		return "", false
	}
	path, err := state.ScrollbackPath(r.dir, e)
	if err != nil {
		r.warn("scrollback file refused", e.Name, "path", e.ScrollbackFile, "error", err)
		return "", false
	}
	_, err = os.Stat(path)
	return path, !errors.Is(err, fs.ErrNotExist)
}

// startDir is where the entry's session starts: its last working directory,
// else the one Portal made it in, else HOME, each only while it is a directory
// the session can start in, else the root.
func startDir(e sessionindex.Entry) string {
	home, _ := os.UserHomeDir()
	for _, dir := range []string{e.Cwd, e.Dir, home} {
		if enterable(dir) {
			return dir
		}
	}
	return "/"
}

// enterable reports whether dir is an absolute path to a directory this
// process may search.
func enterable(dir string) bool {
	if !filepath.IsAbs(dir) {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir() && unix.Access(dir, unix.X_OK) == nil
}
