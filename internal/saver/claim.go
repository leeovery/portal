package saver

import (
	"context"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/sessionindex"
)

// confirmBound bounds, together, the backend calls one poll makes under the
// commit lock: well inside the bound every other writer of the index waits for
// the lock.
const confirmBound = 2 * time.Second

// claim claims each answering session's entry by the one claim rule, with no
// dir, since the saver knows none Portal minted a session in, and records
// where the session works. A claimed entry records the listed creation, so
// the directory recorded is its own; a session reporting none leaves the one
// recorded.
func (s *Saver) claim(ctx context.Context, idx *sessionindex.Index, l listing, rule sessionindex.Rule) {
	ctx, cancel := context.WithTimeout(ctx, confirmBound)
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
// makes none under the lock; a session reporting no creation leaves its entry
// as it is, so is not. Any other claim makes or replaces an entry, or adopts
// one, on the word of a listing taken outside the lock, so it waits on the
// session confirming that it still answers as listed: a kill ends the session
// before it removes the entry, and a claim made between the two would bring
// the session back.
func (s *Saver) claimable(ctx context.Context, idx *sessionindex.Index, live backend.Session) bool {
	e, held := idx.Lookup(live.Name)
	switch {
	case held && e.SameCreation(live.Created, live.PID):
		return true
	case held && live.Created.IsZero():
		return false
	}
	return s.confirmed(ctx, live)
}

// confirmed reports whether the session still answers with the creation the
// listing gave.
func (s *Saver) confirmed(ctx context.Context, live backend.Session) bool {
	current, err := s.backend.Session(ctx, live.Name)
	if err != nil {
		logger.Debug("claim-skipped", "op", "claim-skipped", "session", live.Name, "detail", "it no longer answers as listed", "error", err, "via", via)
		return false
	}
	if current.Created.Equal(live.Created) && current.PID == live.PID {
		return true
	}
	logger.Debug("claim-skipped", "op", "claim-skipped", "session", live.Name, "detail", "it answers as another creation than listed", "via", via)
	return false
}
