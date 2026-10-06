package restore

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// Orchestrator restores every saved session not already live. Per-session
// failures are logged and isolated, and nothing is written to stderr.
type Orchestrator struct {
	Client   *tmux.Client
	StateDir string
	Logger   *slog.Logger

	// Progress is optional; m is the saved-session count, and it fires on every
	// loop iteration, skips included, so a caller's counter always reaches m/m.
	Progress func(n, m int)

	// Exe is optional; nil resolves through os.Executable.
	Exe ExecutableResolver

	// MintToken is optional; nil mints through nanoid.NewPaneTokenGenerator.
	MintToken nanoid.Generator
}

// Restore returns (false, nil) on the happy path and after isolating a
// per-session failure, and (true, err) wrapping state.ErrCorruptIndex when
// sessions.json exists but is unparseable.
func (o *Orchestrator) Restore() (bool, error) {
	idx, skip, err := state.ReadIndex(o.StateDir)
	if skip {
		return o.handleReadIndexSkip(err)
	}

	if len(idx.Sessions) == 0 {
		return false, nil
	}

	liveSet, ok := o.snapshotLiveSessions()
	if !ok {
		return false, nil
	}

	sr := o.newSessionRestorer()

	start := time.Now()
	m := len(idx.Sessions)
	var restoredSessions, restoredWindows, restoredPanes int
	for i, sess := range idx.Sessions {
		// Fires before restoreOne so n advances regardless of the outcome.
		if o.Progress != nil {
			o.Progress(i+1, m)
		}
		if !o.restoreOne(sr, sess, liveSet) {
			continue
		}
		restoredSessions++
		restoredWindows += len(sess.Windows)
		for _, w := range sess.Windows {
			restoredPanes += len(w.Panes)
		}
	}
	o.recordMintedTokens(sr)
	o.logger().Info("skeleton complete",
		"sessions", restoredSessions,
		"windows", restoredWindows,
		"panes", restoredPanes,
		log.Took(start),
	)
	return false, nil
}

// recordMintedTokens ties each token restore minted to its saved record before
// any commit can run, so the first commit after restore finds every restored
// pane's record by its token wherever restore placed the pane. A failure leaves
// those panes judged at their live addresses; it does not undo the restore.
func (o *Orchestrator) recordMintedTokens(sr *SessionRestorer) {
	if err := state.RecordRestoredPaneTokens(o.StateDir, sr.minted); err != nil {
		o.logger().Warn("record restored pane tokens failed", "error", err)
	}
}

func (o *Orchestrator) newSessionRestorer() *SessionRestorer {
	return &SessionRestorer{
		Client:    o.Client,
		StateDir:  o.StateDir,
		Logger:    o.Logger,
		Exe:       o.Exe,
		MintToken: o.MintToken,
	}
}

// The final branch is defensive: ReadIndex is contracted to wrap every non-nil
// error with ErrCorruptIndex.
func (o *Orchestrator) handleReadIndexSkip(err error) (bool, error) {
	if err == nil {
		return false, nil
	}
	o.logger().Warn("ReadIndex failed", "error", err)
	if errors.Is(err, state.ErrCorruptIndex) {
		return true, fmt.Errorf("restore: %w", err)
	}
	return false, nil
}

func (o *Orchestrator) snapshotLiveSessions() (map[string]struct{}, bool) {
	names, err := o.Client.ListSessionNames()
	if err != nil {
		o.logger().Warn("list-sessions failed", "error", err)
		return nil, false
	}
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return set, true
}

// restoreOne reports true only when the session was actually skeleton-restored.
func (o *Orchestrator) restoreOne(sr *SessionRestorer, sess state.Session, liveSet map[string]struct{}) bool {
	if strings.HasPrefix(sess.Name, "_") {
		o.logger().Warn("skipping underscore-prefixed session", "session", sess.Name)
		return false
	}

	if _, alive := liveSet[sess.Name]; alive {
		return false
	}

	if !o.validateTopology(sess) {
		return false
	}

	livePanes, err := sr.Restore(sess)
	if err != nil {
		o.logger().Warn("restore session failed", "session", sess.Name, "error", err)
		return false
	}

	sr.ApplyWindowGeometry(sess, livePanes)
	sr.ApplySkeletonMarkers(sess, livePanes)
	return true
}

func (o *Orchestrator) validateTopology(sess state.Session) bool {
	if len(sess.Windows) == 0 {
		o.logger().Warn("session has zero windows; skipping", "session", sess.Name)
		return false
	}
	for _, w := range sess.Windows {
		if len(w.Panes) == 0 {
			// No attr key exists for the offending window index, and a message
			// must not interpolate values, so only the session is named.
			o.logger().Warn("session window has zero panes; skipping session", "session", sess.Name)
			return false
		}
	}
	return true
}
