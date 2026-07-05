// Tests in this file mutate package-level state (versionChecker, bootstrapDeps,
// listDeps, attachDeps, killDeps) and MUST NOT use t.Parallel.
package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// stubVersionChecker records call count and returns the configured error
// from every invocation. Substituted for the package-level versionChecker.
type stubVersionChecker struct {
	calls int
	err   error
}

func (s *stubVersionChecker) check(_ tmux.Commander) error {
	s.calls++
	return s.err
}

// installStubVersionChecker replaces the package-level versionChecker with the
// given stub. It resets the sync.Once gate up-front so the test sees a fresh
// check (other tests in the same binary may already have consumed the gate),
// and registers cleanup that restores the previous checker and resets the
// gate again so the next test starts clean.
func installStubVersionChecker(t *testing.T, stub *stubVersionChecker) {
	t.Helper()
	prev := versionChecker
	versionChecker = stub.check
	resetVersionCheckForTest()
	t.Cleanup(func() { versionChecker = prev })
	t.Cleanup(resetVersionCheckForTest)
}

func TestVersionGuard_InvokedForNonExemptOpen(t *testing.T) {
	stub := &stubVersionChecker{}
	installStubVersionChecker(t, stub)

	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "/nonexistent/path/that/does/not/exist"})
	// We expect a directory-not-found error from the resolver. The point of
	// the test is that PersistentPreRunE — and therefore the version
	// checker — ran *before* the resolver was reached.
	_ = rootCmd.Execute()

	if stub.calls != 1 {
		t.Errorf("version checker call count = %d, want 1", stub.calls)
	}
}

func TestVersionGuard_InvokedForOtherNonExemptCommands(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		setup func(t *testing.T)
	}{
		{
			name: "portal list",
			args: []string{"list"},
			setup: func(t *testing.T) {
				listDeps = &ListDeps{
					Lister: &mockSessionLister{sessions: []tmux.Session{}},
					IsTTY:  func() bool { return false },
				}
				t.Cleanup(func() { listDeps = nil })
			},
		},
		{
			name: "portal attach",
			args: []string{"attach", "my-session"},
			setup: func(t *testing.T) {
				attachDeps = &AttachDeps{
					Connector: &mockSessionConnector{},
					Validator: &mockSessionValidator{sessions: map[string]bool{"my-session": true}},
				}
				t.Cleanup(func() { attachDeps = nil })
			},
		},
		{
			name: "portal kill",
			args: []string{"kill", "my-session"},
			setup: func(t *testing.T) {
				killDeps = &KillDeps{
					Killer:    &mockSessionKiller{},
					Validator: &mockSessionValidator{sessions: map[string]bool{"my-session": true}},
				}
				t.Cleanup(func() { killDeps = nil })
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubVersionChecker{}
			installStubVersionChecker(t, stub)

			bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
			t.Cleanup(func() { bootstrapDeps = nil })

			tt.setup(t)

			resetRootCmd()
			rootCmd.SetArgs(tt.args)
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if stub.calls != 1 {
				t.Errorf("version checker call count = %d, want 1", stub.calls)
			}
		})
	}
}

func TestVersionGuard_NotInvokedForExemptCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "portal version", args: []string{"version"}},
		{name: "portal init", args: []string{"init", "zsh"}},
		{name: "portal alias list", args: []string{"alias", "list"}},
		{name: "portal clean", args: []string{"clean"}},
		{name: "portal state status", args: []string{"state", "status"}},
		{name: "portal state cleanup", args: []string{"state", "cleanup"}},
		{name: "portal state daemon", args: []string{"state", "daemon"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubVersionChecker{}
			installStubVersionChecker(t, stub)

			// ISOLATION IS LOAD-BEARING HERE. These exempt commands Execute
			// their REAL bodies (that is the point — prove the version
			// checker is skipped on real dispatch), so every side-effect
			// surface must be stubbed or poisoned. Incident of record: this
			// test used to run the real `portal state cleanup` body against
			// tmux.DefaultClient(), which honours the ambient TMUX — the
			// developer's REAL server when tests run inside tmux — so every
			// `go test ./cmd` KILLED the developer's live _portal-saver
			// session (SIGHUP'ing the real daemon) and unregistered the real
			// global hook table. The TMUX poison below is defence-in-depth:
			// any future DefaultClient in an exempt command fails loudly
			// against a dead socket instead of silently reaching the real
			// server.
			t.Setenv("TMUX", "/nonexistent/portal-version-guard-test,0,0")
			t.Setenv("PORTAL_STATE_DIR", t.TempDir())
			t.Setenv("PORTAL_ALIASES_FILE", t.TempDir()+"/aliases")
			t.Setenv("PORTAL_PROJECTS_FILE", t.TempDir()+"/projects.json")
			t.Setenv("PORTAL_HOOKS_FILE", t.TempDir()+"/hooks.json")

			// `portal clean` and `portal state cleanup` build tmux clients in
			// their real bodies — inject their seams so no real client exists.
			cleanDeps = &CleanDeps{AllPaneLister: &mockCleanPaneLister{}}
			t.Cleanup(func() { cleanDeps = nil })
			installStateCleanupDeps(t, &StateCleanupDeps{
				Client:     tmux.NewClient(&recordingCommander{}),
				Unregister: func(*tmux.Client) error { return nil },
			})

			// state daemon's RunE blocks on signal; stub the run-func so the
			// command returns immediately for argv-only assertions.
			if len(tt.args) >= 2 && tt.args[0] == "state" && tt.args[1] == "daemon" {
				t.Setenv("PORTAL_STATE_DIR", t.TempDir())
				prev := daemonRunFunc
				daemonRunFunc = func(_ context.Context, _ *daemonDeps) error { return nil }
				t.Cleanup(func() { daemonRunFunc = prev })
				withDaemonLockFileReset(t)
			}

			// state status now renders a real diagnostic; an empty state
			// dir produces ErrStatusUnhealthy, which is irrelevant to the
			// version-checker assertion below.
			if len(tt.args) >= 2 && tt.args[0] == "state" && tt.args[1] == "status" {
				t.Setenv("PORTAL_STATE_DIR", t.TempDir())
			}

			resetRootCmd()
			resetStateCmdFlags()
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			if err != nil && err != ErrStatusUnhealthy {
				t.Fatalf("unexpected error: %v", err)
			}

			if stub.calls != 0 {
				t.Errorf("version checker call count for exempt command = %d, want 0", stub.calls)
			}
		})
	}
}

func TestVersionGuard_RunsExactlyOnceAcrossRepeatedInvocations(t *testing.T) {
	stub := &stubVersionChecker{}
	installStubVersionChecker(t, stub)

	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	listDeps = &ListDeps{
		Lister: &mockSessionLister{sessions: []tmux.Session{}},
		IsTTY:  func() bool { return false },
	}
	t.Cleanup(func() { listDeps = nil })

	for i := range 3 {
		resetRootCmd()
		rootCmd.SetArgs([]string{"list"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
	}

	if stub.calls != 1 {
		t.Errorf("version checker call count = %d, want 1 across 3 invocations", stub.calls)
	}
}

func TestVersionGuard_ShortCircuitsBootstrapOnFailure(t *testing.T) {
	stub := &stubVersionChecker{err: errors.New("Portal requires tmux \u2265 3.0 (found 2.9). Please upgrade.")}
	installStubVersionChecker(t, stub)

	bootstrapDeps = &BootstrapDeps{Orchestrator: panicRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PersistentPreRunE panicked instead of returning error: %v", r)
		}
	}()

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error from version checker, got nil")
	}
	if stub.calls != 1 {
		t.Errorf("version checker call count = %d, want 1", stub.calls)
	}
}

func TestVersionGuard_PropagatesExactCheckerError(t *testing.T) {
	wantMsg := "Portal requires tmux \u2265 3.0 (found 2.9). Please upgrade."
	stub := &stubVersionChecker{err: errors.New(wantMsg)}
	installStubVersionChecker(t, stub)

	bootstrapDeps = &BootstrapDeps{Orchestrator: panicRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error from version checker, got nil")
	}
	if err.Error() != wantMsg {
		t.Errorf("error = %q, want %q", err.Error(), wantMsg)
	}
	if strings.Contains(err.Error(), "wrap") {
		t.Errorf("error appears wrapped, want exact propagation: %q", err.Error())
	}
}
