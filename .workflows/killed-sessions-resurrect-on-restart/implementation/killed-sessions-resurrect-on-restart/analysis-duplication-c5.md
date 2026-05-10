# Duplication Findings — killed-sessions-resurrect-on-restart (cycle 5)

```
AGENT: duplication
FINDINGS:
- FINDING: `buildReattachOrchestrator` open-codes the `restoreInner := &restore.Orchestrator{...}` + `&bootstrapadapter.RestoreAdapter{Inner: restoreInner}` two-step preamble despite living in the work unit's blast radius and having `bootstrapadapter.NewRestoreAdapter` available
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/cmd/reattach_integration_test.go:173-184, /Users/leeovery/Code/portal/internal/bootstrapadapter/adapters.go:107-115
  DESCRIPTION: T4-3 (commit 815dce7f, this work unit) restructured `buildReattachOrchestrator` extensively — collapsing its inline 23-line `&bootstrap.Orchestrator{...}` literal into a 6-line `bootstrap.NewWithDefaults(...)` call — but kept the `restoreInner := &restore.Orchestrator{Client, StateDir, Logger}` block at lines 173-177 alongside the `bootstrap.WithRestore(&bootstrapadapter.RestoreAdapter{Inner: restoreInner})` wrap at line 183. T6-2 then introduced `bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)` (cycle 3 cleanup) precisely to collapse this two-step preamble into a single call, and adopted it at the four new integration-test sites added by this work unit. The reattach site was left out of the migration scope per cycle 3's recommendation phrasing ("the four new sites then collapses to ... and the pre-existing seven sites can opt in over time"). However, this site is *not* a pre-existing site untouched by the work unit — T4-3 actively rewrote the surrounding orchestrator-construction code on the same lines, and the file is enumerated in the cycle-5 implementation files in scope. The constructor's docstring at adapters.go:97-102 names "cmd/bootstrap_production.go" as the only deliberate non-migrated open-coded site (rationale: "parity with the surrounding inline-struct adapters"); `buildReattachOrchestrator` is not on that exemption list, and its surrounding adapter is `&bootstrapadapter.RestoringMarker{Client: client}` (a one-liner) — there is no inline-struct-adapter parity to preserve. The collapse is mechanical:
    Before (lines 173-184, 12 lines):
        restoreInner := &restore.Orchestrator{
            Client:   client,
            StateDir: stateDir,
            Logger:   logger,
        }
        return bootstrap.NewWithDefaults(
            client,
            stateDir,
            logger,
            &bootstrapadapter.RestoringMarker{Client: client},
            bootstrap.WithRestore(&bootstrapadapter.RestoreAdapter{Inner: restoreInner}),
        )
    After (7 lines):
        return bootstrap.NewWithDefaults(
            client,
            stateDir,
            logger,
            &bootstrapadapter.RestoringMarker{Client: client},
            bootstrap.WithRestore(bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)),
        )
  Net deletion: ~5 lines plus removal of the unused `"github.com/leeovery/portal/internal/restore"` import. Same shape cycle 3 applied at the four new integration sites.
  RECOMMENDATION: Replace lines 173-184 of cmd/reattach_integration_test.go with the single `bootstrap.NewWithDefaults(...)` call shown above, passing `bootstrap.WithRestore(bootstrapadapter.NewRestoreAdapter(client, stateDir, logger))` directly. Drop the now-unused `"github.com/leeovery/portal/internal/restore"` import. Aligns with cycle 3's adoption pattern at the four sibling integration-test sites.

- FINDING: `buildReattachOrchestrator` open-codes the `state.OpenLogger + t.Cleanup` boilerplate that `openTestLogger` exists to consolidate
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/cmd/reattach_integration_test.go:165-171, /Users/leeovery/Code/portal/cmd/bootstrap/orchestrator_builder_test.go:116-124
  DESCRIPTION: `buildReattachOrchestrator` opens its own logger via the same six-line pattern that the canonical `openTestLogger` helper bottles up:
    cmd/reattach_integration_test.go:165-171:
        logger, err := state.OpenLogger(filepath.Join(stateDir, "portal.log"), false)
        if err != nil {
            t.Fatalf("OpenLogger: %v", err)
        }
        t.Cleanup(func() { _ = logger.Close() })
    cmd/bootstrap/orchestrator_builder_test.go:116-124:
        func openTestLogger(t *testing.T, stateDir string) *state.Logger {
            t.Helper()
            logger, err := state.OpenLogger(filepath.Join(stateDir, "portal.log"), false)
            if err != nil {
                t.Fatalf("OpenLogger: %v", err)
            }
            t.Cleanup(func() { _ = logger.Close() })
            return logger
        }
  Bodies are byte-equivalent. The `openTestLogger` docstring explicitly motivates the helper as "Tests that wire a real Logger or any adapter that needs one (FIFOSweeper, HookRegistrar) share this helper to avoid duplicating the OpenLogger + Cleanup pattern" — `buildReattachOrchestrator` is exactly such a site. The helper currently lives in `package bootstrap_test`, and Go test-file symbols are not visible across packages — so cmd/reattach_integration_test.go cannot import it as-is. Same cross-package-test-helper-after-extraction pattern earlier cycles addressed for `WaitForFileExists` (cycle 1 → internal/restoretest) and `RecordingFIFOSignaler` (cycle 2 → internal/statetest). cmd/bootstrap_test invokes openTestLogger at twelve sites; cmd/reattach_integration_test.go is the one cmd-package site that re-implements it inline.
  RECOMMENDATION: Promote `openTestLogger` to a non-test, exported helper in `internal/restoretest` (e.g. `OpenTestLogger(t *testing.T, stateDir string) *state.Logger`), matching the precedent set by `restoretest.WaitForSkeletonMarkersCleared` / `restoretest.SeedSessionsJSON` / `restoretest.WaitForFileExists`. Both consumers (cmd/bootstrap_test twelve sites + cmd/reattach_integration_test.go one site) import the canonical version. Optional alternative: leave openTestLogger in cmd/bootstrap_test and inline at the reattach site (status quo), accepting one-site duplication. Helper-promotion option preferred — aligns with the cross-package-test-helper pattern from prior cycles.

SUMMARY: Cycle 4's two cleanup tasks landed correctly. Two new low-severity duplication candidates remain in cycle 5 scope, both at cmd/reattach_integration_test.go: (a) buildReattachOrchestrator open-codes the restore.Orchestrator + RestoreAdapter two-step preamble that bootstrapadapter.NewRestoreAdapter exists to collapse (file is in this work unit's blast radius and was actively rewritten by T4-3, but the helper from T6-2 was applied at four sibling sites and skipped here), and (b) buildReattachOrchestrator open-codes the state.OpenLogger + t.Cleanup boilerplate that the cmd/bootstrap_test-package-private openTestLogger exists to consolidate at twelve sites. Both are mechanical extract-and-reuse cleanups; do not block correctness.
```
