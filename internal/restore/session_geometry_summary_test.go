package restore_test

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/tmux"
)

func geometrySummaryLine(t *testing.T, sink *logtest.Sink) string {
	t.Helper()
	var found []string
	for _, line := range sink.Lines() {
		if strings.Contains(line, "geometry complete") {
			found = append(found, line)
		}
	}
	if len(found) > 1 {
		t.Fatalf("expected at most one geometry-complete summary; got %d: %v", len(found), found)
	}
	if len(found) == 0 {
		return ""
	}
	return found[0]
}

func TestApplyWindowGeometry_EmitsGeometryCompleteSummaryOnCleanReplay(t *testing.T) {
	mock := geometryFake(t)
	client := tmux.NewClient(mock)
	logger, sink := logtest.NewCaptureLogger(t)
	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := geometrySession("work",
		geomWindow(0, "L0", false, activePane(0), inactivePane(1)),
		geomWindow(1, "L1", false, activePane(0)),
	)
	live := liveCoordsFromSaved(sess, 0, 0)

	r.ApplyWindowGeometry(sess, live)

	line := geometrySummaryLine(t, sink)
	if line == "" {
		t.Fatalf("expected one geometry-complete summary; sink body:\n%s", sink.Body())
	}
	if !strings.HasPrefix(line, "INFO ") {
		t.Errorf("summary level = %q, want INFO", line)
	}
	for _, want := range []string{"geometry complete", "panes=3", "took=", "anomalous=0"} {
		if !strings.Contains(line, want) {
			t.Errorf("summary %q missing %q", line, want)
		}
	}
}

func TestApplyWindowGeometry_SummaryPanesEqualsLivePaneCount(t *testing.T) {
	mock := geometryFake(t)
	client := tmux.NewClient(mock)
	logger, sink := logtest.NewCaptureLogger(t)
	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := geometrySession("work",
		geomWindow(0, "L", false, activePane(0), inactivePane(1), inactivePane(2)),
	)
	live := liveCoordsFromSaved(sess, 0, 0)

	r.ApplyWindowGeometry(sess, live)

	line := geometrySummaryLine(t, sink)
	if !strings.Contains(line, "panes=3") {
		t.Errorf("summary %q: panes must equal len(livePanes)=3", line)
	}
}

func TestApplyWindowGeometry_SummaryHasOnlyPanesTookAnomalousAttrs(t *testing.T) {
	mock := geometryFake(t)
	client := tmux.NewClient(mock)
	logger, sink := logtest.NewCaptureLogger(t)
	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := geometrySession("work",
		geomWindow(0, "L", false, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	rec := sink.Records().WithMessage("geometry complete").Only(t, "geometry complete record")
	got := append([]string(nil), rec.Keys...)
	sort.Strings(got)
	want := []string{"anomalous", "panes", "took"}
	if !equalStringSlices(got, want) {
		t.Errorf("geometry summary attr keys = %v, want exactly %v (no scrollback or other keys)", got, want)
	}
}

func TestApplyWindowGeometry_EmitsExactlyOneSummaryPerCall(t *testing.T) {
	mock := geometryFake(t)
	client := tmux.NewClient(mock)
	logger, sink := logtest.NewCaptureLogger(t)
	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := geometrySession("work",
		geomWindow(0, "L0", false, activePane(0)),
		geomWindow(1, "L1", false, activePane(0)),
		geomWindow(2, "L2", false, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	_ = sink.Records().WithMessage("geometry complete").Only(t, "geometry complete record")
}

func TestApplyWindowGeometry_SelectLayoutFailureIncrementsAnomalousAndRetainsWarn(t *testing.T) {
	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		if len(args) >= 4 && args[0] == "select-layout" && args[3] == "broken" {
			return "", errors.New("layout parse failed")
		}
		return "", nil
	})
	client := tmux.NewClient(mock)
	logger, sink := logtest.NewCaptureLogger(t)
	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := geometrySession("work",
		geomWindow(0, "broken", false, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	line := geometrySummaryLine(t, sink)
	if !strings.Contains(line, "anomalous=1") {
		t.Errorf("summary %q: select-layout failure must increment anomalous to 1", line)
	}
	if !strings.Contains(sink.Body(), "falling back to tiled") {
		t.Errorf("per-step WARN about saved-layout failure must still fire; body:\n%s", sink.Body())
	}
	if findCallTarget(mock.Calls(), "select-pane", "=work:0.0") < 0 {
		t.Errorf("expected select-pane to still run after layout failure; calls: %v", mock.Calls())
	}
}

func TestApplyWindowGeometry_DoubleLayoutFailureIsOneAnomalous(t *testing.T) {
	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "select-layout" {
			return "", errors.New("layout failed")
		}
		return "", nil
	})
	client := tmux.NewClient(mock)
	logger, sink := logtest.NewCaptureLogger(t)
	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := geometrySession("work",
		geomWindow(0, "broken", false, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	line := geometrySummaryLine(t, sink)
	if !strings.Contains(line, "anomalous=1") {
		t.Errorf("summary %q: double-layout-failure must be ONE anomalous, not two", line)
	}
	body := sink.Body()
	if !strings.Contains(body, "falling back to tiled") {
		t.Errorf("first per-step WARN must still fire; body:\n%s", body)
	}
	if !strings.Contains(body, "tiled fallback also failed") {
		t.Errorf("second per-step WARN must still fire; body:\n%s", body)
	}
}

func TestApplyWindowGeometry_SelectPaneFailureIncrementsAnomalous(t *testing.T) {
	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "select-pane" {
			return "", errors.New("select-pane failed")
		}
		return "", nil
	})
	client := tmux.NewClient(mock)
	logger, sink := logtest.NewCaptureLogger(t)
	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := geometrySession("work",
		geomWindow(0, "L", false, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	line := geometrySummaryLine(t, sink)
	if !strings.Contains(line, "anomalous=1") {
		t.Errorf("summary %q: select-pane failure must increment anomalous", line)
	}
	if !strings.Contains(sink.Body(), "select-pane failed") {
		t.Errorf("per-step select-pane WARN must still fire; body:\n%s", sink.Body())
	}
}

func TestApplyWindowGeometry_ZoomFailureIncrementsAnomalous(t *testing.T) {
	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "resize-pane" && args[1] == "-Z" {
			return "", errors.New("resize-pane -Z failed")
		}
		return "", nil
	})
	client := tmux.NewClient(mock)
	logger, sink := logtest.NewCaptureLogger(t)
	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := geometrySession("work",
		geomWindow(0, "L", true, activePane(0)),
	)

	r.ApplyWindowGeometry(sess, liveCoordsFromSaved(sess, 0, 0))

	line := geometrySummaryLine(t, sink)
	if !strings.Contains(line, "anomalous=1") {
		t.Errorf("summary %q: zoom failure must increment anomalous", line)
	}
	if !strings.Contains(sink.Body(), "resize-pane -Z failed") {
		t.Errorf("per-step zoom WARN must still fire; body:\n%s", sink.Body())
	}
}

func TestApplyWindowGeometry_EmptySavedWindowGroupSkippedNotAnomalous(t *testing.T) {
	mock := geometryFake(t)
	client := tmux.NewClient(mock)
	logger, sink := logtest.NewCaptureLogger(t)
	r := &restore.SessionRestorer{Client: client, Logger: logger}

	sess := geometrySession("work",
		geomWindow(0, "L0", false, activePane(0)),
		geomWindow(1, "L1", true, activePane(0)),
	)
	live := []tmux.PaneCoord{{Window: 0, Pane: 0}}

	r.ApplyWindowGeometry(sess, live)

	line := geometrySummaryLine(t, sink)
	if !strings.Contains(line, "anomalous=0") {
		t.Errorf("summary %q: empty saved-window group must NOT be counted anomalous", line)
	}
	if !strings.Contains(line, "panes=1") {
		t.Errorf("summary %q: panes must equal len(livePanes)=1", line)
	}
	if findSelectLayoutTarget(mock.Calls(), "work:1", "L1") >= 0 {
		t.Errorf("did not expect select-layout for empty-group window 1; calls: %v", mock.Calls())
	}
	if findCallTarget(mock.Calls(), "select-pane", "=work:1.0") >= 0 {
		t.Errorf("did not expect select-pane for empty-group window 1; calls: %v", mock.Calls())
	}
	if findResizePaneZoom(mock.Calls(), "=work:1.0") >= 0 {
		t.Errorf("did not expect resize-pane -Z for empty-group window 1; calls: %v", mock.Calls())
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
