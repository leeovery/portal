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
// its own ceiling and past the create's (refuseUnsupported), so a judgement
// late in the create's ceiling is never cut short. env reaches the session
// through zmx's environment, which its daemon keeps and its program inherits.
func (b *Backend) Create(ctx context.Context, name, dir string, command []string, env ...string) (backend.Session, error) {
	if err := b.ValidateName(name); err != nil {
		return backend.Session{}, err
	}
	run, cancel := b.bound(ctx, b.limits.create, "")
	defer cancel()
	if err := b.refuseHeld(run, name); err != nil {
		return backend.Session{}, err
	}
	stdout, runErr := b.attach(run, name, dir, command, env)
	if strings.HasPrefix(stdout, created(name)) {
		s, other := b.made(run, name)
		if s.Answered {
			return s, nil
		}
		if err := b.refuseUnsupported(ctx, name, other); err != nil {
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

// refuseUnsupported refuses the session name, just made, when its daemon
// writes a frame or an Info record that names another protocol version,
// which a daemon of ProtocolVersion never writes, its output broadcast before
// the answer included: the zmx that made it speaks a protocol Portal
// doesn't, so every session it made would be one Portal can't talk to. other
// is the read-back's error for what it read of another version; with none,
// the session is asked once more. A session that answers in Portal's
// version, answers nothing, or has already ended, is made all the same.
//
// The refused session is ended by that zmx's own kill, which speaks its
// daemon's protocol, and nothing else: a signal's proof of which daemon this
// Create made can be fooled into reaching a session Portal didn't make, so
// one the kill can't end is left, and the refusal says so. Each step runs
// past a cancelled ctx, within its own ceiling: a caller giving up between
// the created line and the judgement would otherwise keep a session Portal
// can't talk to as made.
func (b *Backend) refuseUnsupported(ctx context.Context, name string, other error) error {
	ctx = context.WithoutCancel(ctx)
	if other == nil {
		other = b.otherProtocol(ctx, name)
	}
	if other == nil {
		return nil
	}
	refused := fmt.Errorf("create session %q: its daemon doesn't speak zmx protocol %d: %w: %w", name, ProtocolVersion, other, backend.ErrUnsupportedProtocol)
	if err := b.KillByCLI(ctx, name); err != nil {
		return fmt.Errorf("%w; the session is left running: %w", refused, err)
	}
	return fmt.Errorf("%w; zmx kill ended the session", refused)
}

// otherProtocol asks the session name for its Info once, within a probe
// ceiling: the read's error when a frame or the record it read names another
// protocol version, nil for any other read.
func (b *Backend) otherProtocol(ctx context.Context, name string) error {
	ctx, cancel := b.bound(ctx, b.limits.probe, "")
	defer cancel()
	_, err := b.readInfo(ctx, name)
	if errors.Is(err, errProtocolVersion) {
		return err
	}
	return nil
}

// made reads back the session zmx has just made, asking again while its
// fresh daemon comes up, for at most a probe ceiling within the create one.
// A session that has already ended, or does not answer by then, is made all
// the same, and is returned with only its name. A read of a frame or a record
// that names another protocol version ends the asking, since a daemon writes
// in one protocol for good, and its error is returned with the name.
func (b *Backend) made(ctx context.Context, name string) (backend.Session, error) {
	ctx, cancel := b.bound(ctx, b.limits.probe, "")
	defer cancel()
	hostname, _ := b.hostname()
	for {
		i, err := b.readInfo(ctx, name)
		switch {
		case err == nil:
			return sessionOf(name, i, hostname), nil
		case errors.Is(err, errProtocolVersion):
			return backend.Session{Name: name}, err
		case gone(err):
			return backend.Session{Name: name}, nil
		}
		select {
		case <-ctx.Done():
			return backend.Session{Name: name}, nil
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
// and tmux's variables (see env). extra's entries are set over the rest: exec
// keeps the last of a repeated variable.
func (b *Backend) attach(ctx context.Context, name, dir string, command, extra []string) (string, error) {
	cmd := exec.CommandContext(ctx, b.binary, append([]string{"attach", name}, command...)...)
	cmd.Env = append(b.env(envSession, envPrefix), extra...)
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
