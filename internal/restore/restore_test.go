package restore_test

import (
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// orchestratorRunFunc returns a RunFunc that dispatches list-sessions and
// list-panes to per-call hooks, and treats every other command as a
// successful no-op. Hooks may be nil; nil hooks pass through to the default
// success behavior.
type orchestratorRunFunc struct {
	listSessionsOut string
	listSessionsErr error
	listPanesOut    string
	listPanesErr    error
	// onCmd lets a specific command be intercepted (e.g., return an error on
	// the first new-session call to drive a per-session-failure path).
	onCmd map[string]func(args ...string) (string, error)
}

func (o *orchestratorRunFunc) run(args ...string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	cmd := args[0]
	if o.onCmd != nil {
		if hook, ok := o.onCmd[cmd]; ok && hook != nil {
			return hook(args...)
		}
	}
	switch cmd {
	case "list-sessions":
		return o.listSessionsOut, o.listSessionsErr
	case "list-panes":
		return o.listPanesOut, o.listPanesErr
	}
	return "", nil
}

// writeValidIndex writes a minimally valid sessions.json for the supplied
// sessions to dir/sessions.json. Returns the canonical absolute path.
func writeValidIndex(t *testing.T, dir string, sessions []state.Session) {
	t.Helper()
	idx := state.Index{
		Version:  state.SchemaVersion,
		SavedAt:  time.Now().UTC(),
		Sessions: sessions,
	}
	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("encode sessions.json: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(dir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}
}

// writeRawIndex writes raw bytes to dir/sessions.json. Used to drive the
// corrupt-JSON path.
func writeRawIndex(t *testing.T, dir string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(state.SessionsJSON(dir), raw, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}
}

// orchestrator builds an Orchestrator wired against the supplied mock
// commander, state directory, and optional logger. Stderr emission was
// removed in Phase 6 task 6-9 — the corrupt-index case now returns a
// wrapped state.ErrCorruptIndex and the bootstrap orchestrator surfaces
// the user-facing warning via cmd.BootstrapWarningsSink.
func newOrchestrator(t *testing.T, mock *mockCommander, dir string, logger *slog.Logger) *restore.Orchestrator {
	t.Helper()
	client := tmux.NewClient(mock)
	return &restore.Orchestrator{
		Client:   client,
		StateDir: dir,
		Logger:   logger,
	}
}

// openTestLogger returns a capturing *slog.Logger plus its captureSink so
// call sites can assert on the rendered log body. Replaces the old
// file-backed logger after the observability migration; the body is
// now in-memory.
func openTestLogger(t *testing.T, dir string) (*slog.Logger, *captureSink) {
	t.Helper()
	_ = dir
	return newCaptureLogger(t)
}

func TestOrchestrator_NoOpWhenSessionsJSONAbsent(t *testing.T) {
	dir := t.TempDir()
	mock := &mockCommander{RunFunc: defaultRunFunc}
	logger, sink := openTestLogger(t, dir)

	o := newOrchestrator(t, mock, dir, logger)
	corrupt, err := o.Restore()
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if corrupt {
		t.Error("expected corrupt=false on happy path (absent sessions.json); got true")
	}

	if len(mock.Calls) != 0 {
		t.Errorf("expected no tmux calls when sessions.json absent; got %v", mock.Calls)
	}

	body := []byte(sink.body())
	if len(body) != 0 {
		t.Errorf("expected empty log; got %q", string(body))
	}
}

func TestOrchestrator_ReturnsWrappedErrCorruptIndexAndLogsWhenSessionsJSONCorrupt(t *testing.T) {
	dir := t.TempDir()
	writeRawIndex(t, dir, []byte("{not json"))

	mock := &mockCommander{RunFunc: defaultRunFunc}
	logger, sink := openTestLogger(t, dir)

	o := newOrchestrator(t, mock, dir, logger)
	corrupt, err := o.Restore()
	if err == nil {
		t.Fatal("expected Restore to return wrapped ErrCorruptIndex; got nil")
	}
	if !corrupt {
		t.Error("expected corrupt=true on corrupt-index path; got false")
	}
	if !errors.Is(err, state.ErrCorruptIndex) {
		t.Errorf("errors.Is(err, state.ErrCorruptIndex) = false; want true. err=%v", err)
	}

	bodyStr := sink.body()
	if !strings.Contains(bodyStr, "WARN") || !strings.Contains(bodyStr, "ReadIndex") {
		t.Errorf("log %q lacks WARN/ReadIndex entry", bodyStr)
	}

	// No tmux calls should have happened — restoration is fully skipped.
	for _, c := range mock.Calls {
		if len(c) > 0 && c[0] == "list-sessions" {
			t.Errorf("did not expect list-sessions when sessions.json corrupt; got %v", mock.Calls)
		}
		if len(c) > 0 && c[0] == "new-session" {
			t.Errorf("did not expect new-session when sessions.json corrupt; got %v", mock.Calls)
		}
	}
}

func TestOrchestrator_OnlyListsSessionsWhenIndexEmpty(t *testing.T) {
	dir := t.TempDir()
	writeValidIndex(t, dir, []state.Session{})

	mock := &mockCommander{RunFunc: defaultRunFunc}
	logger, _ := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// No new-session, no list-panes, no list-sessions either — empty Sessions
	// returns before listing live names.
	if got := len(findAllCalls(mock.Calls, "new-session")); got != 0 {
		t.Errorf("new-session calls = %d, want 0", got)
	}
	if got := len(findAllCalls(mock.Calls, "list-panes")); got != 0 {
		t.Errorf("list-panes calls = %d, want 0", got)
	}
}

func TestOrchestrator_SkeletonRestoresSingleMissingSession(t *testing.T) {
	dir := t.TempDir()
	sess := state.Session{
		Name:        "work",
		Environment: map[string]string{"LANG": "en_US.UTF-8"},
		Windows: []state.Window{
			{
				Index: 0,
				Name:  "main",
				Panes: []state.Pane{
					{Index: 0, CWD: "/work", ScrollbackFile: "scrollback/work__0.0.bin", Active: true},
				},
			},
		},
	}
	writeValidIndex(t, dir, []state.Session{sess})

	rf := &orchestratorRunFunc{
		listSessionsOut: "", // no live sessions
		listPanesOut:    "0:0",
	}
	mock := &mockCommander{RunFunc: rf.run}
	logger, _ := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls, "new-session")); got != 1 {
		t.Errorf("new-session calls = %d, want 1", got)
	}
	if got := len(findAllCalls(mock.Calls, "set-environment")); got != 1 {
		t.Errorf("set-environment calls = %d, want 1", got)
	}
	// One @portal-skeleton- marker for the single live pane.
	wantMarker := "@portal-skeleton-" + state.SanitizePaneKey("work", 0, 0)
	found := false
	for _, c := range mock.Calls {
		if len(c) >= 4 && c[0] == "set-option" && c[2] == wantMarker {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected set-option for marker %q; calls: %v", wantMarker, mock.Calls)
	}
}

func TestOrchestrator_SilentlySkipsLiveSession(t *testing.T) {
	dir := t.TempDir()
	sess := state.Session{
		Name: "work",
		Windows: []state.Window{
			{Index: 0, Panes: []state.Pane{{Index: 0, CWD: "/work", ScrollbackFile: "scrollback/x.bin"}}},
		},
	}
	writeValidIndex(t, dir, []state.Session{sess})

	rf := &orchestratorRunFunc{
		// Live session named "work" already exists.
		listSessionsOut: "work|1|0",
	}
	mock := &mockCommander{RunFunc: rf.run}
	logger, sink := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls, "new-session")); got != 0 {
		t.Errorf("new-session calls = %d, want 0 (live session must be skipped)", got)
	}

	body := []byte(sink.body())
	if strings.Contains(string(body), "WARN") {
		t.Errorf("expected no log entries on silent live-skip; got %q", string(body))
	}
}

func TestOrchestrator_SkipsUnderscorePrefixedSessions(t *testing.T) {
	dir := t.TempDir()
	sess := state.Session{
		Name: "_portal-saver",
		Windows: []state.Window{
			{Index: 0, Panes: []state.Pane{{Index: 0, CWD: "/work", ScrollbackFile: "scrollback/x.bin"}}},
		},
	}
	writeValidIndex(t, dir, []state.Session{sess})

	rf := &orchestratorRunFunc{listSessionsOut: ""}
	mock := &mockCommander{RunFunc: rf.run}
	logger, sink := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls, "new-session")); got != 0 {
		t.Errorf("new-session calls = %d, want 0 (underscore-prefixed must be skipped)", got)
	}

	body := []byte(sink.body())
	if !strings.Contains(string(body), "underscore-prefixed") {
		t.Errorf("expected log entry mentioning underscore-prefixed; got %q", string(body))
	}
}

func TestOrchestrator_LogsAndSkipsZeroWindowSession(t *testing.T) {
	dir := t.TempDir()
	sess := state.Session{Name: "work", Windows: []state.Window{}}
	writeValidIndex(t, dir, []state.Session{sess})

	rf := &orchestratorRunFunc{listSessionsOut: ""}
	mock := &mockCommander{RunFunc: rf.run}
	logger, sink := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls, "new-session")); got != 0 {
		t.Errorf("new-session calls = %d, want 0 (zero-window must be skipped)", got)
	}

	body := []byte(sink.body())
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "zero windows") {
		t.Errorf("expected log entry mentioning zero windows; got %q", bodyStr)
	}
}

func TestOrchestrator_LogsAndSkipsZeroPaneWindow(t *testing.T) {
	dir := t.TempDir()
	sess := state.Session{
		Name: "work",
		Windows: []state.Window{
			{Index: 0, Name: "main", Panes: []state.Pane{}},
		},
	}
	writeValidIndex(t, dir, []state.Session{sess})

	rf := &orchestratorRunFunc{listSessionsOut: ""}
	mock := &mockCommander{RunFunc: rf.run}
	logger, sink := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls, "new-session")); got != 0 {
		t.Errorf("new-session calls = %d, want 0 (zero-pane window must be skipped)", got)
	}

	body := []byte(sink.body())
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "zero panes") {
		t.Errorf("expected log entry mentioning zero panes; got %q", bodyStr)
	}
}

func TestOrchestrator_IsolatesPerSessionErrors(t *testing.T) {
	// First session: "broken" — uses a non-existent state subdir for FIFO
	// creation so Restore returns an error. Second session: "ok" — succeeds.
	dir := t.TempDir()
	stateOK := dir
	// "broken" will use a different StateDir via a separate Orchestrator? No;
	// the orchestrator owns one StateDir. Instead drive a per-call hook to
	// fail "broken"'s new-session call.
	sessBroken := state.Session{
		Name: "broken",
		Windows: []state.Window{
			{Index: 0, Name: "m", Panes: []state.Pane{{Index: 0, CWD: "/x", ScrollbackFile: "scrollback/x.bin"}}},
		},
	}
	sessOK := state.Session{
		Name: "ok",
		Windows: []state.Window{
			{Index: 0, Name: "m", Panes: []state.Pane{{Index: 0, CWD: "/y", ScrollbackFile: "scrollback/y.bin"}}},
		},
	}
	writeValidIndex(t, stateOK, []state.Session{sessBroken, sessOK})

	rf := &orchestratorRunFunc{
		listSessionsOut: "",
		listPanesOut:    "0:0",
		onCmd: map[string]func(args ...string) (string, error){
			"new-session": func(args ...string) (string, error) {
				// Fail iff -s is "broken".
				for i, a := range args {
					if a == "-s" && i+1 < len(args) && args[i+1] == "broken" {
						return "", errors.New("new-session boom")
					}
				}
				return "", nil
			},
		},
	}
	mock := &mockCommander{RunFunc: rf.run}
	logger, sink := openTestLogger(t, stateOK)

	o := newOrchestrator(t, mock, stateOK, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Two new-session attempts — one failed, one succeeded.
	if got := len(findAllCalls(mock.Calls, "new-session")); got != 2 {
		t.Errorf("new-session calls = %d, want 2 (broken + ok)", got)
	}
	// The "ok" session's marker must still have been set.
	wantMarker := "@portal-skeleton-" + state.SanitizePaneKey("ok", 0, 0)
	foundOK := false
	for _, c := range mock.Calls {
		if len(c) >= 4 && c[0] == "set-option" && c[2] == wantMarker {
			foundOK = true
			break
		}
	}
	if !foundOK {
		t.Errorf("expected ok-session marker %q to be set despite broken-session failure; calls: %v", wantMarker, mock.Calls)
	}

	body := []byte(sink.body())
	if !strings.Contains(string(body), "broken") {
		t.Errorf("expected log to mention broken session; got %q", string(body))
	}
}

func TestOrchestrator_LogsAndReturnsNilWhenListSessionsFails(t *testing.T) {
	dir := t.TempDir()
	sess := state.Session{
		Name: "work",
		Windows: []state.Window{
			{Index: 0, Panes: []state.Pane{{Index: 0, CWD: "/w", ScrollbackFile: "scrollback/x.bin"}}},
		},
	}
	writeValidIndex(t, dir, []state.Session{sess})

	rf := &orchestratorRunFunc{
		// Malformed line — ListSessions fails parsing.
		listSessionsOut: "malformed-line",
	}
	mock := &mockCommander{RunFunc: rf.run}
	logger, sink := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	// No new-session attempts — list-sessions failure aborts.
	if got := len(findAllCalls(mock.Calls, "new-session")); got != 0 {
		t.Errorf("new-session calls = %d, want 0 when list-sessions fails", got)
	}

	body := []byte(sink.body())
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "list-sessions") {
		t.Errorf("expected log entry mentioning list-sessions; got %q", bodyStr)
	}
}

func TestOrchestrator_ReturnsNilWhenEverySessionErrors(t *testing.T) {
	dir := t.TempDir()
	sessions := []state.Session{
		{Name: "a", Windows: []state.Window{{Index: 0, Panes: []state.Pane{{Index: 0, CWD: "/a", ScrollbackFile: "scrollback/a.bin"}}}}},
		{Name: "b", Windows: []state.Window{{Index: 0, Panes: []state.Pane{{Index: 0, CWD: "/b", ScrollbackFile: "scrollback/b.bin"}}}}},
	}
	writeValidIndex(t, dir, sessions)

	rf := &orchestratorRunFunc{
		listSessionsOut: "",
		onCmd: map[string]func(args ...string) (string, error){
			"new-session": func(args ...string) (string, error) {
				return "", errors.New("always-fail")
			},
		},
	}
	mock := &mockCommander{RunFunc: rf.run}
	logger, _ := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore returned error %v, expected nil even when every session errors", err)
	}

	// Both new-sessions still attempted.
	if got := len(findAllCalls(mock.Calls, "new-session")); got != 2 {
		t.Errorf("new-session calls = %d, want 2 (per-session isolation)", got)
	}
}

// skeletonSummaryLine returns the single "skeleton complete" INFO line the
// sink recorded, or "" if none was emitted. Fails the test if more than one
// such line was recorded (the spec mandates exactly one per restore cycle).
func skeletonSummaryLine(t *testing.T, sink *captureSink) string {
	t.Helper()
	var found []string
	for _, line := range sink.lines {
		if strings.Contains(line, "skeleton complete") {
			found = append(found, line)
		}
	}
	if len(found) > 1 {
		t.Fatalf("expected at most one skeleton-complete summary; got %d: %v", len(found), found)
	}
	if len(found) == 0 {
		return ""
	}
	return found[0]
}

func TestOrchestrator_EmitsSkeletonCompleteSummaryAfterRestoringSessions(t *testing.T) {
	dir := t.TempDir()
	// Two restorable sessions: "work" (1 window / 1 pane) and "side"
	// (2 windows: 2 panes + 1 pane = 3 panes). Restored totals must be
	// sessions=2, windows=3, panes=4.
	sessions := []state.Session{
		{
			Name: "work",
			Windows: []state.Window{
				{Index: 0, Name: "main", Panes: []state.Pane{
					{Index: 0, CWD: "/work", ScrollbackFile: "scrollback/work__0.0.bin", Active: true},
				}},
			},
		},
		{
			Name: "side",
			Windows: []state.Window{
				{Index: 0, Name: "a", Panes: []state.Pane{
					{Index: 0, CWD: "/side", ScrollbackFile: "scrollback/side__0.0.bin", Active: true},
					{Index: 1, CWD: "/side", ScrollbackFile: "scrollback/side__0.1.bin"},
				}},
				{Index: 1, Name: "b", Panes: []state.Pane{
					{Index: 0, CWD: "/side", ScrollbackFile: "scrollback/side__1.0.bin"},
				}},
			},
		},
	}
	writeValidIndex(t, dir, sessions)

	rf := &orchestratorRunFunc{
		listSessionsOut: "",
		listPanesOut:    "0:0",
	}
	mock := &mockCommander{RunFunc: rf.run}
	logger, sink := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	line := skeletonSummaryLine(t, sink)
	if line == "" {
		t.Fatalf("expected one skeleton-complete summary; sink body:\n%s", sink.body())
	}
	if !strings.HasPrefix(line, "INFO ") {
		t.Errorf("summary level = %q, want INFO. line=%q", line, line)
	}
	for _, want := range []string{"sessions=2", "windows=3", "panes=4", "took="} {
		if !strings.Contains(line, want) {
			t.Errorf("summary %q missing %q", line, want)
		}
	}
}

func TestOrchestrator_SkeletonSummaryExcludesLiveSkippedSession(t *testing.T) {
	dir := t.TempDir()
	sessions := []state.Session{
		{Name: "work", Windows: []state.Window{
			{Index: 0, Name: "main", Panes: []state.Pane{
				{Index: 0, CWD: "/work", ScrollbackFile: "scrollback/work__0.0.bin", Active: true},
			}},
		}},
		// "live" already exists in tmux — must be excluded from all counts.
		{Name: "live", Windows: []state.Window{
			{Index: 0, Name: "main", Panes: []state.Pane{
				{Index: 0, CWD: "/live", ScrollbackFile: "scrollback/live__0.0.bin"},
				{Index: 1, CWD: "/live", ScrollbackFile: "scrollback/live__0.1.bin"},
			}},
		}},
	}
	writeValidIndex(t, dir, sessions)

	rf := &orchestratorRunFunc{
		listSessionsOut: "live|1|0",
		listPanesOut:    "0:0",
	}
	mock := &mockCommander{RunFunc: rf.run}
	logger, sink := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	line := skeletonSummaryLine(t, sink)
	if line == "" {
		t.Fatalf("expected one skeleton-complete summary; sink body:\n%s", sink.body())
	}
	for _, want := range []string{"sessions=1", "windows=1", "panes=1"} {
		if !strings.Contains(line, want) {
			t.Errorf("summary %q missing %q (live session must be excluded)", line, want)
		}
	}
}

func TestOrchestrator_SkeletonSummaryExcludesUnderscorePrefixedSession(t *testing.T) {
	dir := t.TempDir()
	sessions := []state.Session{
		{Name: "work", Windows: []state.Window{
			{Index: 0, Name: "main", Panes: []state.Pane{
				{Index: 0, CWD: "/work", ScrollbackFile: "scrollback/work__0.0.bin", Active: true},
			}},
		}},
		{Name: "_portal-saver", Windows: []state.Window{
			{Index: 0, Name: "main", Panes: []state.Pane{
				{Index: 0, CWD: "/x", ScrollbackFile: "scrollback/x.bin"},
			}},
		}},
	}
	writeValidIndex(t, dir, sessions)

	rf := &orchestratorRunFunc{listSessionsOut: "", listPanesOut: "0:0"}
	mock := &mockCommander{RunFunc: rf.run}
	logger, sink := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	line := skeletonSummaryLine(t, sink)
	if line == "" {
		t.Fatalf("expected one skeleton-complete summary; sink body:\n%s", sink.body())
	}
	for _, want := range []string{"sessions=1", "windows=1", "panes=1"} {
		if !strings.Contains(line, want) {
			t.Errorf("summary %q missing %q (underscore session must be excluded)", line, want)
		}
	}
}

func TestOrchestrator_SkeletonSummaryExcludesInvalidTopologySessions(t *testing.T) {
	dir := t.TempDir()
	sessions := []state.Session{
		{Name: "work", Windows: []state.Window{
			{Index: 0, Name: "main", Panes: []state.Pane{
				{Index: 0, CWD: "/work", ScrollbackFile: "scrollback/work__0.0.bin", Active: true},
			}},
		}},
		// zero windows — invalid topology, excluded.
		{Name: "nowin", Windows: []state.Window{}},
		// a window with zero panes — invalid topology, excluded.
		{Name: "nopane", Windows: []state.Window{
			{Index: 0, Name: "main", Panes: []state.Pane{}},
		}},
	}
	writeValidIndex(t, dir, sessions)

	rf := &orchestratorRunFunc{listSessionsOut: "", listPanesOut: "0:0"}
	mock := &mockCommander{RunFunc: rf.run}
	logger, sink := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	line := skeletonSummaryLine(t, sink)
	if line == "" {
		t.Fatalf("expected one skeleton-complete summary; sink body:\n%s", sink.body())
	}
	for _, want := range []string{"sessions=1", "windows=1", "panes=1"} {
		if !strings.Contains(line, want) {
			t.Errorf("summary %q missing %q (invalid-topology sessions must be excluded)", line, want)
		}
	}
}

func TestOrchestrator_SkeletonSummaryExcludesRestoreErroredSessionButKeepsWarn(t *testing.T) {
	dir := t.TempDir()
	sessions := []state.Session{
		{Name: "broken", Windows: []state.Window{
			{Index: 0, Name: "m", Panes: []state.Pane{
				{Index: 0, CWD: "/x", ScrollbackFile: "scrollback/x.bin"},
			}},
		}},
		{Name: "ok", Windows: []state.Window{
			{Index: 0, Name: "m", Panes: []state.Pane{
				{Index: 0, CWD: "/y", ScrollbackFile: "scrollback/y.bin"},
			}},
		}},
	}
	writeValidIndex(t, dir, sessions)

	rf := &orchestratorRunFunc{
		listSessionsOut: "",
		listPanesOut:    "0:0",
		onCmd: map[string]func(args ...string) (string, error){
			"new-session": func(args ...string) (string, error) {
				for i, a := range args {
					if a == "-s" && i+1 < len(args) && args[i+1] == "broken" {
						return "", errors.New("new-session boom")
					}
				}
				return "", nil
			},
		},
	}
	mock := &mockCommander{RunFunc: rf.run}
	logger, sink := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	line := skeletonSummaryLine(t, sink)
	if line == "" {
		t.Fatalf("expected one skeleton-complete summary; sink body:\n%s", sink.body())
	}
	// Only "ok" was restored: sessions=1, windows=1, panes=1.
	for _, want := range []string{"sessions=1", "windows=1", "panes=1"} {
		if !strings.Contains(line, want) {
			t.Errorf("summary %q missing %q (errored session must be excluded)", line, want)
		}
	}
	// The per-session WARN for "broken" must still fire.
	body := sink.body()
	if !strings.Contains(body, "WARN") || !strings.Contains(body, "broken") {
		t.Errorf("expected per-session WARN for broken session; body:\n%s", body)
	}
}

func TestOrchestrator_EmitsNoSkeletonSummaryOnPreLoopEarlyReturns(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, dir string) *orchestratorRunFunc
	}{
		{
			name: "sessions.json absent",
			setup: func(_ *testing.T, _ string) *orchestratorRunFunc {
				return &orchestratorRunFunc{}
			},
		},
		{
			name: "zero saved sessions",
			setup: func(t *testing.T, dir string) *orchestratorRunFunc {
				writeValidIndex(t, dir, []state.Session{})
				return &orchestratorRunFunc{listSessionsOut: ""}
			},
		},
		{
			name: "list-sessions fails",
			setup: func(t *testing.T, dir string) *orchestratorRunFunc {
				writeValidIndex(t, dir, []state.Session{
					{Name: "work", Windows: []state.Window{
						{Index: 0, Panes: []state.Pane{{Index: 0, CWD: "/w", ScrollbackFile: "scrollback/x.bin"}}},
					}},
				})
				return &orchestratorRunFunc{listSessionsOut: "malformed-line"}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			rf := tc.setup(t, dir)
			mock := &mockCommander{RunFunc: rf.run}
			logger, sink := openTestLogger(t, dir)
			o := newOrchestrator(t, mock, dir, logger)
			if _, err := o.Restore(); err != nil {
				t.Fatalf("Restore: %v", err)
			}
			if line := skeletonSummaryLine(t, sink); line != "" {
				t.Errorf("expected no skeleton-complete summary on early return; got %q", line)
			}
		})
	}
}

func TestOrchestrator_EmitsNoSkeletonSummaryOnCorruptIndex(t *testing.T) {
	dir := t.TempDir()
	writeRawIndex(t, dir, []byte("{not json"))

	mock := &mockCommander{RunFunc: defaultRunFunc}
	logger, sink := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	corrupt, err := o.Restore()
	if err == nil {
		t.Fatal("expected wrapped ErrCorruptIndex; got nil")
	}
	if !corrupt {
		t.Error("expected corrupt=true; got false")
	}
	if !errors.Is(err, state.ErrCorruptIndex) {
		t.Errorf("errors.Is(err, ErrCorruptIndex) = false; want true. err=%v", err)
	}
	if line := skeletonSummaryLine(t, sink); line != "" {
		t.Errorf("expected no skeleton-complete summary on corrupt index; got %q", line)
	}
}

func TestOrchestrator_AlwaysRunsApplySkeletonMarkersAfterApplyWindowGeometry(t *testing.T) {
	// ApplyWindowGeometry never fails the orchestrator (it returns void).
	// Markers must always run after geometry — drive a session with a layout
	// that fails (forcing fallback) and assert the call ordering. After the
	// 7-9 re-query rework only ONE list-panes call exists per session — the
	// arm phase queries it, then threads the resulting []tmux.PaneCoord
	// through to ApplyWindowGeometry and ApplySkeletonMarkers (neither of
	// which calls list-panes themselves). Expected ordering:
	//   new-session → list-panes (arm) → respawn-pane → select-layout → set-option
	// The invariant under test is that geometry's first call (select-layout)
	// runs before markers' first call (set-option).
	dir := t.TempDir()
	sess := state.Session{
		Name: "work",
		Windows: []state.Window{
			{Index: 0, Name: "main", Layout: "broken-layout", Active: true,
				Panes: []state.Pane{{Index: 0, CWD: "/w", ScrollbackFile: "scrollback/x.bin", Active: true}}},
		},
	}
	writeValidIndex(t, dir, []state.Session{sess})

	rf := &orchestratorRunFunc{
		listSessionsOut: "",
		listPanesOut:    "0:0",
		onCmd: map[string]func(args ...string) (string, error){
			"select-layout": func(args ...string) (string, error) {
				// Fail saved layout and tiled fallback alike, exercising the
				// full geometry failure-tolerance path.
				return "", errors.New("layout failed")
			},
		},
	}
	mock := &mockCommander{RunFunc: rf.run}
	logger, _ := openTestLogger(t, dir)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	newSessionAt := callsAt(mock.Calls, "new-session")
	listPanesIdxs := findAllCalls(mock.Calls, "list-panes")
	layoutAt := callsAt(mock.Calls, "select-layout")
	setOptAt := callsAt(mock.Calls, "set-option")

	if newSessionAt < 0 || len(listPanesIdxs) != 1 || layoutAt < 0 || setOptAt < 0 {
		t.Fatalf("expected calls present (with exactly 1 list-panes): new-session=%d list-panes=%v select-layout=%d set-option=%d; calls: %v",
			newSessionAt, listPanesIdxs, layoutAt, setOptAt, mock.Calls)
	}
	armListPanesAt := listPanesIdxs[0]
	if newSessionAt >= armListPanesAt || armListPanesAt >= layoutAt || layoutAt >= setOptAt {
		t.Errorf("ordering violated: new-session(%d) < list-panes-arm(%d) < select-layout(%d) < set-option(%d) failed",
			newSessionAt, armListPanesAt, layoutAt, setOptAt)
	}
}
