package restore_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

type progressCall struct {
	n int
	m int
}

func newProgressOrchestrator(t *testing.T, mock *commandertest.Scripted, dir string, calls *[]progressCall) *restore.Orchestrator {
	t.Helper()
	logger, _ := logtest.NewCaptureLogger(t)
	o := restoretest.NewFakeExeOrchestrator(t, tmux.NewClient(mock), dir, logger)
	o.Progress = func(n, m int) {
		*calls = append(*calls, progressCall{n: n, m: m})
	}
	return o
}

func TestProgress_FiresOncePerSessionWithNAdvancingAgainstFixedM(t *testing.T) {
	dir := t.TempDir()
	sessions := []state.Session{
		{Name: "a", Windows: []state.Window{{Index: 0, Name: "m", Panes: []state.Pane{
			{Index: 0, CWD: "/a", ScrollbackFile: "scrollback/a__0.0.bin", Active: true},
		}}}},
		{Name: "b", Windows: []state.Window{{Index: 0, Name: "m", Panes: []state.Pane{
			{Index: 0, CWD: "/b", ScrollbackFile: "scrollback/b__0.0.bin", Active: true},
		}}}},
		{Name: "c", Windows: []state.Window{{Index: 0, Name: "m", Panes: []state.Pane{
			{Index: 0, CWD: "/c", ScrollbackFile: "scrollback/c__0.0.bin", Active: true},
		}}}},
	}
	writeValidIndex(t, dir, sessions)

	rf := &orchestratorRunFunc{listSessionsOut: "", listPanesOut: "0:0"}
	mock := commandertest.FromFunc(rf.run)
	var calls []progressCall
	o := newProgressOrchestrator(t, mock, dir, &calls)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	want := []progressCall{{1, 3}, {2, 3}, {3, 3}}
	if len(calls) != len(want) {
		t.Fatalf("progress calls = %v, want %v", calls, want)
	}
	for i, c := range calls {
		if c != want[i] {
			t.Errorf("progress call[%d] = %v, want %v", i, c, want[i])
		}
	}
}

func TestProgress_AdvancesNOnLiveSkippedSessionsSoCounterReachesMM(t *testing.T) {
	dir := t.TempDir()
	sessions := []state.Session{
		{Name: "live", Windows: []state.Window{{Index: 0, Name: "m", Panes: []state.Pane{
			{Index: 0, CWD: "/live", ScrollbackFile: "scrollback/live__0.0.bin"},
		}}}},
		{Name: "_portal-saver", Windows: []state.Window{{Index: 0, Name: "m", Panes: []state.Pane{
			{Index: 0, CWD: "/x", ScrollbackFile: "scrollback/x.bin"},
		}}}},
		{Name: "nowin", Windows: []state.Window{}},
		{Name: "nopane", Windows: []state.Window{{Index: 0, Name: "m", Panes: []state.Pane{}}}},
		{Name: "ok", Windows: []state.Window{{Index: 0, Name: "m", Panes: []state.Pane{
			{Index: 0, CWD: "/ok", ScrollbackFile: "scrollback/ok__0.0.bin", Active: true},
		}}}},
	}
	writeValidIndex(t, dir, sessions)

	rf := &orchestratorRunFunc{listSessionsOut: "live|1|0|", listPanesOut: "0:0"}
	mock := commandertest.FromFunc(rf.run)
	var calls []progressCall
	o := newProgressOrchestrator(t, mock, dir, &calls)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	want := []progressCall{{1, 5}, {2, 5}, {3, 5}, {4, 5}, {5, 5}}
	if len(calls) != len(want) {
		t.Fatalf("progress calls = %v, want %v", calls, want)
	}
	for i, c := range calls {
		if c != want[i] {
			t.Errorf("progress call[%d] = %v, want %v", i, c, want[i])
		}
	}
}

func TestProgress_AdvancesNOnSwallowedPerSessionRestoreFailure(t *testing.T) {
	dir := t.TempDir()
	sessions := []state.Session{
		{Name: "broken", Windows: []state.Window{{Index: 0, Name: "m", Panes: []state.Pane{
			{Index: 0, CWD: "/x", ScrollbackFile: "scrollback/x.bin"},
		}}}},
		{Name: "ok", Windows: []state.Window{{Index: 0, Name: "m", Panes: []state.Pane{
			{Index: 0, CWD: "/y", ScrollbackFile: "scrollback/y.bin"},
		}}}},
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
	var calls []progressCall
	o := newProgressOrchestrator(t, mock, dir, &calls)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	want := []progressCall{{1, 2}, {2, 2}}
	if len(calls) != len(want) {
		t.Fatalf("progress calls = %v, want %v (a swallowed per-session failure must still tick N)", calls, want)
	}
	for i, c := range calls {
		if c != want[i] {
			t.Errorf("progress call[%d] = %v, want %v", i, c, want[i])
		}
	}
}

func TestProgress_FiresZeroCallbacksWhenMIsZero(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{
			name:  "sessions.json absent",
			setup: func(_ *testing.T, _ string) {},
		},
		{
			name: "zero saved sessions",
			setup: func(t *testing.T, dir string) {
				writeValidIndex(t, dir, []state.Session{})
			},
		},
		{
			name: "corrupt sessions.json",
			setup: func(t *testing.T, dir string) {
				writeRawIndex(t, dir, []byte("{not json"))
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(t, dir)
			mock := commandertest.FromFunc(defaultRunFunc)
			var calls []progressCall
			o := newProgressOrchestrator(t, mock, dir, &calls)
			_, _ = o.Restore()
			if len(calls) != 0 {
				t.Errorf("M=0 must fire zero callbacks; got %v", calls)
			}
		})
	}
}

func TestProgress_NilCallbackLeavesRestoreOutcomesUnchanged(t *testing.T) {
	sessions := []state.Session{
		{Name: "work", Windows: []state.Window{{Index: 0, Name: "main", Panes: []state.Pane{
			{Index: 0, CWD: "/work", ScrollbackFile: "scrollback/work__0.0.bin", Active: true},
		}}}},
		{Name: "side", Windows: []state.Window{{Index: 0, Name: "a", Panes: []state.Pane{
			{Index: 0, CWD: "/side", ScrollbackFile: "scrollback/side__0.0.bin", Active: true},
		}}}},
	}

	// Both runs share one state dir so the absolute FIFO and scrollback paths in
	// the recorded tmux args are byte-identical between them.
	dir := t.TempDir()
	run := func(progress func(n, m int)) [][]string {
		writeValidIndex(t, dir, sessions)
		rf := &orchestratorRunFunc{listSessionsOut: "", listPanesOut: "0:0"}
		mock := commandertest.FromFunc(rf.run)
		logger, _ := logtest.NewCaptureLogger(t)
		o := restoretest.NewFakeExeOrchestrator(t, tmux.NewClient(mock), dir, logger)
		o.Progress = progress
		o.MintToken = mintsInOrder("mint0a", "mint1b")
		if _, err := o.Restore(); err != nil {
			t.Fatalf("Restore: %v", err)
		}
		return mock.Calls()
	}

	nilCalls := run(nil)
	cbCalls := run(func(int, int) {})

	if len(nilCalls) != len(cbCalls) {
		t.Fatalf("tmux call count differs: nil=%d callback=%d (instrumentation changed restore behaviour)", len(nilCalls), len(cbCalls))
	}
	for i := range nilCalls {
		if len(nilCalls[i]) != len(cbCalls[i]) {
			t.Fatalf("call[%d] arity differs: nil=%v callback=%v", i, nilCalls[i], cbCalls[i])
		}
		for j := range nilCalls[i] {
			if nilCalls[i][j] != cbCalls[i][j] {
				t.Errorf("call[%d][%d] differs: nil=%q callback=%q", i, j, nilCalls[i][j], cbCalls[i][j])
			}
		}
	}
}
