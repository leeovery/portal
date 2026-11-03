package runnerreg

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/leeovery/portal/internal/runner"
)

// Listing keeps one runner process in the registry while it is attached, its
// attaches coming one at a time. It registers once an attach has begun, moves
// the entry to the next session as a switch attaches it, hands each attach
// the requests sent the runner while that attach lasts, and keeps when the
// runner last forwarded input.
type Listing struct {
	registry Registry
	relay    relay
	reg      *Registration
}

// Listing is a listing in r of a runner not yet registered.
func (r Registry) Listing() *Listing {
	return &Listing{registry: r}
}

// Attach readies cfg's attach to be listed. Once it begins, the runner is
// registered on cfg.Session, or moved there from the session a switch left,
// and takes the requests sent it until Ended.
func (l *Listing) Attach(cfg *runner.Config) {
	requests := make(chan runner.Request)
	session := cfg.Session
	cfg.Requests = requests
	cfg.Forwarded = l.relay.forwarded
	cfg.Began = func() {
		l.list(session)
		l.relay.open(requests)
	}
}

// Ended refuses every request from now until the next attach begins, one
// already waiting for the attach included. Call it once Run returns.
func (l *Listing) Ended() {
	l.relay.close()
}

// Close removes the runner from the registry, as an attach ends other than by
// a switch. The next attach registers it again.
func (l *Listing) Close() {
	l.relay.close()
	if l.reg == nil {
		return
	}
	if err := l.reg.Close(); err != nil {
		logger.Warn("unregister failed", "error", err)
	}
	l.reg = nil
}

// list registers the runner on session, or moves it there. An attach the
// registry can't list goes on unlisted.
func (l *Listing) list(session string) {
	var err error
	if l.reg == nil {
		l.reg, err = l.registry.Register(session, &l.relay)
	} else {
		err = l.reg.SetSession(session)
	}
	if err != nil {
		logger.Warn("register failed", "session", session, "error", err)
	}
}

// errNotAttached refuses a request while no attach is under way to take it.
var errNotAttached = errors.New("the runner is not attached")

// relay is a listed runner's Handler: it hands each request to the attach
// under way, and keeps when the runner last forwarded input.
type relay struct {
	// lastInput is in Unix nanoseconds, 0 for none.
	lastInput atomic.Int64

	mu     sync.Mutex
	attach *gate
}

// gate is one attach's way in: its requests, until closed.
type gate struct {
	requests chan<- runner.Request
	closed   chan struct{}
}

func (r *relay) forwarded() {
	r.lastInput.Store(time.Now().UnixNano())
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

func (r *relay) LastInput() time.Time {
	if n := r.lastInput.Load(); n != 0 {
		return time.Unix(0, n)
	}
	return time.Time{}
}

func (r *relay) Switch(ctx context.Context, session string) error {
	return r.deliver(ctx, runner.Request{Switch: session})
}

func (r *relay) DetachToPicker(ctx context.Context) error {
	return r.deliver(ctx, runner.Request{})
}

// deliver hands req to the attach under way, and returns once the attach has
// taken it, which is its acceptance.
func (r *relay) deliver(ctx context.Context, req runner.Request) error {
	r.mu.Lock()
	g := r.attach
	r.mu.Unlock()
	if g == nil {
		return errNotAttached
	}
	select {
	case g.requests <- req:
		return nil
	case <-g.closed:
		return errNotAttached
	case <-ctx.Done():
		return fmt.Errorf("the attach did not take the request: %w", context.Cause(ctx))
	}
}
