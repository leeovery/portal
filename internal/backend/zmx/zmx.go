// Package zmx implements backend.Backend by running the zmx binary, targeting
// zmx v0.8.1.
package zmx

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/backend"
)

const binary = "zmx"

const (
	envDir     = "ZMX_DIR"
	envSession = "ZMX_SESSION"
	// envPrefix is prepended by zmx to every session name it is given.
	envPrefix = "ZMX_SESSION_PREFIX"
)

// Bounds on each zmx run. zmx gives a session's probe 1 s and a history read
// 5 s, but kill waits on the daemon with no bound of its own, so a wedged
// daemon would hold the caller for ever.
const (
	listTimeout    = 10 * time.Second
	createTimeout  = 10 * time.Second
	killTimeout    = 5 * time.Second
	detachTimeout  = 5 * time.Second
	historyTimeout = 10 * time.Second
)

// What zmx v0.8.1 prints for a session it does not hold, and on stdout once
// attach has made a session, all with the name raw between the quotes.
func historyMissing(name string) string { return `session "` + name + `" does not exist` }
func killMissing(name string) string    { return "failed to kill session=" + name + ": SessionNotFound" }
func created(name string) string        { return `session "` + name + `" created` + "\n" }

// Backend drives the zmx sessions in one zmx directory.
type Backend struct {
	dir     string
	environ func() []string
	runner  Runner
}

var _ backend.Backend = (*Backend)(nil)

// New binds a Backend to the zmx directory dir, which must be absolute.
// environ supplies the environment every zmx run starts from, and the
// process's own ZMX_SESSION.
func New(dir string, environ func() []string, runner Runner) *Backend {
	return &Backend{dir: filepath.Clean(dir), environ: environ, runner: runner}
}

// Dir is the zmx directory the Backend is bound to.
func (b *Backend) Dir() string { return b.dir }

func (b *Backend) List() ([]backend.Session, error) {
	out, err := b.run(listTimeout, b.command("list"), true)
	if err != nil {
		return nil, fmt.Errorf("list zmx sessions: %w", err)
	}
	return parseList(out)
}

func (b *Backend) Exists(name string) (bool, error) {
	sessions, err := b.List()
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(sessions, func(s backend.Session) bool { return s.Name == name }), nil
}

// Create runs `zmx attach` with stdin at EOF, so its client detaches as soon
// as the daemon is up. zmx attach on a live name would attach to that session
// and ignore the command, so a live name is refused first, compared without
// case: on a case-insensitive filesystem foo's socket answers for FOO. When
// the command exits before the client connects, zmx reports the session made
// and then fails; that is still a made session.
func (b *Backend) Create(name, dir string, command []string) error {
	if err := b.ValidateName(name); err != nil {
		return err
	}
	sessions, err := b.List()
	if err != nil {
		return fmt.Errorf("create session %q: %w", name, err)
	}
	if i := slices.IndexFunc(sessions, func(s backend.Session) bool { return strings.EqualFold(s.Name, name) }); i >= 0 {
		return fmt.Errorf("create session %q: %w as %q", name, backend.ErrSessionExists, sessions[i].Name)
	}
	cmd := b.command(append([]string{"attach", name}, command...)...)
	cmd.Dir = dir
	_, err = b.run(createTimeout, cmd, false)
	if ce, ok := errors.AsType[*CommandError](err); ok && strings.Contains(ce.Stdout, created(name)) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create session %q: %w", name, err)
	}
	return nil
}

// AttachCommand keeps the caller's ZMX_SESSION: inside a session, zmx attach
// then switches that session's clients to name instead of nesting.
func (b *Backend) AttachCommand(name string) (backend.Command, error) {
	if err := b.ValidateName(name); err != nil {
		return backend.Command{}, err
	}
	return backend.Command{
		Argv: []string{binary, "attach", name},
		Env:  b.env(envDir, envPrefix),
	}, nil
}

func (b *Backend) Kill(name string) error {
	if err := b.ValidateName(name); err != nil {
		return err
	}
	if err := b.requireHeld("kill", name); err != nil {
		return err
	}
	_, err := b.run(killTimeout, b.command("kill", name), false)
	return sessionError("kill", name, err, killMissing(name))
}

func (b *Backend) Detach(name string) error {
	if err := b.ValidateName(name); err != nil {
		return err
	}
	if err := b.requireHeld("detach", name); err != nil {
		return err
	}
	// zmx detach takes no name; it detaches the session named by ZMX_SESSION.
	cmd := b.command("detach")
	cmd.Env = append(cmd.Env, envSession+"="+name)
	if _, err := b.run(detachTimeout, cmd, false); err != nil {
		return fmt.Errorf("detach session %q: %w", name, err)
	}
	return nil
}

// History treats empty output as a failed read: zmx exits 0 with nothing when
// its read of the daemon times out or breaks, while a live session's VT
// screen always carries at least its cursor position.
func (b *Backend) History(name string) ([]byte, error) {
	if err := b.ValidateName(name); err != nil {
		return nil, err
	}
	if err := b.requireHeld("read history of", name); err != nil {
		return nil, err
	}
	out, err := b.run(historyTimeout, b.command("history", name, "--vt"), true)
	if err != nil {
		return nil, sessionError("read history of", name, err, historyMissing(name))
	}
	if out == "" {
		return nil, fmt.Errorf("read history of session %q: %w", name, backend.ErrHistoryUnavailable)
	}
	return []byte(out), nil
}

func (b *Backend) Current() string {
	return lookup(b.environ(), envSession)
}

// requireHeld fails with ErrNoSuchSession unless zmx holds exactly name. zmx
// finds a session by statting its socket, so on a case-insensitive filesystem
// it would answer for foo when asked for FOO: kill FOO kills foo. zmx detach
// also answers a missing session with success.
func (b *Backend) requireHeld(op, name string) error {
	exists, err := b.Exists(name)
	if err != nil {
		return fmt.Errorf("%s session %q: %w", op, name, err)
	}
	if !exists {
		return fmt.Errorf("%s session %q: %w", op, name, backend.ErrNoSuchSession)
	}
	return nil
}

// run bounds one zmx run by timeout; a run cut short fails with
// backend.ErrTimeout.
func (b *Backend) run(timeout time.Duration, cmd backend.Command, raw bool) (string, error) {
	cause := fmt.Errorf("%w: zmx %s gave no answer within %s", backend.ErrTimeout, cmd.Argv[1], timeout)
	ctx, cancel := context.WithTimeoutCause(context.Background(), timeout, cause)
	defer cancel()
	if raw {
		return b.runner.RunRaw(ctx, cmd)
	}
	return b.runner.Run(ctx, cmd)
}

// command is a zmx run with ZMX_SESSION removed: inherited, it would turn
// attach into a switch of the caller's session and stand in for a missing
// name elsewhere.
func (b *Backend) command(args ...string) backend.Command {
	return backend.Command{
		Argv: append([]string{binary}, args...),
		Env:  b.env(envDir, envPrefix, envSession),
	}
}

// env is the environment without the drop keys, with ZMX_DIR set to the
// Backend's directory.
func (b *Backend) env(drop ...string) []string {
	env := slices.DeleteFunc(slices.Clone(b.environ()), func(kv string) bool {
		key, _, _ := strings.Cut(kv, "=")
		return slices.Contains(drop, key)
	})
	return append(env, envDir+"="+b.dir)
}

func lookup(env []string, key string) string {
	var value string
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); k == key {
			value = v
		}
	}
	return value
}

// sessionError wraps a failed run, as ErrNoSuchSession too when stderr
// carries missing.
func sessionError(op, name string, err error, missing string) error {
	if err == nil {
		return nil
	}
	if ce, ok := errors.AsType[*CommandError](err); ok && strings.Contains(ce.Stderr, missing) {
		return fmt.Errorf("%s session %q: %w: %w", op, name, backend.ErrNoSuchSession, err)
	}
	return fmt.Errorf("%s session %q: %w", op, name, err)
}
