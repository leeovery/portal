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
//     *tmux.Client + real *hooks.Store + real *state.Logger),
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
	"bytes"
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

// setupTransientCleanStaleEnv builds the per-subtest scaffolding.
// Side effects:
//   - calls portaltest.IsolateStateForTest (scrubs HOME / XDG, installs
//     fingerprint backstop),
//   - sets PORTAL_STATE_DIR on the test process so subprocesses
//     resolve the same isolated dir,
//   - sets PORTAL_LOG_LEVEL=debug so step 11's Debug breadcrumbs land
//     in portal.log,
//   - stages the portal binary on PATH so step 5 (EnsureSaver) can
//     spawn `portal state daemon` inside the saver pane,
//   - constructs the isolated tmux socket.
func setupTransientCleanStaleEnv(t *testing.T, sockPrefix string) (env []string, stateDir string, sock *tmuxtest.Socket) {
	t.Helper()

	tmuxtest.SkipIfNoTmux(t)

	_ = portalbintest.StagePortalBinary(t)

	env, stateDir = portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	t.Setenv("PORTAL_LOG_LEVEL", "debug")

	// IsolateStateForTest sets XDG_CONFIG_HOME="" on the test process
	// (load-bearing for its fingerprint backstop), pushing the
	// isolated XDG_CONFIG_HOME=<configDir> into the env slice for
	// subprocesses only. The orchestrator under test runs IN the test
	// process, so cmd-package hook-store path resolution (configFilePath
	// → xdg.ConfigBase → $XDG_CONFIG_HOME) would otherwise resolve to
	// the test process's HOME-based fallback and miss the seeded
	// hooks.json. Override XDG_CONFIG_HOME on the test process too —
	// IsolateStateForTest's pre-snapshot already ran, so this
	// post-snapshot Setenv does not perturb the backstop.
	configDir := configDirFromEnvSlice(t, env)
	t.Setenv("XDG_CONFIG_HOME", configDir)

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
	t.Run("mode_a_list_panes_exit_nonzero", func(t *testing.T) {
		env, stateDir, sock := setupTransientCleanStaleEnv(t, "ptl-cs-a-")

		seedEntries := map[string]string{
			"alpha:0.0": "echo a",
			"beta:0.0":  "echo b",
			"gamma:0.0": "echo c",
		}
		transienttest.SeedHooksJSON(t, env, seedEntries)
		before := transienttest.HooksJSONBytes(t, env)
		if len(before) == 0 {
			t.Fatalf("precondition: hooksJSONBytes returned empty slice after seed")
		}

		installTransientCommanderFactory(t, sock.SocketPath(), transienttest.FailExitNonZero)

		_, _, err := runProductionBootstrap(t)
		if err != nil {
			t.Fatalf("bootstrap fatally aborted under mode (a); want nil err (step 11 must Warn-and-swallow): %v", err)
		}

		after := transienttest.HooksJSONBytes(t, env)
		if !bytes.Equal(before, after) {
			t.Fatalf("hooks.json mutated under mode (a) — the wipe regression has returned\n"+
				"  before: %s\n"+
				"  after:  %s",
				before, after)
		}

		lines := staleHookCleanupLogLines(portaltest.ReadPortalLogSafe(stateDir))
		if len(lines) == 0 {
			t.Fatalf("no `stale-hook cleanup:` lines found in portal.log under mode (a); want at least the propagated-error Warn\n"+
				"  full log:\n%s", portaltest.ReadPortalLogSafe(stateDir))
		}
		if !containsLineMatching(lines, "stale-hook cleanup:", "list-panes failed", "simulated transient") {
			t.Fatalf("missing mode (a) propagated-error Warn line; want a `stale-hook cleanup:` line containing `list-panes failed` and `simulated transient`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
		for _, line := range lines {
			if strings.Contains(line, "live=") {
				t.Fatalf("mode (a) emitted entry-point Debug (`live=...`); must be absent — the err-from-ListAllPanes branch returns before the Debug emission\n"+
					"  offending line: %s", line)
			}
		}
	})

	t.Run("mode_b_list_panes_empty_stdout", func(t *testing.T) {
		env, stateDir, sock := setupTransientCleanStaleEnv(t, "ptl-cs-b-")

		seedEntries := map[string]string{
			"alpha:0.0": "echo a",
			"beta:0.0":  "echo b",
			"gamma:0.0": "echo c",
		}
		transienttest.SeedHooksJSON(t, env, seedEntries)
		before := transienttest.HooksJSONBytes(t, env)
		if len(before) == 0 {
			t.Fatalf("precondition: hooksJSONBytes returned empty slice after seed")
		}

		installTransientCommanderFactory(t, sock.SocketPath(), transienttest.FailEmptyStdout)

		_, _, err := runProductionBootstrap(t)
		if err != nil {
			t.Fatalf("bootstrap fatally aborted under mode (b); want nil err (step 11 must Warn-and-swallow): %v", err)
		}

		after := transienttest.HooksJSONBytes(t, env)
		if !bytes.Equal(before, after) {
			t.Fatalf("hooks.json mutated under mode (b) — the hazard guard failed and the wipe regression has returned\n"+
				"  before: %s\n"+
				"  after:  %s",
				before, after)
		}

		lines := staleHookCleanupLogLines(portaltest.ReadPortalLogSafe(stateDir))
		if len(lines) == 0 {
			t.Fatalf("no `stale-hook cleanup:` lines found in portal.log under mode (b); want the entry-point Debug and the hazard-guard Warn\n"+
				"  full log:\n%s", portaltest.ReadPortalLogSafe(stateDir))
		}
		if !containsLineMatching(lines, "stale-hook cleanup:", "live=0", "persisted=3") {
			t.Fatalf("missing mode (b) entry-point Debug; want a `stale-hook cleanup:` line containing `live=0` and `persisted=3`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
		if !containsLineMatching(lines, "stale-hook cleanup:", "zero live panes", "3 hook(s) present", "mass-deletion hazard") {
			t.Fatalf("missing mode (b) hazard-guard Warn; want a `stale-hook cleanup:` line containing `zero live panes`, `3 hook(s) present`, and `mass-deletion hazard`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
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
