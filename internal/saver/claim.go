package saver

import (
	"context"
	"fmt"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/sessionindex"
)

// confirmBudget bounds, together, the backend calls one poll makes under the
// commit lock. Forget, in the same hold, waits up to 2 s on the hooks lock
// twice, so a poll waits at most 0.5 + 2 × 2 = 4.5 s holding the lock: inside
// the 5 s every other writer of the index waits for it, as a mint recording
// the directory it was opened in must.
var confirmBudget = 500 * time.Millisecond

// claim claims each answering session's entry by the one claim rule, with no
// dir, since the saver knows none Portal minted a session in, and records
// where the session works. A claimed entry records the listed creation, so
// the directory recorded is its own; a session reporting none leaves the one
// recorded.
func (s *Saver) claim(ctx context.Context, idx *sessionindex.Index, l listing, rule sessionindex.Rule) {
	ctx, cancel := context.WithTimeoutCause(ctx, confirmBudget, fmt.Errorf("the poll's %s to confirm claims ran out: %w", confirmBudget, backend.ErrTimeout))
	defer cancel()
	for _, live := range l.answered {
		if !s.claimable(ctx, idx, live) {
			continue
		}
		idx.Claim(live.Name, "", live.Created, live.PID, rule)
		if live.Dir != "" {
			idx.RecordCwd(live.Name, live.Dir)
		}
	}
}

// claimable reports whether the listed session's entry is claimed. An entry
// recording the listed creation is, with no backend call, so a quiet poll
// makes none under the lock. A session reporting no creation is not, as a
// mint, a rename and a hook set refuse one: its entry stays as it is, and it
// gets none, since no drop could ever remove an entry recording no creation.
// Any other claim makes or replaces an entry, or adopts one, on the word of a
// listing taken outside the lock, so it waits on the session confirming that
// it still answers as listed: a kill ends the session before it removes the
// entry, and a claim made between the two would bring the session back.
func (s *Saver) claimable(ctx context.Context, idx *sessionindex.Index, live backend.Session) bool {
	e, held := idx.Lookup(live.Name)
	switch {
	case held && e.SameCreation(live.Created, live.PID):
		return true
	case live.Created.IsZero():
		claimSkipped(live.Name, "it reports no creation", nil)
		return false
	}
	return s.confirmed(ctx, live)
}

// confirmed reports whether the session still answers with the creation the
// listing gave. One that can't be asked before the poll's budget runs out
// waits for the next poll.
func (s *Saver) confirmed(ctx context.Context, live backend.Session) bool {
	current, err := s.backend.Session(ctx, live.Name)
	switch {
	case err != nil && ctx.Err() != nil:
		claimSkipped(live.Name, "it was not confirmed in time", err)
		return false
	case err != nil:
		claimSkipped(live.Name, "it no longer answers as listed", err)
		return false
	case !current.Created.Equal(live.Created) || current.PID != live.PID:
		claimSkipped(live.Name, "it answers as another creation than listed", nil)
		return false
	}
	return true
}

func claimSkipped(session, detail string, err error) {
	attrs := []any{"op", "claim-skipped", "session", session, "detail", detail}
	if err != nil {
		attrs = append(attrs, "error", err)
	}
	logger.Debug("claim-skipped", append(attrs, "via", via)...)
}
