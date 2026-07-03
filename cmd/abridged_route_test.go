package cmd

// Task skip-bootstrap-when-warm-2-3 — the latch-read three-way branch in
// PersistentPreRunE. A single @portal-bootstrapped read computed once upstream
// diverts satisfied commands to the abridged path (liveness-only saver + sync
// plumbing, no orchestrator, no concurrent route); every not-satisfied verdict
// (absent / version-mismatch / read-error / nil client) folds into the existing
// full-bootstrap routing.
//
// Tests mutate package-level state (bootstrapDeps, listDeps, attachDeps,
// openTUIFunc, rootCmd, the bootstrapWarnings sink, the version var, and the
// tmux.BootstrapAliveCheck / tmux.PortalSaverRetryDelay seams) and MUST NOT use
// t.Parallel.

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/warning"
	"github.com/spf13/cobra"
)

// satisfiedLatchAliveSaverCommander returns a recordingCommander whose
// @portal-bootstrapped read (show-option) returns the running version — so the
// latch reads as SATISFIED — and whose _portal-saver pane-pid probe returns a
// live pid, so the abridged path's ensureSaverLiveness is a no-op (present +
// alive). Every other tmux call returns empty/nil; the abridged path issues
// only the latch read plus the saver probe.
func satisfiedLatchAliveSaverCommander() *recordingCommander {
	return &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch {
			case len(args) > 0 && args[0] == "show-option":
				return version, nil // stored latch == running version -> satisfied
			case len(args) > 0 && args[0] == "list-panes" && isPanePIDProbe(args):
				return "12345\n", nil // _portal-saver pane alive
			}
			return "", nil
		},
	}
}

// optionAbsentErr is a *tmux.CommandError whose stderr carries tmux's
// option-absent phrasing, so GetServerOption maps it to ErrOptionNotFound and
// TryGetServerOption collapses it to ("", false, nil) — the "latch absent"
// classification.
func optionAbsentErr() error {
	return &tmux.CommandError{
		Stderr: "unknown option: @portal-bootstrapped",
		Err:    errors.New("exit status 1"),
	}
}

// notSatisfiedLatchClient returns a *tmux.Client whose @portal-bootstrapped read
// is option-absent, so the latch reads as NOT satisfied and PersistentPreRunE
// takes the full-bootstrap path deterministically — independent of any real
// tmux server the developer happens to be running (whose latch may coincide with
// the dev `version`). Every non-latch call returns empty/nil.
func notSatisfiedLatchClient() *tmux.Client {
	return tmux.NewClient(&recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "show-option" {
				return "", optionAbsentErr()
			}
			return "", nil
		},
	})
}

// installMockList wires a no-session listDeps so the `list` command's RunE
// resolves without touching the shared client, restoring it on cleanup.
func installMockList(t *testing.T) {
	t.Helper()
	listDeps = &ListDeps{
		Lister: &mockSessionLister{sessions: []tmux.Session{}},
		IsTTY:  func() bool { return false },
	}
	t.Cleanup(func() { listDeps = nil })
}

func TestPersistentPreRunE_FullBootstrap_WhenLatchAbsent(t *testing.T) {
	resetBootstrapOnce(t)

	client := tmux.NewClient(&recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "show-option" {
				return "", optionAbsentErr() // ("",false,nil) -> not satisfied
			}
			return "", nil
		},
	})
	runner := &recordingRunner{started: false}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

	installMockList(t)

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.calls != 1 {
		t.Errorf("latch absent: orchestrator calls = %d, want 1 (full bootstrap)", runner.calls)
	}
}

func TestPersistentPreRunE_FullBootstrap_OnVersionMismatch(t *testing.T) {
	resetBootstrapOnce(t)

	prevVersion := version
	version = "v2.0.0"
	t.Cleanup(func() { version = prevVersion })

	client := tmux.NewClient(&recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "show-option" {
				return "v1.0.0", nil // present but != running version -> not satisfied
			}
			return "", nil
		},
	})
	runner := &recordingRunner{started: false}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

	installMockList(t)

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.calls != 1 {
		t.Errorf("version mismatch: orchestrator calls = %d, want 1 (full bootstrap re-stamps)", runner.calls)
	}
}

func TestPersistentPreRunE_FullBootstrap_OnLatchReadError(t *testing.T) {
	resetBootstrapOnce(t)

	client := tmux.NewClient(&recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "show-option" {
				return "", errors.New("tmux socket connect failed") // read error -> not satisfied
			}
			return "", nil
		},
	})
	runner := &recordingRunner{started: false}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

	installMockList(t)

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.calls != 1 {
		t.Errorf("latch read error: orchestrator calls = %d, want 1 (folds into full bootstrap)", runner.calls)
	}
}

// TestPersistentPreRunE_Abridged_EmitsWarningsToStderrOnCLIPath proves the
// abridged CLI path drains bootstrapWarnings to stderr before RunE — identical
// to a warm command today — and never runs the full orchestrator.
func TestPersistentPreRunE_Abridged_EmitsWarningsToStderrOnCLIPath(t *testing.T) {
	resetBootstrapOnce(t)
	resetBootstrapWarnings(t)
	stubSaverAliveCheck(t, false)
	shrinkSaverRetryDelay(t)

	client := tmux.NewClient(&recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch {
			case len(args) > 0 && args[0] == "show-option":
				return version, nil // latch satisfied -> abridged
			case len(args) > 0 && args[0] == "list-panes":
				return "", noSuchSessionErr() // saver absent -> revive
			case len(args) > 0 && args[0] == "has-session":
				return "", errors.New("can't find session") // absent
			case len(args) > 0 && args[0] == "new-session":
				return "", errors.New("create denied") // revive fails across all retries
			}
			return "", nil
		},
	})
	runner := &recordingRunner{started: false}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

	installMockList(t)

	errBuf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.calls != 0 {
		t.Errorf("abridged CLI path ran the full orchestrator (%d calls); want 0", runner.calls)
	}

	wantBuf := new(bytes.Buffer)
	warning.WriteLines(wantBuf, []warning.Warning{bootstrap.SaverDownWarning()})
	if errBuf.String() != wantBuf.String() {
		t.Errorf("stderr = %q, want the rendered SaverDownWarning %q", errBuf.String(), wantBuf.String())
	}
}

// TestPersistentPreRunE_Abridged_LeavesWarningsForOpenTUIOnTUIPath proves the
// abridged TUI path does NOT flush warnings in PersistentPreRunE — they are left
// in the package sink for openTUI to stage onto the loading-page notice band.
func TestPersistentPreRunE_Abridged_LeavesWarningsForOpenTUIOnTUIPath(t *testing.T) {
	resetBootstrapOnce(t)
	resetBootstrapWarnings(t)
	stubSaverAliveCheck(t, false)
	shrinkSaverRetryDelay(t)

	client := tmux.NewClient(&recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch {
			case len(args) > 0 && args[0] == "show-option":
				return version, nil // latch satisfied -> abridged
			case len(args) > 0 && args[0] == "list-panes":
				return "", noSuchSessionErr() // saver absent -> revive
			case len(args) > 0 && args[0] == "has-session":
				return "", errors.New("can't find session")
			case len(args) > 0 && args[0] == "new-session":
				return "", errors.New("create denied") // revive fails -> SaverDownWarning
			}
			return "", nil
		},
	})
	runner := &recordingRunner{started: false}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

	var pendingAtOpenTUI []bootstrap.Warning
	origFunc := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		pendingAtOpenTUI = bootstrapWarnings.Drain()
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.calls != 0 {
		t.Errorf("abridged TUI path ran the full orchestrator (%d calls); want 0", runner.calls)
	}
	if len(pendingAtOpenTUI) != 1 {
		t.Fatalf("openTUI saw %d pending warnings, want 1 (SaverDownWarning left for the notice band)", len(pendingAtOpenTUI))
	}
	if !reflect.DeepEqual(pendingAtOpenTUI[0], bootstrap.SaverDownWarning()) {
		t.Errorf("pending warning = %#v, want %#v", pendingAtOpenTUI[0], bootstrap.SaverDownWarning())
	}
}

// TestPersistentPreRunE_Abridged_AttachTakesAbridgedPath proves attach — which
// is deliberately NOT in skipTmuxCheck (the F1 dependency) — hits the abridged
// gate on a satisfied latch: the full orchestrator never runs and the command
// proceeds normally.
func TestPersistentPreRunE_Abridged_AttachTakesAbridgedPath(t *testing.T) {
	resetBootstrapOnce(t)
	resetBootstrapWarnings(t)

	client := tmux.NewClient(satisfiedLatchAliveSaverCommander())
	runner := &recordingRunner{started: false}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

	connector := &mockSessionConnector{}
	attachDeps = &AttachDeps{
		Connector: connector,
		Validator: &mockSessionValidator{sessions: map[string]bool{"proj-abc123": true}},
	}
	t.Cleanup(func() { attachDeps = nil })

	resetRootCmd()
	rootCmd.SetArgs([]string{"attach", "proj-abc123"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.calls != 0 {
		t.Errorf("attach + satisfied latch: orchestrator calls = %d, want 0 (abridged sync path)", runner.calls)
	}
	if connector.connectedTo != "proj-abc123" {
		t.Errorf("attach did not proceed: connectedTo = %q, want proj-abc123", connector.connectedTo)
	}
}
