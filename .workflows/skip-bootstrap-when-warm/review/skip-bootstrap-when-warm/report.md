# Implementation Review: Skip Bootstrap When Warm

**Plan**: skip-bootstrap-when-warm
**QA Verdict**: Approve

## Summary

The feature is fully and faithfully implemented. All 18 leaf tasks across the three implementation phases plus four analysis-cycle chores verify Complete with **zero blocking issues**. The version-stamped `@portal-bootstrapped` latch, the three-way entry-path branch in `PersistentPreRunE`, the liveness-only abridged `EnsureSaver`, the 11→10-step orchestrator (with `CleanStale` fully removed and its seam/adapter deleted), and the daemon-owned throttled hooks cleanup on the tick's idle branch all match the specification precisely — including the load-bearing details (single latch read threaded downstream, latch set as the final pre-return action gated on no *fatal* error, `serverStarted=false`/no-`deferredBootstrapKey` on the abridged path, cleanup placed at the `!dirty && !gap` point after the `@portal-restoring` check, `lastCleanup` initialised to daemon-start). Tests are well-balanced — unit branch-selection, set-point gating, abridged self-heal, and real-tmux integration under `IsolateStateForTest` — and neither under- nor over-tested. The analysis cycles hardened the work further (capture/cleanup decoupling so a `loadHookStore` failure can't crash capture; `runHookStaleCleanup` simplified to its single post-removal shape; closed-vocabulary log-attr compliance). Remaining items are all non-blocking documentation-staleness edits and low-value test-scaffolding polish.

## QA Verification

### Specification Compliance

Implementation aligns with the specification across all sections. Verified spec-critical behaviours:

- **Latch semantics** — `state.BootstrappedLatchSatisfied(checker, runningVersion)` is a leaf-pure, parse-free string-equality helper reusing the `RestoringChecker` seam; every non-match outcome (absent, empty, mismatch, read-error/down-server) folds to *not satisfied → full bootstrap* (§ The Version-Stamped Latch).
- **Set-point & timing** — the latch write is `Run`'s final action after the last soft step and past all fatal early-returns, before the orchestration-complete summary; soft warnings still latch, fatal steps leave it unset (§ Latch Set-Point & Timing). Best-effort write, WARN-and-swallow, not routed to the warnings slice.
- **Entry-path branch** — single `TryGetServerOption` read → `latchSatisfied`, abridged gate upstream of `shouldRunConcurrentBootstrap` (which dropped its `ServerRunning()` probe and re-keys to `isTUIPath && client != nil && !latchSatisfied`), `openTUI` force-true comment reworded to "full bootstrap in progress" (§ Latch-Check Placement & Abridged-Path Wiring).
- **Abridged EnsureSaver** — liveness-only helper in package `cmd` (`SaverPanePIDOrAbsent` → `BootstrapPortalSaver` on absence → `SaverDownWarning` into the shared sink), never calls the version-gate (§ Abridged EnsureSaver — Liveness-Only).
- **Daemon-owned hooks cleanup** — throttled ~10s gate on the idle branch, reuses `runHookStaleCleanup` (mass-delete guard + `EmitCleanStaleSummary` breadcrumb), `lister`=Client, `store`=`loadHookStore()` once at startup, `lastCleanup` init to start time; capture startup decoupled from best-effort store resolution (§ Daemon-Owned Hooks Cleanup).
- **10-step retune** — `totalSteps=10`, `stepLabelTable` dropped key 11 (drop-key, not renumber), `totalBootstrapSteps=10` so the loading bar reaches exactly 1.0 (§ Affected Code Surface).

Deviations from literal task text are all deliberate, tested, cross-task evolutions (documented in the reports): the phase-7 latch-WARN correction (message text vs non-vocabulary `marker` attr), the `runHookStaleCleanup` `swallowListError` drop (task 5-1), and task 3-1's `loadHookStore`-error AC superseded by task 4-2's decoupling. None is a defect.

### Plan Completion

- [x] Phase 1–3 acceptance criteria met (latch + set-point, entry-branch + abridged path, daemon cleanup)
- [x] Phase 4–7 analysis-cycle chores completed (scaffolding consolidation, capture/cleanup decoupling, helper simplification, diagnosability parity, comment/vocabulary corrections)
- [x] All 18 tasks completed and independently verified
- [x] No scope creep — every change traces to a spec section or an analysis finding; the new `AC5` single-caller guard test (daemon is the sole `hooks.CleanStale` caller left) directly enforces a spec invariant

### Code Quality

No issues found. Go conventions are followed (leaf-package purity preserving the no-cycle constraint, closed log-attr vocabulary, DI via existing seams, single-responsibility helpers). The only quality notes are cosmetic (an unexported `lastCleanup` beside an exported sibling) and low-value test dedup — all non-blocking.

### Test Quality

Tests adequately verify requirements without over-testing. Branch selection, soft-vs-fatal set-point gating, abridged self-heal (crash-recovery regression guard), the throttled cadence gate (init-to-start + idle-only + skip-while-restoring), and real-tmux integration are all covered. One integration test justifiably hosts the daemon via the production `_portal-saver` pane rather than a direct `exec.Command` (Component D self-supervision would eject a directly-spawned daemon at ~3.6s, before the 10s cleanup interval could be observed) — a correct, documented mechanism choice. Minor test-polish items are listed below.

## Recommendations

### Do now

1. **Documentation staleness — post-10-step / signature-drop sweep** (all zero-risk doc edits)
   - `cmd/state_daemon.go:26-29` — struct header comment lists tick-mutable fields (`HashMap`, `PrevIndex`, `LastSaveAt`) but omits `lastCleanup`, which `maybeRunHookCleanup` rewrites each cadence; add it (Report 3-1)
   - `cmd/state_daemon.go:421` — `maybeRunHookCleanup` doc still says "here it is standalone"; task 3-3 shipped the gate at `:381`, so reword (e.g. "Placed on the tick's idle branch by task 3-3; independently unit-tested here.") (Report 3-2)
   - `cmd/bootstrap_production.go:5,7` — file-level doc block still lists "internal/hooks" in the wiring and dependency-free sentences; the file no longer references it (grep confirms comment-only), so drop "and internal/hooks" from both (Report 1-3)
   - `cmd/run_hook_stale_cleanup_test.go:30-34` — doc comment describes a `newTempHooksStoreForHelper` "re-declared here" that doesn't exist (real helper is `newTempHooksStore` in `cmd/bootstrap_production_test.go:131`); fix or drop the orphaned comment (Report 5-1)
   - `specification.md:251-253` — pinned dependency-wiring still documents the old 5-arg `runHookStaleCleanup` signature and its `swallowListError` bullet; update to the shipped 4-arg form (Report 5-1)
   - `CLAUDE.md` "Server bootstrap" section — still narrates an "eleven-step" orchestrator with step 11 = `CleanStale`; update to ten steps with `CleanStale` removed and re-homed on the `_portal-saver` daemon (Report 6-1, holistic observation)

### Quick-fixes

2. `cmd/abridged_integration_test.go` — test-scaffolding dedup (both low value; author's documented Rule-of-Three stance noted)
   - Extract a shared `waitForDaemonGone(client, oldPID, budget) bool` — the daemon-death poll loop is duplicated with `concurrent_coldboot_integration_test.go:166-178` (lines 135-145) (Report 2-4)
   - Narrow `setupAbridgedEnv`'s return signature — all six callers discard `ts` and `envSlice` (lines 64, 97) (Report 2-4)
3. `cmd/state_daemon.go:50` — rename `lastCleanup` → `LastCleanup` to match its exported loop-mutated sibling `LastSaveAt`; cosmetic on a package-private struct, touches refs at `:400/409/418/426/432/727/730` and `state_daemon_test.go:950/955` (Report 3-1)
4. `cmd/abridged_saver_test.go:217` — the revive-failure WARN assertion matches only the `error=` key; add the underlying error's rendered content to the substrings so it proves the WARN carries the underlying cause (validate the substring against real render output, as the value may be wrapped by `createPortalSaverWithRetry`) (Report 5-2)

### Ideas

5. `cmd/abridged_saver_test.go:185,213` — the shared `saverAbsentReviveFailsCommander()` fixture dropped the two saver tests' original `t.Fatalf` unexpected-op guard (fixture defaults to `("", nil)`), so a future unrecognised tmux op in `ensureSaverLiveness` would pass silently here; decide whether to add a `countOp`-based allowlist assertion on `cmder.Calls` or keep a stricter local commander (Report 4-1). Accepted consolidation tradeoff; low value.
6. `cmd/run_hook_stale_cleanup_test.go` — no dedicated subtest exercises the `store.CleanStale` error path (`:123 return err`), though the task's Tests list named a "CleanStale error → non-nil return" case; decide whether to add coverage (e.g. read-only dir post-Load) or confirm it's covered elsewhere (Report 5-1).
