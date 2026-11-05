package hooksweep

import (
	"context"
	"errors"
	"regexp"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// The whole cycle rides the component of the subsystem it sweeps, whichever
// caller drove it, so one grep reconstructs the whole of it.
var logger = log.For("hooks")

// The messages the cycle reports itself under. countsMsg and removedMsg are its
// per-cycle detail; sweepFailedMsg reports a cycle that neither ran to
// completion nor stood down for a reason it could name, so no caller has to
// word the sweep's internals in a vocabulary of its own.
const (
	countsMsg      = "stale-hook cleanup counts"
	removedMsg     = "stale-hook cleanup removed"
	sweepFailedMsg = "stale-hook cleanup failed"
)

// Sources are what protect a key from the sweep: every user session List
// names, answering or not, and every entry sessions.json in StateDir holds. StateDir
// also holds the restoring flag that stands the judgement down.
type Sources struct {
	List     func(context.Context) ([]backend.Session, error)
	StateDir string
}

// Outcome is everything one cycle has to say: the keys it removed, or the
// reason it declined to remove any. A caller renders both halves from it, so a
// cycle that declined can never look like one that found nothing.
type Outcome struct {
	Removed       []string
	DeclineReason Reason
}

// errNoStateDir declines a judgement handed no state directory, whose flag
// and index reads would otherwise resolve against the working directory.
var errNoStateDir = errors.New("no state directory")

// restoringStandDown reports whether hook-staleness work may run at all. Both
// the sweep and the diagnosis take it before they judge a single key, so
// neither judges an entry the other protects. A bootstrap's restore is
// creating sessions and recording them as it runs, and a deferred prune costs
// nothing. A flag that can't be read folds into the same stand-down under its
// own reason: it establishes nothing about the flag, so the reason it is
// reported under must not claim a restore the user is not running.
func restoringStandDown(dir string) StandDown {
	if dir == "" {
		return decline(ReasonStateDirUnresolved, "error", errNoStateDir)
	}
	switch raised, err := state.Restoring.Exists(dir); {
	case err != nil:
		return decline(ReasonRestoringReadFailed, "error", err)
	case raised:
		return decline(ReasonRestoring)
	}
	return StandDown{}
}

// listedNames is every user session name the backend lists, answering or not:
// a session that doesn't answer is still a session, and its hook is still its.
// Portal's own sessions are left out here, for the sweep and the diagnosis
// alike, since the driver lists them: counted, the daemon's own _portal-saver
// would keep nothingProtects from ever standing down in the one process that
// sweeps unattended.
func listedNames(ctx context.Context, list func(context.Context) ([]backend.Session, error)) ([]string, StandDown) {
	sessions, err := list(ctx)
	if err != nil {
		return nil, decline(ReasonListFailed, "error", err)
	}
	names := make([]string, 0, len(sessions))
	for _, s := range sessions {
		if !s.PortalOwned() {
			names = append(names, s.Name)
		}
	}
	return names, StandDown{}
}

// indexedNames is every name sessions.json holds an entry for: a saved
// session awaiting its restore keeps its hook. An index this version must not
// read — version 1, corrupt or newer — or can't read at all protects nothing
// it can name, so the judgement stands down on it.
func indexedNames(dir string) ([]string, StandDown) {
	idx, err := sessionindex.Read(dir)
	if err != nil {
		return nil, decline(ReasonIndexUnusable, "error", err)
	}
	entries := idx.Entries()
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
	}
	return names, StandDown{}
}

// nothingProtects stands the judgement down when no session is listed and
// none saved, so no key could be protected: the shape of a state dir and a
// zmx dir pointed somewhere fresh beside a hooks.json that isn't, which would
// otherwise lose every hook in it. An install with no sessions at all loses
// nothing by the wait.
func nothingProtects(listed, indexed []string) StandDown {
	if len(listed) == 0 && len(indexed) == 0 {
		return decline(ReasonNoSessions)
	}
	return StandDown{}
}

// paneCoordinates is the shape of the hook keys the tmux version wrote before
// its pane tokens: <session>:<window>.<pane>.
var paneCoordinates = regexp.MustCompile(`^[^:]+:[0-9]+\.[0-9]+$`)

// tmuxShaped reports whether key has the shape of a key the tmux version
// wrote: a pane token, or a pane's coordinates.
func tmuxShaped(key string) bool {
	return nanoid.IsTokenShaped(key) || paneCoordinates.MatchString(key)
}

// tmuxKeysStandDown stands the judgement down rather than remove any of the
// stale keys in the shape of the tmux version's. hooks.json is shared with
// that version, whose keys name no zmx session: beside a state dir and a zmx
// dir pointed somewhere fresh, as a dev build's are, each would be stale, and
// its hook lost. Only the tmux version's hooks being migrated clears it. A
// key in that shape that a session or an entry names is a zmx session's, and
// never stale.
func tmuxKeysStandDown(stale []string) StandDown {
	n := 0
	for _, key := range stale {
		if tmuxShaped(key) {
			n++
		}
	}
	if n > 0 {
		return decline(ReasonTmuxKeys, "entries", n)
	}
	return StandDown{}
}

// Stale reports the keys of persisted the sweep would remove at this moment,
// or the stand-down that forbids judging them. It takes the sweep's reads in
// the sweep's order and removes nothing, so a diagnosis answers the reaper's
// question with the reaper's own answer.
func Stale(ctx context.Context, src Sources, persisted hooks.Snapshot) ([]string, StandDown) {
	if d := restoringStandDown(src.StateDir); d.Declined() {
		return nil, d
	}
	if len(persisted) == 0 {
		return nil, StandDown{}
	}
	listed, d := listedNames(ctx, src.List)
	if d.Declined() {
		return nil, d
	}
	indexed, d := indexedNames(src.StateDir)
	if d.Declined() {
		return nil, d
	}
	if d := nothingProtects(listed, indexed); d.Declined() {
		return nil, d
	}
	stale := hooks.StaleKeys(persisted, listed, indexed)
	if d := tmuxKeysStandDown(stale); d.Declined() {
		return nil, d
	}
	return stale, StandDown{}
}

// A store holding nothing to sweep is an answer, not a failure: it aborts the
// clean with the file untouched, which is the point of returning it as an error.
var errNothingPersisted = errors.New("no hook entries to sweep")

// clean answers the store's clean with the names that protect a key, or with
// the stand-down that forbids judging any key at all. It carries the snapshot
// and the listing to the index read, which the clean takes later under its
// hold, because only the three together say which keys are stale. The clean
// removes none the snapshot lacks, so judging the snapshot's keys is judging
// every key it may remove.
type clean struct {
	ctx      context.Context
	src      Sources
	snapshot hooks.Snapshot
	listed   []string
}

// listing answers with the names the backend lists.
func (c *clean) listing(snapshot hooks.Snapshot) ([]string, error) {
	// Nothing persisted is nothing to sweep, and everything past this point
	// costs: a listing that asks every session's daemon, and a deletion that
	// creates the config directory, creates the sidecar and takes an exclusive
	// hold. An install that has never registered a hook would otherwise pay
	// them on every cycle, in every `hook set`'s way. The counts line carries
	// no session figure, because none was taken.
	if len(snapshot) == 0 {
		logger.Debug(countsMsg, "entries", 0)
		return nil, errNothingPersisted
	}
	listed, d := listedNames(c.ctx, c.src.List)
	if d.Declined() {
		return nil, declinedError{d}
	}
	logger.Debug(countsMsg, "sessions", len(listed), "entries", len(snapshot))
	c.snapshot, c.listed = snapshot, listed
	return listed, nil
}

// indexing answers with the names sessions.json holds.
func (c *clean) indexing() ([]string, error) {
	indexed, d := indexedNames(c.src.StateDir)
	if !d.Declined() {
		d = nothingProtects(c.listed, indexed)
	}
	if !d.Declined() {
		d = tmuxKeysStandDown(hooks.StaleKeys(c.snapshot, c.listed, indexed))
	}
	if d.Declined() {
		return nil, declinedError{d}
	}
	return indexed, nil
}

// Run runs one hook-staleness cycle, reporting what it removed or the reason it
// declined to remove anything.
func Run(ctx context.Context, src Sources, store *hooks.Store) (Outcome, error) {
	// Taken before the store is read at all: a restore is no time to wait on
	// this file's lock for an answer that cannot be acted on.
	if d := restoringStandDown(src.StateDir); d.Declined() {
		return standDownOutcome(d)
	}

	c := &clean{ctx: ctx, src: src}
	removed, err := store.CleanStale(c.listing, c.indexing)
	if err != nil {
		return declinedSweep(err)
	}

	logger.Debug(removedMsg, "reaped", len(removed))

	return Outcome{Removed: removed}, nil
}

// declinedSweep renders the outcome of a clean that wrote nothing. A guard's
// own stand-down carries the reason it decided on, and a held lock is named
// here because only this cycle knows it costs nothing. Any other failure,
// an unreadable or corrupt hooks.json among them, is a defect: it is reported
// here and still returned, the error driving a caller's own rendered output
// rather than a second log line.
func declinedSweep(err error) (Outcome, error) {
	var declined declinedError
	switch {
	case errors.Is(err, errNothingPersisted):
		return Outcome{}, nil
	case errors.As(err, &declined):
		return standDownOutcome(declined.StandDown)
	case errors.Is(err, hooks.ErrLockHeld):
		// Another writer held the sidecar past the bound: nothing was written
		// and nothing is wrong, so the cycle stands down rather than reporting a
		// defect, and the next cadence retries.
		return standDownOutcome(decline(ReasonLockTimeout, "error", err))
	}
	logger.Warn(sweepFailedMsg, "error", err)
	return Outcome{}, err
}

// standDownOutcome emits a decline and renders it as the cycle's outcome. The
// nil error is the point: the emitted line is the whole report, so a caller
// cannot add a second one for the same event.
func standDownOutcome(d StandDown) (Outcome, error) {
	d.emit()
	return Outcome{DeclineReason: d.reason}, nil
}
