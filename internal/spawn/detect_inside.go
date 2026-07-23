package spawn

import (
	"fmt"

	"github.com/leeovery/portal/internal/tmux"
)

// ClientActivity is one tmux client's detection-relevant data: the client's
// process id (the walk entry point) and its last-activity timestamp (the
// local-only tiebreak). It is the spawn-package mirror of tmux.ClientInfo,
// declared here so the inside-tmux resolution is unit-testable without a tmux
// dependency.
type ClientActivity struct {
	PID      int
	Activity int64
}

// clientLister enumerates the tmux clients attached to a session. It is a
// 1-method DI seam so detectInsideTmux is unit-testable with a fabricated
// client set — the production implementation is tmuxClientLister.
type clientLister interface {
	ListClients(session string) ([]ClientActivity, error)
}

// tmuxClientLister is the production clientLister, adapting *tmux.Client's
// ListClients (which returns tmux.ClientInfo) to the spawn-local ClientActivity
// shape.
type tmuxClientLister struct {
	c *tmux.Client
}

var _ clientLister = tmuxClientLister{}

// ListClients delegates to the tmux client and maps each tmux.ClientInfo to a
// spawn ClientActivity.
func (l tmuxClientLister) ListClients(session string) ([]ClientActivity, error) {
	infos, err := l.c.ListClients(session)
	if err != nil {
		return nil, err
	}
	clients := make([]ClientActivity, 0, len(infos))
	for _, info := range infos {
		clients = append(clients, ClientActivity{PID: info.PID, Activity: info.Activity})
	}
	return clients, nil
}

// detectInsideTmux resolves the host-terminal Identity for a Portal process
// running INSIDE tmux, where the picker's own ancestry leads to the tmux server
// rather than the launching terminal. It must therefore gate locality on the
// client that *triggered* this burst, not on "is any host-local client attached
// to the session?".
//
// The triggering client is the most-active one: client_activity tracks a
// client's sent input (not the received redraws a passive mirror gets), and
// detection runs immediately after the user's trigger keystroke, so the
// just-bumped triggering client is reliably the freshest at detection time.
// The algorithm is therefore select-winner-then-locality-check — the inverse of
// the old filter-then-tiebreak order:
//
//  1. Enumerate the session's clients via lister.ListClients.
//  2. Select the winner: the client with the greatest client_activity across
//     ALL enumerated clients — local and remote alike — with the first-listed
//     client winning an exact tie. No walking happens during selection.
//  3. Walk ONLY that winner's process tree and branch on its locality:
//     a resolved (non-NULL) identity means the trigger is host-local → drive;
//     a clean NULL means the trigger is remote/mosh → honest no-op; a transient
//     walk failure fails safe to NULL + WARN (never spawn on uncertainty).
//
// Because only the winner is walked, the old "one flaky `ps` cannot mask a
// resolvable local client" guarantee is deliberately DROPPED for the winner: a
// legitimate local burst with 2+ local clients, where the most-active client's
// `ps` transiently flakes, now refuses (NULL + WARN) instead of falling back to
// a resolvable lower-activity local. This is an owned, deliberate trade of
// walk-resilience for correctness — the fail-safe is to never spawn on
// uncertainty rather than risk spawning on the wrong machine.
//
// Outcomes:
//
//   - list-clients failure: NULL identity, ErrDetectTransient-wrapped error (the
//     winner is only computed after a successful enumeration).
//   - empty client list: clean NULL, nil error — no winner to select, the
//     honest "no host-local terminal" no-op.
//   - winner walks to a resolved identity: that Identity (the trigger is
//     host-local — drive the burst).
//   - winner walks to a clean NULL: NULL identity, nil error — the trigger is a
//     remote/mosh client, an honest no-op.
//   - winner walk transiently fails: NULL identity, the ErrDetectTransient-
//     wrapped walk error (folds to a spawn WARN) — the dropped-resilience
//     fail-safe.
func detectInsideTmux(session string, lister clientLister, walker ProcessWalker, reader BundleReader) (Identity, error) {
	clients, err := lister.ListClients(session)
	if err != nil {
		return Identity{}, transient(fmt.Sprintf("list tmux clients for session %q", session), err)
	}

	if len(clients) == 0 {
		// No winner to select — the honest no-op, not a transient error.
		return Identity{}, nil
	}

	winner := selectTriggeringClient(clients)

	id, werr := walkToBundle(winner.PID, walker, reader)
	if werr != nil {
		// The triggering client's walk transiently failed. Fail safe to NULL +
		// WARN (already ErrDetectTransient-wrapped) rather than falling back to
		// a lower-activity local — never spawn on uncertainty.
		return Identity{}, werr
	}
	if id.IsNull() {
		// The triggering client is remote/mosh — no host-local terminal in its
		// ancestry. Honest no-op.
		return Identity{}, nil
	}
	return id, nil
}

// selectTriggeringClient picks the burst's triggering client from a non-empty
// enumeration: the client with the strictly-greatest client_activity, with the
// first-listed client winning an exact tie (deterministic, and stable with the
// existing multi-local behaviour). It performs no process walking.
func selectTriggeringClient(clients []ClientActivity) ClientActivity {
	winner := clients[0]
	for _, client := range clients[1:] {
		if client.Activity > winner.Activity {
			winner = client
		}
	}
	return winner
}
