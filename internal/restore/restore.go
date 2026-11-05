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
	// couldn't be, retries and all, whose entries stay for the next bootstrap
	// to retry.
	Restored, Failed int
	// Unsupported is why restore stopped short: a session it made speaks a
	// protocol Portal doesn't support, as every session it made next would.
	// The backend has tried to end that session, the error saying whether it
	// went. The sessions it was to try again count as failed, and they and
	// the entries it didn't reach stay for the next bootstrap.
	Unsupported error
}

// Restorer restores the saved sessions. It is not safe for concurrent use.
type Restorer struct {
	backend  backend.Backend
	dir      string
	exe      string
	logger   *slog.Logger
	progress func(n, m int)
	// claimRule, now and wait are state.ClaimRule over the backend's names,
	// time.Now and a wait on a timer but in tests.
	claimRule func(dir string) sessionindex.Rule
	now       func() time.Time
	wait      func(ctx context.Context, d time.Duration) error
}

// exeName is the base name of every portal binary, as the daemon's identity
// requires of one. A binary named otherwise is none a restored session should
// run: a test binary runs the whole suite, as a debugger's build or a renamed
// go test -c build may.
const exeName = "portal"

// RetryBudget bounds, in all, a restore's retries of the sessions its first
// pass couldn't make for a reason that may pass, as a pty the burst of
// creates couldn't have: the waits between them, and every retry started
// within it. A retry started inside it runs to its end.
const RetryBudget = 15 * time.Second

// retryBackoff is the wait before each round of retries, the last repeating.
var retryBackoff = []time.Duration{time.Second, 3 * time.Second}

// New returns a Restorer over cfg. It refuses a Config with no Backend or no
// StateDir, and an Exe that is not an absolute path to a binary named portal.
func New(cfg Config) (*Restorer, error) {
	switch {
	case cfg.Backend == nil:
		return nil, errors.New("restore: no backend to restore sessions in")
	case cfg.StateDir == "":
		return nil, errors.New("restore: no state directory to restore sessions from")
	case !filepath.IsAbs(cfg.Exe):
		return nil, fmt.Errorf("restore: the hydrate helper's executable must be an absolute path, not %q", cfg.Exe)
	case filepath.Base(cfg.Exe) != exeName:
		return nil, fmt.Errorf("restore: the hydrate helper's executable %q is not named %s", cfg.Exe, exeName)
	}
	return &Restorer{
		backend: cfg.Backend, dir: cfg.StateDir, exe: cfg.Exe, logger: log.OrDiscard(cfg.Logger),
		claimRule: func(dir string) sessionindex.Rule { return state.ClaimRule(dir, cfg.Backend.ValidateName) },
		now:       time.Now, wait: backend.Pause,
	}, nil
}

// SetProgress has Restore report each session it takes up: the nth of the m
// it restores.
func (r *Restorer) SetProgress(fn func(n, m int)) { r.progress = fn }

// MarkOwed marks owed, under the commit lock, every saved entry whose session
// the backend doesn't list, Portal's own aside. A bootstrap that finds no
// saver running calls it before any saver can commit: the saver's world ended
// with those sessions, by a logout, every zmx daemon dying or a crash, so no
// saver saw them end, and each restore brings them back, this boot's
// included, until the helper of one restore makes finishes, or a claim finds
// one live with no restore of it unfinished. It fails, marking nothing,
// when the backend can't be listed or the marks can't be written. An index it
// can't read holds nothing a saver could drop, so it marks nothing there and
// leaves the failure for Restore to report.
func (r *Restorer) MarkOwed(ctx context.Context) error {
	if _, err := sessionindex.Read(r.dir); err != nil {
		return nil
	}
	live, err := r.backend.List(ctx)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	listed := names(live)
	_, err = sessionindex.Update(r.dir, via, func(idx *sessionindex.Index) error {
		for _, e := range idx.Entries() {
			if !listed[e.Name] && !backend.IsPortalOwned(e.Name) {
				idx.Owe(e.Name)
			}
		}
		return nil
	})
	return err
}

// Restore makes again, in index order, the session of every saved entry the
// backend doesn't list, Portal's own aside, that is owed a restore or may
// predate this boot. It skips any other entry from this boot, whose session
// ended while a saver watched and is the saver's to drop; with no boot time
// to judge by, every entry may predate the boot. A session it couldn't make
// for a reason that may pass it tries again, within RetryBudget, once every
// other has had its first try. A session it can't make keeps its entry for
// the next bootstrap. It asks the backend nothing when sessions.json can't be
// read, makes nothing when the backend can't be listed, and makes no further
// session once ctx ends or the backend refuses one it made as speaking a
// protocol Portal doesn't support.
func (r *Restorer) Restore(ctx context.Context) Result {
	start := time.Now()
	pass := &run{Restorer: r, raised: map[string]bool{}}
	pass.restore(ctx)
	r.logger.Info("restore complete", "restored", pass.res.Restored, "failed", pass.res.Failed, "anomalous", pass.anomalous, log.Took(start))
	return pass.res
}

// run is one Restore: what it did, how many warnings its sessions drew, and
// the sessions whose hydrating flag it raised.
type run struct {
	*Restorer
	res       Result
	anomalous int
	raised    map[string]bool
}

// retried is the detail of a failure restore tries again within the
// bootstrap.
const retried = "tried again within this bootstrap"

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
	var again []sessionindex.Entry
	for i, e := range due {
		if ctx.Err() != nil {
			r.cancelled(ctx, fmt.Sprintf("%d of %d sessions left for the next bootstrap", len(due)-i, len(due)))
			r.res.Failed += len(again)
			return
		}
		if r.progress != nil {
			r.progress(i+1, len(due))
		}
		if r.firstTry(ctx, e) {
			again = append(again, e)
		}
		if r.res.Unsupported != nil {
			break
		}
	}
	r.res.Failed += len(r.retry(ctx, again))
}

// firstTry restores e, logging a failure, and reports whether it failed for a
// reason a retry may clear.
func (r *run) firstTry(ctx context.Context, e sessionindex.Entry) bool {
	err := r.restoreOne(ctx, e)
	switch {
	case err == nil:
		return false
	case isForGood(err):
		r.res.Failed++
		r.warn("restore session failed", e.Name, "error", err)
		return false
	}
	r.warn("restore session failed", e.Name, "error", err, "detail", retried)
	return true
}

// retry tries the sessions left again, in rounds retryBackoff apart, starting
// a round only while RetryBudget, counted from the first pass's end, has room
// for its wait, and a session's try only while it lasts. It returns those
// still unmade but for good, which retryRound counts. It stops once ctx ends,
// saying so when it leaves sessions unrestored, and tries nothing once the
// backend has refused a session as unsupported, the first pass's or its own.
func (r *run) retry(ctx context.Context, left []sessionindex.Entry) []sessionindex.Entry {
	deadline := r.now().Add(RetryBudget)
	for round := 0; len(left) > 0 && r.res.Unsupported == nil; round++ {
		backoff := retryBackoff[min(round, len(retryBackoff)-1)]
		if !r.now().Add(backoff).Before(deadline) {
			break
		}
		if r.wait(ctx, backoff) == nil {
			left = r.retryRound(ctx, left, deadline)
		}
		if ctx.Err() != nil && len(left) > 0 {
			r.cancelled(ctx, fmt.Sprintf("%d sessions it was trying again left for the next bootstrap", len(left)))
			break
		}
	}
	return left
}

// retryRound tries each session left once more, and returns those that failed
// again for a reason a retry may clear, with those it had no time or context
// left to try, or that a refusal as unsupported stopped it reaching. One
// failing for good is counted failed here.
func (r *run) retryRound(ctx context.Context, left []sessionindex.Entry, deadline time.Time) []sessionindex.Entry {
	var still []sessionindex.Entry
	for i, e := range left {
		if ctx.Err() != nil || !r.now().Before(deadline) || r.res.Unsupported != nil {
			return append(still, left[i:]...)
		}
		err := r.tryAgain(ctx, e)
		switch {
		case err == nil:
			continue
		case isForGood(err):
			r.res.Failed++
		default:
			still = append(still, e)
		}
		r.logger.Debug("restore retry failed", "session", e.Name, "error", err)
	}
	return still
}

// errUnsettled is why a retry's lookup couldn't settle a session an earlier
// try may have made.
var errUnsettled = errors.New("an earlier try left it unsettled")

// tryAgain tries e once more, and returns nil once e needs no more tries. It
// raises e's flag only once its lookup has found the name free: an earlier
// try may have made the session after all, whose helper may have lowered the
// flag already, and raised again it would keep the saver off that session for
// good. A name found free is made again only while its entry is still due and
// records the creation restore read: one a helper has claimed since, or the
// user's end dropped, is restore's no more. A session running restore's
// helper is restored as it stands, and another holding the name fails for
// good. One that can't be looked up is left for the next round, nothing
// raised and nothing made.
func (r *run) tryAgain(ctx context.Context, e sessionindex.Entry) error {
	live, err := r.backend.Session(ctx, e.Name)
	if !errors.Is(err, backend.ErrNoSuchSession) || heldAs(err) {
		if live, err = r.judge(e.Name, live, err, errUnsettled); err == nil {
			r.restored(e, live, startDir(e))
		}
		return err
	}
	why, err := r.noLongerDue(e)
	switch {
	case err != nil:
		return err
	case why != "":
		r.logger.Debug("session skipped", "session", e.Name, "reason", why)
		return nil
	}
	return r.restoreOne(ctx, e)
}

// noLongerDue says why e, read before restore's first pass, is restore's no
// more as sessions.json holds it now, and is empty while it is still due and
// records the creation restore read.
func (r *run) noLongerDue(e sessionindex.Entry) (string, error) {
	idx, err := sessionindex.Read(r.dir)
	if err != nil {
		return "", fmt.Errorf("read sessions.json again: %w", err)
	}
	latest, ok := idx.Lookup(e.Name)
	switch {
	case !ok:
		return "its entry is gone", nil
	case !latest.RecordsCreationOf(e):
		return "its entry records another creation", nil
	}
	return skipped(latest, nil, r.claimRule(r.dir)), nil
}

func (r *run) cancelled(ctx context.Context, detail string) {
	r.logger.Warn("restore cancelled", "detail", detail, "error", context.Cause(ctx))
}

// due is the entries to restore.
func (r *run) due(entries []sessionindex.Entry, live []backend.Session) []sessionindex.Entry {
	listed := names(live)
	rule := r.claimRule(r.dir)
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

// names is the set of the sessions' names.
func names(sessions []backend.Session) map[string]bool {
	set := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		set[s.Name] = true
	}
	return set
}

// skipped says why the entry's session is not restored, and is empty when it
// is: its session is listed, answering or not, it is one of Portal's own, or
// it is owed no restore and can't predate this boot, as rule judges it.
func skipped(e sessionindex.Entry, listed map[string]bool, rule sessionindex.Rule) string {
	switch {
	case listed[e.Name]:
		return "it is live"
	case backend.IsPortalOwned(e.Name):
		return "it is one of Portal's own"
	case !e.Owed && !rule.MayPredateBoot(e):
		return "it ended this boot"
	}
	return ""
}

// restoreOne makes the entry's session again, and says why it couldn't,
// marked forGood when no retry can help. A name Create refuses is never
// tried. The session's hydrating flag goes up first, keeping the saver off it
// until the helper has replayed its transcript, and a session whose flag can't
// be looked for or raised is not made.
func (r *run) restoreOne(ctx context.Context, e sessionindex.Entry) error {
	if err := r.backend.ValidateName(e.Name); err != nil {
		return forGood(fmt.Errorf("its name is one the backend can never create: %w", err))
	}
	if err := r.raise(e.Name); err != nil {
		return fmt.Errorf("raise its hydrating flag: %w", err)
	}
	dir, argv := startDir(e), r.command(e)
	s, err := r.backend.Create(ctx, e.Name, dir, argv)
	switch {
	case errors.Is(err, backend.ErrUnsupportedProtocol):
		return r.unsupported(e.Name, err)
	case err != nil:
		s, err = r.madeAnyway(ctx, e.Name, err)
	case s.Created.IsZero():
		s, err = r.unconfirmed(ctx, e.Name)
	}
	if err != nil {
		return err
	}
	r.restored(e, s, dir)
	return nil
}

// raise raises the session's hydrating flag, remembered as this run's unless
// it was up already: one raised before the run is another restore's.
func (r *run) raise(name string) error {
	up, err := state.Hydrating.Exists(r.dir, name)
	if err != nil || up {
		return err
	}
	if err := state.Hydrating.Set(r.dir, name); err != nil {
		return err
	}
	r.raised[name] = true
	return nil
}

// madeAnyway looks up the session a Create that failed may have made all the
// same, as one that timed out waiting for it to answer has, and judges it.
func (r *run) madeAnyway(ctx context.Context, name string, createErr error) (backend.Session, error) {
	live, err := r.holder(context.WithoutCancel(ctx), name)
	return r.judge(name, live, err, createErr)
}

// errUnconfirmed is a Create that said it made its session but read no
// creation of it.
var errUnconfirmed = errors.New("zmx reported the session made, but it never answered")

// unconfirmed looks up again the session a Create reported made but read no
// creation of, and judges it. zmx reports a session made before its daemon
// has the session's pty, so one that couldn't have it was never made.
func (r *run) unconfirmed(ctx context.Context, name string) (backend.Session, error) {
	live, err := backend.AskAgain(ctx, r.backend, name, r.wait)
	return r.judge(name, live, err, errUnconfirmed)
}

// judge settles a session a Create may have made, by the session the lookup
// found holding its name, live, or the error it gave, cause saying why the
// Create alone couldn't. A session running restore's helper for the name is
// restored, its flag the helper's to lower. Any other session holding the name
// is not restore's, so that is a failure for good, and its flag comes down
// whoever raised it: left up, it would keep the saver off that session for
// good. With no session under the name the failure may pass, and only a flag
// this run raised comes down. A session that can't be looked up may be the
// helper's, so its flag stays.
func (r *run) judge(name string, live backend.Session, err, cause error) (backend.Session, error) {
	switch {
	case err == nil && r.RunsHelper(live):
		return live, nil
	case err == nil:
		r.lowerHydrating(name)
		return backend.Session{}, forGood(fmt.Errorf("%w; the session holding its name runs %q, not restore's helper", cause, live.Command))
	case heldAs(err):
		r.lowerHydrating(name)
		return backend.Session{}, forGood(fmt.Errorf("%w; %w", cause, err))
	case errors.Is(err, backend.ErrNoSuchSession):
		if r.raised[name] {
			r.lowerHydrating(name)
		}
		return backend.Session{}, cause
	}
	return backend.Session{}, fmt.Errorf("%w; can't tell whether it was made: %w", cause, err)
}

// unsupported stops the run on a Create the backend refused because the
// session it made speaks a protocol Portal doesn't support: every Create
// after it would make another. It is a failure for good, never looked up
// again: a retry would remake the name, or restore a session the backend
// couldn't end. Only a flag this run raised comes down, as for a session that
// isn't there.
func (r *run) unsupported(name string, err error) error {
	if r.raised[name] {
		r.lowerHydrating(name)
	}
	r.res.Unsupported = err
	return forGood(err)
}

// holder is the session holding the name, asked for a second time a probe's
// ceiling after a first answer of none: a Create cut short once zmx has forked
// the session's daemon, before its socket listens, leaves a session that
// answers to no one yet.
func (r *run) holder(ctx context.Context, name string) (backend.Session, error) {
	live, err := r.backend.Session(ctx, name)
	if errors.Is(err, backend.ErrNoSuchSession) && !heldAs(err) {
		_ = r.wait(ctx, backend.RecheckAfter)
		live, err = r.backend.Session(ctx, name)
	}
	return live, err
}

// forGoodError marks a failure no retry within the bootstrap can clear: a name
// Create refuses, one another session holds, or a session of a protocol Portal
// doesn't support.
type forGoodError struct{ error }

func (e forGoodError) Unwrap() error { return e.error }

func forGood(err error) error { return forGoodError{err} }

func isForGood(err error) bool {
	_, ok := errors.AsType[forGoodError](err)
	return ok
}

// heldAs reports whether err is the backend's answer that it holds the name
// only under another spelling.
func heldAs(err error) bool {
	_, ok := errors.AsType[*backend.HeldAsError](err)
	return ok
}

// restored records the session's creation as the entry's own, under the
// commit lock every writer of the index holds, still owed a restore until its
// helper finishes. A session reporting no creation records none: the hydrate
// helper's claim records it.
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
	argv := r.helper(e.Name)
	if file, ok := r.transcript(e); ok {
		argv = append(argv, "--file", file)
	}
	return argv
}

// helper is the hydrate helper's argv for the named session, before the
// transcript it may name.
func (r *Restorer) helper(name string) []string {
	return []string{r.exe, "state", "hydrate", "--session", name}
}

// RunsHelper reports whether s was made running the hydrate helper restore
// gives every session it restores, for s's own name, with a transcript or
// without: such a session's hydrating flag is its helper's to lower, never
// restore's or a sweep's. zmx reports a session's command as it was made, so
// a helper that has handed the session to its shell still counts.
func (r *Restorer) RunsHelper(s backend.Session) bool {
	rest, ok := strings.CutPrefix(s.Command, r.backend.DisplayCommand(r.helper(s.Name)))
	return ok && (rest == "" || strings.HasPrefix(rest, " "))
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
