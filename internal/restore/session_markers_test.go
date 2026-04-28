package restore_test

import (
	"errors"
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

// listPanesRunFunc returns a RunFunc that responds to list-panes -s -t with
// the supplied multi-line output and otherwise returns ("", nil).
func listPanesRunFunc(output string) func(args ...string) (string, error) {
	return func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "list-panes" {
			return output, nil
		}
		return "", nil
	}
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

func TestApplySkeletonMarkers_CallsListPanesAndSetsOneMarkerPerLivePane(t *testing.T) {
	mock := &mockCommander{RunFunc: listPanesRunFunc("0:0\n0:1\n1:0")}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := markersSession("work",
		markersWindow(0, markersPane(0), markersPane(1)),
		markersWindow(1, markersPane(0)),
	)

	if err := r.ApplySkeletonMarkers(sess, 0, 0); err != nil {
		t.Fatalf("ApplySkeletonMarkers: %v", err)
	}

	// One list-panes invocation against -t work.
	listIdxs := findAllCalls(mock.Calls, "list-panes")
	if len(listIdxs) != 1 {
		t.Fatalf("list-panes calls = %d, want 1; calls: %v", len(listIdxs), mock.Calls)
	}
	listArgs := mock.Calls[listIdxs[0]]
	wantArgs := []string{"list-panes", "-s", "-t", "work", "-F", "#{window_index}:#{pane_index}"}
	if len(listArgs) != len(wantArgs) {
		t.Fatalf("list-panes args = %v, want %v", listArgs, wantArgs)
	}
	for i := range wantArgs {
		if listArgs[i] != wantArgs[i] {
			t.Errorf("list-panes args[%d] = %q, want %q", i, listArgs[i], wantArgs[i])
		}
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
	// Predict base 0/0 but tmux reports the pane at 1:0 (drift).
	mock := &mockCommander{RunFunc: listPanesRunFunc("1:0")}
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

	if err := r.ApplySkeletonMarkers(sess, 0, 0); err != nil {
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
	mock := &mockCommander{RunFunc: listPanesRunFunc("1:0")}
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

	if err := r.ApplySkeletonMarkers(sess, 0, 0); err != nil {
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
	mock := &mockCommander{RunFunc: listPanesRunFunc("0:0")}
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

	if err := r.ApplySkeletonMarkers(sess, 0, 0); err != nil {
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
	mock := &mockCommander{RunFunc: listPanesRunFunc("0:0\n0:1")}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := markersSession("work",
		markersWindow(0, markersPane(0), markersPane(1)),
	)

	if err := r.ApplySkeletonMarkers(sess, 0, 0); err != nil {
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
			if len(args) > 0 && args[0] == "list-panes" {
				return "0:0\n0:1\n0:2", nil
			}
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

	if err := r.ApplySkeletonMarkers(sess, 0, 0); err != nil {
		t.Fatalf("ApplySkeletonMarkers returned error %v, expected nil (failure should be logged + continue)", err)
	}

	// All three set-option calls should still have been attempted.
	setIdxs := allSetOptionCalls(mock.Calls)
	if len(setIdxs) != 3 {
		t.Errorf("set-option calls = %d, want 3 (each attempted)", len(setIdxs))
	}
}

func TestApplySkeletonMarkers_ReturnsErrorWhenListPanesFails(t *testing.T) {
	mock := &mockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "list-panes" {
				return "", errors.New("list-panes failed")
			}
			return "", nil
		},
	}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := markersSession("work",
		markersWindow(0, markersPane(0)),
	)

	err := r.ApplySkeletonMarkers(sess, 0, 0)
	if err == nil {
		t.Fatal("expected error from list-panes failure, got nil")
	}
	if !strings.Contains(err.Error(), "work") {
		t.Errorf("error %q lacks session context", err)
	}
	if !strings.Contains(err.Error(), "list-panes failed") {
		t.Errorf("error %q does not wrap underlying error", err)
	}
	// No set-option calls expected when list-panes fails.
	if got := len(allSetOptionCalls(mock.Calls)); got != 0 {
		t.Errorf("set-option calls = %d, want 0 when list-panes fails", got)
	}
}

func TestApplySkeletonMarkers_SetsMarkerValueToLiteralOne(t *testing.T) {
	mock := &mockCommander{RunFunc: listPanesRunFunc("0:0")}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := markersSession("work",
		markersWindow(0, markersPane(0)),
	)

	if err := r.ApplySkeletonMarkers(sess, 0, 0); err != nil {
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
	mock := &mockCommander{RunFunc: listPanesRunFunc("0:0")}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	name := "foo/bar"
	sess := markersSession(name,
		markersWindow(0, markersPane(0)),
	)

	if err := r.ApplySkeletonMarkers(sess, 0, 0); err != nil {
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

func TestApplySkeletonMarkers_EnumeratesLivePanesSortedByWindowThenPane(t *testing.T) {
	// tmux reports out-of-order — function must consume sorted order.
	mock := &mockCommander{RunFunc: listPanesRunFunc("1:1\n0:1\n1:0\n0:0")}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := markersSession("work",
		markersWindow(0, markersPane(0), markersPane(1)),
		markersWindow(1, markersPane(0), markersPane(1)),
	)

	if err := r.ApplySkeletonMarkers(sess, 0, 0); err != nil {
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
	mock := &mockCommander{RunFunc: listPanesRunFunc("0:0\n0:1")}
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

	if err := r.ApplySkeletonMarkers(sess, 0, 0); err != nil {
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
