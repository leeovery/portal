// Package zmx implements backend.Backend on zmx: over zmx's socket protocol
// for listing, kill, detach and history, and by running the zmx binary to
// create a session and to attach to one, since creation is no protocol
// message. It targets the zmx build pinned in scripts/zmx-commit.
package zmx

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/backend"
)

const (
	envDir     = "ZMX_DIR"
	envSession = "ZMX_SESSION"
	// envPrefix is prepended by zmx to every session name it is given.
	envPrefix   = "ZMX_SESSION_PREFIX"
	envTmux     = "TMUX"
	envTmuxPane = "TMUX_PANE"
)

// ceilings bound each operation beneath its caller's context. A probe gets
// the second zmx gives one.
type ceilings struct {
	probe, create, kill, detach, history time.Duration
}

var defaultCeilings = ceilings{
	probe:   time.Second,
	create:  10 * time.Second,
	kill:    5 * time.Second,
	detach:  5 * time.Second,
	history: 10 * time.Second,
}

var errHungUp = errors.New("zmx daemon hung up without ending its session")

// Config binds a Backend.
type Config struct {
	// Binary is the zmx executable Create runs and AttachCommand names.
	Binary string
	// Dir is the zmx directory, absolute.
	Dir string
	// Environ supplies the environment every zmx run starts from, and the
	// process's own ZMX_SESSION; nil is os.Environ.
	Environ func() []string
	// Hostname names this machine, to tell a session's local cwd from one on
	// another host; nil is os.Hostname.
	Hostname func() (string, error)
	// BootTime is when this machine booted, to tell a stale socket from a
	// session not answering; nil reads it from the kernel.
	BootTime func() (time.Time, error)
}

// Backend drives the zmx sessions in one zmx directory.
type Backend struct {
	binary   string
	dir      string
	environ  func() []string
	hostname func() (string, error)
	booted   func() (time.Time, error)
	limits   ceilings
}

var _ backend.Backend = (*Backend)(nil)

// New binds a Backend to cfg.Dir.
func New(cfg Config) *Backend {
	b := &Backend{
		binary:   cfg.Binary,
		dir:      filepath.Clean(cfg.Dir),
		environ:  cfg.Environ,
		hostname: cfg.Hostname,
		booted:   cfg.BootTime,
		limits:   defaultCeilings,
	}
	if b.environ == nil {
		b.environ = os.Environ
	}
	if b.hostname == nil {
		b.hostname = os.Hostname
	}
	if b.booted == nil {
		b.booted = bootTime
	}
	return b
}

// Dir is the zmx directory the Backend is bound to.
func (b *Backend) Dir() string { return b.dir }

// Exists reports whether a live session is held under exactly name. A stale
// socket, a name held only under another spelling, or one zmx cannot hold,
// does not count. A session that refuses connections is neither live nor
// gone, and is ErrTimeout.
func (b *Backend) Exists(ctx context.Context, name string) (bool, error) {
	ctx, cancel := b.bound(ctx, b.limits.probe, "")
	defer cancel()
	c, _, err := b.open(ctx, name)
	if errors.Is(err, backend.ErrNoSuchSession) || errors.Is(err, backend.ErrInvalidName) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check session %q: %w", name, err)
	}
	_ = c.Close()
	return true, nil
}

// AttachCommand keeps the caller's ZMX_SESSION: inside a session, zmx attach
// then switches that session's clients to name instead of nesting. The name
// must be live: zmx attach on a socket that refuses would replace it with a
// new session, orphaning any daemon behind it.
func (b *Backend) AttachCommand(ctx context.Context, name string) (backend.Command, error) {
	ctx, cancel := b.bound(ctx, b.limits.probe, "")
	defer cancel()
	c, _, err := b.open(ctx, name)
	if err != nil {
		return backend.Command{}, fmt.Errorf("attach session %q: %w", name, err)
	}
	_ = c.Close()
	return backend.Command{
		Argv: []string{b.binary, "attach", name},
		Env:  b.env(envPrefix),
	}, nil
}

// Kill waits for the daemon to hang up. zmx unlinks a session's socket before
// it closes the session's clients, so a hangup with the socket gone means the
// name is free. A message to a stopped daemon waits in its socket and lands
// once it runs again.
func (b *Backend) Kill(ctx context.Context, name string) error {
	ctx, cancel := b.bound(ctx, b.limits.kill, "; the kill may still land")
	defer cancel()
	c, s, err := b.open(ctx, name)
	if err != nil {
		return fmt.Errorf("kill session %q: %w", name, err)
	}
	defer func() { _ = c.Close() }()
	if err := c.hangupAfter(tagKill); err != nil {
		return fmt.Errorf("kill session %q: %w", name, failure(ctx, err))
	}
	if b.stillHeld(s) {
		return fmt.Errorf("kill session %q: %w", name, errHungUp)
	}
	return nil
}

// Detach waits for the daemon to hang up: DetachAll answers nothing, but
// closes every connection to the session, this one included.
func (b *Backend) Detach(ctx context.Context, name string) error {
	ctx, cancel := b.bound(ctx, b.limits.detach, "; the detach may still land")
	defer cancel()
	c, _, err := b.open(ctx, name)
	if err != nil {
		return fmt.Errorf("detach session %q: %w", name, err)
	}
	defer func() { _ = c.Close() }()
	if err := c.hangupAfter(tagDetachAll); err != nil {
		return fmt.Errorf("detach session %q: %w", name, failure(ctx, err))
	}
	return nil
}

// History asks for the whole history with a History request, and a bounded
// one with a Capture request: zmx's Capture of 0 rows is the screen alone.
func (b *Backend) History(ctx context.Context, name string, rows int) ([]byte, error) {
	if rows < 0 {
		return nil, fmt.Errorf("read history of session %q: negative row count %d", name, rows)
	}
	ctx, cancel := b.bound(ctx, b.limits.history, "")
	defer cancel()
	c, _, err := b.open(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("read history of session %q: %w", name, err)
	}
	defer func() { _ = c.Close() }()
	request, payload := tagHistory, []byte{historyVT}
	if rows > 0 {
		request, payload = tagCapture, encodeCapture(historyVT, uint32(min(rows, math.MaxUint32)))
	}
	if err := c.send(request, payload); err != nil {
		return nil, fmt.Errorf("read history of session %q: %w", name, failure(ctx, err))
	}
	vt, err := c.await(request)
	if errors.Is(err, errUnsupported) {
		return nil, fmt.Errorf("read history of session %q: %w: %w", name, err, backend.ErrHistoryUnavailable)
	}
	if err != nil {
		return nil, fmt.Errorf("read history of session %q: %w", name, failure(ctx, err))
	}
	if len(vt) == 0 {
		return nil, fmt.Errorf("read history of session %q: %w", name, backend.ErrHistoryUnavailable)
	}
	return vt, nil
}

// Current reads ZMX_SESSION, which zmx sets in every session's processes.
func (b *Backend) Current() string {
	return lookup(b.environ(), envSession)
}

// ProtocolVersion is the socket protocol version that the daemon of the live
// session name speaks, read from the header of its answer. A daemon that
// cannot read the driver's own frames gives no answer, so ErrTimeout.
func (b *Backend) ProtocolVersion(ctx context.Context, name string) (int, error) {
	ctx, cancel := b.bound(ctx, b.limits.probe, "")
	defer cancel()
	c, _, err := b.open(ctx, name)
	if err != nil {
		return 0, fmt.Errorf("read protocol version of session %q: %w", name, err)
	}
	defer func() { _ = c.Close() }()
	if err := c.send(tagInfo, nil); err != nil {
		return 0, fmt.Errorf("read protocol version of session %q: %w", name, failure(ctx, err))
	}
	h, err := c.header()
	if err != nil && !errors.Is(err, errProtocolVersion) {
		return 0, fmt.Errorf("read protocol version of session %q: %w", name, failure(ctx, err))
	}
	return int(h.version), nil
}

// open connects to the live session held under exactly name. A name held only
// under another spelling, or behind a stale socket, is ErrNoSuchSession: on a
// folding filesystem zmx's socket for foo answers for FOO. A socket from this
// boot that refuses is ErrTimeout.
func (b *Backend) open(ctx context.Context, name string) (*conn, socket, error) {
	if err := b.ValidateName(name); err != nil {
		return nil, socket{}, err
	}
	s, ok, err := b.held(name)
	if err != nil {
		return nil, socket{}, err
	}
	if !ok {
		return nil, socket{}, backend.ErrNoSuchSession
	}
	if s.name != name {
		return nil, socket{}, fmt.Errorf("held as %q: %w", s.name, backend.ErrNoSuchSession)
	}
	c, err := b.connect(ctx, name)
	if gone(err) {
		return nil, socket{}, fmt.Errorf("%w: %w", err, backend.ErrNoSuchSession)
	}
	if err != nil {
		return nil, socket{}, failure(ctx, err)
	}
	return c, s, nil
}

// stillHeld reports whether the directory still holds the very socket file s.
func (b *Backend) stillHeld(s socket) bool {
	info, err := os.Stat(b.path(s.name))
	return err == nil && os.SameFile(s.info, info)
}

// bound puts the operation's ceiling beneath ctx. Past it, the operation
// fails with ErrTimeout, and note says what that leaves undecided.
func (b *Backend) bound(ctx context.Context, limit time.Duration, note string) (context.Context, context.CancelFunc) {
	return context.WithTimeoutCause(ctx, limit, fmt.Errorf("no answer in %s%s: %w", limit, note, backend.ErrTimeout))
}

// env is the environment without the drop keys, with ZMX_DIR set to the
// Backend's directory.
func (b *Backend) env(drop ...string) []string {
	env := slices.DeleteFunc(slices.Clone(b.environ()), func(kv string) bool {
		key, _, _ := strings.Cut(kv, "=")
		return key == envDir || slices.Contains(drop, key)
	})
	return append(env, envDir+"="+b.dir)
}

// lookup is key's value in env, the last entry winning as it does in an
// exec.Cmd's Env.
func lookup(env []string, key string) string {
	var value string
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); k == key {
			value = v
		}
	}
	return value
}
