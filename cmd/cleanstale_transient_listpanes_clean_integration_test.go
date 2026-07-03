//go:build integration

// End-to-end integration coverage for the `portal clean` command's
// hook-cleanup tail (cmd/clean.go ~lines 75-141) — now the sole
// destructive consumer of ListAllPanes wired through runHookStaleCleanup
// (the former bootstrap-step callsite was removed when hooks stale-cleanup
// left the orchestrator). This file drives cleanCmd.RunE so a regression
// in that destructive call path fails loudly under the simulated tmux
// `list-panes -a` transients motivating the workstream.
//
// Four subtests pin the four rows of the spec's coverage matrix for
// the `portal clean` analogue:
//
//   - mode_a_list_panes_exit_nonzero — `list-panes -a` returns
//     ("", err); RunE hits the err-from-ListAllPanes branch, emits the
//     propagated-error Warn, returns nil (silence-and-continue at the
//     user boundary), and hooks.json is byte-identical. The entry-point
//     Debug ("stale-hook cleanup counts ...") MUST be absent — the err
//     branch returns before the Debug emission.
//
//   - mode_b_list_panes_empty_stdout — `list-panes -a` returns
//     ("", nil); RunE parses zero live panes, emits the entry-point
//     Debug ("stale-hook cleanup counts panes=0 entries=N"), then the
//     hazard-guard Warn skips the destructive store.CleanStale.
//     hooks.json byte-identical, RunE returns nil, no "Removed stale
//     hook:" lines surface on stdout.
//
//   - normal_path_legitimate_stale_removal_still_works — pass-through
//     Commander targeting a real tmuxtest socket. A live pane exists;
//     hooks.json carries one entry whose key matches the live pane
//     (must survive) and one orphan entry (must be removed). RunE
//     returns nil, stdout reports the orphan removal, the completion
//     Debug ("stale-hook cleanup removed reaped=1") lands in portal.log.
//
//   - persisted_empty_early_exit_emits_breadcrumb — hooks.json absent
//     (or empty). RunE hits the persisted==0 early-exit, emits
//     "stale-hook cleanup: persisted=0, skipping" Debug, and the
//     "stale-hook cleanup counts ..." entry-point Debug is absent
//     (early-exit fires before enumeration). hooks.json remains absent /
//     empty. AllPaneLister MUST NOT be invoked — a panic-on-call stub
//     structurally proves this.
//
// Package: `package cmd` so the test can mutate the unexported
// `cleanDeps` seam directly (same pattern as cmd/clean_test.go).
// Shared scaffolding: the `transienttest.Commander`,
// `transienttest.SocketCommander`, `transienttest.SeedHooksJSON`,
// `transienttest.HooksJSONBytes`,
// `transienttest.ResolveHooksFilePathFromEnv`, and
// `transienttest.FailureMode` (+ PassThrough / FailExitNonZero /
// FailEmptyStdout) shapes live in internal/transienttest. The
// `configDirFromEnvSlice`, `staleHookCleanupLogLines`, and
// `containsLineMatching` helpers live in the sibling
// cmd/cleanstale_transient_listpanes_shared_test.go file (same
// `package cmd`, same `//go:build integration` tag) and are accessible
// here verbatim.
//
// Isolation: every subtest calls portaltest.IsolateStateForTest(t).
// XDG_CONFIG_HOME is re-set on the test process (the helper scrubs it
// on the test process and only injects it into the env slice for
// subprocesses; this command-under-test runs IN the test process and
// must see the isolated config dir for hooks.json + projects.json
// path resolution). PORTAL_LOG_LEVEL=debug surfaces the Debug
// breadcrumbs that the assertions depend on.
//
// No t.Parallel: cmd-package convention. cleanDeps mutation is
// reverted via t.Cleanup in every subtest.

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

// panickingPaneLister is an AllPaneLister whose ListAllPanes panics on
// invocation. Used by the persisted_empty_early_exit subtest to
// structurally prove the early-exit branch never reaches the lister.
type panickingPaneLister struct{}

func (panickingPaneLister) ListAllPanes() ([]string, error) {
	panic("ListAllPanes must not be invoked when persisted==0 (early-exit branch)")
}

// setupCleanTransientEnv is the portal-clean callsite's env-builder.
// The portal-clean entry point has no callsite-specific extras
// beyond the four invariant scaffolding steps (no tmux dependency at
// this layer, no subprocess spawn, no socket), so this helper is a pure
// delegation to isolateCleanStaleTestEnv. It survives as a thin
// pass-through so any future callsite-specific extra can be layered here
// without touching the individual subtests. XDG_CONFIG_HOME re-push
// rationale is documented at isolateCleanStaleTestEnv.
func setupCleanTransientEnv(t *testing.T) (env []string, stateDir string) {
	t.Helper()
	return isolateCleanStaleTestEnv(t)
}

// installCleanDepsForLister wires the supplied AllPaneLister into the
// package-level cleanDeps seam and registers cleanup to revert.
func installCleanDepsForLister(t *testing.T, lister AllPaneLister) {
	t.Helper()
	cleanDeps = &CleanDeps{AllPaneLister: lister}
	t.Cleanup(func() { cleanDeps = nil })
}

// runPortalClean invokes the clean subcommand via the root command
// router (matches the convention used in cmd/clean_test.go) and
// returns the captured combined stdout/stderr plus the RunE error.
func runPortalClean(t *testing.T) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"clean"})
	err := rootCmd.Execute()
	return buf.String(), err
}

// assertNoStaleHookRemovalsOnStdout fails the test if the captured
// stdout/stderr contains any "Removed stale hook:" line for any of
// the seeded paneKeys. Mode (a) / mode (b) MUST produce zero such
// lines — the destructive CleanStale is skipped by design.
func assertNoStaleHookRemovalsOnStdout(t *testing.T, output string, seededKeys ...string) {
	t.Helper()
	for _, key := range seededKeys {
		needle := fmt.Sprintf("Removed stale hook: %s", key)
		if strings.Contains(output, needle) {
			t.Fatalf("stdout reported %q under transient — the wipe regression has surfaced to the user\n"+
				"  full output:\n%s", needle, output)
		}
	}
	// Belt-and-braces: no `Removed stale hook:` lines at all, regardless of key.
	if strings.Contains(output, "Removed stale hook:") {
		t.Fatalf("stdout contained an unexpected `Removed stale hook:` line under transient\n"+
			"  full output:\n%s", output)
	}
}

// TestPortalClean_TmuxTransient_DoesNotWipeHooks pins the `portal clean`
// hook-cleanup tail — now the sole destructive consumer of ListAllPanes
// (the former bootstrap-step callsite was removed when hooks stale-cleanup
// left the orchestrator) — so a regression at this destructive site fails
// loudly under tmux transient.
func TestPortalClean_TmuxTransient_DoesNotWipeHooks(t *testing.T) {
	// Portal-clean-callsite invoker for the table-driven mode
	// subtests. The portal-clean entry point intercepts
	// list-panes -a before any real tmux delegation under both failure
	// modes, so a placeholder *tmux.RealCommander as Inner is safe —
	// the intercept path never reaches Inner. runPortalClean captures
	// combined stdout/stderr, which the shared driver hands to
	// spec.extraAssert so the callsite can verify no
	// "Removed stale hook:" line surfaces to the user.
	cleanInvoker := func(mode transienttest.FailureMode) func(t *testing.T, env []string, stateDir string) (string, error) {
		return func(t *testing.T, env []string, stateDir string) (string, error) {
			t.Helper()
			stub := &transienttest.Commander{
				Inner: &tmux.RealCommander{},
				Mode:  mode,
			}
			installCleanDepsForLister(t, tmux.NewClient(stub))
			return runPortalClean(t)
		}
	}
	// Extra assertion shared by mode (a) and mode (b): the user must
	// not see a "Removed stale hook:" line on stdout under either
	// failure mode. The shared driver doesn't know about stdout — it
	// only handles the byte-identity invariant on hooks.json — so the
	// portal-clean callsite layers this assertion on via
	// spec.extraAssert.
	cleanNoStdoutRemovalsAssert := func(t *testing.T, output string, seededKeys []string) {
		t.Helper()
		assertNoStaleHookRemovalsOnStdout(t, output, seededKeys...)
	}

	t.Run("mode_a_list_panes_exit_nonzero", func(t *testing.T) {
		runTransientCleanStaleModeSubtest(t, transientModeSpec{
			name:        "portal-clean mode (a)",
			mode:        transienttest.FailExitNonZero,
			invoke:      cleanInvoker(transienttest.FailExitNonZero),
			extraAssert: cleanNoStdoutRemovalsAssert,
		})
	})

	t.Run("mode_b_list_panes_empty_stdout", func(t *testing.T) {
		runTransientCleanStaleModeSubtest(t, transientModeSpec{
			name:        "portal-clean mode (b)",
			mode:        transienttest.FailEmptyStdout,
			invoke:      cleanInvoker(transienttest.FailEmptyStdout),
			extraAssert: cleanNoStdoutRemovalsAssert,
		})
	})

	t.Run("normal_path_legitimate_stale_removal_still_works", func(t *testing.T) {
		tmuxtest.SkipIfNoTmux(t)

		env, stateDir := setupCleanTransientEnv(t)
		sock := tmuxtest.New(t, "ptl-clean-pass-")

		// Seed a live tmux session so list-panes -a returns a known
		// structural key. The default session/window/pane indices yield
		// "live:0.0".
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

		// Pass-through Commander targets the real test-isolated tmux
		// socket. list-panes -a is delegated to the real tmux server,
		// returning exactly the keys for the seeded `live` session.
		passThroughStub := &transienttest.Commander{
			Inner: &transienttest.SocketCommander{SocketPath: sock.SocketPath()},
			Mode:  transienttest.PassThrough,
		}
		installCleanDepsForLister(t, tmux.NewClient(passThroughStub))

		output, err := runPortalClean(t)
		if err != nil {
			t.Fatalf("portal clean returned error on the normal path; want nil: %v\n  output:\n%s", err, output)
		}

		// The live entry must survive; the stale entry must be removed.
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
		wantLine := "Removed stale hook: gone:0.0"
		if !strings.Contains(output, wantLine) {
			t.Fatalf("normal-path stdout missing %q\n  full output:\n%s", wantLine, output)
		}
		// And must NOT report a removal for the surviving live entry.
		unwantLine := "Removed stale hook: live:0.0"
		if strings.Contains(output, unwantLine) {
			t.Fatalf("normal-path stdout unexpectedly reported %q (live entry must survive)\n  full output:\n%s", unwantLine, output)
		}

		lines := staleHookCleanupLogLines(portaltest.ReadPortalLogSafe(stateDir))
		if len(lines) == 0 {
			t.Fatalf("no `stale-hook cleanup:` lines found in portal.log on the normal path; want entry-point + completion Debug\n"+
				"  full log:\n%s", portaltest.ReadPortalLogSafe(stateDir))
		}
		// Entry-point Debug: "stale-hook cleanup counts panes=1 entries=2"
		// (observability-migration wording; the pre-migration "persisted=2"
		// phrasing is gone).
		if !containsLineMatching(lines, "stale-hook cleanup counts", "entries=2") {
			t.Fatalf("missing normal-path entry-point Debug; want a `stale-hook cleanup counts` line containing `entries=2`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
		// Completion Debug: "stale-hook cleanup removed reaped=1"
		// (observability-migration wording; the pre-migration "removed=1"
		// attr key is now "reaped").
		if !containsLineMatching(lines, "stale-hook cleanup removed", "reaped=1") {
			t.Fatalf("missing normal-path completion Debug; want a `stale-hook cleanup removed` line containing `reaped=1`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
	})

	t.Run("persisted_empty_early_exit_emits_breadcrumb", func(t *testing.T) {
		env, stateDir := setupCleanTransientEnv(t)

		// Snapshot hooks.json state BEFORE invoking clean. Absent
		// means hooksJSONBytes returns nil — that nil-is-preserved
		// invariant is what we assert after the early-exit.
		before := transienttest.HooksJSONBytes(t, env)
		if before != nil {
			t.Fatalf("precondition: hooks.json must be absent before the early-exit subtest; got %d bytes", len(before))
		}

		// Inject a panicking lister to structurally prove the
		// early-exit branch never invokes ListAllPanes.
		installCleanDepsForLister(t, panickingPaneLister{})

		// runPortalClean swallows panics in goroutines but a panic in
		// the test goroutine would crash the test. The early-exit MUST
		// fire before the lister is touched — if it doesn't, the panic
		// surfaces and fails the test immediately.
		output, err := runPortalClean(t)
		if err != nil {
			t.Fatalf("portal clean returned error on the persisted-empty early-exit path; want nil: %v\n  output:\n%s", err, output)
		}

		// hooks.json must remain absent (no implicit creation).
		after := transienttest.HooksJSONBytes(t, env)
		if after != nil {
			t.Fatalf("persisted-empty early-exit created hooks.json on disk; want it to remain absent\n  after: %s", after)
		}

		// Stdout must not report any hook removal.
		if strings.Contains(output, "Removed stale hook:") {
			t.Fatalf("persisted-empty path unexpectedly reported a hook removal\n  full output:\n%s", output)
		}

		// portal.log must contain the persisted=0 early-exit Debug.
		// Use the unfiltered log here — staleHookCleanupLogLines is
		// fine but the early-exit line is the only stale-hook line we
		// expect.
		fullLog := portaltest.ReadPortalLogSafe(stateDir)
		lines := staleHookCleanupLogLines(fullLog)
		if len(lines) == 0 {
			t.Fatalf("no `stale-hook cleanup:` lines found in portal.log on the persisted-empty path; want the early-exit Debug\n"+
				"  full log:\n%s", fullLog)
		}
		if !containsLineMatching(lines, "stale-hook cleanup:", "persisted=0", "skipping") {
			t.Fatalf("missing persisted-empty early-exit Debug; want a `stale-hook cleanup:` line containing `persisted=0` and `skipping`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
		// And the entry-point Debug ("stale-hook cleanup counts ...") must
		// be absent — the early-exit fires before runHookStaleCleanup runs,
		// so its enumeration breadcrumb never emits.
		for _, line := range lines {
			if strings.Contains(line, "stale-hook cleanup counts") {
				t.Fatalf("persisted-empty path emitted entry-point Debug (`stale-hook cleanup counts ...`); must be absent — the early-exit returns before enumeration\n"+
					"  offending line: %s", line)
			}
		}
	})
}

// Compile-time guard: panickingPaneLister must satisfy AllPaneLister.
var _ AllPaneLister = panickingPaneLister{}
