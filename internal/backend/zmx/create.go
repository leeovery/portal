package zmx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/backend"
)

// pipeGrace bounds the wait for stdout to close after a run is killed, in
// case something it started still holds the pipe.
const pipeGrace = time.Second

// madeRetry spaces the asks of a session zmx has just made.
const madeRetry = 10 * time.Millisecond

// created is what zmx attach prints first on stdout once it has made a
// session, with the name raw between the quotes. It prints no such line when
// it joins a live session.
func created(name string) string { return `session "` + name + `" created` + "\n" }

// Create runs zmx attach, which joins a live session of the name, ignoring
// the command, and replaces a socket that refuses. So a name held under any
// spelling is refused first, and only zmx's created line counts as success,
// whatever its exit code: when the command exits before the client connects,
// zmx reports the session made and then fails. A session made that gives no
// Info within the read-back is judged once that is over, each step within
// its own ceiling and past the create's (refuseOlder), so a judgement late in
// the create's ceiling is never cut short.
func (b *Backend) Create(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
	if err := b.ValidateName(name); err != nil {
		return backend.Session{}, err
	}
	run, cancel := b.bound(ctx, b.limits.create, "")
	defer cancel()
	if err := b.refuseHeld(run, name); err != nil {
		return backend.Session{}, err
	}
	stdout, runErr := b.attach(run, name, dir, command)
	if strings.HasPrefix(stdout, created(name)) {
		s := b.made(run, name)
		if s.Answered {
			return s, nil
		}
		if err := b.refuseOlder(ctx, name); err != nil {
			return backend.Session{}, err
		}
		return s, nil
	}
	if run.Err() != nil {
		return backend.Session{}, fmt.Errorf("create session %q: %w", name, context.Cause(run))
	}
	holder, err := b.holder(run, name)
	if err != nil {
		return backend.Session{}, fmt.Errorf("create session %q: %w", name, errors.Join(runErr, err))
	}
	return backend.Session{}, notCreated(name, runErr, holder)
}

// refuseOlder refuses the session name, just made, when it answers zmx 0.8.1's
// framing: the zmx that made it speaks a protocol Portal can't, so every
// session it made would be one Portal can't talk to. A session that answers
// neither framing, or has already ended, is made all the same.
//
// The refused session is ended first by that zmx's own kill, which speaks its
// daemon's protocol, then, should that fail, by a SIGTERM to its daemon, so it
// never keeps the name for a restore to count held. The refusal says what
// happened. Each step runs past a cancelled ctx, within its own ceiling: a
// caller giving up between the created line and the judgement would
// otherwise keep a session of 0.8.1's as made.
//
// The SIGTERM reaches only the daemon this Create made, pinned before the
// judgement: the one process the table proves a daemon of name, taken while
// the daemon that then answers on the socket was already running, so another
// daemon of the name, one whose socket has gone, makes two and pins none.
// The pinned pid is proved again just before the signal, and a pid gone by
// then, or a name made again since, is sent nothing.
func (b *Backend) refuseOlder(ctx context.Context, name string) error {
	ctx = context.WithoutCancel(ctx)
	pinned, pinErr := b.sessionDaemon(name)
	if older, _ := b.OlderZmx(ctx, name); !older {
		return nil
	}
	refused := fmt.Errorf("create session %q: its daemon speaks zmx 0.8.1's framing: %w", name, backend.ErrUnsupportedProtocol)
	killErr := b.KillByCLI(ctx, name)
	switch {
	case killErr == nil:
		return fmt.Errorf("%w; zmx kill ended the session", refused)
	case pinErr != nil:
		return fmt.Errorf("%w; the session is left running: %w; no SIGTERM sent, its daemon not pinned: %w", refused, killErr, pinErr)
	}
	return b.endPinned(ctx, name, pinned, fmt.Errorf("%w; %w", refused, killErr))
}

// endPinned ends d, the daemon refuseOlder pinned for the session name, whose
// kill failed as refused says, and words what happened.
func (b *Backend) endPinned(ctx context.Context, name string, d daemon, refused error) error {
	gone, err := b.signalProved(d, b.signal)
	switch {
	case err != nil:
		return fmt.Errorf("%w; the session is left running, no SIGTERM sent: %w", refused, err)
	case gone:
		return fmt.Errorf("%w; its daemon, pid %d, has exited since, sent nothing", refused, d.PID)
	}
	if err := b.awaitEnded(ctx, name, d.PID); err != nil {
		return fmt.Errorf("%w; a SIGTERM went to its daemon, pid %d, and the session is left running: %w", refused, d.PID, err)
	}
	return fmt.Errorf("%w; a SIGTERM to its daemon, pid %d, ended the session", refused, d.PID)
}

// made reads back the session zmx has just made, asking again while its
// fresh daemon comes up, for at most a probe ceiling within the create one.
// A session that has already ended, or does not answer by then, is made all
// the same, and is returned with only its name.
func (b *Backend) made(ctx context.Context, name string) backend.Session {
	ctx, cancel := b.bound(ctx, b.limits.probe, "")
	defer cancel()
	hostname, _ := b.hostname()
	for {
		i, err := b.readInfo(ctx, name)
		if err == nil {
			return sessionOf(name, i, hostname)
		}
		if gone(err) {
			return backend.Session{Name: name}
		}
		select {
		case <-ctx.Done():
			return backend.Session{Name: name}
		case <-time.After(madeRetry):
		}
	}
}

// notCreated explains a run that printed no created line. zmx joined the
// session held under holder, or failed; a run that succeeded joined a session
// that has gone since.
func notCreated(name string, runErr error, holder string) error {
	if runErr != nil && holder == "" {
		return fmt.Errorf("create session %q: %w", name, runErr)
	}
	return &backend.SessionExistsError{Name: name, Held: holder}
}

// refuseHeld refuses a name held under any spelling. Only a missing or stale
// socket is left for zmx attach to replace.
func (b *Backend) refuseHeld(ctx context.Context, name string) error {
	holder, err := b.holder(ctx, name)
	if err != nil {
		return fmt.Errorf("create session %q: %w", name, err)
	}
	if holder != "" {
		return &backend.SessionExistsError{Name: name, Held: holder}
	}
	return nil
}

// holder is the entry whose session holds name, or "". A session holds its
// name while its socket answers, or does not accept but may have a daemon
// behind it: one live with its listen backlog full.
func (b *Backend) holder(ctx context.Context, name string) (string, error) {
	s, ok, err := b.held(name)
	if err != nil || !ok {
		return "", err
	}
	c, err := b.connect(ctx, s.name)
	switch {
	case gone(err):
		return "", nil
	case errors.Is(err, errNotAccepting):
		return s.name, nil
	case err != nil:
		return "", failure(ctx, err)
	}
	_ = c.Close()
	return s.name, nil
}

// attach runs zmx attach in dir with stdin at EOF, so its client detaches as
// soon as the daemon is up, and returns what it printed on stdout. The run
// drops ZMX_SESSION, which would turn the attach into a switch of the
// caller's session, ZMX_SESSION_PREFIX, which zmx would prepend to the name,
// and TMUX and TMUX_PANE: the session inherits the run's environment, and
// must not take itself to be inside the caller's tmux.
func (b *Backend) attach(ctx context.Context, name, dir string, command []string) (string, error) {
	cmd := exec.CommandContext(ctx, b.binary, append([]string{"attach", name}, command...)...)
	cmd.Env = b.env(envSession, envPrefix, envTmux, envTmuxPane)
	cmd.Dir = dir
	cmd.WaitDelay = pipeGrace
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), nil
	}
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		return stdout.String(), fmt.Errorf("zmx attach: %s: %w", msg, err)
	}
	return stdout.String(), fmt.Errorf("zmx attach: %w", err)
}
