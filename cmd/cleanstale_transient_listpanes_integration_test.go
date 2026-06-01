//go:build integration

// End-to-end integration coverage for the bootstrap step-11
// stale-hook cleanup path under the tmux `list-panes -a` transient
// that motivated this workstream. The unit tests in
// cmd/bootstrap_production_test.go and cmd/clean_test.go close the
// failure modes at the adapter level; this file closes them
// end-to-end against a real tmux server driven through the production
// buildProductionOrchestrator, exercising:
//
//   - the orchestrator's eleven-step ordering and soft-warning wiring
//     (step 11 must NOT escalate to a fatal abort under either mode),
//   - the production *cleanStaleAdapter (composition: real
//     *tmux.Client + real *hooks.Store + real *slog.Logger),
//   - the Change 4 logging contract (entry-point Debug vs terminal
//     Warn distinguishing mode (a) from mode (b)),
//   - the byte-identical hooks.json invariant ("no wipe").
//
// Three subtests pin the three coverage rows:
//
//   - mode_a_list_panes_exit_nonzero — `list-panes -a` returns
//     ("", err); step 11 hits the err-from-ListAllPanes branch, emits
//     the propagated-error Warn, and returns BEFORE the entry-point
//     Debug. hooks.json byte-identical.
//   - mode_b_list_panes_empty_stdout — `list-panes -a` returns
//     ("", nil); step 11 parses zero live panes, emits the entry-point
//     Debug (live=0 persisted=N), then the hazard-guard Warn skips the
//     destructive store.CleanStale. hooks.json byte-identical.
//   - normal_path_legitimate_stale_removal_still_works — pass-through
//     Commander; a stale entry whose paneKey is not represented by any
//     live pane is removed while live-pane-backed entries survive. The
//     completion Debug (removed=1) fires.
//
// Package: this file lives in `package cmd` so it can call the
// unexported `buildProductionOrchestrator` and override the
// unexported `commanderFactory` seam (declared in
// cmd/bootstrap_production.go).
//
// Shared scaffolding: `transienttest.Commander`,
// `transienttest.SocketCommander`, `transienttest.SeedHooksJSON`,
// `transienttest.HooksJSONBytes`,
// `transienttest.ResolveHooksFilePathFromEnv`, and
// `transienttest.FailureMode` (+ PassThrough / FailExitNonZero /
// FailEmptyStdout) live in internal/transienttest. The earlier
// duplication of these shapes across `package bootstrap_test` and
// `package cmd` has been collapsed into the single canonical
// declaration in that sub-package.
//
// Isolation: every subtest calls portaltest.IsolateStateForTest(t) to
// scrub host env and install the fingerprint-diff backstop. Each
// subtest gets its own tmuxtest.Socket so the isolated tmux server is
// torn down cleanly. PORTAL_STATE_DIR is set on the test process so
// any subprocess spawned by the orchestrator (notably the
// `_portal-saver` pane's `portal state daemon`) resolves the same
// isolated state directory. PORTAL_LOG_LEVEL=debug ensures step 11's
// Debug breadcrumbs land in portal.log (default level is LevelWarn,
// which would suppress them and false-negative the assertions).
//
// PATH: the orchestrator's step 5 (EnsureSaver) spawns the `portal`
// binary inside the tmux server's pane. portalbintest.StagePortalBinary
// puts a freshly-built `portal` on PATH so that subprocess resolves
// correctly; the binary spawn happens inside the test process's
// scrubbed environment, so the daemon writes to the isolated state
// directory.
//
// No t.Parallel: cmd-package convention (mock-injection via
// package-level mutable state cleaned up by t.Cleanup) applies. This
// file mutates `commanderFactory`, restored in t.Cleanup.

package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
	"github.com/leeovery/portal/internal/transienttest"
)

// setupTransientCleanStaleEnv builds the per-subtest scaffolding for
// the bootstrap-step-11 transient integration tests. Layers two
// bootstrap-callsite-specific extras over isolateCleanStaleTestEnv
// (the four invariant scaffolding steps shared with the portal-clean
// callsite):
//
//   - tmux skip-if-absent guard (step 5 EnsureSaver requires a real
//     tmux binary on PATH);
//   - portal-binary staging on PATH so step 5 (EnsureSaver) can spawn
//     `portal state daemon` inside the saver pane;
//   - construction of the isolated tmux socket (returned to the
//     subtest so it can wire it into the Commander seam).
//
// XDG_CONFIG_HOME re-push rationale is documented at
// isolateCleanStaleTestEnv.
func setupTransientCleanStaleEnv(t *testing.T, sockPrefix string) (env []string, stateDir string, sock *tmuxtest.Socket) {
	t.Helper()

	tmuxtest.SkipIfNoTmux(t)
	_ = portalbintest.StagePortalBinary(t)

	env, stateDir = isolateCleanStaleTestEnv(t)

	sock = tmuxtest.New(t, sockPrefix)

	return env, stateDir, sock
}

// configDirFromEnvSlice extracts the XDG_CONFIG_HOME value from the
// env slice produced by portaltest.IsolateStateForTest. The slice
// always contains exactly one such entry — its absence signals an
// isolation regression worth a fatal test failure.
func configDirFromEnvSlice(t *testing.T, env []string) string {
	t.Helper()
	const key = "XDG_CONFIG_HOME="
	for _, e := range env {
		if strings.HasPrefix(e, key) {
			return strings.TrimPrefix(e, key)
		}
	}
	t.Fatalf("configDirFromEnvSlice: XDG_CONFIG_HOME not present in env slice — IsolateStateForTest contract regression")
	return ""
}

// installTransientCommanderFactory installs a commanderFactory that
// wires a transienttest.Commander wrapping a socket-anchored Commander
// targeting the test's isolated tmux socket. The factory is reverted on
// test exit via t.Cleanup.
func installTransientCommanderFactory(t *testing.T, socketPath string, mode transienttest.FailureMode) {
	t.Helper()
	prev := commanderFactory
	commanderFactory = func() tmux.Commander {
		return &transienttest.Commander{
			Inner: &transienttest.SocketCommander{SocketPath: socketPath},
			Mode:  mode,
		}
	}
	t.Cleanup(func() { commanderFactory = prev })
}

// staleHookCleanupLogLines returns the subset of portal.log lines
// that carry the step-11 stale-hook cleanup prefix. Filtering on the
// prefix excludes step 9's CleanStaleMarkers log lines (which have
// their own prefix) and any unrelated bootstrap noise, narrowing the
// assertion surface to exactly the step under test.
func staleHookCleanupLogLines(portalLog string) []string {
	const prefix = "stale-hook cleanup:"
	var matches []string
	for _, line := range strings.Split(portalLog, "\n") {
		if strings.Contains(line, prefix) {
			matches = append(matches, line)
		}
	}
	return matches
}

// containsLineMatching reports whether any line contains every
// substring in needles (AND semantics).
func containsLineMatching(lines []string, needles ...string) bool {
	for _, line := range lines {
		matched := true
		for _, n := range needles {
			if !strings.Contains(line, n) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// runProductionBootstrap invokes buildProductionOrchestrator and
// runs the eleven-step pipeline. Returns the (serverStarted, warnings,
// err) triple verbatim.
func runProductionBootstrap(t *testing.T) (bool, []bootstrap.Warning, error) {
	t.Helper()
	orch, _ := buildProductionOrchestrator()
	return orch.Run(context.Background())
}

// TestBootstrap_CleanStale_TmuxTransient_DoesNotWipeHooks pins the
// end-to-end invariant: under either failure mode of the
// `list-panes -a` transient, the production bootstrap pipeline must
// (a) not fatally abort, (b) preserve hooks.json byte-for-byte, and
// (c) emit the Change 4 log fingerprint distinguishing mode (a) from
// mode (b). The pass-through subtest verifies the normal path
// continues to work — a legitimate stale entry is removed while live
// entries survive.
func TestBootstrap_CleanStale_TmuxTransient_DoesNotWipeHooks(t *testing.T) {
	// Bootstrap-callsite invoker for the table-driven mode subtests.
	// The bootstrap entry point requires one callsite-specific extra
	// not handled by the shared isolateCleanStaleTestEnv: an isolated
	// tmux socket wired into the commanderFactory seam. The other two
	// bootstrap-only steps (tmuxtest.SkipIfNoTmux and
	// portalbintest.StagePortalBinary) MUST run before the shared
	// env-builder calls portaltest.IsolateStateForTest — staging
	// shells out to `go build`, and IsolateStateForTest sets HOME to a
	// fresh t.TempDir which would otherwise redirect the module cache
	// under that TempDir and trigger permission-denied cleanup errors
	// on test exit. They run in the t.Run body before
	// runTransientCleanStaleModeSubtest is called. The invoker returns
	// an empty output string since bootstrap doesn't expose a stdout
	// buffer (warnings drain via the orchestrator's
	// []bootstrap.Warning return).
	bootstrapInvoker := func(sockPrefix string, mode transienttest.FailureMode) func(t *testing.T, env []string, stateDir string) (string, error) {
		return func(t *testing.T, env []string, stateDir string) (string, error) {
			t.Helper()
			sock := tmuxtest.New(t, sockPrefix)
			installTransientCommanderFactory(t, sock.SocketPath(), mode)
			_, _, err := runProductionBootstrap(t)
			return "", err
		}
	}

	t.Run("mode_a_list_panes_exit_nonzero", func(t *testing.T) {
		tmuxtest.SkipIfNoTmux(t)
		_ = portalbintest.StagePortalBinary(t)
		runTransientCleanStaleModeSubtest(t, transientModeSpec{
			name:   "bootstrap mode (a)",
			mode:   transienttest.FailExitNonZero,
			invoke: bootstrapInvoker("ptl-cs-a-", transienttest.FailExitNonZero),
		})
	})

	t.Run("mode_b_list_panes_empty_stdout", func(t *testing.T) {
		tmuxtest.SkipIfNoTmux(t)
		_ = portalbintest.StagePortalBinary(t)
		runTransientCleanStaleModeSubtest(t, transientModeSpec{
			name:   "bootstrap mode (b)",
			mode:   transienttest.FailEmptyStdout,
			invoke: bootstrapInvoker("ptl-cs-b-", transienttest.FailEmptyStdout),
		})
	})

	t.Run("normal_path_legitimate_stale_removal_still_works", func(t *testing.T) {
		env, stateDir, sock := setupTransientCleanStaleEnv(t, "ptl-cs-pass-")

		// Pre-create a live tmux session BEFORE installing the
		// passthrough factory. The orchestrator's step 5 (EnsureSaver)
		// will additionally create `_portal-saver`, so the live pane
		// set on entry to step 11 includes at minimum:
		//   - `live:0.0` (this session's first window/pane)
		//   - `_portal-saver:0.0` (if step 5 succeeded; best-effort)
		if _, err := sock.TryRun("new-session", "-d", "-s", "live"); err != nil {
			t.Fatalf("seed live session: %v", err)
		}

		// Seed two entries: one whose paneKey IS live (must survive),
		// one whose paneKey is NOT live (must be removed).
		seedEntries := map[string]string{
			"live:0.0": "echo live",
			"gone:0.0": "echo gone",
		}
		transienttest.SeedHooksJSON(t, env, seedEntries)

		installTransientCommanderFactory(t, sock.SocketPath(), transienttest.PassThrough)

		_, _, err := runProductionBootstrap(t)
		if err != nil {
			t.Fatalf("bootstrap fatally aborted on the normal path; want nil err: %v", err)
		}

		// After the normal path, the live entry must still be
		// present AND the stale entry must have been removed.
		after := transienttest.HooksJSONBytes(t, env)
		afterStr := string(after)
		if !strings.Contains(afterStr, `"live:0.0"`) {
			t.Fatalf("normal path destroyed the live entry `live:0.0`; want it preserved\n"+
				"  hooks.json after: %s", afterStr)
		}
		if strings.Contains(afterStr, `"gone:0.0"`) {
			t.Fatalf("normal path failed to remove the stale entry `gone:0.0`; want it removed\n"+
				"  hooks.json after: %s", afterStr)
		}

		lines := staleHookCleanupLogLines(portaltest.ReadPortalLogSafe(stateDir))
		if len(lines) == 0 {
			t.Fatalf("no `stale-hook cleanup:` lines found in portal.log on the normal path; want entry-point + completion Debug\n"+
				"  full log:\n%s", portaltest.ReadPortalLogSafe(stateDir))
		}
		// Entry-point Debug: persisted=2 (we seeded two entries).
		// The live= count depends on whether EnsureSaver succeeded —
		// at least 1 (the seeded `live` session's pane). We assert
		// only on persisted=2 to stay robust to saver-step variance.
		if !containsLineMatching(lines, "stale-hook cleanup:", "persisted=2") {
			t.Fatalf("missing normal-path entry-point Debug; want a `stale-hook cleanup:` line containing `persisted=2`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
		// Completion Debug: removed=1 (exactly one stale entry).
		if !containsLineMatching(lines, "stale-hook cleanup:", "removed=1") {
			t.Fatalf("missing normal-path completion Debug; want a `stale-hook cleanup:` line containing `removed=1`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
	})
}
