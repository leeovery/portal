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

// created is what zmx attach prints first on stdout once it has made a
// session, with the name raw between the quotes. It prints no such line when
// it joins a live session.
func created(name string) string { return `session "` + name + `" created` + "\n" }

// Create runs zmx attach, which joins a live session of the name, ignoring
// the command, and replaces a socket that refuses. So a name held under any
// spelling is refused first, and only zmx's created line counts as success,
// whatever its exit code: when the command exits before the client connects,
// zmx reports the session made and then fails.
func (b *Backend) Create(ctx context.Context, name, dir string, command []string) error {
	if err := b.ValidateName(name); err != nil {
		return err
	}
	ctx, cancel := b.bound(ctx, b.limits.create, "")
	defer cancel()
	if err := b.refuseHeld(ctx, name); err != nil {
		return err
	}
	stdout, runErr := b.attach(ctx, name, dir, command)
	if strings.HasPrefix(stdout, created(name)) {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("create session %q: %w", name, context.Cause(ctx))
	}
	holder, err := b.holder(ctx, name)
	if err != nil {
		return fmt.Errorf("create session %q: %w", name, errors.Join(runErr, err))
	}
	return notCreated(name, runErr, holder)
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
// name while its socket answers, or refuses since boot: its daemon may be live
// with its listen backlog full.
func (b *Backend) holder(ctx context.Context, name string) (string, error) {
	s, ok, err := b.held(name)
	if err != nil || !ok {
		return "", err
	}
	c, err := b.connect(ctx, s.name)
	switch {
	case gone(err):
		return "", nil
	case errors.Is(err, errRefused):
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
