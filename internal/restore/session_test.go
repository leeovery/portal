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

// defaultRunFunc returns ErrOptionNotFound for show-option queries (so
// predictLiveIndices defaults to 0/0) and empty-success for everything else.
func defaultRunFunc(args ...string) (string, error) {
	if len(args) >= 2 && args[0] == "show-option" && args[1] == "-sv" {
		return "", errors.New("unknown option")
	}
	return "", nil
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
	mock := &mockCommander{RunFunc: defaultRunFunc}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	sess := newSession("work", nil,
		newWindow(0, "main",
			newPane(0, "/path/to/work", "scrollback/work__0.0.bin"),
		),
	)

	if err := r.Restore(sess); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	// Expect: 2 show-option calls (predict), 1 new-session, no set-environment,
	// no new-window, no split-window.
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
	mock := &mockCommander{RunFunc: defaultRunFunc}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	sess := newSession("work", nil,
		newWindow(0, "main",
			newPane(0, "/work", "scrollback/work__0.0.bin"),
			newPane(1, "/work", "scrollback/work__0.1.bin"),
			newPane(2, "/work", "scrollback/work__0.2.bin"),
		),
	)

	if err := r.Restore(sess); err != nil {
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

	if err := r.Restore(sess); err != nil {
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

	if err := r.Restore(sess); err != nil {
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

	if err := r.Restore(sess); err != nil {
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

	if err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls, "set-environment")); got != 0 {
		t.Errorf("set-environment calls = %d, want 0", got)
	}
}

func TestSessionRestorer_HydrateCommandContainsAbsoluteScrollbackPath(t *testing.T) {
	mock := &mockCommander{RunFunc: defaultRunFunc}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	sess := newSession("work", nil,
		newWindow(0, "main",
			newPane(0, "/work", "scrollback/work__0.0.bin"),
		),
	)

	if err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	idx := callsAt(mock.Calls, "new-session")
	if idx < 0 {
		t.Fatalf("no new-session call")
	}
	c := mock.Calls[idx]
	hydrate := c[len(c)-1]
	wantAbs := filepath.Join(dir, "scrollback/work__0.0.bin")
	if !strings.Contains(hydrate, "--file "+wantAbs) {
		t.Errorf("hydrate cmd %q does not contain --file %s", hydrate, wantAbs)
	}
}

func TestSessionRestorer_HydrateCommandContainsRawHookKey(t *testing.T) {
	mock := &mockCommander{RunFunc: defaultRunFunc}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	// Saved indices are 1, 2 — exercise the raw form rather than 0.0.
	sess := newSession("work", nil,
		newWindow(3, "main",
			newPane(7, "/work", "scrollback/work__3.7.bin"),
		),
	)

	if err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	idx := callsAt(mock.Calls, "new-session")
	c := mock.Calls[idx]
	hydrate := c[len(c)-1]
	wantHookKey := "work:3.7"
	if !strings.Contains(hydrate, "--hook-key "+wantHookKey) {
		t.Errorf("hydrate cmd %q does not contain --hook-key %s", hydrate, wantHookKey)
	}
}

func TestSessionRestorer_FIFOUsesLivePaneKeyWhenBaseIndexDiffers(t *testing.T) {
	mock := &mockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) >= 3 && args[0] == "show-option" && args[1] == "-sv" {
				switch args[2] {
				case "base-index":
					return "1", nil
				case "pane-base-index":
					return "1", nil
				}
			}
			return "", nil
		},
	}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	// Saved at indices 0/0 but live indices will be 1/1.
	sess := newSession("work", nil,
		newWindow(0, "main",
			newPane(0, "/work", "scrollback/work__0.0.bin"),
		),
	)

	if err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	wantLiveKey := state.SanitizePaneKey("work", 1, 1)
	liveFIFO := state.FIFOPath(dir, wantLiveKey)
	if _, err := os.Stat(liveFIFO); err != nil {
		t.Errorf("expected live-key FIFO %s, missing: %v", liveFIFO, err)
	}

	// Hydrate command should reference the LIVE FIFO path.
	idx := callsAt(mock.Calls, "new-session")
	hydrate := mock.Calls[idx][len(mock.Calls[idx])-1]
	if !strings.Contains(hydrate, "--fifo "+liveFIFO) {
		t.Errorf("hydrate cmd %q does not reference live FIFO %s", hydrate, liveFIFO)
	}
	// And hook-key should remain saved (raw) form.
	if !strings.Contains(hydrate, "--hook-key work:0.0") {
		t.Errorf("hydrate cmd %q does not contain raw saved hook-key work:0.0", hydrate)
	}
}

func TestSessionRestorer_PredictsLiveIndicesFromShowOptions(t *testing.T) {
	var seenBase, seenPaneBase bool
	mock := &mockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) >= 3 && args[0] == "show-option" && args[1] == "-sv" {
				switch args[2] {
				case "base-index":
					seenBase = true
					return "1", nil
				case "pane-base-index":
					seenPaneBase = true
					return "1", nil
				}
			}
			return "", nil
		},
	}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	sess := newSession("work", nil,
		newWindow(0, "main", newPane(0, "/work", "scrollback/work__0.0.bin")),
	)

	if err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !seenBase {
		t.Error("expected show-option -sv base-index call")
	}
	if !seenPaneBase {
		t.Error("expected show-option -sv pane-base-index call")
	}
}

func TestSessionRestorer_DefaultsToZeroWhenOptionUnset(t *testing.T) {
	tests := []struct {
		name   string
		runFn  func(args ...string) (string, error)
		expect string
	}{
		{
			name: "ErrOptionNotFound for both",
			runFn: func(args ...string) (string, error) {
				if len(args) >= 2 && args[0] == "show-option" && args[1] == "-sv" {
					return "", errors.New("unknown option")
				}
				return "", nil
			},
		},
		{
			name: "empty string for both",
			runFn: func(args ...string) (string, error) {
				if len(args) >= 2 && args[0] == "show-option" && args[1] == "-sv" {
					return "", nil
				}
				return "", nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCommander{RunFunc: tt.runFn}
			client := tmux.NewClient(mock)
			dir := t.TempDir()
			r := &restore.SessionRestorer{Client: client, StateDir: dir}

			sess := newSession("work", nil,
				newWindow(0, "main", newPane(0, "/work", "scrollback/work__0.0.bin")),
			)

			if err := r.Restore(sess); err != nil {
				t.Fatalf("Restore: %v", err)
			}

			wantKey := state.SanitizePaneKey("work", 0, 0)
			wantFIFO := state.FIFOPath(dir, wantKey)
			if _, err := os.Stat(wantFIFO); err != nil {
				t.Errorf("expected FIFO at zero-default %s, missing: %v", wantFIFO, err)
			}
		})
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

	if err := r.Restore(sess); err != nil {
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
	mock := &mockCommander{RunFunc: defaultRunFunc}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	// "foo/bar" sanitizes to "foo_bar" with hash suffix.
	name := "foo/bar"
	sess := newSession(name, nil,
		newWindow(0, "main", newPane(0, "/work", "scrollback/x.bin")),
	)

	if err := r.Restore(sess); err != nil {
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
	logger, err := state.OpenLogger(filepath.Join(dir, "portal.log"), false)
	if err != nil {
		t.Fatalf("open logger: %v", err)
	}
	defer func() { _ = logger.Close() }()
	r := &restore.SessionRestorer{Client: client, StateDir: dir, Logger: logger}

	sess := newSession("work",
		map[string]string{"AAA": "1", "BREAK": "2", "ZZZ": "3"},
		newWindow(0, "main", newPane(0, "/work", "scrollback/x.bin")),
		newWindow(1, "logs", newPane(0, "/work", "scrollback/y.bin")),
	)

	if err := r.Restore(sess); err != nil {
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

	err := r.Restore(sess)
	if err == nil {
		t.Fatal("expected error from split-window failure, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q does not wrap underlying error", err)
	}
}

func TestSessionRestorer_WrappedErrorOnCreateFIFOFailure(t *testing.T) {
	mock := &mockCommander{RunFunc: defaultRunFunc}
	client := tmux.NewClient(mock)

	// Use a non-existent state dir → mkfifo fails with ENOENT (no parent).
	dir := filepath.Join(t.TempDir(), "missing-parent", "state")
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	sess := newSession("work", nil,
		newWindow(0, "main", newPane(0, "/work", "scrollback/x.bin")),
	)

	err := r.Restore(sess)
	if err == nil {
		t.Fatal("expected error from CreateFIFO failure, got nil")
	}
	if !strings.Contains(err.Error(), "work") {
		t.Errorf("error %q lacks session name context", err)
	}
}

func TestSessionRestorer_HydrateCommandFormat(t *testing.T) {
	mock := &mockCommander{RunFunc: defaultRunFunc}
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	sess := newSession("work", nil,
		newWindow(0, "main", newPane(0, "/work", "scrollback/work__0.0.bin")),
	)

	if err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	idx := callsAt(mock.Calls, "new-session")
	c := mock.Calls[idx]
	hydrate := c[len(c)-1]

	liveKey := state.SanitizePaneKey("work", 0, 0)
	wantFIFO := state.FIFOPath(dir, liveKey)
	wantFile := filepath.Join(dir, "scrollback/work__0.0.bin")
	wantCmd := fmt.Sprintf(
		"sh -c 'portal state hydrate --fifo %s --file %s --hook-key %s; exec $SHELL'",
		wantFIFO, wantFile, "work:0.0",
	)
	if hydrate != wantCmd {
		t.Errorf("hydrate cmd:\n got %q\nwant %q", hydrate, wantCmd)
	}
}

func TestSessionRestorer_RejectsEmptyTopology(t *testing.T) {
	mock := &mockCommander{RunFunc: defaultRunFunc}
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	sess := newSession("work", nil)
	if err := r.Restore(sess); err == nil {
		t.Fatal("expected error for empty windows, got nil")
	}

	sessEmptyPanes := newSession("work", nil, newWindow(0, "main"))
	if err := r.Restore(sessEmptyPanes); err == nil {
		t.Fatal("expected error for empty panes, got nil")
	}
}
