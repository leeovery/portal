package restore

import (
	"errors"
	"fmt"
	"strings"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// Orchestrator is the bootstrap-time entry point for skeleton-only session
// restoration. It reads sessions.json, lists currently-live tmux sessions,
// and runs the create + geometry + skeleton-marker sequence for each saved
// session whose name is not already live. Per-session failures are logged
// and isolated; one bad session never aborts the rest.
//
// Restore returns (false, nil) on the happy path and after isolating any
// per-session failure (logged + swallowed). The one error path is corrupt
// sessions.json: Restore returns (true, err) where err wraps
// state.ErrCorruptIndex so the bootstrap orchestrator's typed branch on
// the corrupt bool surfaces a CorruptSessionsJSONWarning. The contract is
// pinned in cmd/bootstrap.Restorer; this implementation honours it.
//
// All stderr emission was moved to cmd/bootstrap_warnings.go in Phase 6
// task 6-9; this package now only returns and logs. The PersistentPreRunE
// caller wraps this with the @portal-restoring marker (see spec, Bootstrap
// Flow step 6).
type Orchestrator struct {
	Client   *tmux.Client
	StateDir string
	Logger   *state.Logger
}

// Restore is the bootstrap entry point. Returns (false, nil) on the happy
// path and after isolating any per-session failure (logged + swallowed).
// Returns (true, err) wrapping state.ErrCorruptIndex when sessions.json
// exists but is unparseable so the bootstrap orchestrator can classify
// the failure as soft and emit a CorruptSessionsJSONWarning. See the
// cmd/bootstrap.Restorer interface for the typed contract and the spec's
// Bootstrap Flow §5 for the full behaviour.
func (o *Orchestrator) Restore() (bool, error) {
	idx, skip, err := state.ReadIndex(o.StateDir)
	if skip {
		return o.handleReadIndexSkip(err)
	}

	if len(idx.Sessions) == 0 {
		return false, nil // nothing to restore
	}

	liveSet, ok := o.snapshotLiveSessions()
	if !ok {
		return false, nil
	}

	sr := &SessionRestorer{
		Client:   o.Client,
		StateDir: o.StateDir,
		Logger:   o.Logger,
	}
	for _, sess := range idx.Sessions {
		o.restoreOne(sr, sess, liveSet)
	}
	return false, nil
}

// handleReadIndexSkip classifies ReadIndex's skip-with-error path. A clean
// "no sessions.json file" skip carries err == nil and produces no output
// or error. Any "exists but unusable" skip (corrupt content, unreadable
// file via permission denial) is wrapped with state.ErrCorruptIndex by
// ReadIndex, logged WARN, and returned to the caller as (true, wrapped)
// so the bootstrap orchestrator can append a CorruptSessionsJSONWarning.
// The fallthrough branch is defensive — ReadIndex is contracted to wrap
// every non-nil error with ErrCorruptIndex.
func (o *Orchestrator) handleReadIndexSkip(err error) (bool, error) {
	if err == nil {
		return false, nil
	}
	o.Logger.Warn(state.ComponentRestore, "ReadIndex: %v", err)
	if errors.Is(err, state.ErrCorruptIndex) {
		return true, fmt.Errorf("restore: %w", err)
	}
	return false, nil
}

// snapshotLiveSessions queries tmux for the set of currently-live session
// names. Returns (set, true) on success or (nil, false) when list-sessions
// fails — the caller treats false as "abort restoration silently after
// logging" per the spec's degrade-locally-and-continue principle.
func (o *Orchestrator) snapshotLiveSessions() (map[string]struct{}, bool) {
	names, err := o.Client.ListSessionNames()
	if err != nil {
		o.Logger.Warn(state.ComponentRestore, "list-sessions: %v", err)
		return nil, false
	}
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return set, true
}

// restoreOne handles the per-session decision tree: skip live sessions
// silently, skip Portal-internal underscore-prefixed names with a log entry,
// reject malformed topologies (zero windows / zero panes), then dispatch to
// the SessionRestorer's create / geometry / markers sequence with all three
// operations sharing the same live []tmux.PaneCoord that the arm phase
// gathered from list-panes.
func (o *Orchestrator) restoreOne(sr *SessionRestorer, sess state.Session, liveSet map[string]struct{}) {
	if strings.HasPrefix(sess.Name, "_") {
		o.Logger.Warn(state.ComponentRestore, "skipping underscore-prefixed session %q", sess.Name)
		return
	}

	if _, alive := liveSet[sess.Name]; alive {
		// Silent skip per spec — the steady-state common case.
		return
	}

	if !o.validateTopology(sess) {
		return
	}

	livePanes, err := sr.Restore(sess)
	if err != nil {
		o.Logger.Warn(state.ComponentRestore, "Restore %q: %v", sess.Name, err)
		return
	}

	sr.ApplyWindowGeometry(sess, livePanes)
	sr.ApplySkeletonMarkers(sess, livePanes)
}

// validateTopology rejects sessions that cannot be skeleton-restored: zero
// windows, or any window with zero panes. Each rejection is logged and the
// caller treats it as "skip this session." Returns true when the topology
// is well-formed enough to attempt restoration.
func (o *Orchestrator) validateTopology(sess state.Session) bool {
	if len(sess.Windows) == 0 {
		o.Logger.Warn(state.ComponentRestore, "session %q has zero windows; skipping", sess.Name)
		return false
	}
	for _, w := range sess.Windows {
		if len(w.Panes) == 0 {
			o.Logger.Warn(state.ComponentRestore, "session %q window %d has zero panes; skipping session", sess.Name, w.Index)
			return false
		}
	}
	return true
}
