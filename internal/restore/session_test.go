package restore_test

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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

func defaultRunFunc(args ...string) (string, error) {
	return "", nil
}

func restoreRunFunc(livePanesOutput string) func(args ...string) (string, error) {
	return func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "list-panes" {
			return livePanesOutput, nil
		}
		return "", nil
	}
}

func callsAt(calls [][]string, cmd string) int {
	for i, c := range calls {
		if len(c) > 0 && c[0] == cmd {
			return i
		}
	}
	return -1
}

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

func newPaneWithToken(idx int, cwd, scrollback, token string) state.Pane {
	return state.Pane{Index: idx, CWD: cwd, ScrollbackFile: scrollback, PortalPaneID: token}
}

func TestSessionRestorer_SinglePaneNoEnvironment(t *testing.T) {
	mock := commandertest.FromFunc(restoreRunFunc("0:0"))
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

	if got := len(findAllCalls(mock.Calls(), "new-session")); got != 1 {
		t.Errorf("new-session calls = %d, want 1", got)
	}
	if got := len(findAllCalls(mock.Calls(), "set-environment")); got != 0 {
		t.Errorf("set-environment calls = %d, want 0", got)
	}
	if got := len(findAllCalls(mock.Calls(), "new-window")); got != 0 {
		t.Errorf("new-window calls = %d, want 0", got)
	}
	if got := len(findAllCalls(mock.Calls(), "split-window")); got != 0 {
		t.Errorf("split-window calls = %d, want 0", got)
	}

	wantKey := state.SanitizePaneKey("work", 0, 0)
	wantFIFO := state.FIFOPath(dir, wantKey)
	if info, err := os.Stat(wantFIFO); err != nil {
		t.Fatalf("FIFO %s missing: %v", wantFIFO, err)
	} else if info.Mode()&os.ModeNamedPipe == 0 {
		t.Errorf("path %s is not a FIFO (mode=%v)", wantFIFO, info.Mode())
	}
}

func TestSessionRestorer_MultiPaneSingleWindow(t *testing.T) {
	mock := commandertest.FromFunc(restoreRunFunc("0:0\n0:1\n0:2"))
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

	if got := len(findAllCalls(mock.Calls(), "new-session")); got != 1 {
		t.Errorf("new-session calls = %d, want 1", got)
	}
	if got := len(findAllCalls(mock.Calls(), "split-window")); got != 2 {
		t.Errorf("split-window calls = %d, want 2", got)
	}
	if got := len(findAllCalls(mock.Calls(), "new-window")); got != 0 {
		t.Errorf("new-window calls = %d, want 0", got)
	}
}

func TestSessionRestorer_MultiWindowMultiPane(t *testing.T) {
	mock := commandertest.FromFunc(defaultRunFunc)
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

	if got := len(findAllCalls(mock.Calls(), "new-session")); got != 1 {
		t.Errorf("new-session calls = %d, want 1", got)
	}
	if got := len(findAllCalls(mock.Calls(), "new-window")); got != 1 {
		t.Errorf("new-window calls = %d, want 1", got)
	}
	if got := len(findAllCalls(mock.Calls(), "split-window")); got != 3 {
		t.Errorf("split-window calls = %d, want 3", got)
	}
}

func TestSessionRestorer_EnvironmentAppliedAfterNewSessionBeforeNewWindow(t *testing.T) {
	mock := commandertest.FromFunc(defaultRunFunc)
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

	newSessionAt := callsAt(mock.Calls(), "new-session")
	setEnvAt := callsAt(mock.Calls(), "set-environment")
	newWindowAt := callsAt(mock.Calls(), "new-window")

	if newSessionAt < 0 || setEnvAt < 0 || newWindowAt < 0 {
		t.Fatalf("expected new-session, set-environment, new-window all present; got new-session=%d set-environment=%d new-window=%d", newSessionAt, setEnvAt, newWindowAt)
	}
	if newSessionAt >= setEnvAt || setEnvAt >= newWindowAt {
		t.Errorf("ordering violated: new-session(%d) < set-environment(%d) < new-window(%d)", newSessionAt, setEnvAt, newWindowAt)
	}
}

func TestSessionRestorer_EnvironmentAppliedInSortedOrder(t *testing.T) {
	mock := commandertest.FromFunc(defaultRunFunc)
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	sess := newSession("work",
		map[string]string{"ZULU": "z", "ALPHA": "a", "MIKE": "m"},
		newWindow(0, "main", newPane(0, "/work", "scrollback/work__0.0.bin")),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	envIdxs := findAllCalls(mock.Calls(), "set-environment")
	if len(envIdxs) != 3 {
		t.Fatalf("set-environment calls = %d, want 3", len(envIdxs))
	}
	wantKeys := []string{"ALPHA", "MIKE", "ZULU"}
	for i, idx := range envIdxs {
		c := mock.Calls()[idx]
		if len(c) != 5 {
			t.Fatalf("set-environment[%d] args = %v, want 5", i, c)
		}
		if c[3] != wantKeys[i] {
			t.Errorf("set-environment[%d] key = %q, want %q", i, c[3], wantKeys[i])
		}
	}
}

func TestSessionRestorer_EmptyEnvironmentSkipsSetEnvironment(t *testing.T) {
	mock := commandertest.FromFunc(defaultRunFunc)
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	sess := newSession("work", map[string]string{},
		newWindow(0, "main", newPane(0, "/work", "scrollback/work__0.0.bin")),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := len(findAllCalls(mock.Calls(), "set-environment")); got != 0 {
		t.Errorf("set-environment calls = %d, want 0", got)
	}
}

func TestSessionRestorer_HydrateCommandContainsAbsoluteScrollbackPath(t *testing.T) {
	mock := commandertest.FromFunc(restoreRunFunc("0:0"))
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

	hydrate := respawnPaneHydrateCommand(t, mock.Calls())
	wantAbs := filepath.Join(dir, "scrollback/work__0.0.bin")
	if !strings.Contains(hydrate, "--file '"+wantAbs+"'") {
		t.Errorf("hydrate cmd %q does not contain --file '%s'", hydrate, wantAbs)
	}
}

func TestSessionRestorer_HydrateCommandBakesSavedPaneToken(t *testing.T) {
	t.Run("it bakes the saved pane token as the hydrate hook key", func(t *testing.T) {
		mock := commandertest.FromFunc(restoreRunFunc("0:0"))
		client := tmux.NewClient(mock)
		dir := t.TempDir()
		r := &restore.SessionRestorer{Client: client, StateDir: dir}

		sess := newSession("work", nil,
			newWindow(3, "main",
				newPaneWithToken(7, "/work", "scrollback/work__3.7.bin", "tok123"),
			),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		hydrate := respawnPaneHydrateCommand(t, mock.Calls())
		if !strings.Contains(hydrate, "--hook-key 'tok123'") {
			t.Errorf("hydrate cmd %q does not contain --hook-key 'tok123'", hydrate)
		}
	})

	t.Run("it omits the hook-key flag for a saved pane with no token", func(t *testing.T) {
		mock := commandertest.FromFunc(restoreRunFunc("0:0"))
		client := tmux.NewClient(mock)
		dir := t.TempDir()
		r := &restore.SessionRestorer{Client: client, StateDir: dir}

		sess := newSession("work", nil,
			newWindow(3, "main",
				newPane(7, "/work", "scrollback/work__3.7.bin"),
			),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		hydrate := respawnPaneHydrateCommand(t, mock.Calls())
		if strings.Contains(hydrate, "--hook-key") {
			t.Errorf("hydrate cmd %q must carry no --hook-key flag for an untokened pane", hydrate)
		}
	})
}

func TestSessionRestorer_HydrateBakesOneTokenPerPane(t *testing.T) {
	t.Run("it bakes each pane's own token for a multi-pane session", func(t *testing.T) {
		mock := commandertest.FromFunc(restoreRunFunc("0:0\n0:1\n1:0"))
		client := tmux.NewClient(mock)
		dir := t.TempDir()
		r := &restore.SessionRestorer{Client: client, StateDir: dir}

		sess := newSession("work", nil,
			newWindow(0, "main",
				newPaneWithToken(0, "/work", "scrollback/work__0.0.bin", "tokA"),
				newPaneWithToken(1, "/work", "scrollback/work__0.1.bin", "tokB"),
			),
			newWindow(1, "logs",
				newPaneWithToken(0, "/work", "scrollback/work__1.0.bin", "tokC"),
			),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		bakedKeys := respawnPaneHookKeys(t, mock.Calls())
		wantKeys := []string{"tokA", "tokB", "tokC"}
		if len(bakedKeys) != len(wantKeys) {
			t.Fatalf("baked hook keys = %v, want %v", bakedKeys, wantKeys)
		}
		for i, want := range wantKeys {
			if bakedKeys[i] != want {
				t.Errorf("baked hook key[%d] = %q, want %q", i, bakedKeys[i], want)
			}
		}
	})
}

func TestSessionRestorer_HydrateBakesKeyFromSavedStateOnly(t *testing.T) {
	t.Run("it derives the baked key from saved state, never from the live server", func(t *testing.T) {
		mock := commandertest.FromFunc(func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "display-message" {
				t.Errorf("unexpected live read while baking the hook key: %v", args)
			}
			if len(args) > 0 && args[0] == "list-panes" {
				return "0:0", nil
			}
			return "", nil
		})
		client := tmux.NewClient(mock)
		dir := t.TempDir()
		r := &restore.SessionRestorer{Client: client, StateDir: dir}

		sess := newSession("work", nil,
			newWindow(3, "main",
				newPaneWithToken(7, "/work", "scrollback/work__3.7.bin", "tok123"),
			),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		if got := respawnPaneHookKeys(t, mock.Calls()); len(got) != 1 || got[0] != sess.Windows[0].Panes[0].PortalPaneID {
			t.Errorf("baked hook keys = %v, want the saved pane token %q", got, sess.Windows[0].Panes[0].PortalPaneID)
		}
	})
}

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

func respawnPaneHookKeys(t *testing.T, calls [][]string) []string {
	t.Helper()
	var keys []string
	for _, idx := range findAllCalls(calls, "respawn-pane") {
		args := calls[idx]
		if len(args) != 5 {
			t.Fatalf("respawn-pane args = %v, want length 5", args)
		}
		key, found := extractHookKey(t, args[4])
		if !found {
			t.Fatalf("respawn-pane args = %v carry no --hook-key flag", args)
		}
		keys = append(keys, key)
	}
	return keys
}

// The second return distinguishes "no --hook-key flag at all" - what an
// untokened pane is armed with - from a flag carrying an empty value.
func extractHookKey(t *testing.T, hydrate string) (string, bool) {
	t.Helper()
	const marker = "--hook-key '"
	_, rest, found := strings.Cut(hydrate, marker)
	if !found {
		return "", false
	}
	key, _, closed := strings.Cut(rest, "'")
	if !closed {
		t.Fatalf("hydrate cmd %q has an unterminated --hook-key single quote", hydrate)
	}
	return key, true
}

func TestSessionRestorer_FIFOUsesLivePaneKeyFromListPanesReQuery(t *testing.T) {
	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "list-panes" {
			return "5:5", nil
		}
		return "", nil
	})
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	sess := newSession("work", nil,
		newWindow(0, "main",
			newPaneWithToken(0, "/work", "scrollback/work__0.0.bin", "tok123"),
		),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	wantLiveKey := state.SanitizePaneKey("work", 5, 5)
	liveFIFO := state.FIFOPath(dir, wantLiveKey)
	if _, err := os.Stat(liveFIFO); err != nil {
		t.Errorf("expected live-key FIFO %s, missing: %v", liveFIFO, err)
	}
	savedKey := state.SanitizePaneKey("work", 0, 0)
	savedFIFO := state.FIFOPath(dir, savedKey)
	if _, err := os.Stat(savedFIFO); err == nil {
		t.Errorf("did not expect FIFO at saved-key path %s; should only exist at live key", savedFIFO)
	}

	respIdx := callsAt(mock.Calls(), "respawn-pane")
	if respIdx < 0 {
		t.Fatalf("expected respawn-pane call to deliver hydrate command; calls: %v", mock.Calls())
	}
	args := mock.Calls()[respIdx]
	if len(args) != 5 {
		t.Fatalf("respawn-pane args = %v, want length 5", args)
	}
	wantTarget := "=work:5.5"
	if args[3] != wantTarget {
		t.Errorf("respawn-pane target = %q, want %q (live coords)", args[3], wantTarget)
	}
	hydrate := args[4]
	if !strings.Contains(hydrate, "--fifo '"+liveFIFO+"'") {
		t.Errorf("hydrate cmd %q does not reference live FIFO %s", hydrate, liveFIFO)
	}
	if !strings.Contains(hydrate, "--hook-key 'tok123'") {
		t.Errorf("hydrate cmd %q does not contain the saved pane token 'tok123'", hydrate)
	}
	wantFile := filepath.Join(dir, "scrollback/work__0.0.bin")
	if !strings.Contains(hydrate, "--file '"+wantFile+"'") {
		t.Errorf("hydrate cmd %q does not reference saved scrollback %s", hydrate, wantFile)
	}

	nsIdx := callsAt(mock.Calls(), "new-session")
	if nsIdx < 0 {
		t.Fatalf("expected new-session; calls: %v", mock.Calls())
	}
	for _, a := range mock.Calls()[nsIdx] {
		if strings.Contains(a, "state hydrate") {
			t.Errorf("new-session must not carry hydrate command; got args %v", mock.Calls()[nsIdx])
		}
	}
}

func TestSessionRestorer_MultibyteSessionNamePassesUnchangedToNewSession(t *testing.T) {
	mock := commandertest.FromFunc(defaultRunFunc)
	client := tmux.NewClient(mock)
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	name := "café-日本"
	sess := newSession(name, nil,
		newWindow(0, "main", newPane(0, "/work", "scrollback/x.bin")),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	idx := callsAt(mock.Calls(), "new-session")
	if idx < 0 {
		t.Fatalf("no new-session call")
	}
	got := mock.Calls()[idx][3]
	if got != name {
		t.Errorf("new-session -s arg = %q, want %q", got, name)
	}
}

func TestSessionRestorer_HashSuffixedPaneKeyOnSanitizationCollision(t *testing.T) {
	mock := commandertest.FromFunc(restoreRunFunc("0:0"))
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

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
	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		if len(args) >= 4 && args[0] == "set-environment" && args[3] == "BREAK" {
			return "", errors.New("env error")
		}
		if len(args) >= 2 && args[0] == "show-option" && args[1] == "-sv" {
			return "", errors.New("unknown option")
		}
		return "", nil
	})
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
	if got := len(findAllCalls(mock.Calls(), "set-environment")); got != 3 {
		t.Errorf("set-environment calls = %d, want 3 (each attempted)", got)
	}
	if got := len(findAllCalls(mock.Calls(), "new-window")); got != 1 {
		t.Errorf("new-window calls = %d, want 1", got)
	}
}

func TestSessionRestorer_WrappedErrorOnSplitWindowFailure(t *testing.T) {
	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "split-window" {
			return "", errors.New("boom")
		}
		if len(args) >= 2 && args[0] == "show-option" && args[1] == "-sv" {
			return "", errors.New("unknown option")
		}
		return "", nil
	})
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
	mock := commandertest.FromFunc(restoreRunFunc("0:0"))
	client := tmux.NewClient(mock)

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
	mock := commandertest.FromFunc(restoreRunFunc("0:0"))
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{
		Client:   client,
		StateDir: dir,
		Exe:      func() (string, error) { return "/opt/pkg/bin/portal", nil },
	}

	sess := newSession("work", nil,
		newWindow(0, "main", newPaneWithToken(0, "/work", "scrollback/work__0.0.bin", "tok123")),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	hydrate := respawnPaneHydrateCommand(t, mock.Calls())

	liveKey := state.SanitizePaneKey("work", 0, 0)
	wantFIFO := state.FIFOPath(dir, liveKey)
	wantFile := filepath.Join(dir, "scrollback/work__0.0.bin")
	wantCmd := fmt.Sprintf(
		"'/opt/pkg/bin/portal' state hydrate --fifo '%s' --file '%s' --hook-key '%s'",
		wantFIFO, wantFile, "tok123",
	)
	if hydrate != wantCmd {
		t.Errorf("hydrate cmd:\n got %q\nwant %q", hydrate, wantCmd)
	}
}

func TestSessionRestorer_ArmPanesWarnsAndArmsOnlyPairedPanesWhenLiveCountExceedsSaved(t *testing.T) {
	mock := commandertest.FromFunc(restoreRunFunc("0:0\n0:1"))
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logger, sink := logtest.NewCaptureLogger(t)
	r := &restore.SessionRestorer{Client: client, StateDir: dir, Logger: logger}

	sess := newSession("work", nil,
		newWindow(0, "main",
			newPane(0, "/work", "scrollback/work__0.0.bin"),
		),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	respawnIdxs := findAllCalls(mock.Calls(), "respawn-pane")
	if len(respawnIdxs) != 1 {
		t.Errorf("respawn-pane calls = %d, want 1 (only paired pane armed); calls: %v", len(respawnIdxs), mock.Calls())
	}
	if len(respawnIdxs) >= 1 {
		args := mock.Calls()[respawnIdxs[0]]
		if len(args) >= 4 && args[3] != "=work:0.0" {
			t.Errorf("respawn-pane target = %q, want %q (first live pane)", args[3], "=work:0.0")
		}
	}

	pairedFIFO := state.FIFOPath(dir, state.SanitizePaneKey("work", 0, 0))
	if _, statErr := os.Stat(pairedFIFO); statErr != nil {
		t.Errorf("expected FIFO at paired key %s, missing: %v", pairedFIFO, statErr)
	}
	extraFIFO := state.FIFOPath(dir, state.SanitizePaneKey("work", 0, 1))
	if _, statErr := os.Stat(extraFIFO); statErr == nil {
		t.Errorf("did not expect FIFO at extra-pane key %s; only paired panes should get FIFOs", extraFIFO)
	}

	body := sink.Body()
	if !strings.Contains(body, "live pane count") {
		t.Errorf("log body lacks 'live pane count' mismatch warning: %q", body)
	}
}

func TestSessionRestorer_ArmPanesWarnsAndArmsOnlyFirstWhenLiveCountIsLessThanSaved(t *testing.T) {
	mock := commandertest.FromFunc(restoreRunFunc("0:0"))
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	logger, sink := logtest.NewCaptureLogger(t)
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

	respawnIdxs := findAllCalls(mock.Calls(), "respawn-pane")
	if len(respawnIdxs) != 1 {
		t.Errorf("respawn-pane calls = %d, want 1 (only first paired pane armed); calls: %v", len(respawnIdxs), mock.Calls())
	}

	firstFIFO := state.FIFOPath(dir, state.SanitizePaneKey("work", 0, 0))
	if _, statErr := os.Stat(firstFIFO); statErr != nil {
		t.Errorf("expected FIFO at first-pane key %s, missing: %v", firstFIFO, statErr)
	}

	body := sink.Body()
	if !strings.Contains(body, "live pane count") {
		t.Errorf("log body lacks 'live pane count' mismatch warning: %q", body)
	}
}

func TestSessionRestorer_ArmPanesReturnsWrappedErrorOnRespawnPaneFailure(t *testing.T) {
	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "list-panes" {
			return "0:0\n0:1\n0:2", nil
		}
		if len(args) >= 4 && args[0] == "respawn-pane" && args[3] == "=work:0.1" {
			return "", errors.New("respawn boom")
		}
		return "", nil
	})
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

	respawnIdxs := findAllCalls(mock.Calls(), "respawn-pane")
	if len(respawnIdxs) != 2 {
		t.Errorf("respawn-pane calls = %d, want 2 (pane 0 armed, pane 1 failed, pane 2 skipped); calls: %v", len(respawnIdxs), mock.Calls())
	}
}

func TestSessionRestorer_IssuesNoSessionOptionStampDuringSkeletonCreation(t *testing.T) {
	mock := commandertest.FromFunc(restoreRunFunc("0:0"))
	client := tmux.NewClient(mock)
	dir := t.TempDir()
	r := &restore.SessionRestorer{Client: client, StateDir: dir}

	sess := newSession("work", nil,
		newWindow(0, "main", newPaneWithToken(0, "/work", "scrollback/work__0.0.bin", "tok123")),
	)

	if _, err := r.Restore(sess); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	for _, c := range mock.Calls() {
		if len(c) >= 4 && c[0] == "set-option" && c[1] == "-t" && c[2] == "work" {
			t.Errorf("unexpected session-scoped set-option during skeleton creation: %v", c)
		}
	}
}

func TestSessionRestorer_RejectsEmptyTopology(t *testing.T) {
	mock := commandertest.FromFunc(defaultRunFunc)
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

func setPaneOptionCalls(calls [][]string) [][]string {
	var out [][]string
	for _, c := range calls {
		if len(c) == 6 && c[0] == "set-option" && c[1] == "-p" {
			out = append(out, c)
		}
	}
	return out
}

func assertPaneTokenStamp(t *testing.T, call []string, wantTarget, wantToken string) {
	t.Helper()
	if call[3] != wantTarget {
		t.Errorf("stamp target = %q, want %q", call[3], wantTarget)
	}
	if call[4] != state.PortalPaneIDOption {
		t.Errorf("stamp option = %q, want %q", call[4], state.PortalPaneIDOption)
	}
	if call[5] != wantToken {
		t.Errorf("stamp value = %q, want %q", call[5], wantToken)
	}
}

func failOnPaneOptionTarget(livePanesOutput, failTarget string) func(args ...string) (string, error) {
	return func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "list-panes" {
			return livePanesOutput, nil
		}
		if len(args) == 6 && args[0] == "set-option" && args[1] == "-p" && args[3] == failTarget {
			return "", errors.New("stamp boom")
		}
		return "", nil
	}
}

// mintsInOrder hands out tokens in order, then fails.
func mintsInOrder(tokens ...string) func() (string, error) {
	next := 0
	return func() (string, error) {
		if next == len(tokens) {
			return "", errors.New("no more tokens")
		}
		next++
		return tokens[next-1], nil
	}
}

func TestSessionRestorer_ReStampsSavedPaneToken(t *testing.T) {
	t.Run("it stamps each saved token onto its paired live pane", func(t *testing.T) {
		mock := commandertest.FromFunc(restoreRunFunc("0:0\n0:1"))
		r := &restore.SessionRestorer{Client: tmux.NewClient(mock), StateDir: t.TempDir()}

		sess := newSession("work", nil,
			newWindow(0, "main",
				newPaneWithToken(0, "/work", "scrollback/work__0.0.bin", "tokA"),
				newPaneWithToken(1, "/work", "scrollback/work__0.1.bin", "tokB"),
			),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		stamps := setPaneOptionCalls(mock.Calls())
		if len(stamps) != 2 {
			t.Fatalf("set-option -p calls = %d, want 2; calls: %v", len(stamps), mock.Calls())
		}
		assertPaneTokenStamp(t, stamps[0], "=work:0.0", "tokA")
		assertPaneTokenStamp(t, stamps[1], "=work:0.1", "tokB")
	})

	t.Run("it stamps before it arms the pane", func(t *testing.T) {
		mock := commandertest.FromFunc(restoreRunFunc("0:0"))
		r := &restore.SessionRestorer{Client: tmux.NewClient(mock), StateDir: t.TempDir()}

		sess := newSession("work", nil,
			newWindow(0, "main", newPaneWithToken(0, "/work", "scrollback/work__0.0.bin", "tokA")),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		stampIdx := callsAt(mock.Calls(), "set-option")
		respawnIdx := callsAt(mock.Calls(), "respawn-pane")
		if stampIdx < 0 || respawnIdx < 0 {
			t.Fatalf("expected both set-option and respawn-pane; calls: %v", mock.Calls())
		}
		if stampIdx > respawnIdx {
			t.Errorf("set-option at %d follows respawn-pane at %d; the stamp must precede the arm", stampIdx, respawnIdx)
		}
	})

	t.Run("it stamps a freshly minted token on a saved pane with an empty token, before it arms the pane", func(t *testing.T) {
		mock := commandertest.FromFunc(restoreRunFunc("0:0"))
		logger, sink := logtest.NewCaptureLogger(t)
		r := &restore.SessionRestorer{Client: tmux.NewClient(mock), StateDir: t.TempDir(), Logger: logger, MintToken: mintsInOrder("mint0a")}

		sess := newSession("work", nil,
			newWindow(0, "main", newPane(0, "/work", "scrollback/work__0.0.bin")),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		stamps := setPaneOptionCalls(mock.Calls())
		if len(stamps) != 1 {
			t.Fatalf("set-option -p calls = %v, want one stamp of the minted token", stamps)
		}
		assertPaneTokenStamp(t, stamps[0], "=work:0.0", "mint0a")
		if stampIdx, respawnIdx := callsAt(mock.Calls(), "set-option"), callsAt(mock.Calls(), "respawn-pane"); stampIdx > respawnIdx {
			t.Errorf("set-option at %d follows respawn-pane at %d; the stamp must precede the arm", stampIdx, respawnIdx)
		}
		if body := sink.Body(); body != "" {
			t.Errorf("log body = %q, want empty", body)
		}
	})

	t.Run("it mints a token per untokened saved pane and re-stamps each saved token", func(t *testing.T) {
		mock := commandertest.FromFunc(restoreRunFunc("0:0\n1:0\n2:0"))
		r := &restore.SessionRestorer{Client: tmux.NewClient(mock), StateDir: t.TempDir(), MintToken: mintsInOrder("mint0a", "mint1b")}

		sess := newSession("work", nil,
			newWindow(0, "main", newPane(0, "/work", "scrollback/work__0.0.bin")),
			newWindow(2, "mid", newPaneWithToken(0, "/work", "scrollback/work__2.0.bin", "tokA")),
			newWindow(3, "last", newPane(0, "/work", "scrollback/work__3.0.bin")),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		stamps := setPaneOptionCalls(mock.Calls())
		if len(stamps) != 3 {
			t.Fatalf("set-option -p calls = %v, want 3", stamps)
		}
		assertPaneTokenStamp(t, stamps[0], "=work:0.0", "mint0a")
		assertPaneTokenStamp(t, stamps[1], "=work:1.0", "tokA")
		assertPaneTokenStamp(t, stamps[2], "=work:2.0", "mint1b")
	})

	t.Run("it bakes no hook key for a pane it minted a token for", func(t *testing.T) {
		mock := commandertest.FromFunc(restoreRunFunc("0:0"))
		r := &restore.SessionRestorer{Client: tmux.NewClient(mock), StateDir: t.TempDir(), MintToken: mintsInOrder("mint0a")}

		sess := newSession("work", nil,
			newWindow(0, "main", newPane(0, "/work", "scrollback/work__0.0.bin")),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		if key, found := extractHookKey(t, respawnPaneHydrateCommand(t, mock.Calls())); found {
			t.Errorf("hydrate cmd bakes --hook-key %q, want none for a pane saved with no token", key)
		}
	})

	t.Run("it warns, stamps nothing and still arms the pane when minting fails", func(t *testing.T) {
		mock := commandertest.FromFunc(restoreRunFunc("0:0"))
		logger, sink := logtest.NewCaptureLogger(t)
		mintErr := errors.New("entropy boom")
		r := &restore.SessionRestorer{
			Client: tmux.NewClient(mock), StateDir: t.TempDir(), Logger: logger,
			MintToken: func() (string, error) { return "", mintErr },
		}

		sess := newSession("work", nil,
			newWindow(0, "main", newPane(0, "/work", "scrollback/work__0.0.bin")),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore returned %v, want nil (a failed mint must not abort)", err)
		}

		if stamps := setPaneOptionCalls(mock.Calls()); len(stamps) != 0 {
			t.Errorf("set-option -p calls = %v, want none", stamps)
		}
		if got := len(findAllCalls(mock.Calls(), "respawn-pane")); got != 1 {
			t.Errorf("respawn-pane calls = %d, want 1 (the pane is still armed)", got)
		}
		warn := sink.Records().WithMessage("mint pane token failed").Only(t, "mint pane token failed record")
		if warn.Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", warn.Level)
		}
		if wantKeys := []string{"session", "pane_key", "error"}; !slices.Equal(warn.Keys, wantKeys) {
			t.Errorf("attr keys = %v, want %v", warn.Keys, wantKeys)
		}
		if got := warn.ErrorAttr(t, "error"); !errors.Is(got, mintErr) {
			t.Errorf("error attr = %v, want %v", got, mintErr)
		}
	})

	t.Run("it warns and continues when the stamp fails", func(t *testing.T) {
		mock := commandertest.FromFunc(failOnPaneOptionTarget("0:0", "=work:0.0"))
		logger, sink := logtest.NewCaptureLogger(t)
		r := &restore.SessionRestorer{Client: tmux.NewClient(mock), StateDir: t.TempDir(), Logger: logger}

		sess := newSession("work", nil,
			newWindow(0, "main", newPaneWithToken(0, "/work", "scrollback/work__0.0.bin", "tokA")),
		)

		livePanes, err := r.Restore(sess)
		if err != nil {
			t.Fatalf("Restore returned %v, want nil (a failed stamp must not abort)", err)
		}
		if len(livePanes) != 1 {
			t.Errorf("livePanes = %v, want the one live pane", livePanes)
		}
		if got := len(findAllCalls(mock.Calls(), "respawn-pane")); got != 1 {
			t.Errorf("respawn-pane calls = %d, want 1 (the pane is still armed)", got)
		}

		warn := sink.Records().WithMessage("set pane token failed").Only(t, "set pane token failed record")
		if warn.Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", warn.Level)
		}
		wantKeys := []string{"session", "pane_key", "error"}
		if !slices.Equal(warn.Keys, wantKeys) {
			t.Errorf("attr keys = %v, want %v", warn.Keys, wantKeys)
		}
		if got := warn.AttrString(t, "session"); got != "work" {
			t.Errorf("session attr = %q, want %q", got, "work")
		}
	})

	t.Run("it names the live structural key in pane_key", func(t *testing.T) {
		mock := commandertest.FromFunc(failOnPaneOptionTarget("5:5", "=work:5.5"))
		logger, sink := logtest.NewCaptureLogger(t)
		r := &restore.SessionRestorer{Client: tmux.NewClient(mock), StateDir: t.TempDir(), Logger: logger}

		sess := newSession("work", nil,
			newWindow(0, "main", newPaneWithToken(0, "/work", "scrollback/work__0.0.bin", "tokA")),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		rec := sink.Records().Only(t, "log record")
		got := rec.AttrString(t, "pane_key")
		if want := state.SanitizePaneKey("work", 5, 5); got != want {
			t.Errorf("pane_key = %q, want the live structural key %q", got, want)
		}
		if got == state.SanitizePaneKey("work", 0, 0) {
			t.Errorf("pane_key = %q, must not be the saved key", got)
		}
		if got == "tokA" {
			t.Errorf("pane_key = %q, must not be the token", got)
		}
	})

	t.Run("it keeps stamping the remaining panes after one fails", func(t *testing.T) {
		mock := commandertest.FromFunc(failOnPaneOptionTarget("0:0\n0:1", "=work:0.0"))
		logger, _ := logtest.NewCaptureLogger(t)
		r := &restore.SessionRestorer{Client: tmux.NewClient(mock), StateDir: t.TempDir(), Logger: logger}

		sess := newSession("work", nil,
			newWindow(0, "main",
				newPaneWithToken(0, "/work", "scrollback/work__0.0.bin", "tokA"),
				newPaneWithToken(1, "/work", "scrollback/work__0.1.bin", "tokB"),
			),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		stamps := setPaneOptionCalls(mock.Calls())
		if len(stamps) != 2 {
			t.Fatalf("set-option -p calls = %d, want 2 (the second is still attempted); calls: %v", len(stamps), mock.Calls())
		}
		assertPaneTokenStamp(t, stamps[1], "=work:0.1", "tokB")
	})

	t.Run("it stamps only the paired prefix when more live panes than saved", func(t *testing.T) {
		mock := commandertest.FromFunc(restoreRunFunc("0:0\n0:1\n0:2"))
		logger, _ := logtest.NewCaptureLogger(t)
		r := &restore.SessionRestorer{Client: tmux.NewClient(mock), StateDir: t.TempDir(), Logger: logger}

		sess := newSession("work", nil,
			newWindow(0, "main",
				newPaneWithToken(0, "/work", "scrollback/work__0.0.bin", "tokA"),
				newPaneWithToken(1, "/work", "scrollback/work__0.1.bin", "tokB"),
			),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		stamps := setPaneOptionCalls(mock.Calls())
		if len(stamps) != 2 {
			t.Fatalf("set-option -p calls = %d, want 2; calls: %v", len(stamps), mock.Calls())
		}
		assertPaneTokenStamp(t, stamps[0], "=work:0.0", "tokA")
		assertPaneTokenStamp(t, stamps[1], "=work:0.1", "tokB")
		for _, c := range stamps {
			if c[3] == "=work:0.2" {
				t.Errorf("unpaired live pane work:0.2 was stamped: %v", c)
			}
		}
	})

	t.Run("it stamps only the paired prefix when more saved panes than live", func(t *testing.T) {
		mock := commandertest.FromFunc(restoreRunFunc("0:0\n0:1"))
		logger, _ := logtest.NewCaptureLogger(t)
		r := &restore.SessionRestorer{Client: tmux.NewClient(mock), StateDir: t.TempDir(), Logger: logger}

		sess := newSession("work", nil,
			newWindow(0, "main",
				newPaneWithToken(0, "/work", "scrollback/work__0.0.bin", "tokA"),
				newPaneWithToken(1, "/work", "scrollback/work__0.1.bin", "tokB"),
				newPaneWithToken(2, "/work", "scrollback/work__0.2.bin", "tokC"),
			),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		stamps := setPaneOptionCalls(mock.Calls())
		if len(stamps) != 2 {
			t.Fatalf("set-option -p calls = %d, want 2; calls: %v", len(stamps), mock.Calls())
		}
		assertPaneTokenStamp(t, stamps[0], "=work:0.0", "tokA")
		assertPaneTokenStamp(t, stamps[1], "=work:0.1", "tokB")
		for _, c := range stamps {
			if c[5] == "tokC" {
				t.Errorf("unpaired saved token tokC was stamped: %v", c)
			}
		}
	})

	t.Run("it emits no warn on a boot where every saved pane is unstamped", func(t *testing.T) {
		mock := commandertest.FromFunc(restoreRunFunc("0:0\n0:1\n1:0"))
		logger, sink := logtest.NewCaptureLogger(t)
		r := &restore.SessionRestorer{Client: tmux.NewClient(mock), StateDir: t.TempDir(), Logger: logger}

		sess := newSession("work", nil,
			newWindow(0, "main",
				newPane(0, "/work", "scrollback/work__0.0.bin"),
				newPane(1, "/work", "scrollback/work__0.1.bin"),
			),
			newWindow(1, "logs",
				newPane(0, "/work", "scrollback/work__1.0.bin"),
			),
		)

		if _, err := r.Restore(sess); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		for _, rec := range sink.Records() {
			if rec.Level >= slog.LevelWarn {
				t.Errorf("unexpected WARN on an all-unstamped restore: %s %v", rec.Msg, rec.Keys)
			}
		}
	})
}
