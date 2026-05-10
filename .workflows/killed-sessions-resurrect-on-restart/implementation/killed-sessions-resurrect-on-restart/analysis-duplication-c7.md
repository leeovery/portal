# Duplication Findings — killed-sessions-resurrect-on-restart (cycle 7)

```
AGENT: duplication
FINDINGS:
- FINDING: `setupExitClosesPane` in internal/restore/exit_closes_pane_integration_test.go still opens its *state.Logger inline at lines 375-379 — the only remaining inline OpenLogger preamble after cycle 6's restoretest.OpenTestLogger rollout
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/internal/restore/exit_closes_pane_integration_test.go:375-379, /Users/leeovery/Code/portal/internal/restoretest/logger.go:28-36
  DESCRIPTION: Cycle 6's openTestLogger cleanup migrated 13 sites to restoretest.OpenTestLogger. One sibling site was missed: setupExitClosesPane at internal/restore/exit_closes_pane_integration_test.go:375-379 still open-codes the same five-line preamble:
        logger, err := state.OpenLogger(filepath.Join(stateDir, "portal.log"), false)
        if err != nil {
            t.Fatalf("OpenLogger: %v", err)
        }
        t.Cleanup(func() { _ = logger.Close() })
  The file is in scope for this work unit: T2-3 (runHydrate timeout fall-through fires on-resume hooks) and T2-4 (timeout fall-through no-hook degradation regression tests) both touched it. Package import requirements are satisfied — exit_closes_pane_integration_test.go is package `restore_test` and already imports `github.com/leeovery/portal/internal/restoretest`. The pre-existing internal/restore/restore_test.go openTestLogger helper is out of scope.
    Example collapse (5 lines → 1 line):
        Before: logger, err := state.OpenLogger(filepath.Join(stateDir, "portal.log"), false) ... t.Cleanup(func() { _ = logger.Close() })
        After:  logger := restoretest.OpenTestLogger(t, stateDir)
  Net deletion: ~5 lines. The path/filepath and internal/state imports remain needed for other call sites in the file.
  RECOMMENDATION: Replace lines 375-379 in /Users/leeovery/Code/portal/internal/restore/exit_closes_pane_integration_test.go with `logger := restoretest.OpenTestLogger(t, stateDir)`. The restoretest import already exists.

- FINDING: Three in-scope integration tests duplicate a ~25-line cold-start integration preamble — BuildPortalBinaryDir + PrependPATH + newIntegrationStateDir + SeedSessionsJSON + tmuxtest.New + EnsureServer + pre-condition has-session loop + OpenTestLogger + buildIntegrationOrchestrator with NewRestoreAdapter + Run + post-Run list-sessions sanity loop
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:153-223 (AC1 runEagerSignalMultiSessionAC1 inner body), /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:293-347 (AC4), /Users/leeovery/Code/portal/cmd/bootstrap/phase2_hook_fire_integration_test.go:95-194 (AC2)
  DESCRIPTION: Each of the three integration-test bodies runs the same cold-start scaffolding before diverging into AC-specific assertions: build portal binary, prepend PATH, isolated state dir, seed sessions.json, isolated tmux server, EnsureServer, pre-condition has-session loop (sessions not yet live), OpenTestLogger, wire orchestrator with NewRestoreAdapter, Run, post-Run list-sessions sanity loop. The variations between sites are mechanical, not semantic:
    * AC1 inner: binDir is hoisted to the parent test for sub-test sharing, so this site does PrependPATH only. tmuxtest prefix "ptl-eager-".
    * AC4: inline BuildPortalBinaryDir + PrependPATH. tmuxtest prefix "ptl-eager-ac4-".
    * AC2: inline BuildPortalBinaryDir + PrependPATH, *plus* extra Setenv("PORTAL_HOOKS_FILE", ...) + hooks.NewStore(...).Set() between the bin-PATH and seed-sessions steps. tmuxtest prefix "ptl-p2-".
  Each post-preamble divergence is the actual test (eager-signal AC1 marker poll, AC4 scrollback-after-daemon-tick, AC2 hook-fire sentinel). All three files are in scope per cycle 7's file list and all three were rewritten by phase 1-2 work unit tasks. A natural next step is a `bootstrapEagerHydrateScenario(t, prefix, sessions) (...)` helper that returns (ts, client, stateDir, logger, orchestrator) after running the orchestrator. Net deletion per site: ~25 → ~5 lines; total ~60 lines across 3 sites.
  Counter-argument worth recording: the AC2 site has extra PORTAL_HOOKS_FILE + hooks-store steps inline before the orchestrator wiring, so a single helper would either need a pre-orchestrator callback (additional parameter), or AC2 would skip the helper and accept the duplication. Two of three sites is still a worthwhile consolidation.
  RECOMMENDATION: Extract `bootstrapEagerHydrateScenario(t, prefix, sessions) (*tmuxtest.Socket, *tmux.Client, string, *state.Logger, *bootstrap.Orchestrator)` to cmd/bootstrap/orchestrator_builder_test.go that bundles the steps above and returns the post-Run state. Migrate AC1 inner body and AC4. AC2 stays open-coded because of the extra PORTAL_HOOKS_FILE + hooks.Set steps that interleave with the seed-sessions step. Alternative shape if AC2 migration is wanted: accept an optional preOrchestratorSetup callback parameter.

SUMMARY: Cycle 6's two cleanups landed cleanly. Two new low-severity duplication candidates remain at cycle 7 scope: (a) one site in internal/restore/exit_closes_pane_integration_test.go still open-codes the OpenLogger + t.Cleanup-Close preamble that restoretest.OpenTestLogger replaces — a 5-line → 1-line collapse that extends cycle 6's 13-site migration; (b) three integration-test bodies duplicate a ~25-line cold-start scaffolding preamble that could be extracted to a bootstrapEagerHydrateScenario helper. Both are mechanical extract-and-reuse cleanups; neither blocks correctness.
```
