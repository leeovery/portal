package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
)

// newExitError returns a real *exec.ExitError, the shape tmux's has-session
// produces for a genuinely absent session. An absent-saver mock must return
// this: a bare errors.New would be misclassified as a transient fault.
func newExitError(t *testing.T) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit 1").Run()
	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	return err
}

func installUninstallDeps(t *testing.T, deps *UninstallDeps) {
	t.Helper()
	withUninstallDeps(t, *deps)
}

// wantCompletionMessage is hard-coded rather than referenced from production,
// so a drift in the production string fails the test.
const wantCompletionMessage = "Portal's tmux runtime removed. Your saved sessions and config are untouched at ~/.config/portal/.\n" +
	"To remove Portal completely, uninstall the binary and delete that directory.\n"

func TestUninstall_KillsPortalSaverBeforeRemovingHooks(t *testing.T) {
	raw := "session-created[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n"
	cmder := commandertest.New(t,
		commandertest.Returns("", "info"),
		commandertest.Returns("", "has-session"),
		commandertest.Returns("", "kill-session"),
		commandertest.Returns(raw, "show-hooks"),
		commandertest.Returns("", "set-hook"),
	)
	installUninstallDeps(t, &UninstallDeps{Client: tmux.NewClient(cmder)})

	out, _, err := runRootCmd(t, "uninstall")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hasSessionIdx := cmder.CallsMatching("has-session", tmux.PortalSaverName).FirstIndex()
	killIdx := cmder.CallsMatching("kill-session", tmux.PortalSaverName).FirstIndex()
	showHooksIdx := cmder.CallsMatching("show-hooks").FirstIndex()
	setHookIdx := cmder.CallsMatching("set-hook", "-gu").FirstIndex()

	if hasSessionIdx < 0 {
		t.Fatalf("expected has-session %s call, got calls=%v", tmux.PortalSaverName, cmder.Calls())
	}
	if killIdx < 0 {
		t.Fatalf("expected kill-session %s call, got calls=%v", tmux.PortalSaverName, cmder.Calls())
	}
	if showHooksIdx < 0 {
		t.Fatalf("expected show-hooks call, got calls=%v", cmder.Calls())
	}
	if setHookIdx < 0 {
		t.Fatalf("expected set-hook -gu call, got calls=%v", cmder.Calls())
	}
	if hasSessionIdx >= killIdx || killIdx >= showHooksIdx || showHooksIdx >= setHookIdx {
		t.Errorf("expected order has-session(%d) < kill-session(%d) < show-hooks(%d) < set-hook(%d); calls=%v",
			hasSessionIdx, killIdx, showHooksIdx, setHookIdx, cmder.Calls())
	}
	if anchorKillIdx := cmder.CallsMatching("kill-session", tmux.PortalBootstrapName).FirstIndex(); anchorKillIdx >= 0 {
		t.Errorf("kill-session must never target %s (the load-bearing anchor); calls=%v", tmux.PortalBootstrapName, cmder.Calls())
	}
	if out.String() != wantCompletionMessage {
		t.Errorf("completion message mismatch:\n got %q\nwant %q", out.String(), wantCompletionMessage)
	}
}

func TestUninstall_NoServerRunningIsGracefulNoOpAndPrintsMessage(t *testing.T) {
	// Nothing beyond the server probe is scripted: any further call fails the
	// test, which is the "graceful no-op" claim.
	cmder := commandertest.New(t,
		commandertest.Fails(errors.New("no server running on /tmp/tmux-501/default"), "info"),
	)
	installUninstallDeps(t, &UninstallDeps{Client: tmux.NewClient(cmder)})

	out, _, err := runRootCmd(t, "uninstall")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range cmder.Calls() {
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
	// kill-session is deliberately unscripted: invoking it when the saver is
	// absent fails the test through the unmatched default.
	cmder := commandertest.New(t,
		commandertest.Returns("", "info"),
		commandertest.Fails(newExitError(t), "has-session"),
		commandertest.Returns("", "show-hooks"),
	)
	installUninstallDeps(t, &UninstallDeps{Client: tmux.NewClient(cmder)})

	out, _, err := runRootCmd(t, "uninstall")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range cmder.Calls() {
		if len(c) >= 1 && c[0] == "kill-session" {
			t.Errorf("kill-session must not be invoked when saver absent, got %v", c)
		}
	}
	if out.String() != wantCompletionMessage {
		t.Errorf("completion message mismatch:\n got %q\nwant %q", out.String(), wantCompletionMessage)
	}
}

func TestUninstall_PrintsExactCompletionMessage(t *testing.T) {
	cmder := commandertest.New(t,
		commandertest.Returns("", "info"),
		commandertest.Fails(newExitError(t), "has-session"),
		commandertest.Returns("", "show-hooks"),
	)
	installUninstallDeps(t, &UninstallDeps{Client: tmux.NewClient(cmder)})

	out, errBuf, err := runRootCmd(t, "uninstall")
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
	cmder := commandertest.New(t,
		commandertest.Returns("", "info"),
		commandertest.Returns("", "has-session"),
		commandertest.Returns("", "kill-session"),
	)
	installUninstallDeps(t, &UninstallDeps{
		Client:     tmux.NewClient(cmder),
		Unregister: stub,
	})

	out, _, err := runRootCmd(t, "uninstall")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
	}
	if !strings.Contains(err.Error(), "hook removal") {
		t.Errorf("error %q does not contain 'hook removal'", err.Error())
	}
	if cmder.CallsMatching("kill-session", tmux.PortalSaverName).FirstIndex() < 0 {
		t.Errorf("expected kill-session %s despite hook removal failure, got calls=%v", tmux.PortalSaverName, cmder.Calls())
	}
	if out.String() != wantCompletionMessage {
		t.Errorf("completion message must print on partial failure:\n got %q\nwant %q", out.String(), wantCompletionMessage)
	}
}

func TestUninstall_TransientProbeFaultSurfacesErrorNotSilentRemoval(t *testing.T) {
	// An OS-layer fault does not unwrap to *exec.ExitError, so the probe reports
	// present-with-error. Folding it into the returned error is what stops
	// uninstall exiting 0 with a false "removed" claim.
	probeFault := errors.New(`exec: "tmux": executable file not found in $PATH`)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	// kill-session is deliberately unscripted: running it on a transient probe
	// fault fails the test through the unmatched default.
	cmder := commandertest.New(t,
		commandertest.Returns("", "info"),
		commandertest.Fails(probeFault, "has-session"),
	)
	installUninstallDeps(t, &UninstallDeps{
		Client:     tmux.NewClient(cmder),
		Unregister: func(*tmux.Client) error { return nil },
		Logger:     logger,
	})

	out, _, err := runRootCmd(t, "uninstall")
	if err == nil {
		t.Fatal("expected an error from the transient probe fault, got nil (false silent 'removed')")
	}
	if !strings.Contains(err.Error(), "daemon kill") {
		t.Errorf("error %q does not contain the 'daemon kill' wrap", err.Error())
	}
	if !errors.Is(err, probeFault) {
		t.Errorf("error %v does not wrap the underlying probe fault %v", err, probeFault)
	}

	logged := sink.Body()
	if strings.Contains(logged, "killed _portal-saver") {
		t.Errorf("must NOT claim saver removal on a probe fault; log:\n%s", logged)
	}
	if !strings.Contains(logged, "WARN") {
		t.Errorf("expected a WARN log for the probe fault; log:\n%s", logged)
	}

	if out.String() != wantCompletionMessage {
		t.Errorf("completion message must print on a probe fault:\n got %q\nwant %q", out.String(), wantCompletionMessage)
	}
}

func TestUninstall_RegisteredInSkipTmuxCheck(t *testing.T) {
	if !bootstrapExempt["uninstall"] {
		t.Fatal("uninstall must be registered in bootstrapExempt (bootstrap-exempt)")
	}
}

func TestUninstall_KillSessionOtherFailureContributesJoinedErrorAndStillRunsUnregister(t *testing.T) {
	unregisterCalled := false
	stub := func(_ *tmux.Client) error {
		unregisterCalled = true
		return nil
	}
	cmder := commandertest.New(t,
		commandertest.Returns("", "info"),
		commandertest.Returns("", "has-session"),
		commandertest.Fails(errors.New("permission denied"), "kill-session"),
	)
	installUninstallDeps(t, &UninstallDeps{
		Client:     tmux.NewClient(cmder),
		Unregister: stub,
	})

	out, _, err := runRootCmd(t, "uninstall")
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

	cmder := commandertest.New(t,
		commandertest.Returns("", "info"),
		commandertest.Returns("", "has-session"),
		commandertest.Returns("", "kill-session"),
		commandertest.Returns("", "show-hooks"),
	)
	installUninstallDeps(t, &UninstallDeps{
		Client: tmux.NewClient(cmder),
		Logger: logger,
	})

	if _, _, err := runRootCmd(t, "uninstall"); err != nil {
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
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: panicRunner{}})

	cmder := commandertest.New(t,
		commandertest.Returns("", "info"),
		commandertest.Fails(newExitError(t), "has-session"),
		commandertest.Returns("", "show-hooks"),
	)
	installUninstallDeps(t, &UninstallDeps{Client: tmux.NewClient(cmder)})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PersistentPreRunE invoked bootstrap: %v", r)
		}
	}()

	if _, _, err := runRootCmd(t, "uninstall"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// killSaverOutcome runs uninstall against a scripted tmux whose kill-session
// fails with killErr, and reports what the user was told: the captured
// daemon-component log and the returned error.
func killSaverOutcome(t *testing.T, killErr error) (string, error) {
	t.Helper()
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	cmder := commandertest.New(t,
		commandertest.Returns("", "info"),
		commandertest.Returns("", "has-session"),
		commandertest.Fails(killErr, "kill-session"),
	)
	installUninstallDeps(t, &UninstallDeps{
		Client:     tmux.NewClient(cmder),
		Unregister: func(*tmux.Client) error { return nil },
		Logger:     logger,
	})

	out, _, err := runRootCmd(t, "uninstall")
	if out.String() != wantCompletionMessage {
		t.Errorf("completion message mismatch:\n got %q\nwant %q", out.String(), wantCompletionMessage)
	}
	return sink.Body(), err
}

func TestUninstall_ClassifiesKillSessionFailureBySentinel(t *testing.T) {
	t.Run("it reports a clean removal when the saver session is genuinely absent", func(t *testing.T) {
		// The shape a real tmux exit carries: only a *CommandError reaches the
		// client's ErrNoSuchSession wrap.
		absent := &tmux.CommandError{Stderr: "can't find session: _portal-saver", Err: newExitError(t)}

		logged, err := killSaverOutcome(t, absent)
		if err != nil {
			t.Fatalf("expected a clean removal, got error: %v", err)
		}
		if !strings.Contains(logged, killSaverInfoMessage) {
			t.Errorf("log missing the removal message %q; log:\n%s", killSaverInfoMessage, logged)
		}
	})

	t.Run("it reports a failure when the kill failed on an unaddressable saver name", func(t *testing.T) {
		// tmux answers an unaddressable target with the very same stderr a
		// vanished session produces; only the classification tells them apart.
		unaddressable := fmt.Errorf("%w: can't find session: _portal-saver", tmux.ErrUnaddressableSessionName)

		logged, err := killSaverOutcome(t, unaddressable)
		if err == nil {
			t.Fatal("expected an error for an unaddressable saver name, got nil (false 'removed')")
		}
		if !errors.Is(err, tmux.ErrUnaddressableSessionName) {
			t.Errorf("error %v does not wrap tmux.ErrUnaddressableSessionName", err)
		}
		if strings.Contains(logged, killSaverInfoMessage) {
			t.Errorf("must not log a removal for a saver it did not kill; log:\n%s", logged)
		}
		if !strings.Contains(logged, "WARN") {
			t.Errorf("expected a WARN for the failed kill; log:\n%s", logged)
		}
	})

	t.Run("it surfaces any other kill failure as an error", func(t *testing.T) {
		sentinel := errors.New("permission denied")

		logged, err := killSaverOutcome(t, sentinel)
		if err == nil {
			t.Fatal("expected an error from the kill failure, got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("error %v does not wrap the underlying kill failure %v", err, sentinel)
		}
		if strings.Contains(logged, killSaverInfoMessage) {
			t.Errorf("must not log a removal for a failed kill; log:\n%s", logged)
		}
	})

	t.Run("it matches on the sentinel rather than tmux's stderr wording", func(t *testing.T) {
		// Absence classified by the client, worded differently: the sentinel is
		// the whole contract, tmux's phrasing is not.
		reworded := fmt.Errorf("%w: session vanished", tmux.ErrNoSuchSession)

		logged, err := killSaverOutcome(t, reworded)
		if err != nil {
			t.Fatalf("expected a clean removal on the sentinel alone, got error: %v", err)
		}
		if !strings.Contains(logged, killSaverInfoMessage) {
			t.Errorf("log missing the removal message %q; log:\n%s", killSaverInfoMessage, logged)
		}
	})
}
