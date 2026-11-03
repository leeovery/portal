// Package runnerreg is the registry of attach runners. zmx doesn't list its
// clients, so each runner lists itself while it is attached: an entry in the
// state directory's runners/ naming its pid and session, and beside it a
// control socket other Portal processes send requests to, such as a switch to
// another session.
//
// A connection to a runner's socket carries one request, a JSON line naming
// the protocol version, and its answer, one JSON line. A runner refuses a
// version or a request it doesn't know, and the caller then goes on as if no
// runner were there.
package runnerreg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/log"
)

var logger = log.For("attach")

// ErrNoRunner is a session with no live runner to ask: none attached, or none
// that has forwarded input.
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

// Runner is a live registered runner.
type Runner struct {
	// PID is the runner's process.
	PID int
	// Session is the session it is attached to.
	Session string
	// LastInput is when it last forwarded input, zero when it has forwarded
	// none.
	LastInput time.Time
	socket    string
}

// Live lists the registered runners that are alive: the pid an entry names is
// running, and the socket beside it answers as that pid. A pid alone proves
// nothing, since another process may hold it now. An entry found dead is
// removed with its socket. One that can't be read, whose runner refuses, as
// another version's may, or whose socket's path is too long to reach is left,
// as is one the caller's context ended before it was judged. A registry never
// made holds no runners.
func (r Registry) Live(ctx context.Context) ([]Runner, error) {
	tokens, err := r.tokens()
	if err != nil {
		return nil, err
	}
	found := make([]*Runner, len(tokens))
	var probes sync.WaitGroup
	for i, token := range tokens {
		probes.Go(func() { found[i] = r.probe(ctx, token) })
	}
	probes.Wait()
	var live []Runner
	for _, rn := range found {
		if rn != nil {
			live = append(live, *rn)
		}
	}
	return live, nil
}

// On lists the live runners attached to session.
func (r Registry) On(ctx context.Context, session string) ([]Runner, error) {
	live, err := r.Live(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(live, func(rn Runner) bool { return rn.Session != session }), nil
}

// Latest is the runner that last forwarded input: of a session's runners, the
// one whose terminal a command typed in that session came from. A runner that
// has forwarded none is never it, so there is none when no runner has.
func Latest(runners []Runner) (Runner, bool) {
	var latest Runner
	for _, rn := range runners {
		if rn.LastInput.After(latest.LastInput) {
			latest = rn
		}
	}
	return latest, !latest.LastInput.IsZero()
}

// SwitchFrom asks the runner attached to session current whose terminal last
// took input, the one a command typed there came from, to switch to target,
// and returns once it has accepted. Otherwise it returns ErrNoRunner, the
// runner's refusal, wrapping ErrRefused, or the failure to ask it, and the
// caller switches as if there were no runner.
func (r Registry) SwitchFrom(ctx context.Context, current, target string) error {
	attrs := []any{"session", current, "target", target}
	rn, err := r.latestOn(ctx, current)
	if err == nil {
		attrs = append(attrs, "target_pid", rn.PID)
		err = rn.Switch(ctx, target)
	}
	report("switch", err, attrs...)
	return err
}

// DetachToPicker asks every live runner attached to session to detach to the
// picker, and returns how many accepted.
func (r Registry) DetachToPicker(ctx context.Context, session string) (int, error) {
	runners, err := r.On(ctx, session)
	if err != nil {
		return 0, err
	}
	if len(runners) == 0 {
		report("detach to picker", ErrNoRunner, "session", session)
	}
	accepted := 0
	for _, rn := range runners {
		err := rn.DetachToPicker(ctx)
		report("detach to picker", err, "session", session, "target_pid", rn.PID)
		if err == nil {
			accepted++
		}
	}
	return accepted, nil
}

// Switch asks the runner to switch to session, and returns once it has
// accepted.
func (rn Runner) Switch(ctx context.Context, session string) error {
	_, err := rn.ask(ctx, request{Request: reqSwitch, Session: session})
	return err
}

// DetachToPicker asks the runner to detach to the picker, and returns once it
// has accepted.
func (rn Runner) DetachToPicker(ctx context.Context) error {
	_, err := rn.ask(ctx, request{Request: reqDetachToPicker})
	return err
}

func (r Registry) latestOn(ctx context.Context, session string) (Runner, error) {
	runners, err := r.On(ctx, session)
	if err != nil {
		return Runner{}, err
	}
	rn, ok := Latest(runners)
	if !ok {
		return Runner{}, ErrNoRunner
	}
	return rn, nil
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

// tokens names the registry's entries.
func (r Registry) tokens() ([]string, error) {
	files, err := os.ReadDir(r.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the runner registry: %w", err)
	}
	var tokens []string
	for _, f := range files {
		if token, ok := strings.CutSuffix(f.Name(), entryExt); ok && f.Type().IsRegular() {
			tokens = append(tokens, token)
		}
	}
	return tokens, nil
}

// probe is the live runner token's entry names, nil when there is none.
func (r Registry) probe(ctx context.Context, token string) *Runner {
	e, err := r.read(token)
	if err != nil {
		logger.Debug("runner entry unreadable", "path", r.entryPath(token), "error", err)
		return nil
	}
	rn := Runner{PID: e.PID, Session: e.Session, socket: r.socketPath(token)}
	if !running(e.PID) {
		r.remove(token, e.PID, "dead", nil)
		return nil
	}
	a, err := rn.ask(ctx, request{Request: reqLastInput})
	switch {
	case err == nil:
		rn.LastInput = a.LastInput
		logger.Debug("runner live", "session", rn.Session, "target_pid", rn.PID, "detail", "last input "+rn.LastInput.Format(time.RFC3339Nano))
		return &rn
	case ctx.Err() == nil && !errors.Is(err, ErrRefused) && !errors.Is(err, ErrSocketPathTooLong):
		r.remove(token, e.PID, "no answer", err)
	}
	return nil
}

// read reads token's entry, refusing one that names no pid or no session.
func (r Registry) read(token string) (entry, error) {
	data, err := os.ReadFile(r.entryPath(token))
	if err != nil {
		return entry{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var e entry
	if err := dec.Decode(&e); err != nil {
		return entry{}, err
	}
	if e.PID <= 0 || e.Session == "" {
		return entry{}, fmt.Errorf("an entry naming pid %d and session %q", e.PID, e.Session)
	}
	return e, nil
}

// remove removes a dead runner's socket, then its entry.
func (r Registry) remove(token string, pid int, reason string, cause error) {
	_ = os.Remove(r.socketPath(token))
	_ = os.Remove(r.entryPath(token))
	attrs := []any{"path", r.entryPath(token), "target_pid", pid, "reason", reason}
	if cause != nil {
		attrs = append(attrs, "error", cause)
	}
	logger.Debug("stale runner", attrs...)
}

// running reports whether a process holds pid, which must be positive. It may
// not be the runner that registered it.
func running(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
