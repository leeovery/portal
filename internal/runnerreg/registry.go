// Package runnerreg is the registry of attach runners. zmx doesn't list its
// clients, so each runner lists itself while it is attached: an entry in the
// state directory's runners/ naming its pid and session, and beside it a
// control socket other Portal processes send requests to, such as a switch to
// another session. The entry's token is also in the runner's zmx client's
// environment, as TokenVar, which zmx tracks, so the client leading a session
// names its runner.
//
// A connection to a runner's socket carries one request, a JSON line naming
// the protocol version and the asker's deadline, and its answer, one JSON
// line. A runner refuses a version, a request or a field it doesn't know, and
// the caller then goes on as if no runner were there.
package runnerreg

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/nanoid"
)

var logger = log.For("attach")

// TokenVar carries a runner's token in the environment of its zmx client,
// which tracks it, so the backend reports it for the client leading a session.
const TokenVar = "PORTAL_ATTACH"

// ErrNoRunner is a request with no runner to take it: no token, as from a
// raw zmx client or none leading, a token the registry doesn't hold, one
// naming a runner on another session, or a runner proved gone.
var ErrNoRunner = errors.New("no runner to ask")

const (
	entryExt  = ".json"
	socketExt = ".sock"
)

// Registry is the runners registered under one state directory, in its
// runners/: for each, an entry <token>.json and its socket <token>.sock.
type Registry struct {
	dir string
}

// New is the registry under the state directory stateDir.
func New(stateDir string) Registry {
	return Registry{dir: filepath.Join(stateDir, "runners")}
}

// Dir is the directory holding the entries and their sockets.
func (r Registry) Dir() string { return r.dir }

func (r Registry) entryPath(token string) string {
	return filepath.Join(r.dir, token+entryExt)
}

func (r Registry) socketPath(token string) string {
	return filepath.Join(r.dir, token+socketExt)
}

// entry is a runner's registration as its file holds it.
type entry struct {
	PID     int    `json:"pid"`
	Session string `json:"session"`
}

// registered is one runner as its entry names it.
type registered struct {
	token string
	entry
}

// Switch asks the runner registered under token, whose terminal leads the
// session current, to switch to target, and returns once it has accepted.
// Otherwise it returns ErrNoRunner, the runner's refusal, wrapping ErrRefused,
// or the failure to ask it, and the caller switches as if there were no
// runner.
func (r Registry) Switch(ctx context.Context, token, current, target string) error {
	attrs := []any{"session", current, "target", target}
	rn, err := r.lookup(token)
	if err == nil {
		attrs = append(attrs, "target_pid", rn.PID)
		err = r.askSwitch(ctx, rn, current, target)
	}
	report("switch", err, attrs...)
	return err
}

// askSwitch asks rn to switch to target only if its entry says it shows
// current. A token leading current that names a runner on another session is
// in some other client's environment: the shells of a session zmx made
// through a runner's client inherit it.
func (r Registry) askSwitch(ctx context.Context, rn registered, current, target string) error {
	if rn.Session != current {
		return fmt.Errorf("%w: runner %d is on %s, not %s", ErrNoRunner, rn.PID, rn.Session, current)
	}
	return r.ask(ctx, rn, request{Request: reqSwitch, Session: target})
}

// DetachToPicker asks every runner registered on session to detach to the
// picker, and returns how many accepted.
func (r Registry) DetachToPicker(ctx context.Context, session string) (int, error) {
	runners, err := r.on(session)
	if err != nil {
		return 0, err
	}
	if len(runners) == 0 {
		report("detach to picker", ErrNoRunner, "session", session)
	}
	accepted := 0
	for _, rn := range runners {
		err := r.ask(ctx, rn, request{Request: reqDetachToPicker})
		report("detach to picker", err, "session", session, "target_pid", rn.PID)
		if err == nil {
			accepted++
		}
	}
	return accepted, nil
}

// report logs a request's result: accepted, refused, failed, or no runner to
// ask.
func report(msg string, err error, attrs ...any) {
	attrs = append(attrs, "result", resultOf(err))
	if err != nil && !errors.Is(err, ErrNoRunner) {
		attrs = append(attrs, "error", err)
	}
	logger.Info(msg, attrs...)
}

func resultOf(err error) string {
	switch {
	case err == nil:
		return "accepted"
	case errors.Is(err, ErrNoRunner):
		return "no runner"
	case errors.Is(err, ErrRefused):
		return "refused"
	}
	return "failed"
}

// lookup reads the entry token names. A token no runner could hold, as one
// another program set, names none.
func (r Registry) lookup(token string) (registered, error) {
	if !validToken(token) {
		return registered{}, ErrNoRunner
	}
	e, err := r.read(token)
	if errors.Is(err, fs.ErrNotExist) {
		return registered{}, fmt.Errorf("%w: no entry %s", ErrNoRunner, token)
	}
	if err != nil {
		r.leave(token, 0, "unreadable", err)
		return registered{}, err
	}
	return registered{token: token, entry: e}, nil
}

// on is the runners registered on session, read as scan reads them.
func (r Registry) on(session string) ([]registered, error) {
	runners, err := r.scan()
	return slices.DeleteFunc(runners, func(rn registered) bool { return rn.Session != session }), err
}

// sweep removes what scan proves stale, so runners that ended without
// unregistering, killed or with the machine, leave nothing behind for long.
// What it can't read it leaves.
func (r Registry) sweep() {
	_, _ = r.scan()
}

// scan reads the directory for the runners registered in it whose pids are
// running. An entry whose pid isn't running is removed with its socket; one
// it can't read is left. A socket with no entry, which no runner registering
// leaves for longer than it takes to write one, is removed if it refuses
// connections. A registry never made holds no runners.
func (r Registry) scan() ([]registered, error) {
	files, err := os.ReadDir(r.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the runner registry: %w", err)
	}
	entries, sockets := map[string]bool{}, []string{}
	for _, f := range files {
		if token, ok := strings.CutSuffix(f.Name(), entryExt); ok && f.Type().IsRegular() {
			entries[token] = true
		} else if token, ok := strings.CutSuffix(f.Name(), socketExt); ok {
			sockets = append(sockets, token)
		}
	}
	for _, token := range sockets {
		if !entries[token] {
			r.removeOrphan(token)
		}
	}
	var runners []registered
	for token := range entries {
		e, err := r.read(token)
		rn := registered{token: token, entry: e}
		switch {
		case err != nil:
			r.leave(token, 0, "unreadable", err)
		case !running(e.PID):
			r.remove(rn)
		default:
			runners = append(runners, rn)
		}
	}
	return runners, nil
}

// read reads token's entry, refusing one that names no pid or no session.
func (r Registry) read(token string) (entry, error) {
	data, err := os.ReadFile(r.entryPath(token))
	if err != nil {
		return entry{}, err
	}
	var e entry
	if err := decodeStrict(data, fields{"pid": &e.PID, "session": &e.Session}); err != nil {
		return entry{}, err
	}
	if e.PID <= 0 || e.Session == "" {
		return entry{}, fmt.Errorf("an entry naming pid %d and session %q", e.PID, e.Session)
	}
	return e, nil
}

// ask sends req to rn, returning nil once it accepts. It removes rn's entry
// and socket only on proof the runner is gone, its pid not running. Anything
// else leaves them: a live runner's socket refuses connections once its
// backlog is full, as a closed one does; a missing socket or an answer from
// another pid may mean the pid is reused, which can't be told from the
// runner itself; a runner that doesn't answer in time may be busy; and one
// that refuses, or answers unreadably, may be another version's.
func (r Registry) ask(ctx context.Context, rn registered, req request) error {
	if !running(rn.PID) {
		r.remove(rn)
		return fmt.Errorf("%w: runner %d has exited", ErrNoRunner, rn.PID)
	}
	err := send(ctx, r.socketPath(rn.token), rn.PID, req)
	if err != nil {
		r.leave(rn.token, rn.PID, leftReason(err), err)
	}
	return err
}

// leftReason is why a failed request leaves the entry.
func leftReason(err error) string {
	switch {
	case errors.Is(err, ErrRefused):
		return "refused"
	case errors.Is(err, errOtherPID):
		return "another pid"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	case errors.Is(err, fs.ErrNotExist):
		return "missing"
	case errors.Is(err, os.ErrDeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	}
	return "no answer"
}

// remove removes the socket, then the entry, of a runner whose pid isn't
// running.
func (r Registry) remove(rn registered) {
	_ = os.Remove(r.socketPath(rn.token))
	_ = os.Remove(r.entryPath(rn.token))
	logger.Debug("stale runner", "path", r.entryPath(rn.token), "target_pid", rn.PID, "reason", "dead")
}

// leave logs why an entry stays, its runner's pid when it is known.
func (r Registry) leave(token string, pid int, reason string, cause error) {
	attrs := []any{"path", r.entryPath(token), "reason", reason, "error", cause}
	if pid > 0 {
		attrs = append(attrs, "target_pid", pid)
	}
	logger.Debug("runner entry left", attrs...)
}

// removeOrphan removes a socket with no entry once a connection to it is
// refused, which proves no runner listens there.
func (r Registry) removeOrphan(token string) {
	path := r.socketPath(token)
	if checkSocketPath(path) != nil {
		return
	}
	conn, err := net.Dial("unix", path)
	if err == nil {
		_ = conn.Close()
		return
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		_ = os.Remove(path)
		logger.Debug("stale runner", "path", path, "reason", "orphan socket", "error", err)
	}
}

// running reports whether a process holds pid, which must be positive. It may
// not be the runner that registered it.
func running(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// validToken reports whether token could have been minted for a runner, and
// so names a path in the registry and no other.
func validToken(token string) bool {
	if token == "" || len(token) > 32 {
		return false
	}
	for i := range len(token) {
		if strings.IndexByte(nanoid.Alphabet, token[i]) < 0 {
			return false
		}
	}
	return true
}
