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
// rather than the launching terminal. It instead enumerates the current
// session's clients and walks each client's process tree:
//
//   - A client that walks to a resolved (non-NULL) identity is host-local — its
//     app is a terminal on this machine. NULL-filtering is the primary signal.
//   - A client that walks to a clean NULL is remote/mosh (its ancestry never
//     reaches a local `.app`); it is dropped.
//   - A transient walk failure (a flaky `ps`) is recorded but does not abort the
//     scan, so one bad `ps` cannot mask a resolvable local client.
//
// Outcomes:
//
//   - list-clients failure: NULL identity, ErrDetectTransient-wrapped error.
//   - zero host-local clients, no transient seen: clean NULL, nil error (the
//     honest "no host-local terminal" no-op — a purely-remote trigger).
//   - zero host-local clients, a transient walk was seen: NULL identity, the
//     transient walk error (already ErrDetectTransient-wrapped) — detection
//     genuinely could not complete.
//   - exactly one host-local client: its Identity (no tiebreak).
//   - 2+ host-local clients: the one with the highest client_activity, first
//     listed winning an exact tie. client_activity is used ONLY to disambiguate
//     among host-local clients — never as a cross-client primary signal.
func detectInsideTmux(session string, lister clientLister, walker ProcessWalker, reader BundleReader) (Identity, error) {
	clients, err := lister.ListClients(session)
	if err != nil {
		return Identity{}, transient(fmt.Sprintf("list tmux clients for session %q", session), err)
	}

	var (
		best         Identity
		bestActivity int64
		localFound   bool
		firstWalkErr error
	)

	for _, client := range clients {
		id, werr := walkToBundle(client.PID, walker, reader)
		if werr != nil {
			// Record the first transient failure but keep scanning: a flaky ps
			// on one client must not mask a resolvable local client.
			if firstWalkErr == nil {
				firstWalkErr = werr
			}
			continue
		}
		if id.IsNull() {
			// Remote/mosh client — no host-local terminal in its ancestry.
			continue
		}
		if !localFound || client.Activity > bestActivity {
			best = id
			bestActivity = client.Activity
			localFound = true
		}
	}

	if !localFound {
		if firstWalkErr != nil {
			// Nothing local resolved and a walk genuinely failed: surface the
			// transient error (already ErrDetectTransient-wrapped) so the caller
			// WARNs rather than silently reporting no terminal.
			return Identity{}, firstWalkErr
		}
		return Identity{}, nil
	}

	return best, nil
}
