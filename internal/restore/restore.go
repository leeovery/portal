package restore

import (
	"io"
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
// Restore always returns nil. Hard fatals (corrupt sessions.json) emit a
// single user-facing line to Stderr and continue bootstrap; per-session
// errors only land in the structured log. The PersistentPreRunE caller wraps
// this with the @portal-restoring marker (see spec, Bootstrap Flow step 5).
type Orchestrator struct {
	Client   *tmux.Client
	StateDir string
	Logger   *state.Logger
	Stderr   io.Writer
}

// corruptStateMessage is the user-facing one-liner emitted to Stderr when
// sessions.json is unparseable. Its exact wording matches the Observability
// section of the specification.
const corruptStateMessage = "Portal state file is corrupt — restoration skipped.\n" +
	"Check `portal state status` or ~/.config/portal/state/portal.log.\n"

// Restore is the bootstrap entry point. Always returns nil; per-session
// failures are logged and the next session is attempted. See the spec's
// Bootstrap Flow §5 for the full contract.
func (o *Orchestrator) Restore() error {
	idx, skip, err := state.ReadIndex(o.StateDir)
	if skip {
		o.handleReadIndexSkip(err)
		return nil
	}

	if len(idx.Sessions) == 0 {
		return nil // nothing to restore
	}

	liveSet, ok := o.snapshotLiveSessions()
	if !ok {
		return nil
	}

	sr := &SessionRestorer{
		Client:   o.Client,
		StateDir: o.StateDir,
		Logger:   o.Logger,
	}
	for _, sess := range idx.Sessions {
		o.restoreOne(sr, sess, liveSet)
	}
	return nil
}

// handleReadIndexSkip surfaces ReadIndex's skip-with-error path (corrupt or
// unreadable sessions.json) to both the structured log and Stderr. A clean
// "no sessions.json file" skip carries err == nil and produces no output.
func (o *Orchestrator) handleReadIndexSkip(err error) {
	if err == nil {
		return
	}
	if o.Logger != nil {
		o.Logger.Warn(state.ComponentRestore, "ReadIndex: %v", err)
	}
	if o.Stderr != nil {
		_, _ = io.WriteString(o.Stderr, corruptStateMessage)
	}
}

// snapshotLiveSessions queries tmux for the set of currently-live session
// names. Returns (set, true) on success or (nil, false) when list-sessions
// fails — the caller treats false as "abort restoration silently after
// logging" per the spec's degrade-locally-and-continue principle.
func (o *Orchestrator) snapshotLiveSessions() (map[string]struct{}, bool) {
	names, err := o.Client.ListSessionNames()
	if err != nil {
		if o.Logger != nil {
			o.Logger.Warn(state.ComponentRestore, "list-sessions: %v", err)
		}
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
// operations sharing the same predicted live indices.
func (o *Orchestrator) restoreOne(sr *SessionRestorer, sess state.Session, liveSet map[string]struct{}) {
	if strings.HasPrefix(sess.Name, "_") {
		if o.Logger != nil {
			o.Logger.Warn(state.ComponentRestore, "skipping underscore-prefixed session %q", sess.Name)
		}
		return
	}

	if _, alive := liveSet[sess.Name]; alive {
		// Silent skip per spec — the steady-state common case.
		return
	}

	if !o.validateTopology(sess) {
		return
	}

	baseIdx, paneBaseIdx := sr.PredictLiveIndices()

	if err := sr.Restore(sess, baseIdx, paneBaseIdx); err != nil {
		if o.Logger != nil {
			o.Logger.Warn(state.ComponentRestore, "Restore %q: %v", sess.Name, err)
		}
		return
	}

	sr.ApplyWindowGeometry(sess, baseIdx, paneBaseIdx)

	if err := sr.ApplySkeletonMarkers(sess, baseIdx, paneBaseIdx); err != nil {
		if o.Logger != nil {
			o.Logger.Warn(state.ComponentRestore, "ApplySkeletonMarkers %q: %v", sess.Name, err)
		}
	}
}

// validateTopology rejects sessions that cannot be skeleton-restored: zero
// windows, or any window with zero panes. Each rejection is logged and the
// caller treats it as "skip this session." Returns true when the topology
// is well-formed enough to attempt restoration.
func (o *Orchestrator) validateTopology(sess state.Session) bool {
	if len(sess.Windows) == 0 {
		if o.Logger != nil {
			o.Logger.Warn(state.ComponentRestore, "session %q has zero windows; skipping", sess.Name)
		}
		return false
	}
	for _, w := range sess.Windows {
		if len(w.Panes) == 0 {
			if o.Logger != nil {
				o.Logger.Warn(state.ComponentRestore, "session %q window %d has zero panes; skipping session", sess.Name, w.Index)
			}
			return false
		}
	}
	return true
}
