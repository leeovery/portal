package restore_test

import (
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

type orchestratorRunFunc struct {
	listSessionsOut string
	listSessionsErr error
	listPanesOut    string
	listPanesErr    error
	onCmd           map[string]func(args ...string) (string, error)
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

func writeRawIndex(t *testing.T, dir string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(state.SessionsJSON(dir), raw, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}
}

func newOrchestrator(t *testing.T, mock *commandertest.Scripted, dir string, logger *slog.Logger) *restore.Orchestrator {
	t.Helper()
	return restoretest.NewFakeExeOrchestrator(t, tmux.NewClient(mock), dir, logger)
}

func TestOrchestrator_NoOpWhenSessionsJSONAbsent(t *testing.T) {
	dir := t.TempDir()
	mock := commandertest.FromFunc(defaultRunFunc)
	logger, sink := logtest.NewCaptureLogger(t)

	o := newOrchestrator(t, mock, dir, logger)
	corrupt, err := o.Restore()
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if corrupt {
		t.Error("expected corrupt=false on happy path (absent sessions.json); got true")
	}

	if len(mock.Calls()) != 0 {
		t.Errorf("expected no tmux calls when sessions.json absent; got %v", mock.Calls())
	}

	body := []byte(sink.Body())
	if len(body) != 0 {
		t.Errorf("expected empty log; got %q", string(body))
	}
}

func TestOrchestrator_ReturnsWrappedErrCorruptIndexAndLogsWhenSessionsJSONCorrupt(t *testing.T) {
	dir := t.TempDir()
	writeRawIndex(t, dir, []byte("{not json"))

	mock := commandertest.FromFunc(defaultRunFunc)
	logger, sink := logtest.NewCaptureLogger(t)

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

	bodyStr := sink.Body()
	if !strings.Contains(bodyStr, "WARN") || !strings.Contains(bodyStr, "ReadIndex") {
		t.Errorf("log %q lacks WARN/ReadIndex entry", bodyStr)
	}

	for _, c := range mock.Calls() {
		if len(c) > 0 && c[0] == "list-sessions" {
			t.Errorf("did not expect list-sessions when sessions.json corrupt; got %v", mock.Calls())
		}
		if len(c) > 0 && c[0] == "new-session" {
			t.Errorf("did not expect new-session when sessions.json corrupt; got %v", mock.Calls())
		}
	}
}

func TestOrchestrator_OnlyListsSessionsWhenIndexEmpty(t *testing.T) {
	dir := t.TempDir()
	writeValidIndex(t, dir, []state.Session{})

	mock := commandertest.FromFunc(defaultRunFunc)
	logger, _ := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls(), "new-session")); got != 0 {
		t.Errorf("new-session calls = %d, want 0", got)
	}
	if got := len(findAllCalls(mock.Calls(), "list-panes")); got != 0 {
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
		listSessionsOut: "",
		listPanesOut:    "0:0",
	}
	mock := commandertest.FromFunc(rf.run)
	logger, _ := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls(), "new-session")); got != 1 {
		t.Errorf("new-session calls = %d, want 1", got)
	}
	if got := len(findAllCalls(mock.Calls(), "set-environment")); got != 1 {
		t.Errorf("set-environment calls = %d, want 1", got)
	}
	wantMarker := "@portal-skeleton-" + state.SanitizePaneKey("work", 0, 0)
	found := false
	for _, c := range mock.Calls() {
		if len(c) >= 4 && c[0] == "set-option" && c[2] == wantMarker {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected set-option for marker %q; calls: %v", wantMarker, mock.Calls())
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
		listSessionsOut: "work|1|0|",
	}
	mock := commandertest.FromFunc(rf.run)
	logger, sink := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls(), "new-session")); got != 0 {
		t.Errorf("new-session calls = %d, want 0 (live session must be skipped)", got)
	}

	body := []byte(sink.Body())
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
	mock := commandertest.FromFunc(rf.run)
	logger, sink := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls(), "new-session")); got != 0 {
		t.Errorf("new-session calls = %d, want 0 (underscore-prefixed must be skipped)", got)
	}

	body := []byte(sink.Body())
	if !strings.Contains(string(body), "underscore-prefixed") {
		t.Errorf("expected log entry mentioning underscore-prefixed; got %q", string(body))
	}
}

func TestOrchestrator_LogsAndSkipsZeroWindowSession(t *testing.T) {
	dir := t.TempDir()
	sess := state.Session{Name: "work", Windows: []state.Window{}}
	writeValidIndex(t, dir, []state.Session{sess})

	rf := &orchestratorRunFunc{listSessionsOut: ""}
	mock := commandertest.FromFunc(rf.run)
	logger, sink := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls(), "new-session")); got != 0 {
		t.Errorf("new-session calls = %d, want 0 (zero-window must be skipped)", got)
	}

	body := []byte(sink.Body())
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
	mock := commandertest.FromFunc(rf.run)
	logger, sink := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls(), "new-session")); got != 0 {
		t.Errorf("new-session calls = %d, want 0 (zero-pane window must be skipped)", got)
	}

	body := []byte(sink.Body())
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "zero panes") {
		t.Errorf("expected log entry mentioning zero panes; got %q", bodyStr)
	}
}

func TestOrchestrator_NoGeometrySummaryForZeroPaneSession(t *testing.T) {
	dir := t.TempDir()
	sess := state.Session{
		Name: "work",
		Windows: []state.Window{
			{Index: 0, Name: "main", Panes: []state.Pane{}},
		},
	}
	writeValidIndex(t, dir, []state.Session{sess})

	rf := &orchestratorRunFunc{listSessionsOut: ""}
	mock := commandertest.FromFunc(rf.run)
	logger, sink := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if strings.Contains(sink.Body(), "geometry complete") {
		t.Errorf("zero-pane session must not emit a geometry-complete summary; got %q", sink.Body())
	}
}

func TestOrchestrator_IsolatesPerSessionErrors(t *testing.T) {
	dir := t.TempDir()
	stateOK := dir
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
				for i, a := range args {
					if a == "-s" && i+1 < len(args) && args[i+1] == "broken" {
						return "", errors.New("new-session boom")
					}
				}
				return "", nil
			},
		},
	}
	mock := commandertest.FromFunc(rf.run)
	logger, sink := logtest.NewCaptureLogger(t)

	o := newOrchestrator(t, mock, stateOK, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls(), "new-session")); got != 2 {
		t.Errorf("new-session calls = %d, want 2 (broken + ok)", got)
	}
	wantMarker := "@portal-skeleton-" + state.SanitizePaneKey("ok", 0, 0)
	foundOK := false
	for _, c := range mock.Calls() {
		if len(c) >= 4 && c[0] == "set-option" && c[2] == wantMarker {
			foundOK = true
			break
		}
	}
	if !foundOK {
		t.Errorf("expected ok-session marker %q to be set despite broken-session failure; calls: %v", wantMarker, mock.Calls())
	}

	body := []byte(sink.Body())
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
		listSessionsOut: "malformed-line",
	}
	mock := commandertest.FromFunc(rf.run)
	logger, sink := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if got := len(findAllCalls(mock.Calls(), "new-session")); got != 0 {
		t.Errorf("new-session calls = %d, want 0 when list-sessions fails", got)
	}

	body := []byte(sink.Body())
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
	mock := commandertest.FromFunc(rf.run)
	logger, _ := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore returned error %v, expected nil even when every session errors", err)
	}

	if got := len(findAllCalls(mock.Calls(), "new-session")); got != 2 {
		t.Errorf("new-session calls = %d, want 2 (per-session isolation)", got)
	}
}

func skeletonSummaryLine(t *testing.T, sink *logtest.Sink) string {
	t.Helper()
	var found []string
	for _, line := range sink.Lines() {
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
	mock := commandertest.FromFunc(rf.run)
	logger, sink := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	line := skeletonSummaryLine(t, sink)
	if line == "" {
		t.Fatalf("expected one skeleton-complete summary; sink body:\n%s", sink.Body())
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
		{Name: "live", Windows: []state.Window{
			{Index: 0, Name: "main", Panes: []state.Pane{
				{Index: 0, CWD: "/live", ScrollbackFile: "scrollback/live__0.0.bin"},
				{Index: 1, CWD: "/live", ScrollbackFile: "scrollback/live__0.1.bin"},
			}},
		}},
	}
	writeValidIndex(t, dir, sessions)

	rf := &orchestratorRunFunc{
		listSessionsOut: "live|1|0|",
		listPanesOut:    "0:0",
	}
	mock := commandertest.FromFunc(rf.run)
	logger, sink := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	line := skeletonSummaryLine(t, sink)
	if line == "" {
		t.Fatalf("expected one skeleton-complete summary; sink body:\n%s", sink.Body())
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
	mock := commandertest.FromFunc(rf.run)
	logger, sink := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	line := skeletonSummaryLine(t, sink)
	if line == "" {
		t.Fatalf("expected one skeleton-complete summary; sink body:\n%s", sink.Body())
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
		{Name: "nowin", Windows: []state.Window{}},
		{Name: "nopane", Windows: []state.Window{
			{Index: 0, Name: "main", Panes: []state.Pane{}},
		}},
	}
	writeValidIndex(t, dir, sessions)

	rf := &orchestratorRunFunc{listSessionsOut: "", listPanesOut: "0:0"}
	mock := commandertest.FromFunc(rf.run)
	logger, sink := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	line := skeletonSummaryLine(t, sink)
	if line == "" {
		t.Fatalf("expected one skeleton-complete summary; sink body:\n%s", sink.Body())
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
	mock := commandertest.FromFunc(rf.run)
	logger, sink := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	line := skeletonSummaryLine(t, sink)
	if line == "" {
		t.Fatalf("expected one skeleton-complete summary; sink body:\n%s", sink.Body())
	}
	for _, want := range []string{"sessions=1", "windows=1", "panes=1"} {
		if !strings.Contains(line, want) {
			t.Errorf("summary %q missing %q (errored session must be excluded)", line, want)
		}
	}
	body := sink.Body()
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
			mock := commandertest.FromFunc(rf.run)
			logger, sink := logtest.NewCaptureLogger(t)
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

	mock := commandertest.FromFunc(defaultRunFunc)
	logger, sink := logtest.NewCaptureLogger(t)
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
				return "", errors.New("layout failed")
			},
		},
	}
	mock := commandertest.FromFunc(rf.run)
	logger, _ := logtest.NewCaptureLogger(t)
	o := newOrchestrator(t, mock, dir, logger)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	newSessionAt := callsAt(mock.Calls(), "new-session")
	listPanesIdxs := findAllCalls(mock.Calls(), "list-panes")
	layoutAt := callsAt(mock.Calls(), "select-layout")
	setOptAt := callsAt(mock.Calls(), "set-option")

	if newSessionAt < 0 || len(listPanesIdxs) != 1 || layoutAt < 0 || setOptAt < 0 {
		t.Fatalf("expected calls present (with exactly 1 list-panes): new-session=%d list-panes=%v select-layout=%d set-option=%d; calls: %v",
			newSessionAt, listPanesIdxs, layoutAt, setOptAt, mock.Calls())
	}
	armListPanesAt := listPanesIdxs[0]
	if newSessionAt >= armListPanesAt || armListPanesAt >= layoutAt || layoutAt >= setOptAt {
		t.Errorf("ordering violated: new-session(%d) < list-panes-arm(%d) < select-layout(%d) < set-option(%d) failed",
			newSessionAt, armListPanesAt, layoutAt, setOptAt)
	}
}

func TestOrchestrator_RebuildsEverySavedSessionWhenListSessionsFails(t *testing.T) {
	dir := t.TempDir()
	saved := []state.Session{
		newSession("work", nil, state.Window{Index: 0, Panes: []state.Pane{{Index: 0, CWD: "/w", ScrollbackFile: "scrollback/work__0.0.bin", Active: true}}}),
		newSession("notes", nil, state.Window{Index: 0, Panes: []state.Pane{{Index: 0, CWD: "/n", ScrollbackFile: "scrollback/notes__0.0.bin", Active: true}}}),
	}
	writeValidIndex(t, dir, saved)
	sessionsBefore, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read seeded sessions.json: %v", err)
	}
	listErr := &tmux.CommandError{Args: []string{"list-sessions"}, Stderr: "server exited unexpectedly", Err: errors.New("exit status 1")}
	rf := &orchestratorRunFunc{listSessionsErr: listErr, listPanesOut: "0:0"}
	mock := commandertest.FromFunc(rf.run)
	logger, _ := logtest.NewCaptureLogger(t)
	client := tmux.NewClient(mock)

	if _, err := restoretest.NewFakeExeOrchestrator(t, client, dir, logger).Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	var created []string
	for _, i := range findAllCalls(mock.Calls(), "new-session") {
		call := mock.Calls()[i]
		for j := 0; j+1 < len(call); j++ {
			if call[j] == "-s" {
				created = append(created, call[j+1])
			}
		}
	}
	if strings.Join(created, ",") != "work,notes" {
		t.Errorf("sessions created = %v, want every saved session [work notes]", created)
	}

	if _, err := state.RunCommitCycle(state.CommitCycle{
		Client:   client,
		Dir:      dir,
		LoadPrev: func() *state.Index { return &state.Index{} },
	}); !errors.Is(err, listErr) {
		t.Errorf("commit cycle after restore: error = %v, want one wrapping the failed list-sessions", err)
	}
	sessionsAfter, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	if string(sessionsAfter) != string(sessionsBefore) {
		t.Errorf("sessions.json rewritten after restore:\nbefore %s\nafter  %s", sessionsBefore, sessionsAfter)
	}
}
