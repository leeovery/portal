package restore_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// activePane returns a pane with the given index marked active.
func activePane(idx int) state.Pane {
	return state.Pane{Index: idx, Active: true}
}

// inactivePane returns a pane with the given index, not active.
func inactivePane(idx int) state.Pane {
	return state.Pane{Index: idx}
}

// liveCoordsFromSaved synthesises a []tmux.PaneCoord matching the structural
// shape of sess, with each window mapped to live window `baseIdx + wi` and
// each pane to live pane `paneBaseIdx + pj`. Used by ApplyWindowGeometry tests
// that previously took (baseIdx, paneBaseIdx) directly — the new signature
// consumes the live PaneCoord slice that armPanes gathers from list-panes,
// so tests construct the equivalent slice up-front.
func liveCoordsFromSaved(sess state.Session, baseIdx, paneBaseIdx int) []tmux.PaneCoord {
	var out []tmux.PaneCoord
	for wi, w := range sess.Windows {
		for pj := range w.Panes {
			out = append(out, tmux.PaneCoord{Window: baseIdx + wi, Pane: paneBaseIdx + pj})
		}
	}
	return out
}

// geometrySession builds a minimal Session shell whose windows have the given
// layout/zoomed state and pane lists. Pane CWD/scrollback are unused by
// ApplyWindowGeometry — only Active flags and structural ordering matter.
func geometrySession(name string, windows ...state.Window) state.Session {
	return state.Session{Name: name, Windows: windows}
}

// geomWindow builds a state.Window with the given layout, zoom, and panes.
func geomWindow(idx int, layout string, zoomed bool, panes ...state.Pane) state.Window {
	return state.Window{Index: idx, Layout: layout, Zoomed: zoomed, Panes: panes}
}

// findCallTarget returns the first call index whose args[0]==cmd and
// args[2]==target (i.e. tmux "<cmd> -t <target> ..."). Returns -1 if absent.
func findCallTarget(calls [][]string, cmd, target string) int {
	for i, c := range calls {
		if len(c) >= 3 && c[0] == cmd && c[2] == target {
			return i
		}
	}
	return -1
}

// findSelectLayoutTarget returns the first select-layout call index whose
// target matches AND whose layout argument equals wantLayout. Useful for
// disambiguating "<saved>" vs "tiled" calls against the same target.
func findSelectLayoutTarget(calls [][]string, target, wantLayout string) int {
	for i, c := range calls {
		if len(c) >= 4 && c[0] == "select-layout" && c[2] == target && c[3] == wantLayout {
			return i
		}
	}
	return -1
}

// findResizePaneZoom returns the first index of "resize-pane -Z -t <target>"
// in calls, or -1 if absent. Centralised because zoom is asserted in many
// tests with the same shape.
func findResizePaneZoom(calls [][]string, target string) int {
	for i, c := range calls {
		if len(c) >= 4 && c[0] == "resize-pane" && c[1] == "-Z" && c[3] == target {
			return i
		}
	}
	return -1
}

func TestApplyWindowGeometry_AppliesSavedLayoutForEveryWindow(t *testing.T) {
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := geometrySession("work",
		geomWindow(0, "layoutA", false, activePane(0)),
		geomWindow(1, "layoutB", false, activePane(0)),
		geomWindow(2, "layoutC", false, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	for i, wantLayout := range []string{"layoutA", "layoutB", "layoutC"} {
		target := targetWin("work", i)
		if findSelectLayoutTarget(mock.Calls, target, wantLayout) < 0 {
			t.Errorf("expected select-layout %s %q, got calls: %v", target, wantLayout, mock.Calls)
		}
	}
}

func TestApplyWindowGeometry_SelectsLivePaneIndexForActivePane(t *testing.T) {
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	// Window has 3 panes; second one (structural position 1) is active.
	sess := geometrySession("work",
		geomWindow(0, "L", false,
			inactivePane(0),
			activePane(1),
			inactivePane(2),
		),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	wantTarget := "work:0.1"
	if findCallTarget(mock.Calls, "select-pane", wantTarget) < 0 {
		t.Errorf("expected select-pane -t %s, got calls: %v", wantTarget, mock.Calls)
	}
}

func TestApplyWindowGeometry_AppliesZoomAfterLayoutWhenZoomedTrue(t *testing.T) {
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := geometrySession("work",
		geomWindow(0, "L", true, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	layoutIdx := findCallTarget(mock.Calls, "select-layout", "work:0")
	zoomIdx := findResizePaneZoom(mock.Calls, "work:0.0")
	if layoutIdx < 0 {
		t.Fatalf("no select-layout call; calls: %v", mock.Calls)
	}
	if zoomIdx < 0 {
		t.Fatalf("no resize-pane -Z call; calls: %v", mock.Calls)
	}
	if layoutIdx >= zoomIdx {
		t.Errorf("layout(%d) must precede zoom(%d); calls: %v", layoutIdx, zoomIdx, mock.Calls)
	}
}

func TestApplyWindowGeometry_SkipsZoomWhenZoomedFalse(t *testing.T) {
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := geometrySession("work",
		geomWindow(0, "L", false, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	for _, c := range mock.Calls {
		if len(c) >= 2 && c[0] == "resize-pane" && c[1] == "-Z" {
			t.Errorf("did not expect resize-pane -Z when zoomed=false; got %v", c)
		}
	}
}

func TestApplyWindowGeometry_FallsBackToTiledWhenSavedLayoutFails(t *testing.T) {
	mock := &mockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) >= 4 && args[0] == "select-layout" && args[3] == "broken" {
				return "", errors.New("layout parse failed")
			}
			return "", nil
		},
	}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := geometrySession("work",
		geomWindow(0, "broken", false, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	if findSelectLayoutTarget(mock.Calls, "work:0", "broken") < 0 {
		t.Errorf("expected attempted select-layout work:0 broken; calls: %v", mock.Calls)
	}
	if findSelectLayoutTarget(mock.Calls, "work:0", "tiled") < 0 {
		t.Errorf("expected fallback select-layout work:0 tiled; calls: %v", mock.Calls)
	}
}

func TestApplyWindowGeometry_LogsAndContinuesWhenTiledFallbackAlsoFails(t *testing.T) {
	mock := &mockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "select-layout" {
				return "", errors.New("layout failed")
			}
			return "", nil
		},
	}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "portal.log")
	logger, err := state.OpenLogger(logPath, false)
	if err != nil {
		t.Fatalf("open logger: %v", err)
	}
	defer func() { _ = logger.Close() }()
	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := geometrySession("work",
		geomWindow(0, "broken", true, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	// Both attempts must have happened.
	if findSelectLayoutTarget(mock.Calls, "work:0", "broken") < 0 {
		t.Errorf("expected attempted select-layout work:0 broken; calls: %v", mock.Calls)
	}
	if findSelectLayoutTarget(mock.Calls, "work:0", "tiled") < 0 {
		t.Errorf("expected attempted fallback select-layout work:0 tiled; calls: %v", mock.Calls)
	}

	// Subsequent steps must still proceed.
	if findCallTarget(mock.Calls, "select-pane", "work:0.0") < 0 {
		t.Errorf("expected select-pane to still run after layout failure; calls: %v", mock.Calls)
	}
	if findResizePaneZoom(mock.Calls, "work:0.0") < 0 {
		t.Errorf("expected resize-pane -Z to still run after layout failure; calls: %v", mock.Calls)
	}

	// Verify two warn entries land in the log: one for the saved-layout
	// failure (mentions "falling back to tiled"), one for the tiled fallback
	// failure.
	_ = logger.Close() // flush
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "falling back to tiled") {
		t.Errorf("log %q lacks first warning about saved-layout failure", body)
	}
	if !strings.Contains(body, "tiled also failed") {
		t.Errorf("log %q lacks second warning about tiled fallback failure", body)
	}
}

func TestApplyWindowGeometry_DefaultsToStructuralPositionZeroWhenNoPaneActive(t *testing.T) {
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	// No pane has Active=true → default to position 0.
	sess := geometrySession("work",
		geomWindow(0, "L", true,
			inactivePane(0),
			inactivePane(1),
			inactivePane(2),
		),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	if findCallTarget(mock.Calls, "select-pane", "work:0.0") < 0 {
		t.Errorf("expected select-pane -t work:0.0 (structural position 0 default); calls: %v", mock.Calls)
	}
	if findResizePaneZoom(mock.Calls, "work:0.0") < 0 {
		t.Errorf("expected resize-pane -Z -t work:0.0 (structural position 0 default); calls: %v", mock.Calls)
	}
}

func TestApplyWindowGeometry_OrdersLayoutThenPaneThenZoom(t *testing.T) {
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := geometrySession("work",
		geomWindow(0, "L", true, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	layoutIdx := findCallTarget(mock.Calls, "select-layout", "work:0")
	paneIdx := findCallTarget(mock.Calls, "select-pane", "work:0.0")
	zoomIdx := findResizePaneZoom(mock.Calls, "work:0.0")
	if layoutIdx < 0 || paneIdx < 0 || zoomIdx < 0 {
		t.Fatalf("missing call(s): layout=%d pane=%d zoom=%d; calls: %v", layoutIdx, paneIdx, zoomIdx, mock.Calls)
	}
	if layoutIdx >= paneIdx || paneIdx >= zoomIdx {
		t.Errorf("ordering violated: layout(%d) < pane(%d) < zoom(%d) failed", layoutIdx, paneIdx, zoomIdx)
	}
}

func TestApplyWindowGeometry_SinglePaneWindowSelectsThatPane(t *testing.T) {
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := geometrySession("work",
		geomWindow(0, "L", false, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	// Exactly one select-pane call, on work:0.0.
	count := 0
	for _, c := range mock.Calls {
		if len(c) > 0 && c[0] == "select-pane" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("select-pane calls = %d, want 1", count)
	}
	if findCallTarget(mock.Calls, "select-pane", "work:0.0") < 0 {
		t.Errorf("expected select-pane work:0.0; calls: %v", mock.Calls)
	}
}

func TestApplyWindowGeometry_UsesLiveIndicesFromBaseAndPaneBase(t *testing.T) {
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	// baseIdx=1, paneBaseIdx=1 → window 0 maps to live window 1, active pane
	// at structural position 0 maps to live pane 1.
	sess := geometrySession("work",
		geomWindow(0, "L", true, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 1, 1))

	if findSelectLayoutTarget(mock.Calls, "work:1", "L") < 0 {
		t.Errorf("expected select-layout work:1 L; calls: %v", mock.Calls)
	}
	if findCallTarget(mock.Calls, "select-pane", "work:1.1") < 0 {
		t.Errorf("expected select-pane work:1.1; calls: %v", mock.Calls)
	}
	if findResizePaneZoom(mock.Calls, "work:1.1") < 0 {
		t.Errorf("expected resize-pane -Z work:1.1; calls: %v", mock.Calls)
	}
}

func TestApplyWindowGeometry_ContinuesRemainingWindowsWhenOneFails(t *testing.T) {
	mock := &mockCommander{
		RunFunc: func(args ...string) (string, error) {
			// Both saved-layout AND tiled fail for window 0 (work:0).
			if len(args) >= 4 && args[0] == "select-layout" && args[2] == "work:0" {
				return "", errors.New("window 0 layout failed")
			}
			return "", nil
		},
	}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := geometrySession("work",
		geomWindow(0, "L0", true, activePane(0)),
		geomWindow(1, "L1", true, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	// Window 1 should receive all three calls despite window 0 failing.
	if findSelectLayoutTarget(mock.Calls, "work:1", "L1") < 0 {
		t.Errorf("expected select-layout work:1 L1; calls: %v", mock.Calls)
	}
	if findCallTarget(mock.Calls, "select-pane", "work:1.0") < 0 {
		t.Errorf("expected select-pane work:1.0; calls: %v", mock.Calls)
	}
	if findResizePaneZoom(mock.Calls, "work:1.0") < 0 {
		t.Errorf("expected resize-pane -Z work:1.0; calls: %v", mock.Calls)
	}
}

func TestApplyWindowGeometry_FirstActivePaneWins(t *testing.T) {
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	// Two panes marked active — first one wins.
	sess := geometrySession("work",
		geomWindow(0, "L", false,
			inactivePane(0),
			activePane(1),
			activePane(2),
		),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	if findCallTarget(mock.Calls, "select-pane", "work:0.1") < 0 {
		t.Errorf("expected select-pane work:0.1 (first active wins); calls: %v", mock.Calls)
	}
	if findCallTarget(mock.Calls, "select-pane", "work:0.2") >= 0 {
		t.Errorf("did not expect select-pane work:0.2 when earlier pane also active; calls: %v", mock.Calls)
	}
}

// targetWin formats a "session:window" target string. Keeps assertion sites
// readable by hiding the fmt boilerplate.
func targetWin(name string, idx int) string {
	return fmt.Sprintf("%s:%d", name, idx)
}
