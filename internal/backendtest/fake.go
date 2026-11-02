// Package backendtest provides an in-memory backend.Backend for unit tests of
// Portal's commands and TUI. Test-only: production code must not import it.
package backendtest

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
)

// Op names a Backend method, for error injection and the call log.
type Op string

const (
	OpList          Op = "List"
	OpExists        Op = "Exists"
	OpCreate        Op = "Create"
	OpAttachCommand Op = "AttachCommand"
	OpKill          Op = "Kill"
	OpDetach        Op = "Detach"
	OpHistory       Op = "History"
)

// AttachProgram is Argv[0] of every command AttachCommand returns.
const AttachProgram = "backendtest-attach"

// Call is one recorded Backend call. Dir and Command are set for Create only,
// Rows for History only.
type Call struct {
	Op      Op
	Name    string
	Dir     string
	Command []string
	Rows    int
}

// Fake is an in-memory Backend that keeps the driver's contract: names match
// exactly, except that Create refuses a name held under another case, as zmx
// on a case-insensitive filesystem does. A session added with Answered false
// does not answer, as a wedged or busy daemon does not: every call naming it
// fails with ErrTimeout, and Create refuses its name. Create, Kill and Detach
// change the sessions it holds; Fail makes a method return an error before it
// changes anything, and a call with an ended context fails with its cause.
type Fake struct {
	// Now stamps Created on the sessions Create makes; nil is time.Now.
	Now func() time.Time
	// Validate replaces the default name rule, zmx's own without the length
	// limit, which depends on a directory the Fake has none of.
	Validate func(name string) error

	mu       sync.Mutex
	sessions map[string]backend.Session
	history  map[string][]byte
	current  string
	failures map[Op]error
	calls    []Call
	lastPID  int
}

var _ backend.Backend = (*Fake)(nil)

// New returns a Fake holding sessions.
func New(sessions ...backend.Session) *Fake {
	f := &Fake{
		sessions: map[string]backend.Session{},
		history:  map[string][]byte{},
		failures: map[Op]error{},
		lastPID:  1000,
	}
	for _, s := range sessions {
		f.Add(s)
	}
	return f
}

// Add puts s in the Fake, replacing any session of the same name.
func (f *Fake) Add(s backend.Session) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[s.Name] = s
}

// Remove drops a session as if it ended outside Portal.
func (f *Fake) Remove(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, name)
	delete(f.history, name)
}

// Session returns the named session as the Fake holds it, without recording
// a call.
func (f *Fake) Session(name string) (backend.Session, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[name]
	return s, ok
}

// Fail makes every later call of op return err; a nil err clears it.
func (f *Fake) Fail(op Op, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		delete(f.failures, op)
		return
	}
	f.failures[op] = err
}

// SetCurrent sets the session Current reports.
func (f *Fake) SetCurrent(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current = name
}

// SetHistory sets what History returns for a session.
func (f *Fake) SetHistory(name string, vt []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.history[name] = slices.Clone(vt)
}

// Calls returns every recorded call in order.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// CallsTo returns the recorded calls of one op in order.
func (f *Fake) CallsTo(op Op) []Call {
	var out []Call
	for _, c := range f.Calls() {
		if c.Op == op {
			out = append(out, c)
		}
	}
	return out
}

func (f *Fake) List(ctx context.Context) ([]backend.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, Call{Op: OpList}); err != nil {
		return nil, err
	}
	sessions := make([]backend.Session, 0, len(f.sessions))
	for _, name := range slices.Sorted(maps.Keys(f.sessions)) {
		sessions = append(sessions, f.sessions[name])
	}
	return sessions, nil
}

func (f *Fake) Exists(ctx context.Context, name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, Call{Op: OpExists, Name: name}); err != nil {
		return false, err
	}
	s, ok := f.sessions[name]
	if ok && !s.Answered {
		return false, notAnswering(OpExists, name)
	}
	return ok, nil
}

// Create reports the session as zmx does: its Dir with symlinks resolved, as
// the session's own working directory reads, and its Command rendered by
// DisplayCommand.
func (f *Fake) Create(ctx context.Context, name, dir string, command []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, Call{Op: OpCreate, Name: name, Dir: dir, Command: slices.Clone(command)}); err != nil {
		return err
	}
	if err := f.validate(name); err != nil {
		return err
	}
	if held, ok := f.heldUnder(name); ok {
		return &backend.SessionExistsError{Name: name, Held: held}
	}
	f.lastPID++
	f.sessions[name] = backend.Session{
		Name:     name,
		Answered: true,
		Dir:      resolved(dir),
		PID:      f.lastPID,
		Created:  f.now(),
		Command:  zmx.DisplayCommand(command),
	}
	return nil
}

// AttachCommand switches, as the driver's does, when the current session is
// one the Fake holds that answers.
func (f *Fake) AttachCommand(ctx context.Context, name string) (backend.Command, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beginHeld(ctx, Call{Op: OpAttachCommand, Name: name}, zmx.CheckAttach); err != nil {
		return backend.Command{}, err
	}
	current, ok := f.sessions[f.current]
	return backend.Command{Argv: []string{AttachProgram, name}, Switches: ok && current.Answered}, nil
}

func (f *Fake) Kill(ctx context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beginHeld(ctx, Call{Op: OpKill, Name: name}, zmx.CheckLookup); err != nil {
		return err
	}
	delete(f.sessions, name)
	delete(f.history, name)
	return nil
}

func (f *Fake) Detach(ctx context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beginHeld(ctx, Call{Op: OpDetach, Name: name}, zmx.CheckLookup); err != nil {
		return err
	}
	s := f.sessions[name]
	s.Clients = 0
	f.sessions[name] = s
	return nil
}

// History returns what SetHistory gave the session, whatever rows asks for;
// a session given none, or an empty one, is ErrHistoryUnavailable, as the
// driver never returns an empty history.
func (f *Fake) History(ctx context.Context, name string, rows int) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beginHeld(ctx, Call{Op: OpHistory, Name: name, Rows: rows}, zmx.CheckLookup); err != nil {
		return nil, err
	}
	vt := f.history[name]
	if len(vt) == 0 {
		return nil, fmt.Errorf("history %q: %w", name, backend.ErrHistoryUnavailable)
	}
	return slices.Clone(vt), nil
}

func (f *Fake) Current() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current
}

func (f *Fake) ValidateName(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.validate(name)
}

// begin records the call and returns the failure injected for its op, or the
// context's cause. The caller holds mu.
func (f *Fake) begin(ctx context.Context, c Call) error {
	f.calls = append(f.calls, c)
	if err := f.failures[c.Op]; err != nil {
		return err
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	return nil
}

// beginHeld begins a call on a session the Fake must hold, answering, under
// exactly the call's name, which check must accept. The caller holds mu.
func (f *Fake) beginHeld(ctx context.Context, c Call, check func(string) error) error {
	if err := f.begin(ctx, c); err != nil {
		return err
	}
	if err := check(c.Name); err != nil {
		return err
	}
	s, ok := f.sessions[c.Name]
	if !ok {
		return fmt.Errorf("%s session %q: %w", strings.ToLower(string(c.Op)), c.Name, backend.ErrNoSuchSession)
	}
	if !s.Answered {
		return notAnswering(c.Op, c.Name)
	}
	return nil
}

func notAnswering(op Op, name string) error {
	return fmt.Errorf("%s session %q: not answering: %w", strings.ToLower(string(op)), name, backend.ErrTimeout)
}

func (f *Fake) validate(name string) error {
	if f.Validate != nil {
		return f.Validate(name)
	}
	return zmx.CheckName(name)
}

// heldUnder finds the session name answers to, as zmx finds it on a
// case-insensitive filesystem: the exact name first. The caller holds mu.
func (f *Fake) heldUnder(name string) (string, bool) {
	if _, ok := f.sessions[name]; ok {
		return name, true
	}
	for _, held := range slices.Sorted(maps.Keys(f.sessions)) {
		if strings.EqualFold(held, name) {
			return held, true
		}
	}
	return "", false
}

// resolved is dir with its symlinks resolved where it exists, as a process
// working there reads it.
func resolved(dir string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		return real
	}
	return filepath.Clean(dir)
}

func (f *Fake) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}
