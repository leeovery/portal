package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/runnerreg"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/tui"
)

// takeWait bounds the take's wait for no terminal to hold the session, and
// then the wait for the attach it took for to lead it. zmx makes a client
// the session's leader as it attaches only while none leads, and sizes the
// session and draws its first frame for the leader alone, so an attach that
// began beside another terminal would be drawn for that terminal until its
// first keystroke.
const takeWait = 2 * time.Second

// takeLockWait bounds the wait for another take's hold on its session's
// lock: that take's longest hold, each wait at its bound and each backend
// call at its ceiling, zmx's pinned within saverhost's. Its asks of the
// session's runners, all at once; its first read of what holds the session,
// the leader and then its Info; its detach; its wait for the session to be
// alone, whose reads and detaches that wait's bound cuts short; and its
// attach's wait to lead, whose reads it cuts short too; with takeLockSlack
// over. The bound is each waiter's own, from its own start, and flock serves
// waiters in no order, so of three takes of a session at once the third may
// wait out two holds and go on without the lock beside the second, which is
// not stuck. A take that goes on without it is logged, and its attach is at
// worst drawn for the other terminal until its first keystroke.
func takeLockWait() time.Duration {
	return runnerreg.AskBound() + 2*saverhost.ReadCeiling + saverhost.DetachCeiling + 2*takeWait + takeLockSlack
}

// takeLockSlack covers what no bound does: the registry's scan, the process
// table's, the record's write and the attach's start.
const takeLockSlack = 5 * time.Second

// takePoll spaces the reads of the take's waits.
const takePoll = 20 * time.Millisecond

var (
	// errNotAlone is the take's wait running out with a terminal still on the
	// session.
	errNotAlone = errors.New("a terminal still holds the session")
	// errNotLeading is the wait for the attach to lead running out.
	errNotLeading = errors.New("the attach does not lead the session")
)

// take takes name from every other terminal holding it, before this one
// attaches it, so this one is its only terminal and leads it from its first
// frame, and returns the release of name's take lock, which the caller hands
// to awaitLead once the attach has begun. Under the lock, every other Portal
// runner on the session is asked to let it go, saying where it went; then,
// while a terminal still holds it (a raw zmx client, a runner that didn't let
// go or whose client lingers, or a client zmx switched onto it from another
// session, which no runner's entry names), where it went is recorded for
// them and every client is detached through the backend, again on each read
// of the wait that finds one still there, until none holds it, within
// takeWait. It never kills and sends the session's program nothing. Portal's
// own sessions are never taken. A take that fails is logged and the attach
// goes ahead, drawn for another terminal at worst.
func (c attachConnector) take(ctx context.Context, name string) (release func()) {
	if backend.IsPortalOwned(name) {
		return func() {}
	}
	release = c.lockTake(ctx, name)
	by := tui.WhereFrom(os.LookupEnv)
	// The registry logs each runner's answer; one that didn't let go is left
	// to the backend's detach.
	_, _ = c.runners.Take(ctx, name, by)
	held, err := c.held(ctx, name)
	if err != nil {
		attachLogger.Warn("take failed", "session", name, "detail", "read", "error", err)
		return release
	}
	if !held {
		return release
	}
	if err := c.runners.RecordTake(name, by); err != nil {
		attachLogger.Warn("take failed", "session", name, "detail", "record", "error", err)
	}
	if err := c.backend.Detach(ctx, name); err != nil {
		attachLogger.Warn("take failed", "session", name, "detail", "detach", "error", err)
	} else {
		attachLogger.Info("clients detached", "session", name)
	}
	if err := c.awaitAlone(ctx, name); err != nil {
		attachLogger.Warn("take failed", "session", name, "detail", "wait", "error", err)
	}
	return release
}

// lockTake takes name's take lock, within takeLockWait. A lock that can't be
// had is logged, and the take goes ahead without it.
func (c attachConnector) lockTake(ctx context.Context, name string) (release func()) {
	release, err := c.runners.LockTake(ctx, name, takeLockWait())
	if err != nil {
		attachLogger.Warn("take failed", "session", name, "detail", "lock", "error", err)
		return func() {}
	}
	return release
}

// held reports whether a terminal holds name: zmx answers the environment of
// a client leading it, or the process table shows a zmx attach on a terminal
// on it while the session counts a client. Only a zmx attach sends zmx its
// environment: a reader (the saver's capture, a preview's read, a listing's
// probe, a zmx run) sends none, so even one zmx makes the leader, as a zmx
// run's resize can, answers none, and no reader shows in the table, so none
// holds a session. A Portal runner's client always tracks runnertoken.Var
// and a raw zmx attach tracks zmx's default list, so each answers one; a raw
// one started with no variables to track answers none, and holds the
// session only as the table shows it. The leader counts a client zmx
// switched onto name from another session, whose process still names the
// one it left; and that process holds the session it left only while
// something else is connected there. Either way of holding it is enough;
// what neither read can tell is an error, so nothing is detached on it.
func (c attachConnector) held(ctx context.Context, name string) (bool, error) {
	leader, leaderErr := c.backend.LeaderEnv(ctx, name)
	if len(leader) > 0 {
		return true, nil
	}
	attached, err := c.attachedTerminal(ctx, name)
	if attached {
		return true, nil
	}
	return false, errors.Join(leaderErr, err)
}

// attachedTerminal reports whether the process table shows a zmx attach on
// a terminal on name while the session counts a client.
func (c attachConnector) attachedTerminal(ctx context.Context, name string) (bool, error) {
	terminals, err := c.backend.AttachClients(name)
	if err != nil || len(terminals) == 0 {
		return false, err
	}
	s, err := c.backend.Session(ctx, name)
	if err != nil {
		return false, err
	}
	return s.Attached(), nil
}

// awaitAlone waits, within c.takeWait, for no terminal to hold the session,
// detaching every client again whenever a read finds one still holding it:
// one that connected since the last detach, or one still going.
func (c attachConnector) awaitAlone(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeoutCause(ctx, c.takeWait, fmt.Errorf("%w after %s", errNotAlone, c.takeWait))
	defer cancel()
	for {
		held, err := c.held(ctx, name)
		switch {
		case err != nil:
			return err
		case !held:
			return nil
		}
		if err := c.backend.Detach(ctx, name); err != nil {
			return err
		}
		if err := backend.Pause(ctx, takePoll); err != nil {
			return err
		}
	}
}

// awaitLead releases the take lock once leads, the attach's Confirm, finds
// its client leading name, the attach ending first, or c.takeWait passing,
// which is logged. With nothing to confirm by it releases it at once.
func (c attachConnector) awaitLead(ctx context.Context, name string, leads func(context.Context) error, release func()) {
	defer release()
	if leads == nil {
		return
	}
	ctx, cancel := context.WithTimeoutCause(ctx, c.takeWait, fmt.Errorf("%w after %s", errNotLeading, c.takeWait))
	defer cancel()
	for leads(ctx) != nil {
		if err := backend.Pause(ctx, takePoll); err != nil {
			if errors.Is(err, errNotLeading) {
				attachLogger.Warn("take failed", "session", name, "detail", "lead", "error", err)
			}
			return
		}
	}
}

// takenAway is out, the end of an attach of name that began at began, as
// taken when zmx dropped the attach's client while the session lives on and
// another terminal has it: where it went when a take recorded since the
// attach began says so, and otherwise only while another terminal holds the
// session. Any other drop exits to the shell, as one with the session gone,
// which ended the attach itself, does: a detach typed in the session, or
// one the resume panel made, leaves no terminal on it. Each drop's judgement
// is logged, so a landing that surprises can be told.
func (c attachConnector) takenAway(name string, out runner.Outcome, began time.Time) runner.Outcome {
	if !out.Dropped {
		return out
	}
	by, why, err := c.judgeDrop(c.cmd.Context(), name, began)
	result := "shell"
	if why.taken() {
		result = "taken"
		out = runner.Outcome{Taken: true, By: by}
	}
	attrs := []any{"session", name, "result", result, "detail", string(why)}
	if err != nil {
		attrs = append(attrs, "error", err)
	}
	attachLogger.Info("drop judged", attrs...)
	return out
}

// dropWhy is what a drop's judgement found.
type dropWhy string

const (
	dropRecorded dropWhy = "a take since the attach began recorded where the session went"
	dropHeld     dropWhy = "another terminal holds the session"
	dropEnded    dropWhy = "the session ended"
	dropUnheld   dropWhy = "no terminal holds the session"
	dropUnjudged dropWhy = "the session could not be judged"
)

// taken reports whether the drop lands on the taken screen.
func (w dropWhy) taken() bool { return w == dropRecorded || w == dropHeld }

// judgeDrop judges the drop of name's attach that began at began: taken,
// with where it went when the take's record says, or why not.
func (c attachConnector) judgeDrop(ctx context.Context, name string, began time.Time) (by string, why dropWhy, err error) {
	live, err := c.backend.Exists(ctx, name)
	switch {
	case err != nil:
		return "", dropUnjudged, err
	case !live:
		return "", dropEnded, nil
	}
	if by = c.runners.TakenBy(name, began); by != "" {
		return by, dropRecorded, nil
	}
	held, err := c.held(ctx, name)
	switch {
	case err != nil:
		return "", dropUnjudged, err
	case !held:
		return "", dropUnheld, nil
	}
	return "", dropHeld, nil
}
