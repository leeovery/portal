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
	"time"

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

// New returns a Restorer over cfg. It refuses an Exe that is not an absolute
// path.
func New(cfg Config) (*Restorer, error) {
	if !filepath.IsAbs(cfg.Exe) {
		return nil, fmt.Errorf("restore: the hydrate helper's executable must be an absolute path, not %q", cfg.Exe)
	}
	return &Restorer{backend: cfg.Backend, dir: cfg.StateDir, exe: cfg.Exe, logger: log.OrDiscard(cfg.Logger)}, nil
}

// SetProgress has Restore report each session it takes up: the nth of the m
// it restores.
func (r *Restorer) SetProgress(fn func(n, m int)) { r.progress = fn }

// Restore makes again, in index order, the session of every saved entry the
// backend doesn't list, Portal's own aside. A session it can't make keeps its
// entry for the next bootstrap. It asks the backend nothing when sessions.json
// can't be read, and makes nothing when the backend can't be listed.
func (r *Restorer) Restore(ctx context.Context) Result {
	start := time.Now()
	res := r.restore(ctx)
	r.logger.Info("restore complete", "sessions", res.Restored, "anomalous", res.Failed, log.Took(start))
	return res
}

func (r *Restorer) restore(ctx context.Context) Result {
	idx, err := sessionindex.Read(r.dir)
	if err != nil {
		r.logger.Warn("sessions.json unreadable", "error", err)
		return Result{IndexErr: err}
	}
	if idx.Len() == 0 {
		return Result{}
	}
	live, err := r.backend.List(ctx)
	if err != nil {
		r.logger.Warn("list sessions failed", "error", err)
		return Result{ListErr: err}
	}
	due := r.due(idx.Entries(), live)
	var res Result
	for i, e := range due {
		if r.progress != nil {
			r.progress(i+1, len(due))
		}
		switch r.restoreOne(ctx, e) {
		case restored:
			res.Restored++
		case failed:
			res.Failed++
		}
	}
	return res
}

// due is the entries to restore: those whose sessions the listing doesn't
// hold, answering or not, and that aren't Portal's own.
func (r *Restorer) due(entries []sessionindex.Entry, live []backend.Session) []sessionindex.Entry {
	listed := make(map[string]bool, len(live))
	for _, s := range live {
		listed[s.Name] = true
	}
	var due []sessionindex.Entry
	for _, e := range entries {
		switch {
		case listed[e.Name]:
			r.logger.Debug("session skipped", "session", e.Name, "detail", "it is live")
		case backend.IsPortalOwned(e.Name):
			r.logger.Debug("session skipped", "session", e.Name, "detail", "it is one of Portal's own")
		default:
			due = append(due, e)
		}
	}
	return due
}

type outcome int

const (
	restored outcome = iota
	// held is a session the backend holds after all, under another spelling
	// or made since the listing: live, so not restored, and not failed.
	held
	failed
)

// restoreOne makes the entry's session again. Its hydrating flag goes up
// first, keeping the saver off the session until the helper has replayed its
// transcript, and a session whose flag can't be looked for or raised is not
// made. A refused Create lowers only a flag this run raised: one raised before
// is another restore's.
func (r *Restorer) restoreOne(ctx context.Context, e sessionindex.Entry) outcome {
	raisedBefore, err := state.Hydrating.Exists(r.dir, e.Name)
	if err == nil {
		err = state.Hydrating.Set(r.dir, e.Name)
	}
	if err != nil {
		r.logger.Warn("restore session failed", "session", e.Name, "error", fmt.Errorf("raise its hydrating flag: %w", err))
		return failed
	}
	dir := startDir(e)
	s, err := r.backend.Create(ctx, e.Name, dir, r.command(e))
	if err != nil {
		if !raisedBefore {
			r.lowerHydrating(e.Name)
		}
		return r.refused(e.Name, err)
	}
	r.record(e.Name, s)
	r.logger.Debug("session restored", "session", e.Name, "path", dir)
	return restored
}

func (r *Restorer) lowerHydrating(name string) {
	if _, err := state.Hydrating.Clear(r.dir, name); err != nil {
		r.logger.Warn("clear hydrating flag failed", "session", name, "error", err)
	}
}

func (r *Restorer) refused(name string, err error) outcome {
	if _, ok := errors.AsType[*backend.SessionExistsError](err); ok {
		r.logger.Debug("session skipped", "session", name, "detail", "the backend holds it", "error", err)
		return held
	}
	r.logger.Warn("restore session failed", "session", name, "error", err)
	return failed
}

// record makes the creation Create returned the entry's own, under the commit
// lock every writer of the index holds. A session Create read no creation of
// records none: the hydrate helper's claim records it.
func (r *Restorer) record(name string, s backend.Session) {
	if _, err := sessionindex.Update(r.dir, via, func(idx *sessionindex.Index) error {
		idx.Restored(name, s.Created, s.PID)
		return nil
	}); err != nil {
		r.logger.Warn("record session creation failed", "session", name, "error", err)
	}
}

// command is the hydrate helper's argv for the entry's session, naming its
// transcript only when it has one to replay.
func (r *Restorer) command(e sessionindex.Entry) []string {
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
func (r *Restorer) transcript(e sessionindex.Entry) (string, bool) {
	if e.ScrollbackFile == "" {
		return "", false
	}
	path, err := state.ScrollbackPath(r.dir, e)
	if err != nil {
		r.logger.Warn("scrollback file refused", "session", e.Name, "path", e.ScrollbackFile, "error", err)
		return "", false
	}
	_, err = os.Stat(path)
	return path, !errors.Is(err, fs.ErrNotExist)
}

// startDir is where the entry's session starts: its last working directory,
// else the one Portal made it in, each only while it is a directory, else
// HOME.
func startDir(e sessionindex.Entry) string {
	for _, dir := range []string{e.Cwd, e.Dir} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	home, _ := os.UserHomeDir()
	return home
}
