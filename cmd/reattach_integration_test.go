//go:build integration

// The portal binary must be staged: restored panes respawn into `portal state
// hydrate`, and without the binary the helper exits, the pane closes and its
// session dies before the assertions run. A real orchestrator (not a stub) is
// wired so restore step 6 genuinely creates the skeleton.

package cmd

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
	"github.com/spf13/cobra"
)

var reattachBuildOnce sync.Once
var reattachBinDir string
var reattachBuildErr error

// ensurePortalOnPATH stages a built portal ahead of the ambient PATH, for the
// fixtures whose bootstrap starts the `_portal-saver` pane, which runs `portal
// state daemon` by name. It returns the staging directory, so a caller that also
// needs the binary by path — pinning a restore's pane-arming to it — has it
// without a second accessor.
//
// The build dir must outlive the test that triggered the once-Do, so this uses
// BuildPortalBinaryStable rather than t.TempDir — later tests would otherwise
// point at a deleted path.
func ensurePortalOnPATH(t *testing.T) string {
	t.Helper()
	reattachBuildOnce.Do(func() {
		reattachBinDir, reattachBuildErr = restoretest.BuildPortalBinaryStable()
	})
	if reattachBuildErr != nil {
		t.Fatalf("build portal: %v", reattachBuildErr)
	}
	t.Setenv("PATH", reattachBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return reattachBinDir
}

func buildReattachOrchestrator(t *testing.T, client *tmux.Client, stateDir string) *bootstrap.Orchestrator {
	t.Helper()
	logger := restoretest.OpenTestLogger(t, stateDir)
	return bootstrap.NewWithDefaults(
		client,
		stateDir,
		logger,
		&bootstrapadapter.RestoringMarker{Client: client},
		bootstrap.WithRestore(restoretest.StagedRestoreAdapter(t, client, stateDir, logger, ensurePortalOnPATH(t))),
	)
}

func setupReattachEnv(t *testing.T) (*tmuxtest.Socket, *tmux.Client, string) {
	t.Helper()

	ensurePortalOnPATH(t)

	_, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	ts := tmuxtest.New(t, "ptl-reattach-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	resetBootstrapOnce(t)

	return ts, client, stateDir
}

func listSessionNamesViaSocket(t *testing.T, ts *tmuxtest.Socket) []string {
	t.Helper()
	out, err := ts.TryRun("list-sessions", "-F", "#{session_name}")
	if err != nil {
		// list-sessions errors when no server / no sessions; treat as empty.
		return nil
	}
	return splitNonEmptyLines(out)
}

func splitNonEmptyLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			line := s[start:i]
			if n := len(line); n > 0 && line[n-1] == '\r' {
				line = line[:n-1]
			}
			if line != "" {
				out = append(out, line)
			}
			start = i + 1
		}
	}
	return out
}

func containsAll(got, want []string) bool {
	set := make(map[string]struct{}, len(got))
	for _, g := range got {
		set[g] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[w]; !ok {
			return false
		}
	}
	return true
}

func TestReattachIntegration_SteadyStateReattachZeroStructuralRewrites(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ts, client, stateDir := setupReattachEnv(t)

	// Pre-create alpha live so Restore takes the skip branch.
	ts.Run(t, "new-session", "-d", "-s", "alpha")
	ts.WaitForSession(t, "alpha", 2*time.Second)

	// The seeded timestamp must be in the past: capture treats a future one as
	// newer than this run, so the suppression check could pass for the wrong
	// reason.
	preRunSavedAt := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	restoretest.SeedSessionsJSONWithSavedAt(t, stateDir, preRunSavedAt, "alpha")

	connector := &mockSessionConnector{}
	withOpenDeps(t, OpenDeps{SessionLister: client})

	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, name string) error { return connector.Connect(name) })

	withBootstrapDeps(t, BootstrapDeps{
		Orchestrator: buildReattachOrchestrator(t, client, stateDir),
		Client:       client,
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "--session", "alpha"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if connector.connectedTo != "alpha" {
		t.Errorf("connector.Connect = %q; want %q", connector.connectedTo, "alpha")
	}

	// An absent skeleton marker is the structural signal that Restore took the
	// live-session-skip branch.
	wantMarker := state.SkeletonMarkerPrefix + state.SanitizePaneKey("alpha", 0, 0)
	val, found, err := client.TryGetServerOption(wantMarker)
	if err != nil {
		t.Fatalf("TryGetServerOption %s: %v", wantMarker, err)
	}
	if found {
		t.Errorf("@portal-skeleton-* marker set for live alpha (value=%q); want absent — skeleton restore re-ran on a live session", val)
	}

	// The orchestrator wires NoOpSaver, so an advanced saved_at means either the
	// @portal-restoring guard failed or Restore wrote unexpectedly.
	postIdx, skip, err := state.ReadIndex(stateDir)
	if err != nil {
		t.Fatalf("ReadIndex post-Execute: %v", err)
	}
	if skip {
		t.Fatal("ReadIndex post-Execute reported skip=true; sessions.json was unexpectedly removed during Run")
	}
	if !postIdx.SavedAt.Equal(preRunSavedAt) {
		t.Errorf("sessions.json.saved_at advanced during the steady-state reattach window: pre=%v post=%v",
			preRunSavedAt, postIdx.SavedAt)
	}

	got := listSessionNamesViaSocket(t, ts)
	if !containsAll(got, []string{"alpha"}) {
		t.Errorf("live sessions = %v; want alpha present", got)
	}
}

func TestReattachIntegration_HasSessionPostBootstrapForSavedNames(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ts, client, stateDir := setupReattachEnv(t)

	restoretest.SeedSessionsJSON(t, stateDir, "ghost-foo", "ghost-bar")

	for _, name := range []string{"ghost-foo", "ghost-bar"} {
		if _, err := ts.TryRun("has-session", "-t", name); err == nil {
			t.Fatalf("%s unexpectedly live before bootstrap", name)
		}
	}

	connector := &mockSessionConnector{}
	withOpenDeps(t, OpenDeps{SessionLister: client})

	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, name string) error { return connector.Connect(name) })

	withBootstrapDeps(t, BootstrapDeps{
		Orchestrator: buildReattachOrchestrator(t, client, stateDir),
		Client:       client,
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "--session", "ghost-foo"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if connector.connectedTo != "ghost-foo" {
		t.Errorf("connector.Connect = %q; want %q", connector.connectedTo, "ghost-foo")
	}

	// Cross-checked via the raw socket in case the validator's HasSession
	// degrades silently. Restore creates both up-front, not lazily on attach.
	for _, name := range []string{"ghost-foo", "ghost-bar"} {
		if _, err := ts.TryRun("has-session", "-t", name); err != nil {
			t.Errorf("has-session -t %s: %v (expected live post-bootstrap)", name, err)
		}
	}
}

func TestReattachIntegration_AttachInsideTmuxSwitchClientPath(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ts, client, stateDir := setupReattachEnv(t)

	restoretest.SeedSessionsJSON(t, stateDir, "switched-foo")

	if _, err := ts.TryRun("has-session", "-t", "switched-foo"); err == nil {
		t.Fatal("switched-foo unexpectedly live before bootstrap")
	}

	// A real SwitchConnector over a mock SwitchClienter: only the final call is
	// faked, so the inside-tmux dispatch shape is exercised for real.
	switcher := &mockSwitchClient{}
	connector := &SwitchConnector{client: switcher}

	withOpenDeps(t, OpenDeps{SessionLister: client})

	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, name string) error { return connector.Connect(name) })

	withBootstrapDeps(t, BootstrapDeps{
		Orchestrator: buildReattachOrchestrator(t, client, stateDir),
		Client:       client,
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "--session", "switched-foo"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if switcher.switchedTo != "switched-foo" {
		t.Errorf("SwitchClient.SwitchedTo = %q; want %q", switcher.switchedTo, "switched-foo")
	}
}

func TestReattachIntegration_AttachOutsideTmuxAttachSessionPath(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	t.Setenv("TMUX", "")

	ts, client, stateDir := setupReattachEnv(t)

	restoretest.SeedSessionsJSON(t, stateDir, "attached-foo")

	if _, err := ts.TryRun("has-session", "-t", "attached-foo"); err == nil {
		t.Fatal("attached-foo unexpectedly live before bootstrap")
	}

	// Stands in for AttachConnector, whose Connect calls syscall.Exec and would
	// replace the test process.
	connector := &mockSessionConnector{}
	withOpenDeps(t, OpenDeps{SessionLister: client})

	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, name string) error { return connector.Connect(name) })

	withBootstrapDeps(t, BootstrapDeps{
		Orchestrator: buildReattachOrchestrator(t, client, stateDir),
		Client:       client,
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "--session", "attached-foo"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if connector.connectedTo != "attached-foo" {
		t.Errorf("connector.Connect = %q; want %q", connector.connectedTo, "attached-foo")
	}

	if _, err := ts.TryRun("has-session", "-t", "attached-foo"); err != nil {
		t.Errorf("has-session -t attached-foo: %v (expected live post-bootstrap)", err)
	}
}

func TestReattachIntegration_UnknownNameNotFoundError(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	_, client, stateDir := setupReattachEnv(t)

	// No sessions.json at all — the simplest "neither live nor saved" world.

	connector := &mockSessionConnector{}
	withOpenDeps(t, OpenDeps{SessionLister: client})

	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, name string) error { return connector.Connect(name) })

	withBootstrapDeps(t, BootstrapDeps{
		Orchestrator: buildReattachOrchestrator(t, client, stateDir),
		Client:       client,
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "--session", "nope-not-here"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected 'No session found' error, got nil")
	}
	want := "No session found: nope-not-here"
	if err.Error() != want {
		t.Errorf("error = %q; want %q", err.Error(), want)
	}

	if connector.connectedTo != "" {
		t.Errorf("connector.Connect = %q; want empty (not-found short-circuits before dispatch)", connector.connectedTo)
	}
}

func TestReattachIntegration_OpenLaunchesTUIAfterRestoredSkeleton(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ts, client, stateDir := setupReattachEnv(t)

	restoretest.SeedSessionsJSON(t, stateDir, "tui-ghost")

	withBootstrapDeps(t, BootstrapDeps{
		Orchestrator: buildReattachOrchestrator(t, client, stateDir),
		Client:       client,
	})

	// Warm-but-unlatched means the bootstrap is deferred to openTUI, so this
	// stub must drive the deferred runner itself or restore step 6 never runs.
	// Synchronously is fine here — only completion before the stub returns
	// matters.
	var tuiCalled bool
	withFuncSeam(t, &openTUIFunc, func(cmd *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		if d := deferredBootstrapFromContext(cmd); d != nil {
			if _, _, err := d.runner.Run(cmd.Context()); err != nil {
				t.Fatalf("deferred bootstrap Run: %v", err)
			}
		}
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !tuiCalled {
		t.Fatal("openTUIFunc was not invoked for `portal open` with no args")
	}

	if _, err := ts.TryRun("has-session", "-t", "tui-ghost"); err != nil {
		t.Errorf("has-session -t tui-ghost: %v (expected live by the time `portal open` reached the TUI)", err)
	}
}

func TestReattachIntegration_OpenPathResolvesSavedOnlySession(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ts, client, stateDir := setupReattachEnv(t)

	projectDir := filepath.Join(t.TempDir(), "open-saved-proj")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	// The saved name deliberately does not match the alias query: matching it
	// would mean contriving the {project}-{nanoid} naming dance for nothing.
	const savedSession = "open-ghost"
	restoretest.SeedSessionsJSON(t, stateDir, savedSession)

	if _, err := ts.TryRun("has-session", "-t", savedSession); err == nil {
		t.Fatalf("%s unexpectedly live before bootstrap", savedSession)
	}

	withOpenDeps(t, OpenDeps{
		// "mysaved" is not a live session, so the session-domain pre-check
		// misses and resolution falls through to the alias chain.
		SessionLister: client,
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"mysaved": projectDir}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &resolver.OSDirValidator{},
	})

	withBootstrapDeps(t, BootstrapDeps{
		Orchestrator: buildReattachOrchestrator(t, client, stateDir),
		Client:       client,
	})

	// Substitutes for openPath, which would call tmux switch-client (needs a
	// live attached client) or syscall.Exec (would replace the test process).
	var (
		pathOpenerCalled bool
		capturedPath     string
	)
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, resolvedPath string, _ []string) error {
		pathOpenerCalled = true
		capturedPath = resolvedPath
		return nil
	})

	// Reaching the TUI would mean alias resolution produced no PathResult,
	// masking a regression in the path-arg branch under test.
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, landing pickerLanding, _ []string, _ bool) error {
		t.Errorf("openTUIFunc unexpectedly called (landing=%+v); resolver should have produced PathResult", landing)
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "mysaved"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !pathOpenerCalled {
		t.Fatal("openPathFunc was not invoked for `portal open mysaved`; alias resolution did not reach the path-arg branch")
	}
	if capturedPath != projectDir {
		t.Errorf("openPathFunc resolved path = %q; want %q", capturedPath, projectDir)
	}

	if _, err := ts.TryRun("has-session", "-t", savedSession); err != nil {
		t.Errorf("has-session -t %s: %v (expected live by the time `portal open mysaved` reached openPath)", savedSession, err)
	}
}

var _ SessionConnector = (*mockSessionConnector)(nil)
var _ SessionValidator = (*mockSessionValidator)(nil)
var _ SwitchClienter = (*mockSwitchClient)(nil)
var _ bootstrap.Runner = (*bootstrap.Orchestrator)(nil)

var _ func(*cobra.Command, pickerLanding, []string, bool) error = openTUIFunc
var _ func(*cobra.Command, string, []string) error = openPathFunc
