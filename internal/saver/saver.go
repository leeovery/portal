// Package saver is one poll of the saver's cycle: it keeps sessions.json in
// step with the backend's live sessions, dropping the entries of sessions that
// ended, and saves each session's scrollback beside its entry. Nothing here
// runs a poll; `portal state daemon` does, on a tick.
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

// ErrRestoring is the stand-down of a poll that found a bootstrap restoring
// sessions.
var ErrRestoring = errors.New("a restore is in progress")

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
	// Now is the saver's clock, which the drop timing and every throttle are
	// measured on; nil is time.Now.
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
		drops:      &drops{missing: map[string]time.Time{}, held: map[string]bool{}},
		scrollback: &scrollback{backend: cfg.Backend, dir: cfg.StateDir, creations: map[string]creation{}},
	}
}

// Poll runs one poll. It does nothing while the restoring flag is raised,
// returning ErrRestoring, or can't be read, nor when the listing fails.
// Otherwise it saves the listed sessions' scrollback, outside the commit lock,
// then under it claims each session that answers, records where it works,
// drops the entries of sessions that ended and names the scrollback saved.
// What a drop or a replace dropped is forgotten before the lock goes.
func (s *Saver) Poll(ctx context.Context) error {
	switch restoring, err := state.Restoring.Exists(s.dir); {
	case err != nil:
		return fmt.Errorf("read the restoring flag: %w", err)
	case restoring:
		return ErrRestoring
	}
	l, err := s.list(ctx)
	if err != nil {
		return err
	}
	s.drops.seen(l)
	captures := s.scrollback.capture(ctx, l)
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	return s.commit(ctx, l, captures)
}

// listing is one successful List, taken at at: every name it shows, and the
// sessions among them a poll claims and captures, those that answered and are
// not Portal's own.
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
		if live.Answered && !live.PortalOwned() {
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
func (s *Saver) commit(ctx context.Context, l listing, captures []capture) error {
	rule := state.ClaimRule(s.dir)
	var written *sessionindex.Index
	var named []capture
	_, err := sessionindex.UpdateThen(s.dir, via, func(idx *sessionindex.Index) error {
		s.claim(ctx, idx, l, rule)
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
		s.scrollback.clean(written, l.at)
		return nil
	})
	if err != nil {
		return fmt.Errorf("update the session index: %w", err)
	}
	return nil
}
