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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/bootid"
)

const (
	envDir     = "ZMX_DIR"
	envSession = "ZMX_SESSION"
	// envPrefix is prepended by zmx to every session name it is given.
	envPrefix   = "ZMX_SESSION_PREFIX"
	envTmux     = "TMUX"
	envTmuxPane = "TMUX_PANE"
	// envNoDetachKey turns off zmx's ctrl+\ detach.
	envNoDetachKey = "ZMX_NO_DETACH_KEY"
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

var errHungUp = fmt.Errorf("zmx daemon hung up without ending its session: %w", backend.ErrTimeout)

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
	// Signal sends sig to pid, a process the process table proves a
	// session's zmx daemon: Create's end of a session an older zmx made,
	// should that zmx's own kill fail. Nil signals nothing.
	Signal func(pid int, sig syscall.Signal) error
}

// errSignalsNothing is every signal a Backend bound with no Signal is asked
// to send.
var errSignalsNothing = errors.New("this zmx backend signals nothing")

// Backend drives the zmx sessions in one zmx directory.
type Backend struct {
	binary   string
	dir      string
	environ  func() []string
	hostname func() (string, error)
	booted   func() (time.Time, error)
	signal   func(pid int, sig syscall.Signal) error
	limits   ceilings
	// processes is the process table AttachClients, unheld and EndDaemon
	// read.
	processes processTable
}

var _ backend.Backend = (*Backend)(nil)

// New binds a Backend to cfg.Dir, which must be absolute.
func New(cfg Config) (*Backend, error) {
	if !filepath.IsAbs(cfg.Dir) {
		return nil, fmt.Errorf("bind zmx backend: directory %q is not absolute", cfg.Dir)
	}
	b := &Backend{
		binary:    cfg.Binary,
		dir:       filepath.Clean(cfg.Dir),
		environ:   cfg.Environ,
		hostname:  cfg.Hostname,
		booted:    cfg.BootTime,
		signal:    cfg.Signal,
		limits:    defaultCeilings,
		processes: systemProcesses,
	}
	if b.environ == nil {
		b.environ = os.Environ
	}
	if b.hostname == nil {
		b.hostname = os.Hostname
	}
	if b.booted == nil {
		b.booted = bootid.BootTime
	}
	if b.signal == nil {
		b.signal = func(int, syscall.Signal) error { return errSignalsNothing }
	}
	return b, nil
}

// Dir is the zmx directory the Backend is bound to.
func (b *Backend) Dir() string { return b.dir }

// DisplayCommand is command as zmx reports it, as the package's
// DisplayCommand renders it.
func (b *Backend) DisplayCommand(command []string) string { return DisplayCommand(command) }

// ReachEnv is the environment another process needs to reach the sessions b
// does: b's directory. Every zmx Portal runs has ZMX_SESSION_PREFIX removed,
// so no prefix is needed.
func (b *Backend) ReachEnv() []string {
	return []string{envDir + "=" + b.dir}
}

// Session asks the session held under exactly name for its Info record, within
// the probe ceiling, on a connection of its own.
func (b *Backend) Session(ctx context.Context, name string) (backend.Session, error) {
	i, err := b.askWithin(ctx, name)
	if err != nil {
		return backend.Session{}, fmt.Errorf("read session %q: %w", name, err)
	}
	// Without a hostname only a cwd with no host, or localhost, reads as
	// local, as in List.
	hostname, _ := b.hostname()
	return sessionOf(name, i, hostname), nil
}

// Exists reports whether a session that answers is held under exactly name.
// A stale socket, a name held only under another spelling, or one no session
// can have, does not count. A session that gives no well-formed answer is
// neither live nor gone, and is ErrTimeout.
func (b *Backend) Exists(ctx context.Context, name string) (bool, error) {
	_, err := b.askWithin(ctx, name)
	if errors.Is(err, backend.ErrNoSuchSession) || errors.Is(err, backend.ErrInvalidName) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check session %q: %w", name, err)
	}
	return true, nil
}

// AttachCommand runs zmx attach on name, which must answer: zmx attach on a
// socket that refuses would replace it with a new session, orphaning any
// daemon behind it, and on a daemon that does not answer would hang the
// terminal. Inside a session that answers in the Backend's directory, the
// command keeps the caller's ZMX_SESSION, so zmx switches that session's
// clients to name and exits at once instead of nesting. Anywhere else it
// drops ZMX_SESSION, with which zmx attach would switch a session it cannot
// reach, or another directory's session of the same name. It drops TMUX and
// TMUX_PANE, as Create does: a session that ends between the probe and the
// attach is made afresh by zmx attach, and must not take itself to be inside
// the caller's tmux. It drops ZMX_NO_DETACH_KEY: Portal's attach runner ends
// an attach with zmx's ctrl+\, which that turns into input for the session.
// Each session is asked within a probe ceiling of its own, so a slow one
// cannot starve the other.
func (b *Backend) AttachCommand(ctx context.Context, name string) (backend.Command, error) {
	if err := CheckAttach(name); err != nil {
		return backend.Command{}, fmt.Errorf("attach session %q: %w", name, err)
	}
	if _, err := b.askWithin(ctx, name); err != nil {
		return backend.Command{}, fmt.Errorf("attach session %q: %w", name, err)
	}
	switches := b.switches(ctx)
	if ctx.Err() != nil {
		return backend.Command{}, fmt.Errorf("attach session %q: %w", name, context.Cause(ctx))
	}
	cmd := backend.Command{Argv: []string{b.binary, "attach", name}, Switches: switches}
	if switches {
		cmd.Env = b.env(envPrefix, envTmux, envTmuxPane, envNoDetachKey)
	} else {
		cmd.Env = b.env(envPrefix, envSession, envTmux, envTmuxPane, envNoDetachKey)
	}
	return cmd, nil
}

// switches reports whether zmx attach, run in this process's environment,
// would switch the caller's session: one that answers in the zmx directory
// that environment names, when that is the Backend's own.
func (b *Backend) switches(ctx context.Context) bool {
	current := b.Current()
	if current == "" {
		return false
	}
	_, err := b.askWithin(ctx, current)
	return err == nil
}

// askWithin asks the session held under exactly name for its Info record,
// within the probe ceiling.
func (b *Backend) askWithin(ctx context.Context, name string) (info, error) {
	ctx, cancel := b.bound(ctx, b.limits.probe, "")
	defer cancel()
	c, s, err := b.open(ctx, name)
	if err != nil {
		return info{}, err
	}
	defer func() { _ = c.Close() }()
	i, err := c.askInfo()
	if err != nil {
		return info{}, b.noAnswer(ctx, s, err)
	}
	return i, nil
}

// Kill waits for the daemon to hang up. zmx unlinks a session's socket before
// it closes the session's clients, so a hangup with the socket gone means the
// name is free, and one with the socket still there is a daemon not
// answering. A message to a stopped daemon waits in its socket and lands once
// it runs again.
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
// closes every connection to the session, this one included. So a daemon
// that hangs up on every request looks like one that detached.
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
	c, s, err := b.open(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("read history of session %q: %w", name, err)
	}
	defer func() { _ = c.Close() }()
	request, payload := tagHistory, []byte{historyVT}
	if rows > 0 {
		request, payload = tagCapture, encodeCapture(historyVT, uint32(min(rows, math.MaxUint32)))
	}
	if err := c.send(request, payload); err != nil {
		return nil, fmt.Errorf("read history of session %q: %w", name, b.noAnswer(ctx, s, err))
	}
	vt, err := c.await(request)
	if errors.Is(err, errUnsupported) {
		return nil, fmt.Errorf("read history of session %q: %w: %w", name, err, backend.ErrHistoryUnavailable)
	}
	if err != nil {
		return nil, fmt.Errorf("read history of session %q: %w", name, b.noAnswer(ctx, s, err))
	}
	if len(vt) == 0 {
		return nil, fmt.Errorf("read history of session %q: %w", name, backend.ErrHistoryUnavailable)
	}
	return vt, nil
}

// Current reads ZMX_SESSION, which zmx sets in every session's processes,
// when the environment's zmx directory is the Backend's: a session in another
// directory is none of the Backend's.
func (b *Backend) Current() string {
	env := b.environ()
	if filepath.Clean(DirFor(env)) != b.dir {
		return ""
	}
	return CurrentFor(env)
}

// CurrentFor is the session the environment environ runs in, empty outside
// one: what Current reads, with no Backend to bind.
func CurrentFor(environ []string) string {
	return lookup(environ, envSession)
}

// SessionEntry is the entry, ZMX_SESSION=name, that zmx sets in the
// environment of the process the session name runs. That process carries
// ReachEnv's entries as well, but only because every zmx run the Backend
// makes sets them: zmx sets no ZMX_DIR of its own.
func SessionEntry(name string) string {
	return envSession + "=" + name
}

// SessionMarks are the entries the environment of the process session name
// runs carries, given reach, the ReachEnv of the Backend that made it:
// SessionEntry, and reach. It refuses a reach naming no directory, since
// SessionEntry alone marks the session of that name in every directory, a
// developer's own included.
func SessionMarks(name string, reach []string) ([]string, error) {
	if dir, _ := lookupSet(reach, envDir); dir == "" {
		return nil, fmt.Errorf("mark session %s: %q names no %s", name, reach, envDir)
	}
	return append([]string{SessionEntry(name)}, reach...), nil
}

// ProtocolVersion is the socket protocol version that the daemon of the live
// session name speaks, read from the header of its answer to Info alone. Any
// version but ProtocolVersion is unsupported. A zmx 0.8.1 daemon gives no
// answer, so ErrTimeout: it reads the request as a frame still awaiting
// payload, and the output it broadcasts meanwhile is no answer. A socket that
// refuses connections is ErrRefusing.
func (b *Backend) ProtocolVersion(ctx context.Context, name string) (int, error) {
	ctx, cancel := b.bound(ctx, b.limits.probe, "")
	defer cancel()
	c, s, err := b.open(ctx, name)
	if err != nil {
		return 0, fmt.Errorf("read protocol version of session %q: %w", name, err)
	}
	defer func() { _ = c.Close() }()
	if err := c.send(tagInfo, nil); err != nil {
		return 0, fmt.Errorf("read protocol version of session %q: %w", name, b.noAnswer(ctx, s, err))
	}
	version, err := c.answerVersion()
	if err != nil {
		return 0, fmt.Errorf("read protocol version of session %q: %w", name, b.noAnswer(ctx, s, err))
	}
	return int(version), nil
}

// open connects to the live session held under exactly name. A name held only
// under another spelling, or behind a stale socket, is ErrNoSuchSession: on a
// folding filesystem zmx's socket for foo answers for FOO. A socket that does
// not accept, though a daemon may be behind it, is ErrTimeout.
func (b *Backend) open(ctx context.Context, name string) (*conn, socket, error) {
	if err := b.checkLookup(name); err != nil {
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
		return nil, socket{}, &backend.HeldAsError{Held: s.name}
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

// lookup is key's value in env, the first entry winning, as zmx's getenv
// reads its own environment.
func lookup(env []string, key string) string {
	value, _ := lookupSet(env, key)
	return value
}

// lookupSet is lookup that also reports whether key is set at all.
func lookupSet(env []string, key string) (value string, set bool) {
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); k == key {
			return v, true
		}
	}
	return "", false
}

// DirFor is the directory zmx uses in the environment environ, resolved as
// zmx's cfg.zig does, each variable's first entry winning: ZMX_DIR when set,
// otherwise zmx in XDG_RUNTIME_DIR, otherwise zmx-<uid> in TMPDIR, or in /tmp
// when TMPDIR is unset. zmx sets no ZMX_DIR of its own in a session, so a
// session started without one is in this default directory.
func DirFor(environ []string) string {
	if dir, ok := lookupSet(environ, envDir); ok {
		return dir
	}
	if runtimeDir, ok := lookupSet(environ, "XDG_RUNTIME_DIR"); ok {
		return runtimeDir + "/zmx"
	}
	tmp, ok := lookupSet(environ, "TMPDIR")
	if !ok {
		tmp = "/tmp"
	}
	return strings.TrimRight(tmp, "/") + "/zmx-" + strconv.Itoa(os.Getuid())
}
