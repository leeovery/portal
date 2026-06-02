// Tests in this file mutate package-level state (stateCleanupDeps,
// bootstrapDeps) and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// canonicalTempDir was historically required because purgeStateDir compared
// dir against filepath.EvalSymlinks(dir) and rejected any mismatch. macOS's
// /var → /private/var redirect tripped that check. Review remediation cycle 1
// dropped the strict check (relies on Lstat for leaf-symlink protection only),
// so this helper is now a thin alias for t.TempDir kept to avoid churn at the
// callsites.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// recordingCommander is a tmux.Commander that records every Run call and
// dispatches via an optional RunFunc. Mirrors internal/tmux/MockCommander
// shape but lives in the cmd package so cmd-level tests can drive a real
// *tmux.Client end-to-end.
type recordingCommander struct {
	mu      sync.Mutex
	Calls   [][]string
	RunFunc func(args ...string) (string, error)
	Output  string
	Err     error
}

func (r *recordingCommander) Run(args ...string) (string, error) {
	r.mu.Lock()
	r.Calls = append(r.Calls, args)
	r.mu.Unlock()
	if r.RunFunc != nil {
		return r.RunFunc(args...)
	}
	return r.Output, r.Err
}

// RunRaw mirrors Run but represents the no-trim variant. Recording behaviour
// stays identical so test assertions on Calls work regardless of which method
// the production code reaches.
func (r *recordingCommander) RunRaw(args ...string) (string, error) {
	r.mu.Lock()
	r.Calls = append(r.Calls, args)
	r.mu.Unlock()
	if r.RunFunc != nil {
		return r.RunFunc(args...)
	}
	return r.Output, r.Err
}

// setHookCalls returns the "set-hook -gu <target>" calls in invocation order.
func setHookCalls(calls [][]string) []string {
	var out []string
	for _, c := range calls {
		if len(c) >= 3 && c[0] == "set-hook" && c[1] == "-gu" {
			out = append(out, c[2])
		}
	}
	return out
}

// installStateCleanupDeps overrides stateCleanupDeps for the duration of the
// test, restoring the previous value via t.Cleanup.
func installStateCleanupDeps(t *testing.T, deps *StateCleanupDeps) {
	t.Helper()
	prev := stateCleanupDeps
	stateCleanupDeps = deps
	t.Cleanup(func() { stateCleanupDeps = prev })
}

// runStateCleanup executes "portal state cleanup" with the supplied flag args
// and returns stdout/stderr buffers and the Execute error.
func runStateCleanup(t *testing.T, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	resetRootCmd()
	resetStateCmdFlags()
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(append([]string{"state", "cleanup"}, args...))
	err := rootCmd.Execute()
	return outBuf, errBuf, err
}

func TestStateCleanup_RemovesPortalHookEntries(t *testing.T) {
	raw := "session-created[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n" +
		"client-attached[1] run-shell 'command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}'\n"
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil // server running
			case "has-session":
				// _portal-saver absent — saver kill is a no-op for this test.
				return "", errors.New("can't find session: _portal-saver")
			case "show-hooks":
				return raw, nil
			case "set-hook":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	_, _, err := runStateCleanup(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := setHookCalls(cmder.Calls)
	want := []string{"session-created[0]", "client-attached[1]"}
	if len(got) != len(want) {
		t.Fatalf("set-hook -gu calls = %v, want %v", got, want)
	}
	for i, g := range got {
		if g != want[i] {
			t.Errorf("call[%d] = %q, want %q", i, g, want[i])
		}
	}
}

func TestStateCleanup_NoServerRunningExitsZeroAndIssuesZeroSetHookCalls(t *testing.T) {
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			if args[0] == "info" {
				return "", errors.New("no server running on /tmp/tmux-501/default")
			}
			t.Fatalf("unexpected tmux call when no server running: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	_, _, err := runStateCleanup(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := setHookCalls(cmder.Calls); len(got) != 0 {
		t.Errorf("expected 0 set-hook -gu calls, got %d: %v", len(got), got)
	}
	for _, c := range cmder.Calls {
		if c[0] == "show-hooks" {
			t.Errorf("expected no show-hooks call when server not running, got %v", c)
		}
	}
}

func TestStateCleanup_NoPortalHookEntriesExitsZero(t *testing.T) {
	raw := "session-created[0] run-shell 'tmux-resurrect save'\n" +
		"session-closed[0] run-shell 'user-defined notify'\n"
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", errors.New("can't find session: _portal-saver")
			case "show-hooks":
				return raw, nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	_, _, err := runStateCleanup(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := setHookCalls(cmder.Calls); len(got) != 0 {
		t.Errorf("expected 0 set-hook -gu calls, got %d: %v", len(got), got)
	}
}

func TestStateCleanup_UnregisterFailureReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("show-hooks blew up")
	stub := func(_ *tmux.Client) error {
		return sentinel
	}
	installStateCleanupDeps(t, &StateCleanupDeps{
		Client:     tmux.NewClient(&recordingCommander{}), // server-running default (Err=nil)
		Unregister: stub,
	})

	_, _, err := runStateCleanup(t)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
	}
	if !strings.Contains(err.Error(), "hook removal") {
		t.Errorf("error %q does not contain 'hook removal'", err.Error())
	}
}

func TestStateCleanup_IsNoOpOnSecondInvocation(t *testing.T) {
	// Stateful mock: first run sees a live _portal-saver and a Portal hook
	// entry; second run sees neither. Both runs go through ServerRunning ->
	// has-session -> (kill-session) -> show-hooks.
	var saverGone bool
	var removed bool
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				if saverGone {
					return "", errors.New("can't find session: _portal-saver")
				}
				return "", nil
			case "kill-session":
				saverGone = true
				return "", nil
			case "show-hooks":
				if removed {
					return "", nil
				}
				return "session-created[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n", nil
			case "set-hook":
				removed = true
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	if _, _, err := runStateCleanup(t); err != nil {
		t.Fatalf("first run: unexpected error: %v", err)
	}
	firstRun := len(setHookCalls(cmder.Calls))
	if firstRun != 1 {
		t.Fatalf("first run set-hook -gu count = %d, want 1", firstRun)
	}

	cmder.Calls = nil
	if _, _, err := runStateCleanup(t); err != nil {
		t.Fatalf("second run: unexpected error: %v", err)
	}
	if got := setHookCalls(cmder.Calls); len(got) != 0 {
		t.Errorf("second run produced %d removals, want 0 (idempotent): %v", len(got), got)
	}
	for _, c := range cmder.Calls {
		if c[0] == "kill-session" {
			t.Errorf("second run must not invoke kill-session, got %v", c)
		}
	}
}

func TestStateCleanup_AcceptsPurgeFlagWithoutError(t *testing.T) {
	dir := canonicalTempDir(t)
	t.Setenv("PORTAL_STATE_DIR", dir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", errors.New("can't find session: _portal-saver")
			case "show-hooks":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	if _, _, err := runStateCleanup(t, "--purge"); err != nil {
		t.Fatalf("unexpected error with --purge: %v", err)
	}
}

// TestStateCleanup_LeavesStateDirIntactWithoutPurgeFlag asserts that omitting
// --purge never touches the state directory, even when the directory exists
// and contains data.
func TestStateCleanup_LeavesStateDirIntactWithoutPurgeFlag(t *testing.T) {
	dir := canonicalTempDir(t)
	t.Setenv("PORTAL_STATE_DIR", dir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	canary := filepath.Join(dir, "sessions.json")
	if err := os.WriteFile(canary, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write canary: %v", err)
	}

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", errors.New("can't find session: _portal-saver")
			case "show-hooks":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	if _, _, err := runStateCleanup(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("state dir must remain when --purge omitted, got stat err: %v", err)
	}
	if _, err := os.Stat(canary); err != nil {
		t.Fatalf("state dir contents must remain when --purge omitted, got stat err: %v", err)
	}
}

// TestStateCleanup_PurgeRemovesStateDir asserts --purge wipes the state
// directory and its contents.
func TestStateCleanup_PurgeRemovesStateDir(t *testing.T) {
	dir := canonicalTempDir(t)
	t.Setenv("PORTAL_STATE_DIR", dir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scrollback", "pane-0.bin"), []byte("scroll"), 0o600); err != nil {
		t.Fatalf("write scrollback: %v", err)
	}

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", errors.New("can't find session: _portal-saver")
			case "show-hooks":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	if _, _, err := runStateCleanup(t, "--purge"); err != nil {
		t.Fatalf("unexpected error with --purge: %v", err)
	}

	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state dir must be gone after --purge, stat err = %v", err)
	}
}

// TestStateCleanup_PurgeIsIdempotentOnMissingStateDir asserts --purge succeeds
// when PORTAL_STATE_DIR points at a path that does not exist.
func TestStateCleanup_PurgeIsIdempotentOnMissingStateDir(t *testing.T) {
	parent := canonicalTempDir(t)
	dir := filepath.Join(parent, "does-not-exist")
	t.Setenv("PORTAL_STATE_DIR", dir)

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", errors.New("can't find session: _portal-saver")
			case "show-hooks":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	if _, _, err := runStateCleanup(t, "--purge"); err != nil {
		t.Fatalf("unexpected error with --purge on missing dir: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing dir must remain absent after --purge, stat err = %v", err)
	}
}

// TestStateCleanup_PurgeLogsInfoOnSuccess asserts a successful purge writes an
// INFO/ComponentDaemon entry to portal.log.
func TestStateCleanup_PurgeLogsInfoOnSuccess(t *testing.T) {
	// The captured slog logger survives the purge of the state dir under test
	// (it is in-memory), so no separate on-disk log directory is needed; we
	// inject it directly via StateCleanupDeps.Logger.
	stateDir := canonicalTempDir(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	logger, sink := newCaptureLoggerForComponent(t, "daemon")

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", errors.New("can't find session: _portal-saver")
			case "show-hooks":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{
		Client: tmux.NewClient(cmder),
		Logger: logger,
	})

	if _, _, err := runStateCleanup(t, "--purge"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logged := sink.Body()
	if !strings.Contains(logged, "INFO") {
		t.Errorf("log missing INFO level entry: %q", logged)
	}
	if !strings.Contains(logged, "daemon") {
		t.Errorf("log missing %q component: %q", "daemon", logged)
	}
	if !strings.Contains(logged, "purged state directory") {
		t.Errorf("log missing purge confirmation: %q", logged)
	}
	if !strings.Contains(logged, stateDir) {
		t.Errorf("log missing state dir path %q: %q", stateDir, logged)
	}
}

// TestStateCleanup_PurgeRefusesSymlinkedStateDir asserts --purge declines to
// remove a state directory whose path is itself a symlink.
func TestStateCleanup_PurgeRefusesSymlinkedStateDir(t *testing.T) {
	parent := canonicalTempDir(t)
	target := filepath.Join(parent, "real-state")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	link := filepath.Join(parent, "link-state")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	t.Setenv("PORTAL_STATE_DIR", link)

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", errors.New("can't find session: _portal-saver")
			case "show-hooks":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	_, _, err := runStateCleanup(t, "--purge")
	if err == nil {
		t.Fatal("expected error refusing to purge symlinked state dir, got nil")
	}
	if !strings.Contains(err.Error(), "refusing to purge symlinked") {
		t.Errorf("error %q does not contain 'refusing to purge symlinked'", err.Error())
	}
	if !strings.Contains(err.Error(), "purge state dir") {
		t.Errorf("error %q does not wrap with 'purge state dir' prefix", err.Error())
	}

	// Symlink and target must remain intact.
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("symlink must survive refusal: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("symlink target must survive refusal: %v", err)
	}
}

// TestStateCleanup_PurgeAllowsSymlinkedIntermediatePathComponents is the
// review-cycle-1 regression guard for the dropped EvalSymlinks strict-equality
// check. Earlier revisions rejected paths whose intermediate components were
// symlinks (e.g. ~/.config symlinked to a different volume), forcing users to
// purge manually. Today purgeStateDir trusts Lstat at the leaf for symlink
// protection — intermediate symlinks are fine.
func TestStateCleanup_PurgeAllowsSymlinkedIntermediatePathComponents(t *testing.T) {
	parent := t.TempDir()

	// Real config root that the leaf "state" directory will live under.
	realConfig := filepath.Join(parent, "real-config")
	if err := os.MkdirAll(realConfig, 0o700); err != nil {
		t.Fatalf("mkdir real-config: %v", err)
	}

	// Symlink that mimics ~/.config → real-config.
	linkConfig := filepath.Join(parent, "link-config")
	if err := os.Symlink(realConfig, linkConfig); err != nil {
		t.Fatalf("symlink intermediate: %v", err)
	}

	// State dir is a regular directory living UNDER the symlinked intermediate.
	stateDir := filepath.Join(linkConfig, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("mkdir state via symlink: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "sentinel"), []byte("x"), 0o600); err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", errors.New("can't find session: _portal-saver")
			case "show-hooks":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	if _, _, err := runStateCleanup(t, "--purge"); err != nil {
		t.Fatalf("purge with symlinked intermediate path component must succeed; got %v", err)
	}
	if _, err := os.Stat(stateDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("state dir under symlinked intermediate not removed: %v", err)
	}
	// The symlink and its real target must remain — only the leaf was purged.
	if _, err := os.Lstat(linkConfig); err != nil {
		t.Errorf("intermediate symlink must survive: %v", err)
	}
	if _, err := os.Stat(realConfig); err != nil {
		t.Errorf("intermediate symlink target must survive: %v", err)
	}
}

// TestStateCleanup_PurgeContributesJoinedErrorOnRemoveAllFailure asserts a
// RemoveAll failure surfaces via errors.Join with a "purge state dir" prefix
// and does not abort the join.
func TestStateCleanup_PurgeContributesJoinedErrorOnRemoveAllFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses 0o500 directory permissions")
	}

	dir := canonicalTempDir(t)
	t.Setenv("PORTAL_STATE_DIR", dir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	// Create a child directory that RemoveAll must traverse, then strip its
	// write+execute perms so RemoveAll on dir cannot recurse into it.
	stuck := filepath.Join(dir, "stuck")
	if err := os.MkdirAll(filepath.Join(stuck, "deep"), 0o700); err != nil {
		t.Fatalf("mkdir stuck: %v", err)
	}
	if err := os.Chmod(stuck, 0o500); err != nil {
		t.Fatalf("chmod stuck: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(stuck, 0o700)
	})

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", errors.New("can't find session: _portal-saver")
			case "show-hooks":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	_, _, err := runStateCleanup(t, "--purge")
	if err == nil {
		t.Fatal("expected non-nil error from RemoveAll failure")
	}
	if !strings.Contains(err.Error(), "purge state dir") {
		t.Errorf("error %q missing 'purge state dir' wrapper", err.Error())
	}
}

// TestStateCleanup_PurgeRemovesFIFOAndBinFiles asserts that --purge sweeps
// FIFOs and .bin scrollback files alongside ordinary regular files.
func TestStateCleanup_PurgeRemovesFIFOAndBinFiles(t *testing.T) {
	dir := canonicalTempDir(t)
	t.Setenv("PORTAL_STATE_DIR", dir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	binPath := filepath.Join(dir, "scrollback", "pane-0.bin")
	if err := os.WriteFile(binPath, []byte("scroll"), 0o600); err != nil {
		t.Fatalf("write bin: %v", err)
	}
	fifoPath := filepath.Join(dir, "hydrate-pane-0.fifo")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	regular := filepath.Join(dir, "sessions.json")
	if err := os.WriteFile(regular, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", errors.New("can't find session: _portal-saver")
			case "show-hooks":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	if _, _, err := runStateCleanup(t, "--purge"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, p := range []string{binPath, fifoPath, regular, dir} {
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected %s gone after --purge, stat err = %v", p, err)
		}
	}
}

func TestStateCleanup_DoesNotInvokeBootstrap(t *testing.T) {
	bootstrapDeps = &BootstrapDeps{Orchestrator: panicRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", errors.New("can't find session: _portal-saver")
			case "show-hooks":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PersistentPreRunE invoked bootstrap: %v", r)
		}
	}()

	if _, _, err := runStateCleanup(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// callIndex returns the position in calls of the first tmux invocation whose
// argv[0] matches op (and, when targetSubstr is non-empty, whose joined argv
// contains targetSubstr). Returns -1 when not found.
func callIndex(calls [][]string, op, targetSubstr string) int {
	for i, c := range calls {
		if len(c) == 0 || c[0] != op {
			continue
		}
		if targetSubstr == "" {
			return i
		}
		if strings.Contains(strings.Join(c, " "), targetSubstr) {
			return i
		}
	}
	return -1
}

func TestStateCleanup_KillsPortalSaverBeforeRemovingHooks(t *testing.T) {
	raw := "session-created[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n"
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", nil // saver present
			case "kill-session":
				return "", nil
			case "show-hooks":
				return raw, nil
			case "set-hook":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	if _, _, err := runStateCleanup(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hasSessionIdx := callIndex(cmder.Calls, "has-session", tmux.PortalSaverName)
	killIdx := callIndex(cmder.Calls, "kill-session", tmux.PortalSaverName)
	showHooksIdx := callIndex(cmder.Calls, "show-hooks", "")
	setHookIdx := callIndex(cmder.Calls, "set-hook", "-gu")

	if hasSessionIdx < 0 {
		t.Fatalf("expected has-session %s call, got calls=%v", tmux.PortalSaverName, cmder.Calls)
	}
	if killIdx < 0 {
		t.Fatalf("expected kill-session %s call, got calls=%v", tmux.PortalSaverName, cmder.Calls)
	}
	if showHooksIdx < 0 {
		t.Fatalf("expected show-hooks call, got calls=%v", cmder.Calls)
	}
	if setHookIdx < 0 {
		t.Fatalf("expected set-hook -gu call, got calls=%v", cmder.Calls)
	}
	if hasSessionIdx >= killIdx || killIdx >= showHooksIdx || showHooksIdx >= setHookIdx {
		t.Errorf("expected order has-session(%d) < kill-session(%d) < show-hooks(%d) < set-hook(%d); calls=%v",
			hasSessionIdx, killIdx, showHooksIdx, setHookIdx, cmder.Calls)
	}
}

func TestStateCleanup_IsIdempotentWhenSaverAbsent(t *testing.T) {
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", errors.New("can't find session: _portal-saver")
			case "show-hooks":
				return "", nil
			}
			if args[0] == "kill-session" {
				t.Fatalf("kill-session must not be invoked when saver absent: %v", args)
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	if _, _, err := runStateCleanup(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range cmder.Calls {
		if len(c) >= 1 && c[0] == "kill-session" {
			t.Errorf("kill-session must not be invoked when saver absent, got %v", c)
		}
	}
}

func TestStateCleanup_ToleratesKillSessionCantFindSessionError(t *testing.T) {
	raw := "session-created[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n"
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", nil // present at probe
			case "kill-session":
				// Race: tmux auto-destroyed between has-session and kill-session.
				return "", errors.New("can't find session: _portal-saver")
			case "show-hooks":
				return raw, nil
			case "set-hook":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{Client: tmux.NewClient(cmder)})

	if _, _, err := runStateCleanup(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Hook removal must still proceed.
	if got := setHookCalls(cmder.Calls); len(got) != 1 || got[0] != "session-created[0]" {
		t.Errorf("expected hook removal to run after idempotent kill error; got set-hook -gu calls=%v", got)
	}
}

func TestStateCleanup_KillSessionOtherFailureContributesJoinedErrorAndStillRunsUnregister(t *testing.T) {
	unregisterCalled := false
	stub := func(_ *tmux.Client) error {
		unregisterCalled = true
		return nil
	}
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", nil // present
			case "kill-session":
				return "", errors.New("permission denied")
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{
		Client:     tmux.NewClient(cmder),
		Unregister: stub,
	})

	_, _, err := runStateCleanup(t)
	if err == nil {
		t.Fatal("expected non-nil error from kill failure")
	}
	if !strings.Contains(err.Error(), "daemon kill") {
		t.Errorf("error %q does not contain 'daemon kill'", err.Error())
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error %q does not propagate underlying tmux error", err.Error())
	}
	if !unregisterCalled {
		t.Error("UnregisterPortalHooks must still be invoked after KillSession failure")
	}
}

func TestStateCleanup_LogsInfoWhenSaverKilledSuccessfully(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "info")

	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	logger, sink := newCaptureLoggerForComponent(t, "daemon")

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", nil
			case "kill-session":
				return "", nil
			case "show-hooks":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installStateCleanupDeps(t, &StateCleanupDeps{
		Client: tmux.NewClient(cmder),
		Logger: logger,
	})

	if _, _, err := runStateCleanup(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logged := sink.Body()
	if !strings.Contains(logged, "INFO") {
		t.Errorf("log missing INFO level entry: %q", logged)
	}
	if !strings.Contains(logged, "daemon") {
		t.Errorf("log missing %q component: %q", "daemon", logged)
	}
	if !strings.Contains(logged, "killed _portal-saver") {
		t.Errorf("log missing kill confirmation: %q", logged)
	}
	if !strings.Contains(logged, "SIGHUP") {
		t.Errorf("log missing SIGHUP wording: %q", logged)
	}
}
