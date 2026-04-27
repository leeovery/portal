// Tests in this file mutate package-level state (stateCleanupDeps,
// bootstrapDeps) and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

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
	// Stateful mock: first run sees a Portal entry, second run sees an empty
	// hook table. Both runs go through ServerRunning -> show-hooks.
	var removed bool
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
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
}

func TestStateCleanup_AcceptsPurgeFlagWithoutError(t *testing.T) {
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
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

// stateCleanupPanicBootstrapper implements ServerBootstrapper but panics on
// any call. Used to prove that PersistentPreRunE never invokes bootstrap for
// the state cleanup command (state is in skipTmuxCheck).
type stateCleanupPanicBootstrapper struct{}

func (stateCleanupPanicBootstrapper) EnsureServer() (bool, error) {
	panic("state cleanup must not invoke bootstrap (state is in skipTmuxCheck)")
}

func TestStateCleanup_DoesNotInvokeBootstrap(t *testing.T) {
	bootstrapDeps = &BootstrapDeps{Bootstrapper: stateCleanupPanicBootstrapper{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
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

func TestStateCleanup_DoesNotKillPortalSaverInPhase1(t *testing.T) {
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "show-hooks":
				return "", nil
			}
			if args[0] == "kill-session" {
				t.Fatalf("phase 1 must not kill _portal-saver: %v", args)
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
			t.Errorf("kill-session must not be invoked in phase 1, got %v", c)
		}
	}
}
