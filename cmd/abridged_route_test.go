package cmd

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/warning"
	"github.com/spf13/cobra"
)

func satisfiedLatchAliveSaverCommander() *commandertest.Scripted {
	return commandertest.Quiet(
		commandertest.Returns(version, "show-option"),    // stored latch == running version -> satisfied
		commandertest.When(panePIDProbe, "12345\n", nil), // _portal-saver pane alive
	)
}

// Carries no latch arm: ensureSaverLiveness never reads the latch.
func saverAbsentReviveFailsCommander() *commandertest.Scripted {
	return commandertest.Quiet(
		commandertest.Fails(noSuchSessionErr(), "list-panes"),                // saver absent -> revive
		commandertest.Fails(errors.New("can't find session"), "has-session"), // absent
		commandertest.Fails(errors.New("create denied"), "new-session"),      // revive fails across all retries
	)
}

func satisfiedLatchSaverAbsentCommander() *commandertest.Scripted {
	return commandertest.Delegating(
		saverAbsentReviveFailsCommander(),
		commandertest.Returns(version, "show-option"), // stored latch == running version -> satisfied
	)
}

// Carries tmux's option-absent phrasing, so TryGetServerOption collapses it
// to ("", false, nil) — the "latch absent" classification.
func optionAbsentErr() error {
	return &tmux.CommandError{
		Stderr: "unknown option: @portal-bootstrapped",
		Err:    errors.New("exit status 1"),
	}
}

// Reads the latch as absent, so the route is deterministic regardless of any
// real tmux server the developer is running (whose latch may coincide with
// the dev version).
func notSatisfiedLatchClient() *tmux.Client {
	return tmux.NewClient(commandertest.Quiet(
		commandertest.Fails(optionAbsentErr(), "show-option"),
	))
}

func installMockList(t *testing.T) {
	t.Helper()
	withListDeps(t, ListDeps{
		Lister: &mockSessionLister{sessions: []tmux.Session{}},
		IsTTY:  func() bool { return false },
	})
}

func TestPersistentPreRunE_FullBootstrap_WhenNotSatisfied(t *testing.T) {
	cases := []struct {
		name            string
		showOptValue    string
		showOptErr      error
		versionOverride string // "" -> leave the running version untouched
		reason          string // parenthetical rationale in the assertion message
	}{
		{
			name:       "latch absent",
			showOptErr: optionAbsentErr(), // ("",false,nil) -> not satisfied
			reason:     "full bootstrap",
		},
		{
			name:            "version mismatch",
			showOptValue:    "v1.0.0", // present but != running version -> not satisfied
			versionOverride: "v2.0.0",
			reason:          "full bootstrap re-stamps",
		},
		{
			name:       "latch read error",
			showOptErr: errors.New("tmux socket connect failed"), // read error -> not satisfied
			reason:     "folds into full bootstrap",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetBootstrapOnce(t)

			if tc.versionOverride != "" {
				prevVersion := version
				version = tc.versionOverride
				t.Cleanup(func() { version = prevVersion })
			}

			client := tmux.NewClient(commandertest.Quiet(
				commandertest.When(commandertest.ArgvPrefix("show-option"), tc.showOptValue, tc.showOptErr),
			))
			runner := &recordingRunner{started: false}
			withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner, Client: client})

			installMockList(t)

			resetRootCmd()
			rootCmd.SetArgs([]string{"list"})
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if runner.calls != 1 {
				t.Errorf("%s: orchestrator calls = %d, want 1 (%s)", tc.name, runner.calls, tc.reason)
			}
		})
	}
}

func TestPersistentPreRunE_Abridged_EmitsWarningsToStderrOnCLIPath(t *testing.T) {
	resetBootstrapOnce(t)
	resetBootstrapWarnings(t)
	stubSaverAliveCheck(t, false)
	shrinkSaverRetryDelay(t)

	client := tmux.NewClient(satisfiedLatchSaverAbsentCommander())
	runner := &recordingRunner{started: false}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner, Client: client})

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

func TestPersistentPreRunE_Abridged_LeavesWarningsForOpenTUIOnTUIPath(t *testing.T) {
	resetBootstrapOnce(t)
	resetBootstrapWarnings(t)
	stubSaverAliveCheck(t, false)
	shrinkSaverRetryDelay(t)

	client := tmux.NewClient(satisfiedLatchSaverAbsentCommander())
	runner := &recordingRunner{started: false}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner, Client: client})

	var pendingAtOpenTUI []bootstrap.Warning
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		pendingAtOpenTUI = bootstrapWarnings.Drain()
		return nil
	})

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

func TestPersistentPreRunE_Abridged_OpenSessionTakesAbridgedPath(t *testing.T) {
	resetBootstrapOnce(t)
	resetBootstrapWarnings(t)

	client := tmux.NewClient(satisfiedLatchAliveSaverCommander())
	runner := &recordingRunner{started: false}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner, Client: client})

	connector := &mockSessionConnector{}
	withOpenDeps(t, OpenDeps{SessionLister: &testSessionLister{names: []string{"proj-abc123"}}})

	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, name string) error { return connector.Connect(name) })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "--session", "proj-abc123"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.calls != 0 {
		t.Errorf("open --session + satisfied latch: orchestrator calls = %d, want 0 (abridged sync path is command-agnostic)", runner.calls)
	}
	if connector.connectedTo != "proj-abc123" {
		t.Errorf("open --session did not proceed: connectedTo = %q, want proj-abc123", connector.connectedTo)
	}
}
