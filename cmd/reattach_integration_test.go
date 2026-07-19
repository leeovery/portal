//go:build integration

// Phase 5 task 5-10 — `portal open --session NAME` / `portal open` reattach
// integration test. (Originally written for the retired `portal attach NAME`;
// migrated to `open --session NAME`, its exact replacement, when attach was
// deleted in cli-verb-surface-redesign Phase 5.)
//
// This file locks in the Phase 5 acceptance bullet (planning.md L166):
//
//	`portal open --session NAME` and `portal open` continue to resolve names
//	that only exist in `sessions.json` at bootstrap time (skeleton is
//	created before the command's own attach logic runs).
//
// The seven planning task 5-10 acceptance bullets (phase-5-tasks.md
// L941-L947) → their asserting test in this file:
//
//  1. "open --session NAME resolves a name present only in sessions.json
//     (bare shell)" → TestReattachIntegration_AttachOutsideTmuxAttachSessionPath.
//  2. "portal open PATH resolves a session name present only in
//     sessions.json" → TestReattachIntegration_OpenLaunchesTUIAfterRestoredSkeleton
//     (no-arg TUI branch) AND
//     TestReattachIntegration_OpenPathResolvesSavedOnlySession (path-arg
//     branch through alias resolution into openPath).
//  3. "open --session NAME resolves a name present only in sessions.json
//     (inside tmux switch-client)" →
//     TestReattachIntegration_AttachInsideTmuxSwitchClientPath.
//  4. "steady-state reattach with saved session already live performs
//     zero structural rewrites" →
//     TestReattachIntegration_SteadyStateReattachZeroStructuralRewrites.
//  5. "open --session NAME returns the existing not-found error for names
//     in neither live nor saved state" →
//     TestReattachIntegration_UnknownNameNotFoundError.
//  6. "has-session returns true for every name in sessions.json
//     post-bootstrap" →
//     TestReattachIntegration_HasSessionPostBootstrapForSavedNames.
//  7. "saved_at is not advanced during a steady-state reattach window" →
//     TestReattachIntegration_SteadyStateReattachZeroStructuralRewrites
//     (asserts pre/post sessions.json.saved_at equality alongside the
//     pane-id preservation check).
//
// The five enumerated edge cases (planning task 5-10 Solution section)
// covered here:
//
//  1. Steady-state reattach (saved session already live) does zero
//     structural rewrites.
//  2. `has-session` post-bootstrap returns true for every name in
//     `sessions.json` (bootstrap creates skeleton sessions before
//     downstream commands run).
//  3. `switch-client` (inside-tmux) attach path verified end-to-end.
//  4. `exec attach-session` (bare-shell) attach path verified
//     end-to-end (mock connector substitutes for the real syscall.Exec
//     to avoid PTY hand-off in tests).
//  5. Name in neither live nor saved still fails with the existing
//     "No session found" error.
//
// Why this file lives in cmd/ (and not cmd/bootstrap/):
//   - The Phase 5 acceptance criterion is at the cmd-level: a user-
//     facing `portal open --session NAME` invocation must work against a
//     saved-only name. That means we need access to the cmd package's
//     injection seams (`bootstrapDeps`, `openDeps`, `openSessionFunc`,
//     `openTUIFunc`) which are unexported package-level state. cmd/bootstrap/'s
//     reboot_roundtrip_test verifies the bootstrap orchestrator's
//     reboot pipeline; this file verifies the cmd-layer pipeline that
//     consumes its output.
//
// Why we drive a real bootstrap.Orchestrator (and not a stub):
//   - The criterion is "skeleton is created before the command's own
//     attach logic runs" — i.e. the Restore step's contract. A stub
//     Orchestrator would not exercise that contract. We wire a real
//     bootstrap.Orchestrator with NoOp shims for the steps incidental
//     to this scenario (Hooks, Saver, Sweeper) and real
//     RestoringMarker + RestoreAdapter so step 6 actually runs.
//
// Why we build the portal binary on PATH:
//   - Restore arms each created pane via `respawn-pane -k 'portal
//     state hydrate --fifo X --file Y --hook-key Z'`. If `portal` is
//     not on PATH the helper exits immediately, the pane closes
//     (default tmux remain-on-exit=off), the window closes, and the
//     session itself dies — has-session would then return false even
//     though Restore ran successfully. Building the binary keeps the
//     helper blocked on its FIFO open(O_RDONLY) so the pane (and its
//     session) stays alive long enough for the cmd-layer assertions.
//
// Build & run:
//
//	go test -tags=integration ./cmd/...
//
// Tests in this file are NOT included in the default `go test ./...`
// run because the `//go:build integration` tag gates them off. They
// also call `testing.Short()` so `go test -short -tags=integration`
// skips them — useful for quick-check CI lanes.

package cmd

// Tests in this file mutate package-level state (bootstrapDeps,
// openDeps, openSessionFunc, openTUIFunc, openPathFunc) and MUST NOT use
// t.Parallel.

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
	"github.com/spf13/cobra"
)

// reattachBuildOnce caches the portal-binary build across all five test
// cases in this file. The build itself is ~1-2 seconds and produces a
// process-wide PATH side effect; doing it once amortises the cost
// without introducing TestMain (the package has no other shared setup).
var reattachBuildOnce sync.Once
var reattachBinDir string
var reattachBuildErr error

// ensurePortalOnPATH builds the portal binary into a stable temp dir
// (once per test process via sync.Once) and prepends that dir to PATH
// for the lifetime of the calling test. The temp dir lives until
// process exit — sharing it across tests is intentional because the
// only consumer is the in-pane hydrate helper, which never mutates the
// binary. We delegate to restoretest.BuildPortalBinaryStable (which
// uses os.MkdirTemp, NOT t.TempDir) because t.TempDir would be removed
// when the test that triggered the once-Do exits, leaving subsequent
// tests pointing at a deleted path.
func ensurePortalOnPATH(t *testing.T) {
	t.Helper()
	reattachBuildOnce.Do(func() {
		reattachBinDir, reattachBuildErr = restoretest.BuildPortalBinaryStable()
	})
	if reattachBuildErr != nil {
		t.Fatalf("build portal: %v", reattachBuildErr)
	}
	t.Setenv("PATH", reattachBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// buildReattachOrchestrator constructs a bootstrap.Orchestrator wired
// for the reattach integration scenario: real RestoringMarker
// (Set/Clear) and real RestoreAdapter (so step 6 actually creates the
// skeleton from sessions.json), with NoOp shims for the steps
// incidental to this scenario (Hooks, Saver, StaleMarkers, Sweeper).
// The same adapter shape used by Phase 5's
// TestPhase5_RestoreCreatesMissingSession in
// cmd/bootstrap/phase5_integration_test.go.
//
// Delegates to bootstrap.NewWithDefaults (cmd/bootstrap/defaults.go) so
// the NoOp defaulting policy lives in one place. The conditional
// EagerSignaler default-to-real semantics (T4-2) is also computed
// inside the helper: passing WithRestore with a non-nil Restorer
// produces a real *bootstrap.EagerSignalCore wired with the same
// client / stateDir / logger triple. Reattach tests that need to
// suppress the eager step would pass an explicit
// bootstrap.WithEagerSignaler(bootstrap.NoOpEagerHydrateSignaler{}) —
// none of the current sites do.
//
// stateDir holds the seeded sessions.json and the per-pane scrollback
// files (written by the helper after hydration). client points at the
// isolated test socket via tmuxtest.Socket.Client.
func buildReattachOrchestrator(t *testing.T, client *tmux.Client, stateDir string) *bootstrap.Orchestrator {
	t.Helper()
	logger := restoretest.OpenTestLogger(t, stateDir)
	return bootstrap.NewWithDefaults(
		client,
		stateDir,
		logger,
		&bootstrapadapter.RestoringMarker{Client: client},
		bootstrap.WithRestore(bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)),
	)
}

// setupReattachEnv builds the per-test scaffolding shared by every
// case: isolated tmux socket, state dir wired via PORTAL_STATE_DIR,
// portal binary on PATH, the test's bootstrap orchestrator wired into
// bootstrapDeps, and a t.Cleanup that resets every package-level seam
// it mutated. Returns the socket and the *tmux.Client so the caller
// can seed live sessions / drive assertions.
func setupReattachEnv(t *testing.T) (*tmuxtest.Socket, *tmux.Client, string) {
	t.Helper()

	ensurePortalOnPATH(t)

	ts := tmuxtest.New(t, "ptl-reattach-")
	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	resetBootstrapOnce(t)

	return ts, client, stateDir
}

// listSessionNamesViaSocket returns the names of currently-live tmux
// sessions on the isolated socket. Used to assert pre/post bootstrap
// states without depending on test-process-level Run-trim semantics.
func listSessionNamesViaSocket(t *testing.T, ts *tmuxtest.Socket) []string {
	t.Helper()
	out, err := ts.TryRun("list-sessions", "-F", "#{session_name}")
	if err != nil {
		// list-sessions errors when no server / no sessions; treat as empty.
		return nil
	}
	return splitNonEmptyLines(out)
}

// splitNonEmptyLines splits s on newlines and trims whitespace,
// dropping any empty results. Pulled into a helper so callers do not
// repeat the boilerplate around `strings.Split + trim + skip empty`.
func splitNonEmptyLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			line := s[start:i]
			// Trim trailing carriage return (Windows-safe; harmless on Unix).
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

// containsAll reports whether every want item is present in got.
// Order-independent — both inputs are flat string slices.
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

// TestReattachIntegration_SteadyStateReattachZeroStructuralRewrites
// covers planning task 5-10 case 1 AND task 5-10 acceptance bullet
// "saved_at is not advanced during a steady-state reattach window"
// (phase-5-tasks.md L947). When a saved session is already live,
// bootstrap's Restore step must NOT issue a second new-session (which
// would error with "session already exists" or worse silently recreate
// it). Steady-state reattach should be a single-list-sessions no-op for
// that name AND must not bump sessions.json.saved_at — the suppression
// invariant the @portal-restoring marker exists to enforce.
//
// We assert this by: pre-creating "alpha" on the live server, seeding
// sessions.json with "alpha" at a known pre-Run saved_at, running
// bootstrap, then verifying that
//
//	(a) alpha is still alive,
//	(b) no @portal-skeleton-<paneKey> marker was set for it (skeleton
//	    markers are a Restore side effect — their absence proves
//	    Restore took the live-session-skip branch), and
//	(c) sessions.json.saved_at is byte-identical to the seeded value
//	    (proves nothing in the orchestrator's pipeline rewrote
//	    sessions.json during the @portal-restoring window — see
//	    cmd/bootstrap/phase5_marker_suppression_integration_test.go's
//	    `saved_at` invariance assertion for the sibling
//	    sub-orchestrator-level guard).
func TestReattachIntegration_SteadyStateReattachZeroStructuralRewrites(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ts, client, stateDir := setupReattachEnv(t)

	// Pre-create alpha live so Restore takes the skip branch.
	ts.Run(t, "new-session", "-d", "-s", "alpha")
	ts.WaitForSession(t, "alpha", 2*time.Second)

	// Seed sessions.json with the same name and a fixed saved_at the
	// post-Run assertion can compare against. The timestamp is
	// arbitrary but MUST be in the past — capture.go would treat a
	// future timestamp as "newer than this run" and the suppression
	// test could pass for the wrong reason.
	preRunSavedAt := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	restoretest.SeedSessionsJSONWithSavedAt(t, stateDir, preRunSavedAt, "alpha")

	// Wire orchestrator + cmd-layer mocks. openDeps' SessionLister points
	// at the real socket-backed client so `open --session` resolves against
	// the live tmux server. openSessionFunc routes the resolved hit into a
	// mock connector so Connect is captured without trying to do a real attach.
	connector := &mockSessionConnector{}
	openDeps = &OpenDeps{SessionLister: client}
	t.Cleanup(func() { openDeps = nil })

	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, name string) error { return connector.Connect(name) }
	t.Cleanup(func() { openSessionFunc = origSession })

	bootstrapDeps = &BootstrapDeps{
		Orchestrator: buildReattachOrchestrator(t, client, stateDir),
		Client:       client,
	}
	t.Cleanup(func() { bootstrapDeps = nil })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "--session", "alpha"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Connect must have been called — alpha was already live so the
	// session pin resolved and dispatch reached the connector.
	if connector.connectedTo != "alpha" {
		t.Errorf("connector.Connect = %q; want %q", connector.connectedTo, "alpha")
	}

	// Skeleton marker MUST be absent — its absence is the structural
	// signal that Restore took the live-session-skip branch and did not
	// rewrite the live session.
	wantMarker := state.SkeletonMarkerPrefix + state.SanitizePaneKey("alpha", 0, 0)
	val, found, err := client.TryGetServerOption(wantMarker)
	if err != nil {
		t.Fatalf("TryGetServerOption %s: %v", wantMarker, err)
	}
	if found {
		t.Errorf("@portal-skeleton-* marker set for live alpha (value=%q); want absent — skeleton restore re-ran on a live session", val)
	}

	// saved_at MUST equal the seeded pre-Run value — suppression
	// invariant. The orchestrator wires NoOpSaver, so any advance in
	// saved_at would indicate either the @portal-restoring guard
	// failed OR an unintended write path inside Restore. Mirrors the
	// `saved_at` invariance assertion in
	// cmd/bootstrap/phase5_marker_suppression_integration_test.go.
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

	// Belt-and-braces: there must still be exactly one live session named alpha.
	got := listSessionNamesViaSocket(t, ts)
	if !containsAll(got, []string{"alpha"}) {
		t.Errorf("live sessions = %v; want alpha present", got)
	}
}

// TestReattachIntegration_HasSessionPostBootstrapForSavedNames covers
// planning task 5-10 case 2: every name present in sessions.json must
// be queryable via has-session after bootstrap completes. This is the
// central acceptance criterion — `portal attach NAME` only resolves a
// saved-only name when bootstrap's Restore step (5) has already
// skeleton-created it BEFORE the command's RunE inspects HasSession.
//
// We seed two saved-only names ("ghost-foo", "ghost-bar"), confirm
// neither is live before bootstrap, run a tmux-using command (which
// triggers PersistentPreRunE → bootstrap), and assert both names are
// live afterwards.
func TestReattachIntegration_HasSessionPostBootstrapForSavedNames(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ts, client, stateDir := setupReattachEnv(t)

	// Two saved-only names; nothing live yet.
	restoretest.SeedSessionsJSON(t, stateDir, "ghost-foo", "ghost-bar")

	// Pre-condition: neither name is live.
	for _, name := range []string{"ghost-foo", "ghost-bar"} {
		if _, err := ts.TryRun("has-session", "-t", name); err == nil {
			t.Fatalf("%s unexpectedly live before bootstrap", name)
		}
	}

	connector := &mockSessionConnector{}
	openDeps = &OpenDeps{SessionLister: client}
	t.Cleanup(func() { openDeps = nil })

	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, name string) error { return connector.Connect(name) }
	t.Cleanup(func() { openSessionFunc = origSession })

	bootstrapDeps = &BootstrapDeps{
		Orchestrator: buildReattachOrchestrator(t, client, stateDir),
		Client:       client,
	}
	t.Cleanup(func() { bootstrapDeps = nil })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "--session", "ghost-foo"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// open --session must have dispatched to the connector — proving the
	// session pin resolved the saved-only name post-bootstrap.
	if connector.connectedTo != "ghost-foo" {
		t.Errorf("connector.Connect = %q; want %q", connector.connectedTo, "ghost-foo")
	}

	// Both saved names must be live on the server (Restore creates them
	// up-front, not lazily on attach). Cross-check via the raw socket
	// in case the validator's HasSession degrades silently.
	for _, name := range []string{"ghost-foo", "ghost-bar"} {
		if _, err := ts.TryRun("has-session", "-t", name); err != nil {
			t.Errorf("has-session -t %s: %v (expected live post-bootstrap)", name, err)
		}
	}
}

// TestReattachIntegration_AttachInsideTmuxSwitchClientPath covers
// planning task 5-10 case 3: when Portal is invoked from inside an
// existing tmux session, `portal attach NAME` must dispatch to a
// SwitchConnector (which calls tmux switch-client). We verify the
// dispatch path with full type fidelity by routing openSessionFunc into
// a real *SwitchConnector wrapped around a mock SwitchClienter — only the
// SwitchClient call itself is mocked, so the cmd path through
// openResolved's session arm and the SwitchConnector's Connect method is
// end-to-end exercised.
//
// The TMUX env variable is intentionally NOT consulted here because the
// openSessionFunc override bypasses tmux.InsideTmux() — that gating is
// covered separately by TestBuildSessionConnector. The contribution
// of this case is verifying the end-to-end pipeline against a
// saved-only name when the inside-tmux Connector type is wired.
func TestReattachIntegration_AttachInsideTmuxSwitchClientPath(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ts, client, stateDir := setupReattachEnv(t)

	restoretest.SeedSessionsJSON(t, stateDir, "switched-foo")

	// Pre-condition: switched-foo is not yet live.
	if _, err := ts.TryRun("has-session", "-t", "switched-foo"); err == nil {
		t.Fatal("switched-foo unexpectedly live before bootstrap")
	}

	// Real SwitchConnector wrapping a mock SwitchClienter — proves the
	// cmd-layer dispatches through SwitchConnector.Connect → underlying
	// SwitchClient. This is the inside-tmux path's structural shape.
	switcher := &mockSwitchClient{}
	connector := &SwitchConnector{client: switcher}

	openDeps = &OpenDeps{SessionLister: client}
	t.Cleanup(func() { openDeps = nil })

	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, name string) error { return connector.Connect(name) }
	t.Cleanup(func() { openSessionFunc = origSession })

	bootstrapDeps = &BootstrapDeps{
		Orchestrator: buildReattachOrchestrator(t, client, stateDir),
		Client:       client,
	}
	t.Cleanup(func() { bootstrapDeps = nil })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "--session", "switched-foo"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// SwitchClient must have been called with the saved-only name —
	// proves Restore skeleton-created switched-foo (HasSession=true)
	// AND that SwitchConnector.Connect reached the underlying client.
	if switcher.switchedTo != "switched-foo" {
		t.Errorf("SwitchClient.SwitchedTo = %q; want %q", switcher.switchedTo, "switched-foo")
	}
}

// TestReattachIntegration_AttachOutsideTmuxAttachSessionPath covers
// planning task 5-10 case 4: when Portal is invoked from a bare shell
// (TMUX unset), `portal attach NAME` must dispatch through the
// outside-tmux pipeline that production wires to AttachConnector
// (`syscall.Exec` + `tmux attach-session -t NAME`). We substitute a
// mockSessionConnector so the test does not require a real PTY hand-
// off — per the planning brief's "mock connectors so the test does
// not require a real PTY" guidance.
//
// What this case uniquely contributes (vs case 3): it proves the cmd
// pipeline reaches Connector.Connect with the saved-only name when
// the bare-shell wiring is in effect. The TMUX env variable is
// intentionally cleared at the test process level via t.Setenv so
// `tmux.InsideTmux()` would observe the bare-shell state if the test
// path were to consult it (the openSessionFunc override bypasses it;
// the env clear is documentation more than mechanism).
func TestReattachIntegration_AttachOutsideTmuxAttachSessionPath(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	// Document bare-shell semantics: TMUX unset.
	t.Setenv("TMUX", "")

	ts, client, stateDir := setupReattachEnv(t)

	restoretest.SeedSessionsJSON(t, stateDir, "attached-foo")

	// Pre-condition: attached-foo is not yet live.
	if _, err := ts.TryRun("has-session", "-t", "attached-foo"); err == nil {
		t.Fatal("attached-foo unexpectedly live before bootstrap")
	}

	// mockSessionConnector stands in for AttachConnector — real
	// AttachConnector.Connect calls syscall.Exec which would replace
	// the test process. The mock captures the connection by name,
	// matching what the production tmux-attach-session-with-name path
	// would target.
	connector := &mockSessionConnector{}
	openDeps = &OpenDeps{SessionLister: client}
	t.Cleanup(func() { openDeps = nil })

	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, name string) error { return connector.Connect(name) }
	t.Cleanup(func() { openSessionFunc = origSession })

	bootstrapDeps = &BootstrapDeps{
		Orchestrator: buildReattachOrchestrator(t, client, stateDir),
		Client:       client,
	}
	t.Cleanup(func() { bootstrapDeps = nil })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "--session", "attached-foo"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if connector.connectedTo != "attached-foo" {
		t.Errorf("connector.Connect = %q; want %q", connector.connectedTo, "attached-foo")
	}

	// Cross-check: skeleton restoration actually ran. attached-foo
	// must be live after bootstrap.
	if _, err := ts.TryRun("has-session", "-t", "attached-foo"); err != nil {
		t.Errorf("has-session -t attached-foo: %v (expected live post-bootstrap)", err)
	}
}

// TestReattachIntegration_UnknownNameNotFoundError covers planning task
// 5-10 case 5: a name that exists in NEITHER live tmux NOR sessions.json
// must surface the existing "No session found: <name>" error from
// cmd/open.go's session pin (ResolveSessionPin), with no Connector.Connect
// dispatch. This guards against a future regression where bootstrap
// silently creates skeletons for names not in sessions.json (it must not).
func TestReattachIntegration_UnknownNameNotFoundError(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	// ts is unused in this case — we never query the live socket
	// because the assertion is on the cmd-layer error message, not on
	// any post-bootstrap server state. ts's t.Cleanup still tears down
	// the isolated server via tmuxtest.New's registered cleanup.
	_, client, stateDir := setupReattachEnv(t)

	// No sessions.json at all — empty state dir is the simplest
	// "neither live nor saved" world.

	connector := &mockSessionConnector{}
	openDeps = &OpenDeps{SessionLister: client}
	t.Cleanup(func() { openDeps = nil })

	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, name string) error { return connector.Connect(name) }
	t.Cleanup(func() { openSessionFunc = origSession })

	bootstrapDeps = &BootstrapDeps{
		Orchestrator: buildReattachOrchestrator(t, client, stateDir),
		Client:       client,
	}
	t.Cleanup(func() { bootstrapDeps = nil })

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

	// Connector must NOT have been invoked — the session-pin miss in
	// cmd/open.go (ResolveSessionPin) hard-fails before openResolved dispatch.
	if connector.connectedTo != "" {
		t.Errorf("connector.Connect = %q; want empty (not-found short-circuits before dispatch)", connector.connectedTo)
	}
}

// TestReattachIntegration_OpenLaunchesTUIAfterRestoredSkeleton covers
// the `portal open` half of planning task 5-10's acceptance bullet:
// when invoked with no positional arg, `portal open` launches the TUI
// — and by the time the TUI's session lister consults tmux, every
// saved name has already been skeleton-restored by bootstrap step 6.
//
// Phase-2 routing note: setupReattachEnv warms the tmux server but never
// stamps the `@portal-bootstrapped` latch, so warm+unlatched
// `portal open` (no args, TUI path) now takes the CONCURRENT/DEFERRED
// route — PersistentPreRunE stashes the orchestrator on the context
// (deferredBootstrapKey) instead of running it synchronously, and the
// real openTUI is what drives it (pipe.start → runner.Run). Restore
// step 6 therefore rides that deferred bootstrap, NOT the synchronous
// PersistentPreRunE path.
//
// We override openTUIFunc to capture its inputs without launching a real
// Bubble Tea program (the TUI requires a TTY which test harnesses do not
// provide) — but the stub must DRIVE the deferred bootstrap to
// completion first, exactly as the real openTUI does on this route, or
// restore step 6 never runs and tui-ghost is never created. The
// post-Run assertion is that the live tmux server observable by the
// TUI's lister includes the saved-only name — equivalent to "the TUI
// would render it as a selectable session".
func TestReattachIntegration_OpenLaunchesTUIAfterRestoredSkeleton(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ts, client, stateDir := setupReattachEnv(t)

	restoretest.SeedSessionsJSON(t, stateDir, "tui-ghost")

	bootstrapDeps = &BootstrapDeps{
		Orchestrator: buildReattachOrchestrator(t, client, stateDir),
		Client:       client,
	}
	t.Cleanup(func() { bootstrapDeps = nil })

	// Capture-only TUI launcher — does not start the Bubble Tea program.
	// On the warm+unlatched concurrent route PersistentPreRunE deferred
	// the orchestrator (deferredBootstrapKey) instead of running it, and
	// the real openTUI is what drives it via pipe.start → runner.Run.
	// This stub replaces openTUI, so it must drive that deferred runner
	// to completion itself — otherwise restore step 6 never runs and
	// tui-ghost is never created. We run the runner synchronously here
	// (the real route runs it in a goroutine streaming progress; the
	// difference is immaterial to the post-Run has-session assertion,
	// which only needs restore to have completed by the time the stub
	// returns).
	var tuiCalled bool
	origFunc := openTUIFunc
	openTUIFunc = func(cmd *cobra.Command, _ string, _ []string, _ bool) error {
		tuiCalled = true
		if d := deferredBootstrapFromContext(cmd); d != nil {
			if _, _, err := d.runner.Run(cmd.Context()); err != nil {
				t.Fatalf("deferred bootstrap Run: %v", err)
			}
		}
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !tuiCalled {
		t.Fatal("openTUIFunc was not invoked for `portal open` with no args")
	}

	// The TUI's lister would query list-sessions on the same server
	// the bootstrap orchestrator wired. tui-ghost must be live by now.
	if _, err := ts.TryRun("has-session", "-t", "tui-ghost"); err != nil {
		t.Errorf("has-session -t tui-ghost: %v (expected live by the time `portal open` reached the TUI)", err)
	}
}

// TestReattachIntegration_OpenPathResolvesSavedOnlySession covers the
// path-argument half of planning task 5-10 acceptance bullet #2 (
// "portal open PATH resolves a session name present only in
// sessions.json", phase-5-tasks.md L942). The sibling
// TestReattachIntegration_OpenLaunchesTUIAfterRestoredSkeleton covers
// the no-arg → TUI-launch branch; this case covers the path-arg branch
// that runs through alias resolution → resolver.PathResult → openPath
// against a sessions.json that holds a saved-only name.
//
// Wiring:
//   - openDeps overrides the resolver's AliasLookup so the test does not
//     need to write to ~/.config/portal/aliases. The alias maps a known
//     query string to a temp directory whose basename matches the name
//     of the saved-only session — exercising the same shape as a user
//     who registered an alias pointing at a project directory whose
//     session name was previously persisted to sessions.json.
//   - openPathFunc is overridden to capture (resolvedPath, command)
//     rather than running the real openPath, because openPath calls
//     either tmux switch-client (inside-tmux) or syscall.Exec (bare
//     shell). The former requires a live attached client (impossible in
//     a test harness); the latter would replace the test process.
//   - openTUIFunc is overridden to fail the test if the resolver ever
//     reached the picker — ensures we are exercising the path-arg
//     PathResult branch and not falling through (a total miss now
//     hard-fails rather than launching the TUI).
//
// What this case uniquely contributes (vs the no-arg sibling): it
// proves the cmd-layer resolver chain reaches openPath when alias
// resolution succeeds, AND that by the time openPath would dispatch,
// bootstrap step 6 has already skeleton-restored the saved-only
// session on the live tmux server (queryable via has-session). This
// is the path-arg analogue of the "skeleton is created before the
// command's own attach logic runs" guarantee.
func TestReattachIntegration_OpenPathResolvesSavedOnlySession(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ts, client, stateDir := setupReattachEnv(t)

	// Project dir for the alias to point at. The basename is incidental
	// to the assertion (openPathFunc is mocked) but documents the
	// shape: aliases map names to project directories, and the saved
	// session would have been created from such a directory.
	projectDir := filepath.Join(t.TempDir(), "open-saved-proj")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	// Saved-only session whose name does NOT need to match the alias
	// query — the assertion is "skeleton was restored AND openPath was
	// dispatched", not "the open command attached to that exact name"
	// (which would require contriving the {project}-{nanoid} naming
	// dance).
	const savedSession = "open-ghost"
	restoretest.SeedSessionsJSON(t, stateDir, savedSession)

	// Pre-condition: the saved-only session is not yet live.
	if _, err := ts.TryRun("has-session", "-t", savedSession); err == nil {
		t.Fatalf("%s unexpectedly live before bootstrap", savedSession)
	}

	// Wire resolver dependencies so `portal open mysaved` resolves to
	// projectDir via the alias chain (no zoxide, no real disk lookup
	// for the alias itself — the OSDirValidator does the existence
	// check on projectDir which we just mkdir'd).
	openDeps = &OpenDeps{
		// The real client is the user-visible session set: "mysaved" is not a
		// live session, so the session-domain pre-check misses and resolution
		// falls through to the alias chain (as production does).
		SessionLister: client,
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"mysaved": projectDir}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &resolver.OSDirValidator{},
	}
	t.Cleanup(func() { openDeps = nil })

	bootstrapDeps = &BootstrapDeps{
		Orchestrator: buildReattachOrchestrator(t, client, stateDir),
		Client:       client,
	}
	t.Cleanup(func() { bootstrapDeps = nil })

	// Capture-only path opener — substitutes for openPath so the test
	// does not invoke real tmux switch-client or syscall.Exec. Records
	// the resolved path so we can prove the resolver chain produced a
	// PathResult and dispatched into the path-arg branch.
	var (
		pathOpenerCalled bool
		capturedPath     string
	)
	origOpenPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, resolvedPath string, _ []string) error {
		pathOpenerCalled = true
		capturedPath = resolvedPath
		return nil
	}
	t.Cleanup(func() { openPathFunc = origOpenPath })

	// TUI launcher must NOT be reached — reaching it would mean the alias
	// resolution failed to produce a PathResult, which would mask a
	// regression in the path-arg branch we're testing.
	origOpenTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, query string, _ []string, _ bool) error {
		t.Errorf("openTUIFunc unexpectedly called (query=%q); resolver should have produced PathResult", query)
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origOpenTUI })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "mysaved"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// openPath dispatch must have been reached with the alias-resolved
	// project directory — proves the resolver chain produced a
	// PathResult and the command's RunE took the path-arg branch.
	if !pathOpenerCalled {
		t.Fatal("openPathFunc was not invoked for `portal open mysaved`; alias resolution did not reach the path-arg branch")
	}
	if capturedPath != projectDir {
		t.Errorf("openPathFunc resolved path = %q; want %q", capturedPath, projectDir)
	}

	// Skeleton restoration must have run: the saved-only session is
	// live on the same server openPath would have targeted. This is
	// the path-arg equivalent of the "saved name resolves
	// post-bootstrap" guarantee — by the time openPath dispatched,
	// has-session for the saved name would have returned true.
	if _, err := ts.TryRun("has-session", "-t", savedSession); err != nil {
		t.Errorf("has-session -t %s: %v (expected live by the time `portal open mysaved` reached openPath)", savedSession, err)
	}
}

// Compile-time assertions: keep this file's reuse of cmd-package types
// honest. If any of these symbols change shape, the failure surfaces
// here rather than as a runtime mock-mismatch panic deep in a t.Run.
var _ SessionConnector = (*mockSessionConnector)(nil)
var _ SessionValidator = (*mockSessionValidator)(nil)
var _ SwitchClienter = (*mockSwitchClient)(nil)
var _ bootstrap.Runner = (*bootstrap.Orchestrator)(nil)

// Compile-time assertions that openTUIFunc and openPathFunc signatures
// match what the reattach tests inject. Cobra calls go through cmd.RunE,
// so a drift between the registered seam and the test's stub would only
// surface at runtime under the integration tag.
var _ func(*cobra.Command, string, []string, bool) error = openTUIFunc
var _ func(*cobra.Command, string, []string) error = openPathFunc
