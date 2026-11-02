// Package zmx implements backend.Backend by running the zmx binary, targeting
// zmx v0.8.1.
package zmx

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/leeovery/portal/internal/backend"
)

const binary = "zmx"

const (
	envDir     = "ZMX_DIR"
	envSession = "ZMX_SESSION"
	// envPrefix is prepended by zmx to every session name it is given.
	envPrefix = "ZMX_SESSION_PREFIX"
)

// What zmx v0.8.1 prints, after "error: ", for a session it does not hold.
const (
	historyMissing = `session %q does not exist`
	killMissing    = `failed to kill session=%s: SessionNotFound`
)

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
	out, err := b.runner.RunRaw(b.command("list"))
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
// and ignore the command, so a live name is refused first.
func (b *Backend) Create(name, dir string, command []string) error {
	if err := b.ValidateName(name); err != nil {
		return err
	}
	exists, err := b.Exists(name)
	if err != nil {
		return fmt.Errorf("create session %q: %w", name, err)
	}
	if exists {
		return fmt.Errorf("create session %q: %w", name, backend.ErrSessionExists)
	}
	cmd := b.command(append([]string{"attach", name}, command...)...)
	cmd.Dir = dir
	if _, err := b.runner.Run(cmd); err != nil {
		return fmt.Errorf("create session %q: %w", name, err)
	}
	return nil
}

// AttachCommand keeps the caller's ZMX_SESSION: inside a session, zmx attach
// then switches that session's clients to name instead of nesting.
func (b *Backend) AttachCommand(name string) backend.Command {
	return backend.Command{
		Argv: []string{binary, "attach", name},
		Env:  b.env(envDir, envPrefix),
	}
}

func (b *Backend) Kill(name string) error {
	if err := b.ValidateName(name); err != nil {
		return err
	}
	_, err := b.runner.Run(b.command("kill", name))
	return sessionError("kill", name, err, fmt.Sprintf(killMissing, name))
}

// Detach checks the session exists first: zmx detach answers a missing
// session with success.
func (b *Backend) Detach(name string) error {
	if err := b.ValidateName(name); err != nil {
		return err
	}
	exists, err := b.Exists(name)
	if err != nil {
		return fmt.Errorf("detach session %q: %w", name, err)
	}
	if !exists {
		return fmt.Errorf("detach session %q: %w", name, backend.ErrNoSuchSession)
	}
	// zmx detach takes no name; it detaches the session named by ZMX_SESSION.
	cmd := b.command("detach")
	cmd.Env = append(cmd.Env, envSession+"="+name)
	if _, err := b.runner.Run(cmd); err != nil {
		return fmt.Errorf("detach session %q: %w", name, err)
	}
	return nil
}

func (b *Backend) History(name string) ([]byte, error) {
	if err := b.ValidateName(name); err != nil {
		return nil, err
	}
	out, err := b.runner.RunRaw(b.command("history", name, "--vt"))
	if err != nil {
		return nil, sessionError("read history of", name, err, fmt.Sprintf(historyMissing, name))
	}
	return []byte(out), nil
}

func (b *Backend) Current() string {
	return lookup(b.environ(), envSession)
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
