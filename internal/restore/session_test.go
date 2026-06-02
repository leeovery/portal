package restore_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// mockCommander dispatches Run via an optional RunFunc. Tests configure the
// dispatcher to drive specific tmux call paths.
type mockCommander struct {
	Calls   [][]string
	RunFunc func(args ...string) (string, error)
}

func (m *mockCommander) Run(args ...string) (string, error) {
	m.Calls = append(m.Calls, args)
	if m.RunFunc != nil {
		return m.RunFunc(args...)
	}
	return "", nil
}

func (m *mockCommander) RunRaw(args ...string) (string, error) {
	m.Calls = append(m.Calls, args)
	if m.RunFunc != nil {
		return m.RunFunc(args...)
	}
	return "", nil
}

// defaultRunFunc returns empty-success for every tmux call. Tests that
// exercise the create-then-arm Restore path should use restoreRunFunc to
// provide a list-panes oracle reflecting their topology.
func defaultRunFunc(args ...string) (string, error) {
	return "", nil
}

// restoreRunFunc returns a RunFunc that responds to list-panes queries with
// `livePanesOutput` (newline-separated `<window>:<pane>` lines) and returns
// empty-success for every other call. Used by Restore tests where the arm
// phase re-queries list-panes to discover live indices for FIFO creation and
// send-keys targeting.
func restoreRunFunc(livePanesOutput string) func(args ...string) (string, error) {
	return func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "list-panes" {
			return livePanesOutput, nil
		}
		return "", nil
	}
}

// callsAt returns the index of the first call matching cmd (subcommand at
// args[0]). Returns -1 if not present.
func callsAt(calls [][]string, cmd string) int {
	for i, c := range calls {
		if len(c) > 0 && c[0] == cmd {
			return i
		}
	}
	return -1
}

// findAllCalls returns the indices of every call whose args[0] equals cmd.
func findAllCalls(calls [][]string, cmd string) []int {
	var out []int
	for i, c := range calls {
		if len(c) > 0 && c[0] == cmd {
			out = append(out, i)
		}
	}
	return out
}

func newSession(name string, env map[string]string, windows ...state.Window) state.Session {
	return state.Session{Name: name, Environment: env, Windows: windows}
}

func newWindow(idx int, name string, panes ...state.Pane) state.Window {
	return state.Window{Index: idx, Name: name, Panes: panes}
}

func newPane(idx int, cwd, scrollback string) state.Pane {
	return state.Pane{Index: idx, CWD: cwd, ScrollbackFile: scrollback}
}

func TestSessionRestorer_SinglePaneNoEnvironment(t *testing.T) {
	mock := &mockCommander{RunFunc: restoreRunFunc("0:0")}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	sess := newSession("work", nil,
		newWindow(0, "main",
			newPane(0, "/path/to/work", "scrollback/work__0.0.bin"),
		),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	// Expect: 1 new-session, no set-environment, no new-window, no split-window.
	if got := len(findAllCalls(mock.Calls, "new-session")); got != 1 {
		t.Errorf("new-session calls = %d, want 1", got)
	}
	if got := len(findAllCalls(mock.Calls, "set-environment")); got != 0 {
		t.Errorf("set-environment calls = %d, want 0", got)
	}
	if got := len(findAllCalls(mock.Calls, "new-window")); got != 0 {
		t.Errorf("new-window calls = %d, want 0", got)
	}
	if got := len(findAllCalls(mock.Calls, "split-window")); got != 0 {
		t.Errorf("split-window calls = %d, want 0", got)
	}

	// FIFO must exist at live paneKey (base 0/0 default).
	wantKey := state.SanitizePaneKey("work", 0, 0)
	wantFIFO := state.FIFOPath(dir, wantKey)
	if info, err := os.Stat(wantFIFO); err != nil {
		t.Fatalf("FIFO %s missing: %v", wantFIFO, err)
	} else if info.Mode()&os.ModeNamedPipe == 0 {
		t.Errorf("path %s is not a FIFO (mode=%v)", wantFIFO, info.Mode())
	}
}

func TestSessionRestorer_MultiPaneSingleWindow(t *testing.T) {
	mock := &mockCommander{RunFunc: restoreRunFunc("0:0\n0:1\n0:2")}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	sess := newSession("work", nil,
		newWindow(0, "main",
			newPane(0, "/work", "scrollback/work__0.0.bin"),
			newPane(1, "/work", "scrollback/work__0.1.bin"),
			newPane(2, "/work", "scrollback/work__0.2.bin"),
		),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if got := len(findAllCalls(mock.Calls, "new-session")); got != 1 {
		t.Errorf("new-session calls = %d, want 1", got)
	}
	if got := len(findAllCalls(mock.Calls, "split-window")); got != 2 {
		t.Errorf("split-window calls = %d, want 2", got)
	}
	if got := len(findAllCalls(mock.Calls, "new-window")); got != 0 {
		t.Errorf("new-window calls = %d, want 0", got)
	}
}

func TestSessionRestorer_MultiWindowMultiPane(t *testing.T) {
	mock := &mockCommander{RunFunc: defaultRunFunc}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	sess := newSession("work", nil,
		newWindow(0, "main",
			newPane(0, "/work", "scrollback/work__0.0.bin"),
			newPane(1, "/work", "scrollback/work__0.1.bin"),
		),
		newWindow(1, "logs",
			newPane(0, "/work", "scrollback/work__1.0.bin"),
			newPane(1, "/work", "scrollback/work__1.1.bin"),
			newPane(2, "/work", "scrollback/work__1.2.bin"),
		),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if got := len(findAllCalls(mock.Calls, "new-session")); got != 1 {
		t.Errorf("new-session calls = %d, want 1", got)
	}
	if got := len(findAllCalls(mock.Calls, "new-window")); got != 1 {
		t.Errorf("new-window calls = %d, want 1", got)
	}
	if got := len(findAllCalls(mock.Calls, "split-window")); got != 3 {
		t.Errorf("split-window calls = %d, want 3", got)
	}
}

func TestSessionRestorer_EnvironmentAppliedAfterNewSessionBeforeNewWindow(t *testing.T) {
	mock := &mockCommander{RunFunc: defaultRunFunc}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	sess := newSession("work", map[string]string{"LANG": "en_US.UTF-8"},
		newWindow(0, "main",
			newPane(0, "/work", "scrollback/work__0.0.bin"),
		),
		newWindow(1, "logs",
			newPane(0, "/work", "scrollback/work__1.0.bin"),
		),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	newSessionAt := callsAt(mock.Calls, "new-session")
	setEnvAt := callsAt(mock.Calls, "set-environment")
	newWindowAt := callsAt(mock.Calls, "new-window")

	if newSessionAt < 0 || setEnvAt < 0 || newWindowAt < 0 {
		t.Fatalf("expected new-session, set-environment, new-window all present; got new-session=%d set-environment=%d new-window=%d", newSessionAt, setEnvAt, newWindowAt)
	}
	if newSessionAt >= setEnvAt || setEnvAt >= newWindowAt {
		t.Errorf("ordering violated: new-session(%d) < set-environment(%d) < new-window(%d)", newSessionAt, setEnvAt, newWindowAt)
	}
}

func TestSessionRestorer_EnvironmentAppliedInSortedOrder(t *testing.T) {
	mock := &mockCommander{RunFunc: defaultRunFunc}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	sess := newSession("work",
		map[string]string{"ZULU": "z", "ALPHA": "a", "MIKE": "m"},
		newWindow(0, "main", newPane(0, "/work", "scrollback/work__0.0.bin")),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	envIdxs := findAllCalls(mock.Calls, "set-environment")
	if len(envIdxs) != 3 {
		t.Fatalf("set-environment calls = %d, want 3", len(envIdxs))
	}
	wantKeys := []string{"ALPHA", "MIKE", "ZULU"}
	for i, idx := range envIdxs {
		c := mock.Calls[idx]
		// args = [set-environment, -t, work, KEY, VALUE]
		if len(c) != 5 {
			t.Fatalf("set-environment[%d] args = %v, want 5", i, c)
		}
		if c[3] != wantKeys[i] {
			t.Errorf("set-environment[%d] key = %q, want %q", i, c[3], wantKeys[i])
		}
	}
}

func TestSessionRestorer_EmptyEnvironmentSkipsSetEnvironment(t *testing.T) {
	mock := &mockCommander{RunFunc: defaultRunFunc}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	sess := newSession("work", map[string]string{},
		newWindow(0, "main", newPane(0, "/work", "scrollback/work__0.0.bin")),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls, "set-environment")); got != 0 {
		t.Errorf("set-environment calls = %d, want 0", got)
	}
}

func TestSessionRestorer_HydrateCommandContainsAbsoluteScrollbackPath(t *testing.T) {
	mock := &mockCommander{RunFunc: restoreRunFunc("0:0")}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	sess := newSession("work", nil,
		newWindow(0, "main",
			newPane(0, "/work", "scrollback/work__0.0.bin"),
		),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	hydrate := respawnPaneHydrateCommand(t, mock.Calls)
	wantAbs := filepath.Join(dir, "scrollback/work__0.0.bin")
	if !strings.Contains(hydrate, "--file '"+wantAbs+"'") {
		t.Errorf("hydrate cmd %q does not contain --file '%s'", hydrate, wantAbs)
	}
}

func TestSessionRestorer_HydrateCommandContainsRawHookKey(t *testing.T) {
	mock := &mockCommander{RunFunc: restoreRunFunc("0:0")}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	// Saved indices are 3, 7 — exercise the raw form rather than 0.0.
	sess := newSession("work", nil,
		newWindow(3, "main",
			newPane(7, "/work", "scrollback/work__3.7.bin"),
		),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	hydrate := respawnPaneHydrateCommand(t, mock.Calls)
	wantHookKey := "work:3.7"
	if !strings.Contains(hydrate, "--hook-key '"+wantHookKey+"'") {
		t.Errorf("hydrate cmd %q does not contain --hook-key '%s'", hydrate, wantHookKey)
	}
}

// respawnPaneHydrateCommand returns the hydrate command argument from the
// first respawn-pane call in calls. Fails the test if no respawn-pane call is
// present or if its argv shape does not match
// [respawn-pane, -k, -t, <target>, <cmd>].
func respawnPaneHydrateCommand(t *testing.T, calls [][]string) string {
	t.Helper()
	idx := callsAt(calls, "respawn-pane")
	if idx < 0 {
		t.Fatalf("no respawn-pane call to deliver hydrate command; calls: %v", calls)
	}
	args := calls[idx]
	if len(args) != 5 {
		t.Fatalf("respawn-pane args = %v, want length 5", args)
	}
	return args[4]
}

func TestSessionRestorer_FIFOUsesLivePaneKeyFromListPanesReQuery(t *testing.T) {
	// Saved structure has indices 0/0, but live tmux reports 5:5 — full drift
	// scenario where neither the saved indices nor any prediction match what
	// list-panes returns. Restore must source FIFO paths and send-keys targets
	// from the re-queried live (window, pane).
	mock := &mockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "list-panes" {
				return "5:5", nil
			}
			return "", nil
		},
	}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	sess := newSession("work", nil,
		newWindow(0, "main",
			newPane(0, "/work", "scrollback/work__0.0.bin"),
		),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// FIFO must be at the LIVE paneKey (5,5), not the saved (0,0).
	wantLiveKey := state.SanitizePaneKey("work", 5, 5)
	liveFIFO := state.FIFOPath(dir, wantLiveKey)
	if _, err := os.Stat(liveFIFO); err != nil {
		t.Errorf("expected live-key FIFO %s, missing: %v", liveFIFO, err)
	}
	// And NO FIFO at the saved (0,0) key.
	savedKey := state.SanitizePaneKey("work", 0, 0)
	savedFIFO := state.FIFOPath(dir, savedKey)
	if _, err := os.Stat(savedFIFO); err == nil {
		t.Errorf("did not expect FIFO at saved-key path %s; should only exist at live key", savedFIFO)
	}

	// The hydrate command is dispatched via respawn-pane to the LIVE pane
	// target, not embedded in the new-session call. respawn-pane atomically
	// kills the default shell created by new-session and replaces it with the
	// helper, which is closer to the spec's "helper as initial process"
	// invariant than send-keys (which would let the default shell briefly run
	// before the helper takes over and could leave rc-file output / prompts in
	// scrollback above the dumped saved scrollback).
	respIdx := callsAt(mock.Calls, "respawn-pane")
	if respIdx < 0 {
		t.Fatalf("expected respawn-pane call to deliver hydrate command; calls: %v", mock.Calls)
	}
	args := mock.Calls[respIdx]
	// args = [respawn-pane, -k, -t, <target>, <command>]
	if len(args) != 5 {
		t.Fatalf("respawn-pane args = %v, want length 5", args)
	}
	wantTarget := "work:5.5"
	if args[3] != wantTarget {
		t.Errorf("respawn-pane target = %q, want %q (live coords)", args[3], wantTarget)
	}
	hydrate := args[4]
	if !strings.Contains(hydrate, "--fifo '"+liveFIFO+"'") {
		t.Errorf("hydrate cmd %q does not reference live FIFO %s", hydrate, liveFIFO)
	}
	// hook-key must remain saved (raw) form regardless of live drift.
	if !strings.Contains(hydrate, "--hook-key 'work:0.0'") {
		t.Errorf("hydrate cmd %q does not contain raw saved hook-key 'work:0.0'", hydrate)
	}
	// Scrollback --file uses the SAVED path (sessions.json was written under
	// saved indices) — verify it's the saved path, not the live one.
	wantFile := filepath.Join(dir, "scrollback/work__0.0.bin")
	if !strings.Contains(hydrate, "--file '"+wantFile+"'") {
		t.Errorf("hydrate cmd %q does not reference saved scrollback %s", hydrate, wantFile)
	}

	// new-session must NOT carry the hydrate command (separation of create
	// from arm).
	nsIdx := callsAt(mock.Calls, "new-session")
	if nsIdx < 0 {
		t.Fatalf("expected new-session; calls: %v", mock.Calls)
	}
	for _, a := range mock.Calls[nsIdx] {
		if strings.Contains(a, "portal state hydrate") {
			t.Errorf("new-session must not carry hydrate command; got args %v", mock.Calls[nsIdx])
		}
	}
}

func TestSessionRestorer_MultibyteSessionNamePassesUnchangedToNewSession(t *testing.T) {
	mock := &mockCommander{RunFunc: defaultRunFunc}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	name := "café-日本"
	sess := newSession(name, nil,
		newWindow(0, "main", newPane(0, "/work", "scrollback/x.bin")),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	idx := callsAt(mock.Calls, "new-session")
	if idx < 0 {
		t.Fatalf("no new-session call")
	}
	// args: [new-session, -d, -s, <name>, -c, <cwd>, <hydrate-cmd>]
	got := mock.Calls[idx][3]
	if got != name {
		t.Errorf("new-session -s arg = %q, want %q", got, name)
	}
}

func TestSessionRestorer_HashSuffixedPaneKeyOnSanitizationCollision(t *testing.T) {
	mock := &mockCommander{RunFunc: restoreRunFunc("0:0")}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	// "foo/bar" sanitizes to "foo_bar" with hash suffix.
	name := "foo/bar"
	sess := newSession(name, nil,
		newWindow(0, "main", newPane(0, "/work", "scrollback/x.bin")),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	wantKey := state.SanitizePaneKey(name, 0, 0)
	if !strings.Contains(wantKey, "foo_bar-") {
		t.Fatalf("sanity: paneKey %q should contain hash suffix marker 'foo_bar-'", wantKey)
	}
	wantFIFO := state.FIFOPath(dir, wantKey)
	if _, err := os.Stat(wantFIFO); err != nil {
		t.Errorf("expected FIFO at hash-suffixed %s, missing: %v", wantFIFO, err)
	}
}

func TestSessionRestorer_LogsAndContinuesOnSetEnvironmentFailure(t *testing.T) {
	mock := &mockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) >= 4 && args[0] == "set-environment" && args[3] == "BREAK" {
				return "", errors.New("env error")
			}
			if len(args) >= 2 && args[0] == "show-option" && args[1] == "-sv" {
				return "", errors.New("unknown option")
			}
			return "", nil
		},
	}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logger := restoretest.OpenTestLogger(t, dir)
	r := &restore.SessionRestorer{Client: client, StateDir: dir, Logger: logger}

	sess := newSession("work",
		map[string]string{"AAA": "1", "BREAK": "2", "ZZZ": "3"},
		newWindow(0, "main", newPane(0, "/work", "scrollback/x.bin")),
		newWindow(1, "logs", newPane(0, "/work", "scrollback/y.bin")),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore returned error %v, expected nil (failure should be logged + continue)", err)
	}
	// All three set-environment calls should still be attempted.
	if got := len(findAllCalls(mock.Calls, "set-environment")); got != 3 {
		t.Errorf("set-environment calls = %d, want 3 (each attempted)", got)
	}
	// new-window must still have run.
	if got := len(findAllCalls(mock.Calls, "new-window")); got != 1 {
		t.Errorf("new-window calls = %d, want 1", got)
	}
}

func TestSessionRestorer_WrappedErrorOnSplitWindowFailure(t *testing.T) {
	mock := &mockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "split-window" {
				return "", errors.New("boom")
			}
			if len(args) >= 2 && args[0] == "show-option" && args[1] == "-sv" {
				return "", errors.New("unknown option")
			}
			return "", nil
		},
	}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	sess := newSession("work", nil,
		newWindow(0, "main",
			newPane(0, "/work", "scrollback/a.bin"),
			newPane(1, "/work", "scrollback/b.bin"),
		),
	)

	_, err := r.Restore(sess)
	if err == nil {
		t.Fatal("expected error from split-window failure, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q does not wrap underlying error", err)
	}
}

func TestSessionRestorer_WrappedErrorOnCreateFIFOFailure(t *testing.T) {
	mock := &mockCommander{RunFunc: restoreRunFunc("0:0")}
	client := tmux.NewClient(mock)

	// Use a non-existent state dir → mkfifo fails with ENOENT (no parent).
	dir := filepath.Join(t.TempDir(), "missing-parent", "state")
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	sess := newSession("work", nil,
		newWindow(0, "main", newPane(0, "/work", "scrollback/x.bin")),
	)

	_, err := r.Restore(sess)
	if err == nil {
		t.Fatal("expected error from CreateFIFO failure, got nil")
	}
	if !strings.Contains(err.Error(), "work") {
		t.Errorf("error %q lacks session name context", err)
	}
}

func TestSessionRestorer_HydrateCommandFormat(t *testing.T) {
	mock := &mockCommander{RunFunc: restoreRunFunc("0:0")}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	sess := newSession("work", nil,
		newWindow(0, "main", newPane(0, "/work", "scrollback/work__0.0.bin")),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	hydrate := respawnPaneHydrateCommand(t, mock.Calls)

	liveKey := state.SanitizePaneKey("work", 0, 0)
	wantFIFO := state.FIFOPath(dir, liveKey)
	wantFile := filepath.Join(dir, "scrollback/work__0.0.bin")
	wantCmd := fmt.Sprintf(
		"portal state hydrate --fifo '%s' --file '%s' --hook-key '%s'",
		wantFIFO, wantFile, "work:0.0",
	)
	if hydrate != wantCmd {
		t.Errorf("hydrate cmd:\n got %q\nwant %q", hydrate, wantCmd)
	}
}

func TestSessionRestorer_ArmPanesWarnsAndArmsOnlyPairedPanesWhenLiveCountExceedsSaved(t *testing.T) {
	// Saved 1 pane, live tmux reports 2 (extra pane appeared between create
	// and arm — pathological but defensively handled). armPanes should log a
	// warning, arm the single saved pane against the first live pane, and
	// leave the second live pane untouched (still running its default shell,
	// no respawn-pane / FIFO for it).
	mock := &mockCommander{RunFunc: restoreRunFunc("0:0\n0:1")}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logger, sink := newCaptureLogger(t)
	r := &restore.SessionRestorer{Client: client, StateDir: dir, Logger: logger}

	sess := newSession("work", nil,
		newWindow(0, "main",
			newPane(0, "/work", "scrollback/work__0.0.bin"),
		),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Exactly one respawn-pane call — the saved pane gets armed, the extra
	// live pane is left running its default shell.
	respawnIdxs := findAllCalls(mock.Calls, "respawn-pane")
	if len(respawnIdxs) != 1 {
		t.Errorf("respawn-pane calls = %d, want 1 (only paired pane armed); calls: %v", len(respawnIdxs), mock.Calls)
	}
	if len(respawnIdxs) >= 1 {
		args := mock.Calls[respawnIdxs[0]]
		if len(args) >= 4 && args[3] != "work:0.0" {
			t.Errorf("respawn-pane target = %q, want %q (first live pane)", args[3], "work:0.0")
		}
	}

	// FIFO created for the paired pane only — none for the extra.
	pairedFIFO := state.FIFOPath(dir, state.SanitizePaneKey("work", 0, 0))
	if _, statErr := os.Stat(pairedFIFO); statErr != nil {
		t.Errorf("expected FIFO at paired key %s, missing: %v", pairedFIFO, statErr)
	}
	extraFIFO := state.FIFOPath(dir, state.SanitizePaneKey("work", 0, 1))
	if _, statErr := os.Stat(extraFIFO); statErr == nil {
		t.Errorf("did not expect FIFO at extra-pane key %s; only paired panes should get FIFOs", extraFIFO)
	}

	// Warning logged.
	body := sink.Body()
	if !strings.Contains(body, "live pane count") {
		t.Errorf("log body lacks 'live pane count' mismatch warning: %q", body)
	}
}

func TestSessionRestorer_ArmPanesWarnsAndArmsOnlyFirstWhenLiveCountIsLessThanSaved(t *testing.T) {
	// Saved 2 panes, live tmux reports 1 (split-window failed to land or a
	// pane was killed between create and arm — also defensive). armPanes
	// should log a warning, arm the first paired pane, and return nil (no
	// error — partial restoration is preferable to abort per spec).
	mock := &mockCommander{RunFunc: restoreRunFunc("0:0")}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logger, sink := newCaptureLogger(t)
	r := &restore.SessionRestorer{Client: client, StateDir: dir, Logger: logger}

	sess := newSession("work", nil,
		newWindow(0, "main",
			newPane(0, "/work", "scrollback/work__0.0.bin"),
			newPane(1, "/work", "scrollback/work__0.1.bin"),
		),
	)

	livePanes, err := r.Restore(sess)
	if err != nil {
		t.Fatalf("Restore returned error %v, want nil (partial-restore tolerance)", err)
	}
	if len(livePanes) != 1 {
		t.Errorf("livePanes length = %d, want 1 (the actual live count)", len(livePanes))
	}

	// Exactly one respawn-pane call — only the first saved pane is paired.
	respawnIdxs := findAllCalls(mock.Calls, "respawn-pane")
	if len(respawnIdxs) != 1 {
		t.Errorf("respawn-pane calls = %d, want 1 (only first paired pane armed); calls: %v", len(respawnIdxs), mock.Calls)
	}

	// FIFO created for the first saved pane only — none for the second.
	firstFIFO := state.FIFOPath(dir, state.SanitizePaneKey("work", 0, 0))
	if _, statErr := os.Stat(firstFIFO); statErr != nil {
		t.Errorf("expected FIFO at first-pane key %s, missing: %v", firstFIFO, statErr)
	}

	// Warning logged.
	body := sink.Body()
	if !strings.Contains(body, "live pane count") {
		t.Errorf("log body lacks 'live pane count' mismatch warning: %q", body)
	}
}

func TestSessionRestorer_ArmPanesReturnsWrappedErrorOnRespawnPaneFailure(t *testing.T) {
	// Three saved panes, three live panes. Fail respawn-pane on the second
	// pane (target work:0.1) — armPanes should return a wrapped error
	// referencing the failing target so operators can locate the offending
	// pane. Subsequent panes are NOT armed (fail-fast — a missing helper
	// means scrollback never replays for that pane).
	mock := &mockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "list-panes" {
				return "0:0\n0:1\n0:2", nil
			}
			if len(args) >= 4 && args[0] == "respawn-pane" && args[3] == "work:0.1" {
				return "", errors.New("respawn boom")
			}
			return "", nil
		},
	}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logger := restoretest.OpenTestLogger(t, dir)
	r := &restore.SessionRestorer{Client: client, StateDir: dir, Logger: logger}

	sess := newSession("work", nil,
		newWindow(0, "main",
			newPane(0, "/work", "scrollback/work__0.0.bin"),
			newPane(1, "/work", "scrollback/work__0.1.bin"),
			newPane(2, "/work", "scrollback/work__0.2.bin"),
		),
	)

	_, restoreErr := r.Restore(sess)
	if restoreErr == nil {
		t.Fatal("expected error from respawn-pane failure on pane 1 of 3, got nil")
	}
	if !strings.Contains(restoreErr.Error(), "respawn boom") {
		t.Errorf("error %q does not wrap underlying respawn-pane error", restoreErr)
	}
	if !strings.Contains(restoreErr.Error(), "work:0.1") {
		t.Errorf("error %q does not include failing pane target work:0.1", restoreErr)
	}
	if !strings.Contains(restoreErr.Error(), "work") {
		t.Errorf("error %q lacks session name context", restoreErr)
	}

	// First pane armed, second failed, third not attempted (fail-fast).
	respawnIdxs := findAllCalls(mock.Calls, "respawn-pane")
	if len(respawnIdxs) != 2 {
		t.Errorf("respawn-pane calls = %d, want 2 (pane 0 armed, pane 1 failed, pane 2 skipped); calls: %v", len(respawnIdxs), mock.Calls)
	}
}

func TestSessionRestorer_RejectsEmptyTopology(t *testing.T) {
	mock := &mockCommander{RunFunc: defaultRunFunc}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	sess := newSession("work", nil)
	if _, err := r.Restore(sess); err == nil {
		t.Fatal("expected error for empty windows, got nil")
	}

	sessEmptyPanes := newSession("work", nil, newWindow(0, "main"))
	if _, err := r.Restore(sessEmptyPanes); err == nil {
		t.Fatal("expected error for empty panes, got nil")
	}
}
