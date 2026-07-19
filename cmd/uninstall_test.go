// Tests in this file mutate package-level state (uninstallDeps, bootstrapDeps)
// and MUST NOT use t.Parallel.
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
// *tmux.Client end-to-end. Shared by many cmd tests.
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

// installUninstallDeps overrides uninstallDeps for the duration of the test,
// restoring the previous value via t.Cleanup.
func installUninstallDeps(t *testing.T, deps *UninstallDeps) {
	t.Helper()
	prev := uninstallDeps
	uninstallDeps = deps
	t.Cleanup(func() { uninstallDeps = prev })
}

// runUninstall executes "portal uninstall" and returns stdout/stderr buffers
// and the Execute error.
func runUninstall(t *testing.T, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(append([]string{"uninstall"}, args...))
	err := rootCmd.Execute()
	return outBuf, errBuf, err
}

// wantCompletionMessage is the byte-exact two-line completion message printed
// on every uninstall path (spec § uninstall — Runtime-Only Teardown). Hard-
// coded here (not referenced from production) so a drift in the production
// string fails the test.
const wantCompletionMessage = "Portal's tmux runtime removed. Your saved sessions and config are untouched at ~/.config/portal/.\n" +
	"To remove Portal completely, uninstall the binary and delete that directory.\n"

func TestUninstall_KillsPortalSaverBeforeRemovingHooks(t *testing.T) {
	raw := "session-created[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n"
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil // server running
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
	installUninstallDeps(t, &UninstallDeps{Client: tmux.NewClient(cmder)})

	out, _, err := runUninstall(t)
	if err != nil {
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
	if out.String() != wantCompletionMessage {
		t.Errorf("completion message mismatch:\n got %q\nwant %q", out.String(), wantCompletionMessage)
	}
}

func TestUninstall_NoServerRunningIsGracefulNoOpAndPrintsMessage(t *testing.T) {
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			if args[0] == "info" {
				return "", errors.New("no server running on /tmp/tmux-501/default")
			}
			t.Fatalf("unexpected tmux call when no server running: %v", args)
			return "", nil
		},
	}
	installUninstallDeps(t, &UninstallDeps{Client: tmux.NewClient(cmder)})

	out, _, err := runUninstall(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Neither the kill nor the unregister may run when the server is down.
	for _, c := range cmder.Calls {
		switch c[0] {
		case "has-session", "kill-session", "show-hooks", "set-hook":
			t.Errorf("expected no %q call when server down, got %v", c[0], c)
		}
	}
	if out.String() != wantCompletionMessage {
		t.Errorf("completion message mismatch on down server:\n got %q\nwant %q", out.String(), wantCompletionMessage)
	}
}

func TestUninstall_IsIdempotentWhenSaverAbsent(t *testing.T) {
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
	installUninstallDeps(t, &UninstallDeps{Client: tmux.NewClient(cmder)})

	out, _, err := runUninstall(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range cmder.Calls {
		if len(c) >= 1 && c[0] == "kill-session" {
			t.Errorf("kill-session must not be invoked when saver absent, got %v", c)
		}
	}
	if out.String() != wantCompletionMessage {
		t.Errorf("completion message mismatch:\n got %q\nwant %q", out.String(), wantCompletionMessage)
	}
}

func TestUninstall_PrintsExactCompletionMessage(t *testing.T) {
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
	installUninstallDeps(t, &UninstallDeps{Client: tmux.NewClient(cmder)})

	out, errBuf, err := runUninstall(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.String() != wantCompletionMessage {
		t.Errorf("stdout mismatch:\n got %q\nwant %q", out.String(), wantCompletionMessage)
	}
	if errBuf.Len() != 0 {
		t.Errorf("expected no stderr, got %q", errBuf.String())
	}
}

func TestUninstall_AccumulatesHookRemovalFailureWithoutSkippingKill(t *testing.T) {
	sentinel := errors.New("show-hooks blew up")
	stub := func(_ *tmux.Client) error {
		return sentinel
	}
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "info":
				return "", nil
			case "has-session":
				return "", nil // saver present
			case "kill-session":
				return "", nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	installUninstallDeps(t, &UninstallDeps{
		Client:     tmux.NewClient(cmder),
		Unregister: stub,
	})

	out, _, err := runUninstall(t)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
	}
	if !strings.Contains(err.Error(), "hook removal") {
		t.Errorf("error %q does not contain 'hook removal'", err.Error())
	}
	// The kill must still have run despite the hook-removal failure.
	if callIndex(cmder.Calls, "kill-session", tmux.PortalSaverName) < 0 {
		t.Errorf("expected kill-session %s despite hook removal failure, got calls=%v", tmux.PortalSaverName, cmder.Calls)
	}
	// The completion message must still print on a partial-failure return.
	if out.String() != wantCompletionMessage {
		t.Errorf("completion message must print on partial failure:\n got %q\nwant %q", out.String(), wantCompletionMessage)
	}
}

func TestUninstall_RegisteredInSkipTmuxCheck(t *testing.T) {
	if !skipTmuxCheck["uninstall"] {
		t.Fatal("uninstall must be registered in skipTmuxCheck (bootstrap-exempt)")
	}
}

func TestUninstall_ToleratesKillSessionCantFindSessionError(t *testing.T) {
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
	installUninstallDeps(t, &UninstallDeps{Client: tmux.NewClient(cmder)})

	out, _, err := runUninstall(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Hook removal must still proceed after the idempotent kill error.
	if got := setHookCalls(cmder.Calls); len(got) != 1 || got[0] != "session-created[0]" {
		t.Errorf("expected hook removal to run after idempotent kill error; got set-hook -gu calls=%v", got)
	}
	if out.String() != wantCompletionMessage {
		t.Errorf("completion message mismatch:\n got %q\nwant %q", out.String(), wantCompletionMessage)
	}
}

func TestUninstall_KillSessionOtherFailureContributesJoinedErrorAndStillRunsUnregister(t *testing.T) {
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
	installUninstallDeps(t, &UninstallDeps{
		Client:     tmux.NewClient(cmder),
		Unregister: stub,
	})

	out, _, err := runUninstall(t)
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
		t.Error("hook removal must still be invoked after KillSession failure")
	}
	if out.String() != wantCompletionMessage {
		t.Errorf("completion message must print on kill failure:\n got %q\nwant %q", out.String(), wantCompletionMessage)
	}
}

func TestUninstall_LogsInfoWhenSaverKilledSuccessfully(t *testing.T) {
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
	installUninstallDeps(t, &UninstallDeps{
		Client: tmux.NewClient(cmder),
		Logger: logger,
	})

	if _, _, err := runUninstall(t); err != nil {
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

func TestUninstall_DoesNotInvokeBootstrap(t *testing.T) {
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
	installUninstallDeps(t, &UninstallDeps{Client: tmux.NewClient(cmder)})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PersistentPreRunE invoked bootstrap: %v", r)
		}
	}()

	if _, _, err := runUninstall(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
