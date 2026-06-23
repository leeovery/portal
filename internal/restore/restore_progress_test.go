package restore_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// progressCall records one (n, m) callback invocation so tests can assert the
// exact firing sequence the per-session loop produced.
type progressCall struct {
	n int
	m int
}

// newProgressOrchestrator builds an Orchestrator with the Progress callback
// wired to append into the supplied slice. Mirrors newOrchestrator (restore_test.go)
// but for the §10.4 N/M progress source.
func newProgressOrchestrator(t *testing.T, mock *mockCommander, dir string, calls *[]progressCall) *restore.Orchestrator {
	t.Helper()
	client := tmux.NewClient(mock)
	logger, _ := newCaptureLogger(t)
	return &restore.Orchestrator{
		Client:   client,
		StateDir: dir,
		Logger:   logger,
		Progress: func(n, m int) {
			*calls = append(*calls, progressCall{n: n, m: m})
		},
	}
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
	mock := &mockCommander{RunFunc: rf.run}
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
		// already live — skipped, but must still tick N.
		{Name: "live", Windows: []state.Window{{Index: 0, Name: "m", Panes: []state.Pane{
			{Index: 0, CWD: "/live", ScrollbackFile: "scrollback/live__0.0.bin"},
		}}}},
		// underscore-prefixed — skipped, but must still tick N.
		{Name: "_portal-saver", Windows: []state.Window{{Index: 0, Name: "m", Panes: []state.Pane{
			{Index: 0, CWD: "/x", ScrollbackFile: "scrollback/x.bin"},
		}}}},
		// zero windows — invalid topology, skipped, but must still tick N.
		{Name: "nowin", Windows: []state.Window{}},
		// a window with zero panes — invalid topology, skipped, but must still tick N.
		{Name: "nopane", Windows: []state.Window{{Index: 0, Name: "m", Panes: []state.Pane{}}}},
		// restorable — ticks N.
		{Name: "ok", Windows: []state.Window{{Index: 0, Name: "m", Panes: []state.Pane{
			{Index: 0, CWD: "/ok", ScrollbackFile: "scrollback/ok__0.0.bin", Active: true},
		}}}},
	}
	writeValidIndex(t, dir, sessions)

	rf := &orchestratorRunFunc{listSessionsOut: "live|1|0|", listPanesOut: "0:0"}
	mock := &mockCommander{RunFunc: rf.run}
	var calls []progressCall
	o := newProgressOrchestrator(t, mock, dir, &calls)
	if _, err := o.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// M is fixed at len(idx.Sessions)==5; N advances 1..5 even though four of
	// the five sessions are skipped — the counter must reach M/M.
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
		// "broken" — new-session fails; restoreOne logs + swallows. N must still tick.
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
	mock := &mockCommander{RunFunc: rf.run}
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
			mock := &mockCommander{RunFunc: defaultRunFunc}
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
	// Restore the same fixture twice — once with a nil Progress callback, once
	// with a recording callback — and assert the tmux call sequence is identical.
	// Additive instrumentation must not alter per-session restore behaviour.
	sessions := []state.Session{
		{Name: "work", Windows: []state.Window{{Index: 0, Name: "main", Panes: []state.Pane{
			{Index: 0, CWD: "/work", ScrollbackFile: "scrollback/work__0.0.bin", Active: true},
		}}}},
		{Name: "side", Windows: []state.Window{{Index: 0, Name: "a", Panes: []state.Pane{
			{Index: 0, CWD: "/side", ScrollbackFile: "scrollback/side__0.0.bin", Active: true},
		}}}},
	}

	// Both runs share ONE state dir so the absolute FIFO/scrollback paths in the
	// recorded tmux args are byte-identical — the only thing under test is whether
	// the Progress callback perturbs the per-session restore call sequence, not
	// the temp-dir naming. The dir is re-seeded between runs (the second
	// Restore sees no live sessions, so it re-runs the same skeleton sequence).
	dir := t.TempDir()
	run := func(progress func(n, m int)) [][]string {
		writeValidIndex(t, dir, sessions)
		rf := &orchestratorRunFunc{listSessionsOut: "", listPanesOut: "0:0"}
		mock := &mockCommander{RunFunc: rf.run}
		client := tmux.NewClient(mock)
		logger, _ := newCaptureLogger(t)
		o := &restore.Orchestrator{
			Client:   client,
			StateDir: dir,
			Logger:   logger,
			Progress: progress,
		}
		if _, err := o.Restore(); err != nil {
			t.Fatalf("Restore: %v", err)
		}
		return mock.Calls
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
