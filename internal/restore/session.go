// Package restore implements skeleton-eager tmux session restoration from
// Portal's persisted sessions.json.
//
// The package recreates the structural topology (sessions, windows, panes) of
// each saved session in detached form. Each pane is created with the user's
// default shell, then armed via `respawn-pane -k` with `portal state hydrate`
// — a blocking helper that injects scrollback at attach time. respawn-pane -k
// is load-bearing: it kills the default shell and replaces the pane's process
// with the helper in a single atomic tmux call, preserving the spec's
// "helper as initial process" invariant. The create-then-arm split is
// required so FIFO paths and skeleton-marker keys can be derived from the
// live (window, pane) indices tmux assigned during creation rather than from
// any prediction, making restore robust to base-index / pane-base-index
// drift between save and restore.
//
// Layout, active-pane selection, zoom, and skeleton markers are applied by
// ApplyWindowGeometry and ApplySkeletonMarkers — exposed as separate methods
// so the orchestrator in restore.go can sequence them around the create-arm
// step. Both consume the live []tmux.PaneCoord threaded through from
// Restore, so all three operations (arm, geometry, markers) share one
// list-panes re-query and never disagree on live indices under base-index
// drift.
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

// SessionRestorer recreates a single saved tmux session in detached form. It
// runs in two phases: phase 1 creates the structural topology (sessions,
// windows, panes) with each pane initially running the user's default shell;
// phase 2 re-queries `list-panes` to discover the actual live (window, pane)
// indices tmux assigned, then arms each pane by creating its FIFO at the live
// paneKey and dispatching the `portal state hydrate` helper via
// `respawn-pane -k` (which kills the default shell atomically and replaces it
// with the helper).
//
// The two-phase split is mandated by the spec ("Index Semantics and base-index
// / pane-base-index"): live indices may differ from saved indices when the
// user changed `base-index` / `pane-base-index` between save and restore, so
// FIFO paths, skeleton markers, and `signal-hydrate` enumeration must all
// agree on the *live* paneKey rather than any prediction.
type SessionRestorer struct {
	Client   *tmux.Client
	StateDir string
	Logger   *state.Logger
}

// savedPaneArmInfo is the per-pane data retained from the create phase so the
// arm phase can build each pane's hydrate command without re-walking the saved
// session structure. `scrollAbs` is the absolute path to the saved scrollback
// file (saved-indexed, deliberately not live-indexed — see spec § Index
// Semantics). `hookKey` is the raw saved structural identifier preserved
// across base-index drift so hooks.json lookups stay addressable.
type savedPaneArmInfo struct {
	scrollAbs string
	hookKey   string
}

// Restore creates the session with all of its windows and panes in their
// saved order, then re-queries live tmux indices and arms each live pane with
// its hydrate helper. Environment is applied between new-session and the
// first new-window so subsequent panes inherit it at creation time.
//
// Per the spec's Index Semantics section, FIFO paths and respawn-pane targets
// are derived from the re-queried live (window, pane) tuples, not from
// predictions. This makes restoration robust to `base-index` /
// `pane-base-index` drift between save and restore: even if live indices
// differ from any prediction, the helper inside each pane reads the right
// FIFO path because both sides (signal-hydrate's enumeration and the helper's
// `--fifo` flag) are computed from the same live indices.
//
// Returns the []tmux.PaneCoord that armPanes gathered from list-panes so
// callers can thread it into ApplyWindowGeometry / ApplySkeletonMarkers and
// avoid duplicate list-panes round-trips plus prediction-based targeting for
// select-pane / select-layout / resize-pane.
func (r *SessionRestorer) Restore(sess state.Session) ([]tmux.PaneCoord, error) {
	if len(sess.Windows) == 0 || len(sess.Windows[0].Panes) == 0 {
		return nil, fmt.Errorf("session %q: no windows/panes", sess.Name)
	}

	armInfos := r.collectArmInfos(sess)

	if err := r.createSkeleton(sess); err != nil {
		return nil, err
	}

	return r.armPanes(sess, armInfos)
}

// collectArmInfos walks the saved topology in window-then-pane order, building
// one savedPaneArmInfo per pane. Output order matches the ordering used by
// `list-panes -s` (sorted by window then pane), so callers can index it
// linearly against the live re-query result.
func (r *SessionRestorer) collectArmInfos(sess state.Session) []savedPaneArmInfo {
	var infos []savedPaneArmInfo
	for _, w := range sess.Windows {
		for _, p := range w.Panes {
			infos = append(infos, savedPaneArmInfo{
				scrollAbs: filepath.Join(r.StateDir, p.ScrollbackFile),
				hookKey:   tmux.PaneTarget(sess.Name, w.Index, p.Index),
			})
		}
	}
	return infos
}

// createSkeleton runs the create phase: new-session for the root pane,
// applyEnvironment between session and first new-window, then split-window /
// new-window / split-window for every remaining pane. Panes are created with
// no initial command — they default to the user's shell — so that the arm
// phase can dispatch the hydrate helper via `respawn-pane -k` against live
// indices.
//
// Splits and new-windows target `<session>:` (the session's currently-active
// window). After new-session the first window is active; after each new-window
// the freshly-created window becomes active, so subsequent splits land in the
// correct window without any predicted index.
func (r *SessionRestorer) createSkeleton(sess state.Session) error {
	rootCWD := sess.Windows[0].Panes[0].CWD
	if err := r.Client.NewSessionWithCommand(sess.Name, rootCWD, ""); err != nil {
		return err
	}

	r.applyEnvironment(sess)

	target := fmt.Sprintf("%s:", sess.Name)

	// Window 0: any remaining panes via split-window against the active window.
	for pj := 1; pj < len(sess.Windows[0].Panes); pj++ {
		p := sess.Windows[0].Panes[pj]
		if err := r.Client.SplitWindow(target, p.CWD, ""); err != nil {
			return err
		}
	}

	// Subsequent windows: new-window for the first pane (becomes active), then
	// split-window for the rest into that now-active window.
	for wi := 1; wi < len(sess.Windows); wi++ {
		w := sess.Windows[wi]
		firstPane := w.Panes[0]
		if err := r.Client.NewWindow(target, w.Name, firstPane.CWD, ""); err != nil {
			return err
		}
		for pj := 1; pj < len(w.Panes); pj++ {
			p := w.Panes[pj]
			if err := r.Client.SplitWindow(target, p.CWD, ""); err != nil {
				return err
			}
		}
	}

	return nil
}

// armPanes runs the arm phase: re-query `list-panes` to discover live
// (window, pane) indices, then for each saved pane create the FIFO at the
// live paneKey and dispatch the hydrate command to the live pane via
// respawn-pane -k.
//
// respawn-pane (rather than send-keys) is load-bearing for the spec's "helper
// as initial process" invariant: it atomically kills the default shell that
// new-session / split-window created and replaces it with the hydrate helper
// in a single tmux call. Under send-keys the default shell would briefly run
// (rendering rc-file output and a prompt) before the helper took over,
// leaving artefacts in scrollback above the dumped saved scrollback. The -k
// flag also wipes any pre-helper output the default shell may have already
// written.
//
// list-panes returns coords sorted by (window, pane); collectArmInfos emits
// armInfos in the same saved-then-pane order, so the i-th armInfo pairs with
// the i-th live pane. This pairing assumes tmux preserved structural ordering
// during creation (every saved pane corresponds to exactly one live pane in
// the same relative position) — which is the case when restoration runs
// against an empty session and no concurrent process is creating panes.
//
// On a count mismatch (live != len(armInfos)) we log a warning and pair up to
// the shorter list. CreateFIFO failures are wrapped and abort restoration;
// RespawnPane failures are wrapped and aborted (the helper would never start,
// so there's no usable state to continue from). This is more aggressive than
// ApplySkeletonMarkers, which keeps going on per-pane errors — but a missing
// FIFO or unrespawned helper means the pane's saved scrollback will never be
// hydrated, so failing fast surfaces the problem to the operator.
//
// Returns the live []tmux.PaneCoord gathered from the re-query so callers can
// thread it into ApplyWindowGeometry / ApplySkeletonMarkers and avoid a
// duplicate list-panes round-trip plus prediction-based targeting.
func (r *SessionRestorer) armPanes(sess state.Session, armInfos []savedPaneArmInfo) ([]tmux.PaneCoord, error) {
	livePanes, err := r.Client.ListPanesInSession(sess.Name)
	if err != nil {
		return nil, fmt.Errorf("session %q: list live panes: %w", sess.Name, err)
	}

	if len(livePanes) != len(armInfos) {
		r.Logger.Warn(state.ComponentRestore, "session %q: live pane count %d != saved count %d (pairing up to shorter list)", sess.Name, len(livePanes), len(armInfos))
	}

	pairCount := len(armInfos)
	if len(livePanes) < pairCount {
		pairCount = len(livePanes)
	}

	for i := 0; i < pairCount; i++ {
		live := livePanes[i]
		info := armInfos[i]

		liveKey := state.SanitizePaneKey(sess.Name, live.Window, live.Pane)
		fifo := state.FIFOPath(r.StateDir, liveKey)
		if err := state.CreateFIFO(fifo); err != nil {
			return nil, fmt.Errorf("session %q: %w", sess.Name, err)
		}

		hydrateCmd := buildHydrateCommand(fifo, info.scrollAbs, info.hookKey)
		liveTarget := tmux.PaneTarget(sess.Name, live.Window, live.Pane)
		if err := r.Client.RespawnPane(liveTarget, hydrateCmd); err != nil {
			return nil, fmt.Errorf("session %q: arm pane %s: %w", sess.Name, liveTarget, err)
		}
	}

	return livePanes, nil
}

// ApplyWindowGeometry replays the saved layout, active-pane selection, and
// zoom state for every window in sess against the live tmux session of the
// same name. livePanes is the list of live (window, pane) coords that the arm
// phase already gathered from `list-panes -s` — sourcing geometry targets
// from this slice (rather than from PredictLiveIndices) keeps the create-arm
// path and the geometry path consistent under base-index drift, so a single
// re-query is the source of truth for every operation in the restore phase.
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
func (r *SessionRestorer) ApplyWindowGeometry(sess state.Session, livePanes []tmux.PaneCoord) {
	groups := groupLivePanesBySavedWindow(sess, livePanes)

	for wi, win := range sess.Windows {
		group := groups[wi]
		if len(group) == 0 {
			// No live pane mapped to this saved window — skip; logging at the
			// arm phase already surfaced the count mismatch.
			continue
		}
		liveWin := group[0].Window
		liveActivePane := group[activePanePosition(win.Panes)%len(group)].Pane

		r.applyLayoutWithFallback(sess.Name, liveWin, win.Layout)
		r.applyActivePane(sess.Name, liveWin, liveActivePane)
		if win.Zoomed {
			r.applyZoom(sess.Name, liveWin, liveActivePane)
		}
	}
}

// groupLivePanesBySavedWindow buckets livePanes into one slice per saved
// window ordinal, preserving structural order. flattenSavedPanePositions and
// list-panes both walk in (window, pane) sorted order, so the i-th saved pane
// pairs with the i-th livePane and saved window ordinals map onto live window
// groups by structural position.
//
// On count mismatch, extras (live panes beyond the saved sequence) are
// silently dropped — the arm-phase warning has already surfaced the mismatch
// and geometry is best-effort. Saved windows with no live coverage end up as
// empty slices, which the caller treats as "skip this saved window."
func groupLivePanesBySavedWindow(sess state.Session, livePanes []tmux.PaneCoord) [][]tmux.PaneCoord {
	out := make([][]tmux.PaneCoord, len(sess.Windows))
	cursor := 0
	for wi, w := range sess.Windows {
		end := cursor + len(w.Panes)
		if end > len(livePanes) {
			end = len(livePanes)
		}
		if cursor < end {
			out[wi] = livePanes[cursor:end]
		}
		cursor = end
	}
	return out
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

// ApplySkeletonMarkers sets the `@portal-skeleton-<paneKey>` server option on
// every live pane in livePanes (which the caller obtains from the arm phase's
// list-panes re-query, threaded through Restore). Markers use the **live**
// paneKey returned by tmux — sharing one re-query across arm, geometry, and
// markers keeps all three operations consistent under base-index drift.
//
// Behavior:
//   - On set-option failure for a single pane: logs a warning and continues
//     setting markers for the remaining panes.
//   - When live pane count differs from saved count: logs a sanity warning and
//     pairs by structural order up to the shorter list. Extra live panes are
//     still marked using their live paneKey.
//
// The function only writes markers; it does not clear them. Markers are
// volatile (server-option scope) and are unset by the hydrate helper after
// successful scrollback dump. It is a pure write primitive — drift diagnostics
// live in the orchestrator, alongside the prediction (PredictLiveIndices),
// rather than coupling a write primitive to a diagnostic concern.
func (r *SessionRestorer) ApplySkeletonMarkers(sess state.Session, livePanes []tmux.PaneCoord) {
	savedCount := countSavedPanes(sess)
	r.warnOnPaneCountMismatch(sess.Name, len(livePanes), savedCount)

	for _, live := range livePanes {
		liveKey := state.SanitizePaneKey(sess.Name, live.Window, live.Pane)
		r.setSkeletonMarker(sess.Name, liveKey)
	}
}

// countSavedPanes returns the total number of panes across every saved window.
func countSavedPanes(sess state.Session) int {
	n := 0
	for _, w := range sess.Windows {
		n += len(w.Panes)
	}
	return n
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
	if err := state.SetSkeletonMarker(r.Client, liveKey); err != nil {
		r.Logger.Warn(state.ComponentRestore, "set-option %s on %q: %v", state.SkeletonMarkerPrefix+liveKey, sessionName, err)
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
// As of the 8-6 signature simplification this is consulted **only** by the
// orchestrator's drift-warning helper (Orchestrator.warnOnPaneKeyDrift in
// restore.go). Restore arms panes from the live list-panes re-query (via
// armPanes), and ApplyWindowGeometry / ApplySkeletonMarkers consume the same
// live []tmux.PaneCoord threaded through from Restore — neither consumes the
// predicted indices. The drift warning compares the predicted paneKey to the
// actual live paneKey so users notice when base-index / pane-base-index
// changed between save and restore.
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

// buildHydrateCommand returns the spec-canonical `sh -c '...; exec $SHELL'`
// form delivered to a freshly-created pane via respawn-pane -k. respawn-pane
// kills the default shell and replaces the pane's process with this command
// in a single atomic call, so no leading `exec` prefix is needed (and would
// be redundant — tmux's respawn already replaces, not stacks).
//
// The trailing `exec $SHELL` inside sh -c lets the helper hand off to the
// user's shell without spawning a further process so the shell never sees
// the helper's command line and exiting the shell ends the pane (rather than
// dropping back into a parent sh).
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
