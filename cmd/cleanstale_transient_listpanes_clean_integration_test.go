//go:build integration

// End-to-end integration coverage for the second destructive consumer
// of ListAllPanes — the `portal clean` command's hook-cleanup tail
// (cmd/clean.go ~lines 75-141). The companion file
// cmd/cleanstale_transient_listpanes_integration_test.go covers the
// bootstrap-step-11 callsite; this file covers the analogous callsite
// inside cleanCmd.RunE so a regression at *either* destructive call
// path fails loudly under the simulated tmux `list-panes -a`
// transients motivating the workstream.
//
// Four subtests pin the four rows of the spec's coverage matrix for
// the `portal clean` analogue:
//
//   - mode_a_list_panes_exit_nonzero — `list-panes -a` returns
//     ("", err); RunE hits the err-from-ListAllPanes branch, emits the
//     propagated-error Warn, returns nil (silence-and-continue at the
//     user boundary), and hooks.json is byte-identical. The
//     entry-point Debug (`live=...`) MUST be absent — the err branch
//     returns before the Debug emission.
//
//   - mode_b_list_panes_empty_stdout — `list-panes -a` returns
//     ("", nil); RunE parses zero live panes, emits the entry-point
//     Debug (live=0 persisted=N), then the hazard-guard Warn skips the
//     destructive store.CleanStale. hooks.json byte-identical, RunE
//     returns nil, no "Removed stale hook:" lines surface on stdout.
//
//   - normal_path_legitimate_stale_removal_still_works — pass-through
//     Commander targeting a real tmuxtest socket. A live pane exists;
//     hooks.json carries one entry whose key matches the live pane
//     (must survive) and one orphan entry (must be removed). RunE
//     returns nil, stdout reports the orphan removal, the completion
//     Debug (removed=1) lands in portal.log.
//
//   - persisted_empty_early_exit_emits_breadcrumb — hooks.json absent
//     (or empty). RunE hits the persisted==0 early-exit, emits
//     "stale-hook cleanup: persisted=0, skipping" Debug, and the
//     `live=` substring is absent (early-exit fires before
//     enumeration). hooks.json remains absent / empty. AllPaneLister
//     MUST NOT be invoked — a panic-on-call stub structurally proves
//     this.
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
// `containsLineMatching` helpers remain in the sibling
// cmd/cleanstale_transient_listpanes_integration_test.go file (same
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

// setupCleanTransientEnv builds the per-subtest scaffolding for the
// portal-clean transient integration tests. Side effects:
//   - calls portaltest.IsolateStateForTest (scrubs HOME / XDG,
//     installs the fingerprint-diff backstop);
//   - sets PORTAL_STATE_DIR on the test process so openNoRotateLogger
//     and ReadPortalLogSafe resolve the same isolated dir;
//   - sets PORTAL_LOG_LEVEL=debug so the entry-point / completion
//     Debug breadcrumbs are emitted;
//   - re-pushes XDG_CONFIG_HOME onto the test process so the cmd-
//     package config-path resolution (loadHookStore, loadProjectStore)
//     observes the isolated config dir (same workaround as the
//     bootstrap-tail companion file's setupTransientCleanStaleEnv).
//
// Returns the env slice (carries the isolated XDG_CONFIG_HOME for any
// would-be subprocesses) and stateDir (passed to ReadPortalLogSafe).
func setupCleanTransientEnv(t *testing.T) (env []string, stateDir string) {
	t.Helper()
	env, stateDir = portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	t.Setenv("PORTAL_LOG_LEVEL", "debug")
	t.Setenv("XDG_CONFIG_HOME", configDirFromEnvSlice(t, env))
	return env, stateDir
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

// TestPortalClean_TmuxTransient_DoesNotWipeHooks closes the
// `portal clean` half of the defect-class scope — the second
// destructive consumer of ListAllPanes (the first being bootstrap
// step 11). Mirrors TestBootstrap_CleanStale_TmuxTransient_DoesNotWipeHooks
// row-for-row at this callsite so a regression at either destructive
// site fails loudly under tmux transient.
func TestPortalClean_TmuxTransient_DoesNotWipeHooks(t *testing.T) {
	t.Run("mode_a_list_panes_exit_nonzero", func(t *testing.T) {
		env, stateDir := setupCleanTransientEnv(t)

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

		// Wrap a *tmux.Client around a transient-listpanes Commander.
		// The Inner Commander is irrelevant for mode (a) — every
		// list-panes -a call is intercepted before reaching Inner — so
		// a placeholder *tmux.RealCommander is acceptable; the
		// intercept path never delegates to it.
		stub := &transienttest.Commander{
			Inner: &tmux.RealCommander{},
			Mode:  transienttest.FailExitNonZero,
		}
		installCleanDepsForLister(t, tmux.NewClient(stub))

		output, err := runPortalClean(t)
		if err != nil {
			t.Fatalf("portal clean returned error under mode (a); want nil (RunE must Warn-and-swallow): %v\n  output:\n%s", err, output)
		}

		after := transienttest.HooksJSONBytes(t, env)
		if !bytes.Equal(before, after) {
			t.Fatalf("hooks.json mutated under mode (a) — the wipe regression has returned at the portal-clean callsite\n"+
				"  before: %s\n"+
				"  after:  %s",
				before, after)
		}

		assertNoStaleHookRemovalsOnStdout(t, output, "alpha:0.0", "beta:0.0", "gamma:0.0")

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
		env, stateDir := setupCleanTransientEnv(t)

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

		stub := &transienttest.Commander{
			Inner: &tmux.RealCommander{},
			Mode:  transienttest.FailEmptyStdout,
		}
		installCleanDepsForLister(t, tmux.NewClient(stub))

		output, err := runPortalClean(t)
		if err != nil {
			t.Fatalf("portal clean returned error under mode (b); want nil (hazard guard must Warn-and-swallow): %v\n  output:\n%s", err, output)
		}

		after := transienttest.HooksJSONBytes(t, env)
		if !bytes.Equal(before, after) {
			t.Fatalf("hooks.json mutated under mode (b) — the hazard guard failed and the wipe regression has returned at the portal-clean callsite\n"+
				"  before: %s\n"+
				"  after:  %s",
				before, after)
		}

		assertNoStaleHookRemovalsOnStdout(t, output, "alpha:0.0", "beta:0.0", "gamma:0.0")

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
		// Entry-point Debug: persisted=2.
		if !containsLineMatching(lines, "stale-hook cleanup:", "persisted=2") {
			t.Fatalf("missing normal-path entry-point Debug; want a `stale-hook cleanup:` line containing `persisted=2`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
		// Completion Debug: removed=1.
		if !containsLineMatching(lines, "stale-hook cleanup:", "removed=1") {
			t.Fatalf("missing normal-path completion Debug; want a `stale-hook cleanup:` line containing `removed=1`\n"+
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
		// And the `live=` substring must be absent — the early-exit
		// fires before the entry-point Debug (which is the only line
		// in this path that would include `live=`).
		for _, line := range lines {
			if strings.Contains(line, "live=") {
				t.Fatalf("persisted-empty path emitted entry-point Debug (`live=...`); must be absent — the early-exit returns before enumeration\n"+
					"  offending line: %s", line)
			}
		}
	})
}

// Compile-time guard: panickingPaneLister must satisfy AllPaneLister.
var _ AllPaneLister = panickingPaneLister{}
