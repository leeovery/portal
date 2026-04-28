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

// markersSession builds a minimal Session shell with the given saved windows
// and panes. Only structural counts and indices matter for skeleton-marker
// tests — CWD/scrollback fields are unused.
func markersSession(name string, windows ...state.Window) state.Session {
	return state.Session{Name: name, Windows: windows}
}

func markersWindow(idx int, panes ...state.Pane) state.Window {
	return state.Window{Index: idx, Panes: panes}
}

func markersPane(idx int) state.Pane {
	return state.Pane{Index: idx}
}

// parseLivePanes parses a "<window>:<pane>\n…" string (the same format
// list-panes -F '#{window_index}:#{pane_index}' emits) into a sorted
// []tmux.PaneCoord. Used by markers/geometry tests to build the slice that
// armPanes would have produced from the live re-query.
func parseLivePanes(t *testing.T, output string) []tmux.PaneCoord {
	t.Helper()
	if output == "" {
		return nil
	}
	var out []tmux.PaneCoord
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			t.Fatalf("parseLivePanes: bad line %q", line)
		}
		var w, p int
		if _, err := fmt.Sscanf(parts[0], "%d", &w); err != nil {
			t.Fatalf("parseLivePanes: bad window %q: %v", parts[0], err)
		}
		if _, err := fmt.Sscanf(parts[1], "%d", &p); err != nil {
			t.Fatalf("parseLivePanes: bad pane %q: %v", parts[1], err)
		}
		out = append(out, tmux.PaneCoord{Window: w, Pane: p})
	}
	return out
}

// findSetOptionMarker returns the index of the first set-option call whose
// option name matches markerName. Expects argv shape [set-option, -s, name, value].
func findSetOptionMarker(calls [][]string, markerName string) int {
	for i, c := range calls {
		if len(c) >= 4 && c[0] == "set-option" && c[2] == markerName {
			return i
		}
	}
	return -1
}

// allSetOptionCalls returns the indices of every set-option call.
func allSetOptionCalls(calls [][]string) []int {
	var out []int
	for i, c := range calls {
		if len(c) > 0 && c[0] == "set-option" {
			out = append(out, i)
		}
	}
	return out
}

func TestApplySkeletonMarkers_SetsOneMarkerPerSuppliedLivePane(t *testing.T) {
	// New contract: ApplySkeletonMarkers consumes the live PaneCoord slice
	// threaded through from Restore — it does NOT call list-panes itself.
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := markersSession("work",
		markersWindow(0, markersPane(0), markersPane(1)),
		markersWindow(1, markersPane(0)),
	)
	livePanes := parseLivePanes(t, "0:0\n0:1\n1:0")

	if err := r.ApplySkeletonMarkers(sess, livePanes, 0, 0); err != nil {
		t.Fatalf("ApplySkeletonMarkers: %v", err)
	}

	// No list-panes invocation — the slice is supplied by the caller.
	if got := len(findAllCalls(mock.Calls, "list-panes")); got != 0 {
		t.Errorf("list-panes calls = %d, want 0 (caller supplies livePanes); calls: %v", got, mock.Calls)
	}

	// Three set-option calls — one per live pane.
	setIdxs := allSetOptionCalls(mock.Calls)
	if len(setIdxs) != 3 {
		t.Fatalf("set-option calls = %d, want 3; calls: %v", len(setIdxs), mock.Calls)
	}

	// Each marker name corresponds to a live paneKey at (0,0), (0,1), (1,0).
	wantMarkers := []string{
		"@portal-skeleton-" + state.SanitizePaneKey("work", 0, 0),
		"@portal-skeleton-" + state.SanitizePaneKey("work", 0, 1),
		"@portal-skeleton-" + state.SanitizePaneKey("work", 1, 0),
	}
	for _, m := range wantMarkers {
		if findSetOptionMarker(mock.Calls, m) < 0 {
			t.Errorf("expected set-option for marker %q; calls: %v", m, mock.Calls)
		}
	}
}

func TestApplySkeletonMarkers_UsesLivePaneKeyWhenPredictionDiffers(t *testing.T) {
	// Predict base 0/0 but the live PaneCoord slice reports the pane at 1:0
	// (drift). Markers must use the supplied live paneKey, not the predicted
	// one.
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logger, err := state.OpenLogger(filepath.Join(dir, "portal.log"), false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := markersSession("work",
		markersWindow(0, markersPane(0)),
	)
	livePanes := parseLivePanes(t, "1:0")

	if err := r.ApplySkeletonMarkers(sess, livePanes, 0, 0); err != nil {
		t.Fatalf("ApplySkeletonMarkers: %v", err)
	}

	// Marker should be set with the live paneKey (1:0), NOT the predicted (0:0).
	wantLive := "@portal-skeleton-" + state.SanitizePaneKey("work", 1, 0)
	dontWant := "@portal-skeleton-" + state.SanitizePaneKey("work", 0, 0)
	if findSetOptionMarker(mock.Calls, wantLive) < 0 {
		t.Errorf("expected set-option for live marker %q; calls: %v", wantLive, mock.Calls)
	}
	if findSetOptionMarker(mock.Calls, dontWant) >= 0 {
		t.Errorf("did not expect set-option for predicted marker %q; calls: %v", dontWant, mock.Calls)
	}
}

func TestApplySkeletonMarkers_LogsDriftWarningWhenPredictedAndLiveDiffer(t *testing.T) {
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "portal.log")
	logger, err := state.OpenLogger(logPath, false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := markersSession("work",
		markersWindow(0, markersPane(0)),
	)
	livePanes := parseLivePanes(t, "1:0")

	if err := r.ApplySkeletonMarkers(sess, livePanes, 0, 0); err != nil {
		t.Fatalf("ApplySkeletonMarkers: %v", err)
	}

	_ = logger.Close()
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "WARN") {
		t.Errorf("log body lacks WARN entry: %q", bodyStr)
	}
	if !strings.Contains(bodyStr, "predicted=") {
		t.Errorf("log body lacks predicted=... drift detail: %q", bodyStr)
	}
	if !strings.Contains(bodyStr, "live=") {
		t.Errorf("log body lacks live=... drift detail: %q", bodyStr)
	}
}

func TestApplySkeletonMarkers_LogsSanityWarningOnPaneCountMismatch(t *testing.T) {
	// Saved 2 panes, live reports 1 → sanity warning expected.
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "portal.log")
	logger, err := state.OpenLogger(logPath, false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := markersSession("work",
		markersWindow(0, markersPane(0), markersPane(1)),
	)
	livePanes := parseLivePanes(t, "0:0")

	if err := r.ApplySkeletonMarkers(sess, livePanes, 0, 0); err != nil {
		t.Fatalf("ApplySkeletonMarkers: %v", err)
	}

	_ = logger.Close()
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "live pane count") {
		t.Errorf("log body lacks 'live pane count' sanity warning: %q", bodyStr)
	}
}

func TestApplySkeletonMarkers_UsesServerScopeFlagAndNeverGlobal(t *testing.T) {
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := markersSession("work",
		markersWindow(0, markersPane(0), markersPane(1)),
	)
	livePanes := parseLivePanes(t, "0:0\n0:1")

	if err := r.ApplySkeletonMarkers(sess, livePanes, 0, 0); err != nil {
		t.Fatalf("ApplySkeletonMarkers: %v", err)
	}

	for _, idx := range allSetOptionCalls(mock.Calls) {
		args := mock.Calls[idx]
		hasS := false
		for _, a := range args {
			if a == "-s" {
				hasS = true
			}
			if a == "-g" {
				t.Errorf("set-option call %v uses -g (global); expected server-scope -s only", args)
			}
		}
		if !hasS {
			t.Errorf("set-option call %v is missing required -s flag", args)
		}
	}
}

func TestApplySkeletonMarkers_ContinuesWhenOneSetOptionFails(t *testing.T) {
	failMarker := "@portal-skeleton-" + state.SanitizePaneKey("work", 0, 0)
	mock := &mockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "set-option" {
				for _, a := range args {
					if a == failMarker {
						return "", errors.New("set-option failure")
					}
				}
			}
			return "", nil
		},
	}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logger, err := state.OpenLogger(filepath.Join(dir, "portal.log"), false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	defer func() { _ = logger.Close() }()
	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := markersSession("work",
		markersWindow(0, markersPane(0), markersPane(1), markersPane(2)),
	)
	livePanes := parseLivePanes(t, "0:0\n0:1\n0:2")

	if err := r.ApplySkeletonMarkers(sess, livePanes, 0, 0); err != nil {
		t.Fatalf("ApplySkeletonMarkers returned error %v, expected nil (failure should be logged + continue)", err)
	}

	// All three set-option calls should still have been attempted.
	setIdxs := allSetOptionCalls(mock.Calls)
	if len(setIdxs) != 3 {
		t.Errorf("set-option calls = %d, want 3 (each attempted)", len(setIdxs))
	}
}

// TestApplySkeletonMarkers_ReturnsErrorWhenListPanesFails was removed — the
// markers function no longer queries list-panes itself; the caller threads
// the live PaneCoord slice through from Restore. The error path that test
// covered is now exercised in Restore tests (armPanes' list-panes call is the
// failure point in the new wiring).

func TestApplySkeletonMarkers_SetsMarkerValueToLiteralOne(t *testing.T) {
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := markersSession("work",
		markersWindow(0, markersPane(0)),
	)
	livePanes := parseLivePanes(t, "0:0")

	if err := r.ApplySkeletonMarkers(sess, livePanes, 0, 0); err != nil {
		t.Fatalf("ApplySkeletonMarkers: %v", err)
	}

	setIdxs := allSetOptionCalls(mock.Calls)
	if len(setIdxs) != 1 {
		t.Fatalf("set-option calls = %d, want 1", len(setIdxs))
	}
	args := mock.Calls[setIdxs[0]]
	// args = [set-option, -s, <name>, <value>]
	if len(args) != 4 {
		t.Fatalf("set-option args = %v, want length 4", args)
	}
	if args[3] != "1" {
		t.Errorf("set-option value = %q, want %q", args[3], "1")
	}
}

func TestApplySkeletonMarkers_UsesHashedPaneKeyForCollisionSession(t *testing.T) {
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	name := "foo/bar"
	sess := markersSession(name,
		markersWindow(0, markersPane(0)),
	)
	livePanes := parseLivePanes(t, "0:0")

	if err := r.ApplySkeletonMarkers(sess, livePanes, 0, 0); err != nil {
		t.Fatalf("ApplySkeletonMarkers: %v", err)
	}

	wantKey := state.SanitizePaneKey(name, 0, 0)
	if !strings.Contains(wantKey, "foo_bar-") {
		t.Fatalf("sanity: paneKey %q should contain hash suffix marker 'foo_bar-'", wantKey)
	}
	wantMarker := "@portal-skeleton-" + wantKey
	if findSetOptionMarker(mock.Calls, wantMarker) < 0 {
		t.Errorf("expected set-option for hashed marker %q; calls: %v", wantMarker, mock.Calls)
	}
}

func TestApplySkeletonMarkers_EnumeratesLivePanesInSuppliedOrder(t *testing.T) {
	// The caller (armPanes) hands in a slice already sorted by (window, pane);
	// markers walks it in the order received. The test passes pre-sorted input
	// (since that's the caller's contract) and asserts the output ordering
	// matches.
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := markersSession("work",
		markersWindow(0, markersPane(0), markersPane(1)),
		markersWindow(1, markersPane(0), markersPane(1)),
	)
	livePanes := parseLivePanes(t, "0:0\n0:1\n1:0\n1:1")

	if err := r.ApplySkeletonMarkers(sess, livePanes, 0, 0); err != nil {
		t.Fatalf("ApplySkeletonMarkers: %v", err)
	}

	setIdxs := allSetOptionCalls(mock.Calls)
	if len(setIdxs) != 4 {
		t.Fatalf("set-option calls = %d, want 4", len(setIdxs))
	}

	// Markers should appear in (window, pane) sorted order: 0:0, 0:1, 1:0, 1:1.
	wantOrder := []string{
		"@portal-skeleton-" + state.SanitizePaneKey("work", 0, 0),
		"@portal-skeleton-" + state.SanitizePaneKey("work", 0, 1),
		"@portal-skeleton-" + state.SanitizePaneKey("work", 1, 0),
		"@portal-skeleton-" + state.SanitizePaneKey("work", 1, 1),
	}
	for i, idx := range setIdxs {
		args := mock.Calls[idx]
		if len(args) < 3 {
			t.Fatalf("set-option[%d] args = %v, want length >= 3", i, args)
		}
		if args[2] != wantOrder[i] {
			t.Errorf("set-option[%d] marker = %q, want %q (sorted order)", i, args[2], wantOrder[i])
		}
	}
}

func TestApplySkeletonMarkers_MarksExtraLivePanesWhenLiveCountExceedsSaved(t *testing.T) {
	// Saved 1 pane, live reports 2 — both extras must still be marked using
	// their live paneKey.
	mock := &mockCommander{}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logger, err := state.OpenLogger(filepath.Join(dir, "portal.log"), false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	defer func() { _ = logger.Close() }()
	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := markersSession("work",
		markersWindow(0, markersPane(0)),
	)
	livePanes := parseLivePanes(t, "0:0\n0:1")

	if err := r.ApplySkeletonMarkers(sess, livePanes, 0, 0); err != nil {
		t.Fatalf("ApplySkeletonMarkers: %v", err)
	}

	wantMarkers := []string{
		"@portal-skeleton-" + state.SanitizePaneKey("work", 0, 0),
		"@portal-skeleton-" + state.SanitizePaneKey("work", 0, 1),
	}
	for _, m := range wantMarkers {
		if findSetOptionMarker(mock.Calls, m) < 0 {
			t.Errorf("expected set-option for marker %q; calls: %v", m, mock.Calls)
		}
	}
}
