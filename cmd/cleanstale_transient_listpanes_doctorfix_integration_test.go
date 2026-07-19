//go:build integration

// End-to-end integration coverage for `portal doctor --fix`'s stale-hook
// prune (pruneDoctorStaleHooks, cmd/doctor.go) — the user-facing destructive
// consumer of ListAllPaneHookKeys wired through runHookStaleCleanup. The
// former `portal clean` hook-cleanup tail was DELETED in the
// cli-verb-surface redesign (portal clean → doctor --fix + automatic daemon
// pruning); the daemon's maybeRunHookCleanup is now the automatic consumer of
// this destructive path and doctor --fix the manual one. This file drives
// pruneDoctorStaleHooks so a regression at that destructive site fails loudly
// under the simulated tmux `list-panes -a` transients motivating the
// workstream — coverage that the daemon happy-path integration test
// (cmd/state_daemon_hook_cleanup_integration_test.go) does NOT provide (it
// exercises only the reap/retain success path, never the transient failure
// modes).
//
// Three subtests pin the transient-hazard rows of the spec's coverage matrix:
//
//   - mode_a_list_panes_exit_nonzero — `list-panes -a` returns ("", err);
//     runHookStaleCleanup hits the err-from-ListAllPaneHookKeys branch, emits
//     the propagated-error Warn, returns nil (silence-and-continue at the
//     repair boundary — repairs never drive the exit), and hooks.json is
//     byte-identical. The entry-point Debug ("stale-hook cleanup counts ...")
//     MUST be absent — the err branch returns before the Debug emission.
//
//   - mode_b_list_panes_empty_stdout — `list-panes -a` returns ("", nil);
//     runHookStaleCleanup parses zero live panes, emits the entry-point Debug
//     ("stale-hook cleanup counts panes=0 entries=N"), then the hazard-guard
//     Warn skips the destructive store.CleanStale. hooks.json byte-identical,
//     no "Pruned stale hook:" line surfaces on stdout.
//
//   - normal_path_legitimate_stale_removal_still_works — pass-through
//     Commander targeting a real tmuxtest socket. A live pane exists;
//     hooks.json carries one entry whose key matches the live pane (must
//     survive) and one orphan entry (must be pruned). pruneDoctorStaleHooks
//     surfaces the orphan removal on stdout, and the completion Debug
//     ("stale-hook cleanup removed reaped=1") lands in portal.log.
//
// The former persisted_empty_early_exit subtest is intentionally dropped: it
// exercised the deleted cleanStaleHooks persisted==0 short-circuit (which fired
// BEFORE the lister via a panic-on-call stub). doctor --fix has no analogue —
// pruneDoctorStaleHooks calls runHookStaleCleanup directly, which always
// enumerates the live set first — so the panic-proof-of-early-exit assertion no
// longer applies.
//
// Package: `package cmd` so the test can call the unexported
// pruneDoctorStaleHooks + loadHookStore directly. Shared scaffolding — the
// transienttest.Commander / SocketCommander / SeedHooksJSON / HooksJSONBytes /
// ResolveHooksFilePathFromEnv / FailureMode shapes plus the
// isolateCleanStaleTestEnv, runTransientCleanStaleModeSubtest,
// configDirFromEnvSlice, staleHookCleanupLogLines, and containsLineMatching
// helpers — lives in cmd/cleanstale_transient_listpanes_shared_test.go (same
// package cmd, same //go:build integration tag).
//
// Isolation: every subtest calls portaltest.IsolateStateForTest(t) via the
// shared env-builder. XDG_CONFIG_HOME is re-set on the test process (the
// pruneDoctorStaleHooks call runs IN the test process and must see the isolated
// config dir for hooks.json path resolution). PORTAL_LOG_LEVEL=debug surfaces
// the Debug breadcrumbs the assertions depend on.
//
// No t.Parallel: cmd-package convention.

package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
	"github.com/leeovery/portal/internal/transienttest"
)

// runDoctorFixHookPrune builds a minimal DoctorDeps wired to the isolated
// hooks.json (loaded via the same loadHookStore() production wiring uses) and
// the supplied transient-wrapped HookLister, then runs doctor --fix's stale-hook
// prune (pruneDoctorStaleHooks) and returns the captured stdout. loadHookStore()
// resolves PORTAL_HOOKS_FILE from the isolated env the shared scaffolding sets on
// the test process, so it observes the SAME hooks.json SeedHooksJSON writes.
func runDoctorFixHookPrune(t *testing.T, lister AllPaneLister) (string, error) {
	t.Helper()
	hookStore, err := loadHookStore()
	if err != nil {
		return "", err
	}
	w := new(bytes.Buffer)
	pruneDoctorStaleHooks(w, &DoctorDeps{HookLister: lister, HookStore: hookStore})
	return w.String(), nil
}

// assertNoStaleHookPrunesOnStdout fails the test if the captured stdout contains
// any "Pruned stale hook:" line for any of the seeded paneKeys. Mode (a) /
// mode (b) MUST produce zero such lines — the destructive CleanStale is skipped
// by design.
func assertNoStaleHookPrunesOnStdout(t *testing.T, output string, seededKeys ...string) {
	t.Helper()
	for _, key := range seededKeys {
		needle := fmt.Sprintf("Pruned stale hook: %s", key)
		if strings.Contains(output, needle) {
			t.Fatalf("stdout reported %q under transient — the wipe regression has surfaced to the user\n"+
				"  full output:\n%s", needle, output)
		}
	}
	// Belt-and-braces: no `Pruned stale hook:` lines at all, regardless of key.
	if strings.Contains(output, "Pruned stale hook:") {
		t.Fatalf("stdout contained an unexpected `Pruned stale hook:` line under transient\n"+
			"  full output:\n%s", output)
	}
}

// TestDoctorFix_TmuxTransient_DoesNotWipeHooks pins the doctor --fix stale-hook
// prune — the user-facing destructive consumer of ListAllPaneHookKeys wired
// through runHookStaleCleanup — so a regression at this destructive site fails
// loudly under tmux transient.
func TestDoctorFix_TmuxTransient_DoesNotWipeHooks(t *testing.T) {
	// doctor-fix-callsite invoker for the table-driven mode subtests. The
	// runHookStaleCleanup helper intercepts list-panes -a before any real tmux
	// delegation under both failure modes, so a placeholder *tmux.RealCommander
	// as Inner is safe — the intercept path never reaches Inner.
	// runDoctorFixHookPrune captures stdout, which the shared driver hands to
	// spec.extraAssert so the callsite can verify no "Pruned stale hook:" line
	// surfaces to the user.
	doctorInvoker := func(mode transienttest.FailureMode) func(t *testing.T, env []string, stateDir string) (string, error) {
		return func(t *testing.T, env []string, stateDir string) (string, error) {
			t.Helper()
			stub := &transienttest.Commander{
				Inner: &tmux.RealCommander{},
				Mode:  mode,
			}
			return runDoctorFixHookPrune(t, tmux.NewClient(stub))
		}
	}
	// Extra assertion shared by mode (a) and mode (b): the user must not see a
	// "Pruned stale hook:" line on stdout under either failure mode.
	doctorNoStdoutPrunesAssert := func(t *testing.T, output string, seededKeys []string) {
		t.Helper()
		assertNoStaleHookPrunesOnStdout(t, output, seededKeys...)
	}

	t.Run("mode_a_list_panes_exit_nonzero", func(t *testing.T) {
		runTransientCleanStaleModeSubtest(t, transientModeSpec{
			name:        "doctor-fix mode (a)",
			mode:        transienttest.FailExitNonZero,
			invoke:      doctorInvoker(transienttest.FailExitNonZero),
			extraAssert: doctorNoStdoutPrunesAssert,
		})
	})

	t.Run("mode_b_list_panes_empty_stdout", func(t *testing.T) {
		runTransientCleanStaleModeSubtest(t, transientModeSpec{
			name:        "doctor-fix mode (b)",
			mode:        transienttest.FailEmptyStdout,
			invoke:      doctorInvoker(transienttest.FailEmptyStdout),
			extraAssert: doctorNoStdoutPrunesAssert,
		})
	})

	t.Run("normal_path_legitimate_stale_removal_still_works", func(t *testing.T) {
		tmuxtest.SkipIfNoTmux(t)

		env, stateDir := isolateCleanStaleTestEnv(t)
		sock := tmuxtest.New(t, "ptl-doctorfix-pass-")

		// Seed a live tmux session so list-panes -a returns a known structural
		// key. The default session/window/pane indices yield "live:0.0".
		if _, err := sock.TryRun("new-session", "-d", "-s", "live"); err != nil {
			t.Fatalf("seed live session: %v", err)
		}

		// Seed two entries: one whose paneKey IS live (must survive), one whose
		// paneKey is NOT live (must be pruned).
		seedEntries := map[string]string{
			"live:0.0": "echo live",
			"gone:0.0": "echo gone",
		}
		transienttest.SeedHooksJSON(t, env, seedEntries)

		// Pass-through Commander targets the real test-isolated tmux socket.
		// list-panes -a is delegated to the real tmux server, returning exactly
		// the keys for the seeded `live` session.
		passThroughStub := &transienttest.Commander{
			Inner: &transienttest.SocketCommander{SocketPath: sock.SocketPath()},
			Mode:  transienttest.PassThrough,
		}

		output, err := runDoctorFixHookPrune(t, tmux.NewClient(passThroughStub))
		if err != nil {
			t.Fatalf("doctor --fix hook prune returned error on the normal path; want nil: %v\n  output:\n%s", err, output)
		}

		// The live entry must survive; the stale entry must be pruned.
		afterStr := string(transienttest.HooksJSONBytes(t, env))
		if !strings.Contains(afterStr, `"live:0.0"`) {
			t.Fatalf("normal path destroyed the live entry `live:0.0`; want it preserved\n"+
				"  hooks.json after: %s", afterStr)
		}
		if strings.Contains(afterStr, `"gone:0.0"`) {
			t.Fatalf("normal path failed to remove the stale entry `gone:0.0`; want it removed\n"+
				"  hooks.json after: %s", afterStr)
		}

		// stdout must report exactly the orphan's removal.
		wantLine := "Pruned stale hook: gone:0.0"
		if !strings.Contains(output, wantLine) {
			t.Fatalf("normal-path stdout missing %q\n  full output:\n%s", wantLine, output)
		}
		// And must NOT report a removal for the surviving live entry.
		unwantLine := "Pruned stale hook: live:0.0"
		if strings.Contains(output, unwantLine) {
			t.Fatalf("normal-path stdout unexpectedly reported %q (live entry must survive)\n  full output:\n%s", unwantLine, output)
		}

		lines := staleHookCleanupLogLines(portaltest.ReadPortalLogSafe(stateDir))
		if len(lines) == 0 {
			t.Fatalf("no `stale-hook cleanup:` lines found in portal.log on the normal path; want entry-point + completion Debug\n"+
				"  full log:\n%s", portaltest.ReadPortalLogSafe(stateDir))
		}
		// Entry-point Debug: "stale-hook cleanup counts panes=1 entries=2".
		if !containsLineMatching(lines, "stale-hook cleanup counts", "entries=2") {
			t.Fatalf("missing normal-path entry-point Debug; want a `stale-hook cleanup counts` line containing `entries=2`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
		// Completion Debug: "stale-hook cleanup removed reaped=1".
		if !containsLineMatching(lines, "stale-hook cleanup removed", "reaped=1") {
			t.Fatalf("missing normal-path completion Debug; want a `stale-hook cleanup removed` line containing `reaped=1`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
	})
}
