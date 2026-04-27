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
func (r *SessionRestorer) Restore(sess state.Session) error {
	if len(sess.Windows) == 0 || len(sess.Windows[0].Panes) == 0 {
		return fmt.Errorf("session %q: no windows/panes", sess.Name)
	}

	baseIdx, paneBaseIdx := r.predictLiveIndices()

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
	if r.Logger != nil {
		r.Logger.Warn("restore", "select-layout %s:%d %q failed: %v; falling back to tiled", session, window, layout, err)
	}
	if err := r.Client.SelectLayout(session, window, "tiled"); err != nil && r.Logger != nil {
		r.Logger.Warn("restore", "select-layout %s:%d tiled also failed: %v", session, window, err)
	}
}

// applyActivePane sets the active pane within a live window. Failure is
// logged and ignored.
func (r *SessionRestorer) applyActivePane(session string, window, pane int) {
	if err := r.Client.SelectPane(session, window, pane); err != nil && r.Logger != nil {
		r.Logger.Warn("restore", "select-pane %s:%d.%d failed: %v", session, window, pane, err)
	}
}

// applyZoom toggles zoom on the active pane after layout has been applied.
// Failure is logged and ignored.
func (r *SessionRestorer) applyZoom(session string, window, pane int) {
	if err := r.Client.ResizePaneZoom(session, window, pane); err != nil && r.Logger != nil {
		r.Logger.Warn("restore", "resize-pane -Z %s:%d.%d failed: %v", session, window, pane, err)
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
			if r.Logger != nil {
				r.Logger.Warn("restore", "set-environment %s on %q: %v", k, sess.Name, err)
			}
			// Continue per spec — environment is best-effort.
		}
	}
}

// predictLiveIndices reads the server's base-index and pane-base-index. Both
// ErrOptionNotFound and an empty value are treated as "use default 0" — tmux's
// documented default when the user has not customised either option.
func (r *SessionRestorer) predictLiveIndices() (int, int) {
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
