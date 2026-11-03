package runnerreg

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/runner"
)

// Tracker marks an attach command's client with a variable the backend
// reports while that client leads its session: backend.Backend's Track.
type Tracker interface {
	Track(command backend.Command, name, value string) backend.Command
}

// Listing keeps one runner process in the registry while it is attached, its
// attaches coming one at a time. Each attach's command carries the runner's
// token, tracked. The runner registers once an attach has begun, moves to the
// next session as a switch attaches it, takes the requests sent it while an
// attach lasts, and is unlisted as soon as it no longer knows the session it
// shows.
type Listing struct {
	registry Registry
	tracker  Tracker
	relay    relay
	token    string
	reg      *Registration
}

// Listing is a listing in r of a runner not yet registered, marking each
// attach's client through t.
func (r Registry) Listing(t Tracker) *Listing {
	return &Listing{registry: r, tracker: t}
}

// Attach readies cfg's attach to be listed: its command's client carries
// the runner's token in TokenVar, tracked. Once the attach begins, the runner
// is registered on cfg.Session, or moved there from the session a switch
// left, and takes the requests sent it until Ended; once zmx switches the
// attach away, it is unlisted. A token that can't be minted leaves the
// attach unlisted.
func (l *Listing) Attach(cfg *runner.Config) {
	session := cfg.Session
	if l.token == "" {
		token, err := nanoid.NewGenerator()()
		if err != nil {
			logger.Warn("register failed", "session", session, "error", err)
			return
		}
		l.token = token
	}
	cfg.Command = l.tracker.Track(cfg.Command, TokenVar, l.token)
	requests := make(chan runner.Request)
	cfg.Requests = requests
	cfg.Began = func() {
		if l.list(session) {
			l.relay.open(requests)
		}
	}
	cfg.SwitchedAway = l.Close
}

// Ended refuses every request from now until the next attach begins, one
// already waiting for the attach included. Call it once Run returns.
func (l *Listing) Ended() {
	l.relay.close()
}

// Close removes the runner from the registry, as an attach ends other than by
// a switch. The next attach registers it again, under a token of its own.
func (l *Listing) Close() {
	l.relay.close()
	l.token = ""
	if l.reg == nil {
		return
	}
	if err := l.reg.Close(); err != nil {
		logger.Warn("unregister failed", "error", err)
	}
	l.reg = nil
}

// list registers the runner on session, or moves it there, and reports
// whether it is listed. An attach the registry can't list, or can't move,
// goes on unlisted, never listed on the session it left.
func (l *Listing) list(session string) bool {
	var err error
	if l.reg == nil {
		l.reg, err = l.registry.Register(l.token, session, &l.relay)
	} else {
		err = l.reg.SetSession(session)
	}
	if err != nil {
		logger.Warn("register failed", "session", session, "error", err)
		l.Close()
		return false
	}
	return true
}

// errNotAttached refuses a request while no attach is under way to take it.
var errNotAttached = errors.New("the runner is not attached")

// relay is a listed runner's Handler: it hands each request to the attach
// under way.
type relay struct {
	mu     sync.Mutex
	attach *gate
}

// gate is one attach's way in: its requests, until closed.
type gate struct {
	requests chan<- runner.Request
	closed   chan struct{}
}

func (r *relay) open(requests chan<- runner.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attach = &gate{requests: requests, closed: make(chan struct{})}
}

func (r *relay) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.attach != nil {
		close(r.attach.closed)
		r.attach = nil
	}
}

func (r *relay) Switch(ctx context.Context, session string) error {
	return r.deliver(ctx, runner.Request{Switch: session})
}

func (r *relay) DetachToPicker(ctx context.Context) error {
	return r.deliver(ctx, runner.Request{})
}

// deliver hands req to the attach under way, carrying ctx's deadline, and
// returns the attach's answer: nil once it has taken the request, which is
// its acceptance.
func (r *relay) deliver(ctx context.Context, req runner.Request) error {
	r.mu.Lock()
	g := r.attach
	r.mu.Unlock()
	if g == nil {
		return errNotAttached
	}
	answer := make(chan error, 1)
	req.Deadline, _ = ctx.Deadline()
	req.Answer = answer
	select {
	case g.requests <- req:
		return <-answer
	case <-g.closed:
		return errNotAttached
	case <-ctx.Done():
		return fmt.Errorf("the attach did not take the request: %w", context.Cause(ctx))
	}
}
