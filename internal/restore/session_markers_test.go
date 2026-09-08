package restore_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

func markersSession(name string, windows ...state.Window) state.Session {
	return state.Session{Name: name, Windows: windows}
}

func markersWindow(idx int, panes ...state.Pane) state.Window {
	return state.Window{Index: idx, Panes: panes}
}

func markersPane(idx int) state.Pane {
	return state.Pane{Index: idx}
}

func parseLivePanes(t *testing.T, output string) []tmux.PaneCoord {
	t.Helper()
	if output == "" {
		return nil
	}
	var out []tmux.PaneCoord
	for line := range strings.SplitSeq(output, "\n") {
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

func findSetOptionMarker(calls [][]string, markerName string) int {
	for i, c := range calls {
		if len(c) >= 4 && c[0] == "set-option" && c[2] == markerName {
			return i
		}
	}
	return -1
}

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
	mock := commandertest.New(t, commandertest.Returns("", "set-option"))
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := markersSession("work",
		markersWindow(0, markersPane(0), markersPane(1)),
		markersWindow(1, markersPane(0)),
	)
	livePanes := parseLivePanes(t, "0:0\n0:1\n1:0")

	r.ApplySkeletonMarkers(sess, livePanes)

	if got := len(findAllCalls(mock.Calls(), "list-panes")); got != 0 {
		t.Errorf("list-panes calls = %d, want 0 (caller supplies livePanes); calls: %v", got, mock.Calls())
	}

	setIdxs := allSetOptionCalls(mock.Calls())
	if len(setIdxs) != 3 {
		t.Fatalf("set-option calls = %d, want 3; calls: %v", len(setIdxs), mock.Calls())
	}

	wantMarkers := []string{
		"@portal-skeleton-" + state.SanitizePaneKey("work", 0, 0),
		"@portal-skeleton-" + state.SanitizePaneKey("work", 0, 1),
		"@portal-skeleton-" + state.SanitizePaneKey("work", 1, 0),
	}
	for _, m := range wantMarkers {
		if findSetOptionMarker(mock.Calls(), m) < 0 {
			t.Errorf("expected set-option for marker %q; calls: %v", m, mock.Calls())
		}
	}
}

func TestApplySkeletonMarkers_UsesLivePaneKey(t *testing.T) {
	mock := commandertest.New(t, commandertest.Returns("", "set-option"))
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logger := restoretest.OpenTestLogger(t, dir)

	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := markersSession("work",
		markersWindow(0, markersPane(0)),
	)
	livePanes := parseLivePanes(t, "1:0")

	r.ApplySkeletonMarkers(sess, livePanes)

	wantLive := "@portal-skeleton-" + state.SanitizePaneKey("work", 1, 0)
	if findSetOptionMarker(mock.Calls(), wantLive) < 0 {
		t.Errorf("expected set-option for live marker %q; calls: %v", wantLive, mock.Calls())
	}
}

func TestApplySkeletonMarkers_LogsSanityWarningOnPaneCountMismatch(t *testing.T) {
	mock := commandertest.New(t, commandertest.Returns("", "set-option"))
	client := tmux.NewClient(mock)
	logger, sink := logtest.NewCaptureLogger(t)

	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := markersSession("work",
		markersWindow(0, markersPane(0), markersPane(1)),
	)
	livePanes := parseLivePanes(t, "0:0")

	r.ApplySkeletonMarkers(sess, livePanes)

	bodyStr := sink.Body()
	if !strings.Contains(bodyStr, "live pane count") {
		t.Errorf("log body lacks 'live pane count' sanity warning: %q", bodyStr)
	}
}

func TestApplySkeletonMarkers_UsesServerScopeFlagAndNeverGlobal(t *testing.T) {
	mock := commandertest.New(t, commandertest.Returns("", "set-option"))
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := markersSession("work",
		markersWindow(0, markersPane(0), markersPane(1)),
	)
	livePanes := parseLivePanes(t, "0:0\n0:1")

	r.ApplySkeletonMarkers(sess, livePanes)

	for _, idx := range allSetOptionCalls(mock.Calls()) {
		args := mock.Calls()[idx]
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
	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "set-option" {
			if slices.Contains(args, failMarker) {
				return "", errors.New("set-option failure")
			}
		}
		return "", nil
	})
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logger := restoretest.OpenTestLogger(t, dir)
	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := markersSession("work",
		markersWindow(0, markersPane(0), markersPane(1), markersPane(2)),
	)
	livePanes := parseLivePanes(t, "0:0\n0:1\n0:2")

	r.ApplySkeletonMarkers(sess, livePanes)

	setIdxs := allSetOptionCalls(mock.Calls())
	if len(setIdxs) != 3 {
		t.Errorf("set-option calls = %d, want 3 (each attempted)", len(setIdxs))
	}
}

func TestApplySkeletonMarkers_SetsMarkerValueToLiteralOne(t *testing.T) {
	mock := commandertest.New(t, commandertest.Returns("", "set-option"))
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := markersSession("work",
		markersWindow(0, markersPane(0)),
	)
	livePanes := parseLivePanes(t, "0:0")

	r.ApplySkeletonMarkers(sess, livePanes)

	setIdxs := allSetOptionCalls(mock.Calls())
	if len(setIdxs) != 1 {
		t.Fatalf("set-option calls = %d, want 1", len(setIdxs))
	}
	args := mock.Calls()[setIdxs[0]]
	if len(args) != 4 {
		t.Fatalf("set-option args = %v, want length 4", args)
	}
	if args[3] != "1" {
		t.Errorf("set-option value = %q, want %q", args[3], "1")
	}
}

func TestApplySkeletonMarkers_UsesHashedPaneKeyForCollisionSession(t *testing.T) {
	mock := commandertest.New(t, commandertest.Returns("", "set-option"))
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	name := "foo/bar"
	sess := markersSession(name,
		markersWindow(0, markersPane(0)),
	)
	livePanes := parseLivePanes(t, "0:0")

	r.ApplySkeletonMarkers(sess, livePanes)

	wantKey := state.SanitizePaneKey(name, 0, 0)
	if !strings.Contains(wantKey, "foo_bar-") {
		t.Fatalf("sanity: paneKey %q should contain hash suffix marker 'foo_bar-'", wantKey)
	}
	wantMarker := "@portal-skeleton-" + wantKey
	if findSetOptionMarker(mock.Calls(), wantMarker) < 0 {
		t.Errorf("expected set-option for hashed marker %q; calls: %v", wantMarker, mock.Calls())
	}
}

func TestApplySkeletonMarkers_EnumeratesLivePanesInSuppliedOrder(t *testing.T) {
	mock := commandertest.New(t, commandertest.Returns("", "set-option"))
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client}

	sess := markersSession("work",
		markersWindow(0, markersPane(0), markersPane(1)),
		markersWindow(1, markersPane(0), markersPane(1)),
	)
	livePanes := parseLivePanes(t, "0:0\n0:1\n1:0\n1:1")

	r.ApplySkeletonMarkers(sess, livePanes)

	setIdxs := allSetOptionCalls(mock.Calls())
	if len(setIdxs) != 4 {
		t.Fatalf("set-option calls = %d, want 4", len(setIdxs))
	}

	wantOrder := []string{
		"@portal-skeleton-" + state.SanitizePaneKey("work", 0, 0),
		"@portal-skeleton-" + state.SanitizePaneKey("work", 0, 1),
		"@portal-skeleton-" + state.SanitizePaneKey("work", 1, 0),
		"@portal-skeleton-" + state.SanitizePaneKey("work", 1, 1),
	}
	for i, idx := range setIdxs {
		args := mock.Calls()[idx]
		if len(args) < 3 {
			t.Fatalf("set-option[%d] args = %v, want length >= 3", i, args)
		}
		if args[2] != wantOrder[i] {
			t.Errorf("set-option[%d] marker = %q, want %q (sorted order)", i, args[2], wantOrder[i])
		}
	}
}

func TestApplySkeletonMarkers_MarksExtraLivePanesWhenLiveCountExceedsSaved(t *testing.T) {
	mock := commandertest.New(t, commandertest.Returns("", "set-option"))
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logger := restoretest.OpenTestLogger(t, dir)
	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := markersSession("work",
		markersWindow(0, markersPane(0)),
	)
	livePanes := parseLivePanes(t, "0:0\n0:1")

	r.ApplySkeletonMarkers(sess, livePanes)

	wantMarkers := []string{
		"@portal-skeleton-" + state.SanitizePaneKey("work", 0, 0),
		"@portal-skeleton-" + state.SanitizePaneKey("work", 0, 1),
	}
	for _, m := range wantMarkers {
		if findSetOptionMarker(mock.Calls(), m) < 0 {
			t.Errorf("expected set-option for marker %q; calls: %v", m, mock.Calls())
		}
	}
}
