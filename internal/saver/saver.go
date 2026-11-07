// Package saver is one poll of the saver's cycle: it keeps sessions.json in
// step with the backend's live sessions, dropping the entries of sessions that
// ended, and saves each session's scrollback beside its entry. Nothing here
// runs a poll; `portal state daemon` does, on a tick, and flushes once as it
// shuts down.
package saver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// via is the surface every index write a poll makes records.
const via = "internal"

var logger = log.For("sessions")

// SessionName is the session `portal state daemon` runs in, one of Portal's
// own.
const SessionName = "_portal-saver"

// ErrRestoring is the stand-down of a poll or a flush that found a bootstrap
// restoring sessions.
var ErrRestoring = errors.New("a restore is in progress")

// Result is what a poll or a flush did, once its write to the index landed:
// the entries it claimed, making, adopting, replacing or filling one in; the
// entries of ended sessions it dropped; the sessions whose scrollback it saved
// and named; and the scrollback files no entry names it removed.
type Result struct {
	Claimed, Dropped, Captured, Removed int
}

// Quiet reports whether it did nothing.
func (r Result) Quiet() bool { return r == Result{} }

// Config is what a Saver works over.
type Config struct {
	// Backend holds the sessions the saver keeps.
	Backend backend.Backend
	// StateDir is the state directory holding sessions.json and what Portal
	// keeps beside each entry.
	StateDir string
	// Hooks is the store whose resume hooks go with the entries a poll drops
	// or replaces; nil keeps every hook.
	Hooks *hooks.Store
	// Now is the saver's clock, which the drop timing, every throttle and the
	// captures' budget are measured on; nil is time.Now.
	Now func() time.Time
}

// Saver runs the saver's polls. Between them it remembers, for its own run
// alone, when each entry went missing and what each session's scrollback held
// when last saved. It is not safe for concurrent use.
type Saver struct {
	backend    backend.Backend
	dir        string
	hooks      *hooks.Store
	now        func() time.Time
	drops      *drops
	scrollback *scrollback
}

// New returns a Saver over cfg that remembers nothing yet, so its first poll
// takes every entry missing as newly missing and writes every session's
// scrollback once.
func New(cfg Config) *Saver {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Saver{
		backend:    cfg.Backend,
		dir:        cfg.StateDir,
		hooks:      cfg.Hooks,
		now:        now,
		drops:      &drops{missing: map[string]time.Time{}, held: map[string]sessionindex.Entry{}},
		scrollback: &scrollback{backend: cfg.Backend, dir: cfg.StateDir, now: now, creations: map[string]creation{}, tooLarge: map[string]bool{}},
	}
}

// Poll runs one poll. It does nothing while the restoring flag is raised,
// returning ErrRestoring, or can't be read, nor when the listing fails.
// Otherwise it saves the listed sessions' scrollback, outside the commit lock,
// then under it claims each session that answers, records where it works,
// drops the entries of sessions that ended and names the scrollback saved.
// What a drop or a replace dropped is forgotten before the lock goes. It
// returns what it did, nothing when it fails.
func (s *Saver) Poll(ctx context.Context) (Result, error) {
	l, err := s.begin(ctx)
	if err != nil {
		return Result{}, err
	}
	s.drops.seen(l)
	captures := s.scrollback.capture(ctx, l)
	if ctx.Err() != nil {
		return Result{}, context.Cause(ctx)
	}
	return s.commit(ctx, l, captures)
}

// Flush saves, once, the scrollback of each session the listing shows
// answering whose history differs from the one its entry last named, for a
// daemon shutting down. It asks every such session, with no limit but ctx's
// and none of a poll's spacing, then names what it saved in one hold of the
// commit lock whatever ctx says by then, under the rules a poll names by. It
// claims, drops and removes nothing. It stands down as a poll does, and
// returns what it did, nothing when it fails.
func (s *Saver) Flush(ctx context.Context) (Result, error) {
	l, err := s.begin(ctx)
	if err != nil {
		return Result{}, err
	}
	return s.name(s.scrollback.flush(ctx, l))
}

// begin lists the sessions for a poll or a flush, unless the restoring flag
// is raised or can't be read.
func (s *Saver) begin(ctx context.Context) (listing, error) {
	switch restoring, err := state.Restoring.Exists(s.dir); {
	case err != nil:
		return listing{}, fmt.Errorf("read the restoring flag: %w", err)
	case restoring:
		return listing{}, ErrRestoring
	}
	return s.list(ctx)
}

// listing is one successful List, taken at at: every name it shows, and the
// sessions among them a poll claims and captures, those that answered, are not
// Portal's own, and have a name Create accepts. A session made outside Portal
// under any other name could never be restored, so it is never saved.
type listing struct {
	at       time.Time
	names    map[string]bool
	answered []backend.Session
}

func (s *Saver) list(ctx context.Context) (listing, error) {
	l := listing{at: s.now()}
	sessions, err := s.backend.List(ctx)
	if err != nil {
		return listing{}, fmt.Errorf("list sessions: %w", err)
	}
	l.names = make(map[string]bool, len(sessions))
	for _, live := range sessions {
		l.names[live.Name] = true
		if live.Answered && !live.PortalOwned() && s.backend.ValidateName(live.Name) == nil {
			l.answered = append(l.answered, live)
		}
	}
	return l, nil
}

// commit makes every decision on the index as read under the commit lock: the
// listing, taken outside it, may be stale by then, as a kill, rename, mint or
// restore can have landed since. What goes with an entry dropped, and the
// files no entry names, are removed before the lock goes, against the index as
// written.
//
// A commit whose ctx ended while it waited for the lock writes nothing: the
// daemon's shutdown bound could otherwise end the process between a drop's
// write and the forgetting that goes with it. One that held the lock first
// finishes, within the bound.
func (s *Saver) commit(ctx context.Context, l listing, captures []capture) (Result, error) {
	rule := state.ClaimRule(s.dir, s.backend.ValidateName)
	var result Result
	var written *sessionindex.Index
	var named []capture
	_, err := sessionindex.UpdateThen(s.dir, via, func(idx *sessionindex.Index) error {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		result.Claimed = s.claim(ctx, idx, l, rule)
		drop(idx, s.drops.judge(idx, l, rule))
		named = s.scrollback.name(idx, captures)
		written = idx
		return nil
	}, func(outcome sessionindex.Outcome) error {
		if dropped := outcome.Dropped(); len(dropped) > 0 {
			state.Forget(s.dir, s.hooks, via, dropped...)
		}
		s.drops.dropped(outcome.Removed, l.at)
		s.scrollback.saved(named)
		result.Dropped, result.Captured = len(outcome.Removed), len(named)
		result.Removed = s.scrollback.clean(written, l.at)
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("update the session index: %w", err)
	}
	return result, nil
}

// name names, under the commit lock, the file of each of a flush's captures
// whose entry still records the creation captured and is not restoring.
func (s *Saver) name(captures []capture) (Result, error) {
	if len(captures) == 0 {
		return Result{}, nil
	}
	var named []capture
	_, err := sessionindex.UpdateThen(s.dir, via, func(idx *sessionindex.Index) error {
		named = s.scrollback.name(idx, captures)
		return nil
	}, func(sessionindex.Outcome) error {
		s.scrollback.saved(named)
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("update the session index: %w", err)
	}
	return Result{Captured: len(named)}, nil
}
