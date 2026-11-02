// Package backendtest provides an in-memory backend.Backend for unit tests of
// Portal's commands and TUI. Test-only: production code must not import it.
package backendtest

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/portal/internal/backend"
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

// Call is one recorded Backend call. Dir and Command are set for Create only.
type Call struct {
	Op      Op
	Name    string
	Dir     string
	Command []string
}

// Fake is an in-memory Backend. Create, Kill and Detach change the sessions it
// holds; Fail makes a method return an error before it changes anything.
type Fake struct {
	// Now stamps Created on the sessions Create makes; nil is time.Now.
	Now func() time.Time
	// Validate replaces the default name rule, which refuses only "".
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

func (f *Fake) List() ([]backend.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(Call{Op: OpList}); err != nil {
		return nil, err
	}
	sessions := make([]backend.Session, 0, len(f.sessions))
	for _, name := range slices.Sorted(maps.Keys(f.sessions)) {
		sessions = append(sessions, f.sessions[name])
	}
	return sessions, nil
}

func (f *Fake) Exists(name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(Call{Op: OpExists, Name: name}); err != nil {
		return false, err
	}
	_, ok := f.sessions[name]
	return ok, nil
}

// Create records the command joined with spaces as the session's Command.
func (f *Fake) Create(name, dir string, command []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(Call{Op: OpCreate, Name: name, Dir: dir, Command: slices.Clone(command)}); err != nil {
		return err
	}
	if err := f.validate(name); err != nil {
		return err
	}
	if _, ok := f.sessions[name]; ok {
		return fmt.Errorf("create session %q: %w", name, backend.ErrSessionExists)
	}
	f.lastPID++
	f.sessions[name] = backend.Session{
		Name:    name,
		Dir:     dir,
		PID:     f.lastPID,
		Created: f.now(),
		Command: strings.Join(command, " "),
	}
	return nil
}

func (f *Fake) AttachCommand(name string) backend.Command {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Op: OpAttachCommand, Name: name})
	return backend.Command{Argv: []string{AttachProgram, name}}
}

func (f *Fake) Kill(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.held(OpKill, name); err != nil {
		return err
	}
	delete(f.sessions, name)
	delete(f.history, name)
	return nil
}

func (f *Fake) Detach(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.held(OpDetach, name); err != nil {
		return err
	}
	s := f.sessions[name]
	s.Clients = 0
	f.sessions[name] = s
	return nil
}

func (f *Fake) History(name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.held(OpHistory, name); err != nil {
		return nil, err
	}
	return append([]byte{}, f.history[name]...), nil
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

// begin records the call and returns the failure injected for its op. The
// caller holds mu.
func (f *Fake) begin(c Call) error {
	f.calls = append(f.calls, c)
	return f.failures[c.Op]
}

// held begins a call on a named session the Fake must hold. The caller holds
// mu.
func (f *Fake) held(op Op, name string) error {
	if err := f.begin(Call{Op: op, Name: name}); err != nil {
		return err
	}
	if err := f.validate(name); err != nil {
		return err
	}
	if _, ok := f.sessions[name]; !ok {
		return fmt.Errorf("%s session %q: %w", strings.ToLower(string(op)), name, backend.ErrNoSuchSession)
	}
	return nil
}

func (f *Fake) validate(name string) error {
	if f.Validate != nil {
		return f.Validate(name)
	}
	if name == "" {
		return fmt.Errorf("%w: empty", backend.ErrInvalidName)
	}
	return nil
}

func (f *Fake) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}
