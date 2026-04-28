// Package restore implements skeleton-eager tmux session restoration from
// Portal's persisted sessions.json.
//
// The package recreates the structural topology (sessions, windows, panes) of
// each saved session in detached form, with each pane's initial process being
// `portal state hydrate` — a blocking helper that injects scrollback at attach
// time. Layout, active-pane selection, zoom, and skeleton markers are applied
// elsewhere in the bootstrap flow; this package owns the create-and-wire step
// only.
package restore

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// SessionRestorer recreates a single saved tmux session in detached form,
// pre-creating the per-pane hydration FIFO for each pane before the pane is
// born so the helper inside the pane can immediately block on read without a
// race window against signal-hydrate.
type SessionRestorer struct {
	Client   *tmux.Client
	StateDir string
	Logger   *state.Logger
}

// paneInfo bundles the per-pane fields the restoration sequence needs to keep
// in correspondence: liveWin (the predicted tmux window index used as the
// split-window target for subsequent panes in that window) and the derived
// hydrate command. Saved indices are baked into hydrateCmd via the hook key
// and are not retained separately.
type paneInfo struct {
	liveWin    int
	hydrateCmd string
}

// Restore creates the session with all of its windows and panes in their
// saved order. FIFOs are created up front so that no helper inside a pane can
// race signal-hydrate. Environment is applied between new-session and the
// first new-window so subsequent panes inherit it at creation time.
//
// baseIdx and paneBaseIdx are the predicted live tmux indices (typically from
// PredictLiveIndices); the orchestrator pre-computes them once per session and
// passes the same values to ApplyWindowGeometry and ApplySkeletonMarkers so
// all three operations agree on pane targets.
func (r *SessionRestorer) Restore(sess state.Session, baseIdx, paneBaseIdx int) error {
	if len(sess.Windows) == 0 || len(sess.Windows[0].Panes) == 0 {
		return fmt.Errorf("session %q: no windows/panes", sess.Name)
	}

	allPanes, err := r.buildPaneInfo(sess, baseIdx, paneBaseIdx)
	if err != nil {
		return err
	}

	first := allPanes[0]
	rootCWD := sess.Windows[0].Panes[0].CWD
	if err := r.Client.NewSessionWithCommand(sess.Name, rootCWD, first.hydrateCmd); err != nil {
		return err
	}

	r.applyEnvironment(sess)

	// Window 0: any remaining panes via split-window. Index into allPanes
	// follows saved-order traversal so paneIdx tracks the count consumed so far.
	paneIdx := 1
	win0 := sess.Windows[0]
	for pj := 1; pj < len(win0.Panes); pj++ {
		p := win0.Panes[pj]
		info := allPanes[paneIdx]
		target := fmt.Sprintf("%s:%d", sess.Name, first.liveWin)
		if err := r.Client.SplitWindow(target, p.CWD, info.hydrateCmd); err != nil {
			return err
		}
		paneIdx++
	}

	// Subsequent windows: new-window for the first pane, split-window for the rest.
	for wi := 1; wi < len(sess.Windows); wi++ {
		w := sess.Windows[wi]
		firstPane := w.Panes[0]
		windowInfo := allPanes[paneIdx]
		// Append-form target: trailing colon means "in this session".
		target := fmt.Sprintf("%s:", sess.Name)
		if err := r.Client.NewWindow(target, w.Name, firstPane.CWD, windowInfo.hydrateCmd); err != nil {
			return err
		}
		paneIdx++
		for pj := 1; pj < len(w.Panes); pj++ {
			p := w.Panes[pj]
			splitInfo := allPanes[paneIdx]
			splitTarget := fmt.Sprintf("%s:%d", sess.Name, windowInfo.liveWin)
			if err := r.Client.SplitWindow(splitTarget, p.CWD, splitInfo.hydrateCmd); err != nil {
				return err
			}
			paneIdx++
		}
	}

	return nil
}

// buildPaneInfo walks the saved topology in window-then-pane order, predicting
// the live (window, pane) index for each saved entry, pre-creating the
// hydration FIFO at the live paneKey, and assembling the hydrate command. The
// returned slice is in the same order as restoration's tmux call sequence so
// the caller can index it linearly.
func (r *SessionRestorer) buildPaneInfo(sess state.Session, baseIdx, paneBaseIdx int) ([]paneInfo, error) {
	var allPanes []paneInfo
	for wi, w := range sess.Windows {
		for pj, p := range w.Panes {
			liveWin := baseIdx + wi
			livePane := paneBaseIdx + pj
			paneKey := state.SanitizePaneKey(sess.Name, liveWin, livePane)
			fifo := state.FIFOPath(r.StateDir, paneKey)
			if err := state.CreateFIFO(fifo); err != nil {
				return nil, fmt.Errorf("session %q: %w", sess.Name, err)
			}
			scrollAbs := filepath.Join(r.StateDir, p.ScrollbackFile)
			hookKey := fmt.Sprintf("%s:%d.%d", sess.Name, w.Index, p.Index)
			allPanes = append(allPanes, paneInfo{
				liveWin:    liveWin,
				hydrateCmd: buildHydrateCommand(fifo, scrollAbs, hookKey),
			})
		}
	}
	return allPanes, nil
}

// ApplyWindowGeometry replays the saved layout, active-pane selection, and
// zoom state for every window in sess against the live tmux session of the
// same name. baseIdx and paneBaseIdx are the predicted live indices (from
// predictLiveIndices) used to translate saved structural positions to live
// (window, pane) targets.
//
// Per the spec's "Per-Window Restoration Order", the call sequence per window
// is select-layout → select-pane → resize-pane -Z; zoom is applied only when
// the saved zoomed flag was true and only after layout, since resize-pane -Z
// is a toggle whose effect depends on the freshly-applied geometry.
//
// Errors are best-effort: a select-layout failure falls back to "tiled" and
// continues; any other per-step failure is logged and the next step (or next
// window) proceeds. The function returns nothing because the broader restore
// flow degrades locally and continues per spec.
func (r *SessionRestorer) ApplyWindowGeometry(sess state.Session, baseIdx, paneBaseIdx int) {
	for wi, win := range sess.Windows {
		liveWin := baseIdx + wi
		liveActivePane := paneBaseIdx + activePanePosition(win.Panes)

		r.applyLayoutWithFallback(sess.Name, liveWin, win.Layout)
		r.applyActivePane(sess.Name, liveWin, liveActivePane)
		if win.Zoomed {
			r.applyZoom(sess.Name, liveWin, liveActivePane)
		}
	}
}

// activePanePosition returns the structural index of the first pane marked
// Active. If no pane is marked active, it returns 0 — matching the spec's
// "default to first pane" fallback.
func activePanePosition(panes []state.Pane) int {
	for i, p := range panes {
		if p.Active {
			return i
		}
	}
	return 0
}

// applyLayoutWithFallback attempts the saved layout first; on failure, logs
// a warning and tries "tiled". If tiled also fails, logs and proceeds — the
// caller continues with the remaining geometry steps regardless.
func (r *SessionRestorer) applyLayoutWithFallback(session string, window int, layout string) {
	err := r.Client.SelectLayout(session, window, layout)
	if err == nil {
		return
	}
	r.Logger.Warn(state.ComponentRestore, "select-layout %s:%d %q failed: %v; falling back to tiled", session, window, layout, err)
	if err := r.Client.SelectLayout(session, window, "tiled"); err != nil {
		r.Logger.Warn(state.ComponentRestore, "select-layout %s:%d tiled also failed: %v", session, window, err)
	}
}

// applyActivePane sets the active pane within a live window. Failure is
// logged and ignored.
func (r *SessionRestorer) applyActivePane(session string, window, pane int) {
	if err := r.Client.SelectPane(session, window, pane); err != nil {
		r.Logger.Warn(state.ComponentRestore, "select-pane %s:%d.%d failed: %v", session, window, pane, err)
	}
}

// applyZoom toggles zoom on the active pane after layout has been applied.
// Failure is logged and ignored.
func (r *SessionRestorer) applyZoom(session string, window, pane int) {
	if err := r.Client.ResizePaneZoom(session, window, pane); err != nil {
		r.Logger.Warn(state.ComponentRestore, "resize-pane -Z %s:%d.%d failed: %v", session, window, pane, err)
	}
}

// ApplySkeletonMarkers re-queries live panes for sess and sets the
// `@portal-skeleton-<paneKey>` server option on each one. Markers use the
// **live** paneKey returned by tmux, not the predicted indices, so any drift
// between the predicted base / pane-base values and the actual tmux indices
// surfaces in the markers themselves rather than silently corrupting the
// daemon's enumeration.
//
// predictedBase and predictedPaneBase are the values used by the create phase
// to produce predicted indices; they are passed in here purely so a drift
// warning can be logged when the predicted live paneKey for a given saved
// position differs from the actual live paneKey.
//
// Behavior:
//   - On list-panes failure: returns a wrapped error; no markers are set.
//   - On set-option failure for a single pane: logs a warning and continues
//     setting markers for the remaining panes.
//   - When live pane count differs from saved count: logs a sanity warning and
//     pairs by structural order up to the shorter list. Extra live panes are
//     still marked using their live paneKey.
//
// The function only writes markers; it does not clear them. Markers are
// volatile (server-option scope) and are unset by the hydrate helper after
// successful scrollback dump.
func (r *SessionRestorer) ApplySkeletonMarkers(sess state.Session, predictedBase, predictedPaneBase int) error {
	livePanes, err := r.Client.ListPanesInSession(sess.Name)
	if err != nil {
		return fmt.Errorf("apply skeleton markers for %q: %w", sess.Name, err)
	}

	savedSeq := flattenSavedPanePositions(sess)
	r.warnOnPaneCountMismatch(sess.Name, len(livePanes), len(savedSeq))

	pairCount := len(livePanes)
	if len(savedSeq) < pairCount {
		pairCount = len(savedSeq)
	}

	for i := 0; i < pairCount; i++ {
		live := livePanes[i]
		sv := savedSeq[i]

		liveKey := state.SanitizePaneKey(sess.Name, live.Window, live.Pane)
		predictedKey := state.SanitizePaneKey(sess.Name, predictedBase+sv.windowOrdinal, predictedPaneBase+sv.paneOrdinal)
		r.warnOnPaneKeyDrift(sess.Name, i, predictedKey, liveKey)

		r.setSkeletonMarker(sess.Name, liveKey)
	}

	// Extras: live panes beyond the saved sequence still get marked under
	// their live paneKey. Defensive — keeps the daemon from capturing them as
	// "user state" during the restore window.
	for i := pairCount; i < len(livePanes); i++ {
		live := livePanes[i]
		liveKey := state.SanitizePaneKey(sess.Name, live.Window, live.Pane)
		r.setSkeletonMarker(sess.Name, liveKey)
	}

	return nil
}

// savedPanePos is the structural ordinal pair (window position, pane position
// within that window) — used to compute the predicted live paneKey for a
// saved entry by adding base / pane-base offsets.
type savedPanePos struct {
	windowOrdinal int
	paneOrdinal   int
}

// flattenSavedPanePositions walks the session's windows in saved order,
// emitting one savedPanePos per pane. Output order matches restoration order
// so callers can pair structural index with live list-panes output one-to-one.
func flattenSavedPanePositions(sess state.Session) []savedPanePos {
	var out []savedPanePos
	for wi, w := range sess.Windows {
		for pj := range w.Panes {
			out = append(out, savedPanePos{windowOrdinal: wi, paneOrdinal: pj})
		}
	}
	return out
}

// warnOnPaneCountMismatch logs a sanity warning when the count of live panes
// differs from the saved pane count. Both signed: too few live panes hints at
// restoration incompletely; too many hints at user-created panes leaking in.
func (r *SessionRestorer) warnOnPaneCountMismatch(name string, liveCount, savedCount int) {
	if liveCount == savedCount {
		return
	}
	r.Logger.Warn(state.ComponentRestore, "session %q live pane count %d != saved count %d", name, liveCount, savedCount)
}

// warnOnPaneKeyDrift logs a warning when the predicted live paneKey for a
// saved position does not match the actual live paneKey. Drift is non-fatal —
// the marker still gets set under the live key — but worth surfacing so users
// notice that base-index / pane-base-index changed between save and restore.
func (r *SessionRestorer) warnOnPaneKeyDrift(name string, position int, predictedKey, liveKey string) {
	if predictedKey == liveKey {
		return
	}
	r.Logger.Warn(state.ComponentRestore, "session %q: pane %d predicted=%s live=%s", name, position, predictedKey, liveKey)
}

// setSkeletonMarker writes the `@portal-skeleton-<liveKey>` server option for
// the given live pane. Failures are logged and ignored so that one bad pane
// does not block markers for the rest.
func (r *SessionRestorer) setSkeletonMarker(sessionName, liveKey string) {
	markerName := state.SkeletonMarkerPrefix + liveKey
	if err := r.Client.SetServerOption(markerName, "1"); err != nil {
		r.Logger.Warn(state.ComponentRestore, "set-option %s on %q: %v", markerName, sessionName, err)
	}
}

// applyEnvironment sets every saved environment variable on the named session,
// in sorted-key order for deterministic call ordering. Per the spec, a single
// failure is logged and skipped — restoration must continue.
func (r *SessionRestorer) applyEnvironment(sess state.Session) {
	if len(sess.Environment) == 0 {
		return
	}
	keys := make([]string, 0, len(sess.Environment))
	for k := range sess.Environment {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := r.Client.SetSessionEnvironment(sess.Name, k, sess.Environment[k]); err != nil {
			r.Logger.Warn(state.ComponentRestore, "set-environment %s on %q: %v", k, sess.Name, err)
			// Continue per spec — environment is best-effort.
		}
	}
}

// PredictLiveIndices reads the server's base-index and pane-base-index. Both
// ErrOptionNotFound and an empty value are treated as "use default 0" — tmux's
// documented default when the user has not customised either option.
//
// The orchestrator calls this once per saved session and passes the result to
// Restore, ApplyWindowGeometry, and ApplySkeletonMarkers so all three operations
// share the same predicted indices and a single pair of show-option queries
// covers the entire restoration of one session.
func (r *SessionRestorer) PredictLiveIndices() (int, int) {
	return readIndexOption(r.Client, "base-index"), readIndexOption(r.Client, "pane-base-index")
}

// readIndexOption returns the parsed integer value of name, defaulting to 0
// when the option is unset (ErrOptionNotFound), empty, or unparseable. Each
// failure mode collapses to the same fallback because tmux's documented
// default for both base-index and pane-base-index is 0.
func readIndexOption(client *tmux.Client, name string) int {
	v, err := client.GetServerOption(name)
	if err != nil || v == "" {
		return 0
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return i
}

// buildHydrateCommand returns the exact `sh -c '...'` form that becomes the
// pane's initial process. The trailing `exec $SHELL` lets the helper hand off
// to the user's shell without spawning a new process so the shell never sees
// the helper's command line.
func buildHydrateCommand(fifo, file, hookKey string) string {
	return fmt.Sprintf(
		"sh -c 'portal state hydrate --fifo %s --file %s --hook-key %s; exec $SHELL'",
		shellQuoteSingle(fifo),
		shellQuoteSingle(file),
		shellQuoteSingle(hookKey),
	)
}

// shellQuoteSingle escapes embedded single quotes for inclusion inside an
// outer single-quoted shell string. Each ' becomes '\” (close, escaped quote,
// reopen). Defensive against pathological session names.
func shellQuoteSingle(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}
