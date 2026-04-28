package restore_test

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// sessionsForMarkerTest returns a single minimally-valid saved session whose
// presence in sessions.json drives Restore() through to list-sessions. The
// session itself is intentionally not "live" in the mock, so the orchestrator
// will attempt to skeleton-create it — which is fine for these tests; we only
// assert on @portal-restoring sequencing, not on the create path.
func sessionsForMarkerTest() []state.Session {
	return []state.Session{
		{
			Name: "work",
			Windows: []state.Window{
				{
					Index: 0,
					Name:  "main",
					Panes: []state.Pane{
						{Index: 0, CWD: "/work", ScrollbackFile: "scrollback/work__0.0.bin", Active: true},
					},
				},
			},
		},
	}
}

// markerRunFunc is a small RunFunc dispatcher tailored for marker-wrapper
// tests. It recognises the set/unset of @portal-restoring and routes all other
// calls to behaviour appropriate for the orchestrator's Restore() path:
// list-sessions returns "" (no live sessions) and show-option returns
// ErrOptionNotFound (so PredictLiveIndices defaults to 0/0).
type markerRunFunc struct {
	// setErr, when non-nil, is returned for set-option -s @portal-restoring 1.
	setErr error
	// unsetErr, when non-nil, is returned for set-option -su @portal-restoring.
	unsetErr error
	// listSessionsPanic, when true, makes list-sessions panic. Used to verify
	// the deferred clear runs even when Restore panics.
	listSessionsPanic bool
	// listSessionsErr, when non-nil, is returned from list-sessions.
	listSessionsErr error
}

func (m *markerRunFunc) run(args ...string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	switch args[0] {
	case "set-option":
		// set-option -s @portal-restoring 1 → set
		// set-option -su @portal-restoring → unset
		if len(args) >= 3 && args[1] == "-s" && args[2] == "@portal-restoring" {
			return "", m.setErr
		}
		if len(args) >= 3 && args[1] == "-su" && args[2] == "@portal-restoring" {
			return "", m.unsetErr
		}
		return "", nil
	case "list-sessions":
		if m.listSessionsPanic {
			panic("list-sessions blew up")
		}
		return "", m.listSessionsErr
	case "show-option":
		return "", errors.New("unknown option")
	}
	return "", nil
}

// findSetRestoringIdx returns the index of the first call that is
// `set-option -s @portal-restoring 1`. -1 if absent.
func findSetRestoringIdx(calls [][]string) int {
	for i, c := range calls {
		if len(c) >= 4 && c[0] == "set-option" && c[1] == "-s" && c[2] == "@portal-restoring" && c[3] == "1" {
			return i
		}
	}
	return -1
}

// findUnsetRestoringIdx returns the index of the first call that is
// `set-option -su @portal-restoring`. -1 if absent.
func findUnsetRestoringIdx(calls [][]string) int {
	for i, c := range calls {
		if len(c) >= 3 && c[0] == "set-option" && c[1] == "-su" && c[2] == "@portal-restoring" {
			return i
		}
	}
	return -1
}

func TestRestoreWithMarker_SetsBeforeRestoreCalls(t *testing.T) {
	dir := t.TempDir()
	mock := &mockCommander{RunFunc: (&markerRunFunc{}).run}
	logger, _ := openTestLogger(t, dir)
	defer func() { _ = logger.Close() }()
	stderr := &bytes.Buffer{}

	o := newOrchestrator(t, mock, dir, logger, stderr)
	if err := o.RestoreWithMarker(); err != nil {
		t.Fatalf("RestoreWithMarker returned error: %v", err)
	}

	setIdx := findSetRestoringIdx(mock.Calls)
	if setIdx == -1 {
		t.Fatalf("expected set-option -s @portal-restoring 1; calls: %v", mock.Calls)
	}
	// No sessions.json → no list-sessions; setIdx must be call 0 still.
	if setIdx != 0 {
		t.Errorf("expected set @portal-restoring to be the first call; got index %d, calls: %v", setIdx, mock.Calls)
	}
}

func TestRestoreWithMarker_SetPrecedesListSessions(t *testing.T) {
	dir := t.TempDir()
	// Drive Restore() to actually call list-sessions by writing a non-empty index.
	writeValidIndex(t, dir, sessionsForMarkerTest())

	mock := &mockCommander{RunFunc: (&markerRunFunc{}).run}
	logger, _ := openTestLogger(t, dir)
	defer func() { _ = logger.Close() }()
	stderr := &bytes.Buffer{}

	o := newOrchestrator(t, mock, dir, logger, stderr)
	if err := o.RestoreWithMarker(); err != nil {
		t.Fatalf("RestoreWithMarker returned error: %v", err)
	}

	setIdx := findSetRestoringIdx(mock.Calls)
	listIdx := callsAt(mock.Calls, "list-sessions")
	if setIdx == -1 {
		t.Fatalf("expected set @portal-restoring; calls: %v", mock.Calls)
	}
	if listIdx == -1 {
		t.Fatalf("expected list-sessions to be invoked; calls: %v", mock.Calls)
	}
	if setIdx >= listIdx {
		t.Errorf("set @portal-restoring (idx %d) must precede list-sessions (idx %d); calls: %v", setIdx, listIdx, mock.Calls)
	}
}

func TestRestoreWithMarker_ClearsAfterRestoreReturns(t *testing.T) {
	dir := t.TempDir()
	mock := &mockCommander{RunFunc: (&markerRunFunc{}).run}
	logger, _ := openTestLogger(t, dir)
	defer func() { _ = logger.Close() }()
	stderr := &bytes.Buffer{}

	o := newOrchestrator(t, mock, dir, logger, stderr)
	if err := o.RestoreWithMarker(); err != nil {
		t.Fatalf("RestoreWithMarker returned error: %v", err)
	}

	unsetIdx := findUnsetRestoringIdx(mock.Calls)
	if unsetIdx == -1 {
		t.Fatalf("expected set-option -su @portal-restoring; calls: %v", mock.Calls)
	}
	if unsetIdx != len(mock.Calls)-1 {
		t.Errorf("clear @portal-restoring must be the LAST call; got index %d of %d, calls: %v", unsetIdx, len(mock.Calls), mock.Calls)
	}
}

func TestRestoreWithMarker_ClearsEvenWhenRestorePanics(t *testing.T) {
	dir := t.TempDir()
	// Need a valid index so Restore reaches list-sessions.
	writeValidIndex(t, dir, sessionsForMarkerTest())

	rf := &markerRunFunc{listSessionsPanic: true}
	mock := &mockCommander{RunFunc: rf.run}
	logger, _ := openTestLogger(t, dir)
	defer func() { _ = logger.Close() }()
	stderr := &bytes.Buffer{}

	o := newOrchestrator(t, mock, dir, logger, stderr)

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = o.RestoreWithMarker()
	}()
	if recovered == nil {
		t.Fatalf("expected RestoreWithMarker to propagate panic; got none")
	}

	if findSetRestoringIdx(mock.Calls) == -1 {
		t.Errorf("expected set @portal-restoring before panic; calls: %v", mock.Calls)
	}
	if findUnsetRestoringIdx(mock.Calls) == -1 {
		t.Errorf("expected deferred clear @portal-restoring after panic; calls: %v", mock.Calls)
	}
}

func TestRestoreWithMarker_ReturnsSetErrorAndSkipsRestore(t *testing.T) {
	dir := t.TempDir()
	writeValidIndex(t, dir, sessionsForMarkerTest())

	rf := &markerRunFunc{setErr: errors.New("set blew up")}
	mock := &mockCommander{RunFunc: rf.run}
	logger, _ := openTestLogger(t, dir)
	defer func() { _ = logger.Close() }()
	stderr := &bytes.Buffer{}

	o := newOrchestrator(t, mock, dir, logger, stderr)
	err := o.RestoreWithMarker()
	if err == nil {
		t.Fatalf("expected error from RestoreWithMarker, got nil")
	}
	if !strings.Contains(err.Error(), "set @portal-restoring") {
		t.Errorf("error %q lacks 'set @portal-restoring' wrapper", err.Error())
	}
	if !strings.Contains(err.Error(), "set blew up") {
		t.Errorf("error %q does not unwrap underlying cause", err.Error())
	}

	// Restore must NOT have been called: no list-sessions, no list-panes,
	// no new-session, no unset.
	if got := callsAt(mock.Calls, "list-sessions"); got != -1 {
		t.Errorf("did not expect list-sessions when set fails; calls: %v", mock.Calls)
	}
	if got := callsAt(mock.Calls, "new-session"); got != -1 {
		t.Errorf("did not expect new-session when set fails; calls: %v", mock.Calls)
	}
	if got := findUnsetRestoringIdx(mock.Calls); got != -1 {
		t.Errorf("did not expect unset when set failed (set never succeeded); calls: %v", mock.Calls)
	}

	// Exactly one set-option call: the failed set.
	setOpts := findAllCalls(mock.Calls, "set-option")
	if len(setOpts) != 1 {
		t.Errorf("expected exactly 1 set-option call (the failed set); got %d, calls: %v", len(setOpts), mock.Calls)
	}
}

func TestRestoreWithMarker_LogsButDoesNotReturnClearError(t *testing.T) {
	dir := t.TempDir()
	rf := &markerRunFunc{unsetErr: errors.New("unset blew up")}
	mock := &mockCommander{RunFunc: rf.run}
	logger, logPath := openTestLogger(t, dir)
	defer func() { _ = logger.Close() }()
	stderr := &bytes.Buffer{}

	o := newOrchestrator(t, mock, dir, logger, stderr)
	if err := o.RestoreWithMarker(); err != nil {
		t.Fatalf("RestoreWithMarker should not propagate clear error; got %v", err)
	}

	_ = logger.Close()
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "WARN") {
		t.Errorf("log %q lacks WARN entry", bodyStr)
	}
	if !strings.Contains(bodyStr, "ClearRestoring") {
		t.Errorf("log %q lacks ClearRestoring marker", bodyStr)
	}
	if !strings.Contains(bodyStr, "unset blew up") {
		t.Errorf("log %q lacks underlying error", bodyStr)
	}
}

func TestRestoreWithMarker_UsesCorrectArgvShapes(t *testing.T) {
	dir := t.TempDir()
	mock := &mockCommander{RunFunc: (&markerRunFunc{}).run}
	logger, _ := openTestLogger(t, dir)
	defer func() { _ = logger.Close() }()
	stderr := &bytes.Buffer{}

	o := newOrchestrator(t, mock, dir, logger, stderr)
	if err := o.RestoreWithMarker(); err != nil {
		t.Fatalf("RestoreWithMarker: %v", err)
	}

	setIdx := findSetRestoringIdx(mock.Calls)
	unsetIdx := findUnsetRestoringIdx(mock.Calls)
	if setIdx == -1 || unsetIdx == -1 {
		t.Fatalf("expected both set and unset calls; calls: %v", mock.Calls)
	}

	// Set: exactly ["set-option", "-s", "@portal-restoring", "1"].
	gotSet := strings.Join(mock.Calls[setIdx], " ")
	wantSet := "set-option -s @portal-restoring 1"
	if gotSet != wantSet {
		t.Errorf("set call argv = %q, want %q", gotSet, wantSet)
	}

	// Unset: exactly ["set-option", "-su", "@portal-restoring"]; NOT 4 elements.
	gotUnset := strings.Join(mock.Calls[unsetIdx], " ")
	wantUnset := "set-option -su @portal-restoring"
	if gotUnset != wantUnset {
		t.Errorf("unset call argv = %q, want %q", gotUnset, wantUnset)
	}
	if len(mock.Calls[unsetIdx]) != 3 {
		t.Errorf("unset call should have 3 argv elements (no value); got %d: %v", len(mock.Calls[unsetIdx]), mock.Calls[unsetIdx])
	}
}

func TestRestoreWithMarker_TolerantOfPreExistingMarker(t *testing.T) {
	// Mock behaves as tmux does: set-option succeeds even when the option is
	// already set to the same value (idempotent). The wrapper makes no special
	// effort to detect or reject this case.
	dir := t.TempDir()
	mock := &mockCommander{RunFunc: (&markerRunFunc{}).run}
	logger, _ := openTestLogger(t, dir)
	defer func() { _ = logger.Close() }()
	stderr := &bytes.Buffer{}

	o := newOrchestrator(t, mock, dir, logger, stderr)
	if err := o.RestoreWithMarker(); err != nil {
		t.Fatalf("RestoreWithMarker: %v", err)
	}

	if findSetRestoringIdx(mock.Calls) == -1 {
		t.Errorf("expected set @portal-restoring; calls: %v", mock.Calls)
	}
	if findUnsetRestoringIdx(mock.Calls) == -1 {
		t.Errorf("expected unset @portal-restoring; calls: %v", mock.Calls)
	}
}

func TestRestoreWithMarker_NeverIssuesEmptyValueSetOption(t *testing.T) {
	dir := t.TempDir()
	writeValidIndex(t, dir, sessionsForMarkerTest())
	mock := &mockCommander{RunFunc: (&markerRunFunc{}).run}
	logger, _ := openTestLogger(t, dir)
	defer func() { _ = logger.Close() }()
	stderr := &bytes.Buffer{}

	o := newOrchestrator(t, mock, dir, logger, stderr)
	if err := o.RestoreWithMarker(); err != nil {
		t.Fatalf("RestoreWithMarker: %v", err)
	}

	for _, c := range mock.Calls {
		// Skip non-set-option calls.
		if len(c) == 0 || c[0] != "set-option" {
			continue
		}
		// Unset form: ["set-option", "-su", name] — exactly 3 elements, no value slot.
		if len(c) >= 2 && c[1] == "-su" {
			continue
		}
		// Set form: ["set-option", "-s", name, value]. Value must not be empty.
		if len(c) < 4 {
			t.Errorf("set-option call missing value slot: %v", c)
			continue
		}
		if c[3] == "" {
			t.Errorf("set-option called with empty-string value: %v", c)
		}
	}
}
