package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/tui"
)

// takeWait bounds the take's wait for zmx to count no other client on the
// session. zmx makes a client the session's leader as it attaches only while
// none leads, and sizes the session and draws its first frame for the leader
// alone, so an attach that began before the others had gone would be drawn
// for another terminal until its first keystroke.
const takeWait = 2 * time.Second

// takePoll spaces the reads of the take's wait.
const takePoll = 10 * time.Millisecond

// take takes name from every other terminal attached to it, before this one
// attaches it, so this one is its only client and leads it from its first
// frame. Every other Portal runner on the session is asked to let it go,
// saying where it went; then, while any client is still attached, a raw zmx
// client or a runner that didn't let go, where it went is recorded for them
// and every client is detached through the backend; then the take waits,
// within c.takeWait, for the session to have no other client. It never kills
// and sends the session's program nothing. Portal's own sessions are never
// taken. A take that fails is logged and the attach goes ahead, drawn for
// another terminal at worst.
func (c attachConnector) take(ctx context.Context, name string) {
	if backend.IsPortalOwned(name) {
		return
	}
	by := tui.WhereFrom(os.LookupEnv)
	// The registry logs each runner's answer; one that didn't let go is left
	// to the backend's detach.
	_, _ = c.runners.Take(ctx, name, by)
	s, err := c.backend.Session(ctx, name)
	if err != nil {
		attachLogger.Warn("take failed", "session", name, "detail", "read", "error", err)
		return
	}
	if !s.Attached() {
		return
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
}

// errNotAlone is the take's wait running out with clients still attached.
var errNotAlone = errors.New("clients still attached")

// awaitAlone waits, within c.takeWait, for the session to have no client.
func (c attachConnector) awaitAlone(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeoutCause(ctx, c.takeWait, fmt.Errorf("%w after %s", errNotAlone, c.takeWait))
	defer cancel()
	for {
		s, err := c.backend.Session(ctx, name)
		switch {
		case err != nil:
			return err
		case !s.Attached():
			return nil
		}
		if err := backend.Pause(ctx, takePoll); err != nil {
			return err
		}
	}
}

// takenAway is out, the end of an attach of name that began at began, as
// taken when zmx dropped the attach's client and the session lives on: a
// detach from elsewhere dropped it, and a take that recorded where the
// session went since the attach began says where. A session gone ended the
// attach itself, as one that can't be read is taken to have, and either
// exits to the shell.
func (c attachConnector) takenAway(name string, out runner.Outcome, began time.Time) runner.Outcome {
	if !out.Dropped {
		return out
	}
	live, err := c.backend.Exists(c.cmd.Context(), name)
	if err != nil {
		attachLogger.Debug("drop unjudged", "session", name, "error", err)
	}
	if err != nil || !live {
		return out
	}
	return runner.Outcome{Taken: true, By: c.runners.TakenBy(name, began)}
}
