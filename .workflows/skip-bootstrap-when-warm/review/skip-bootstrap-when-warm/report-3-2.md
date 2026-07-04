TASK: 3-2 — Throttled hooks-cleanup gate calling runHookStaleCleanup (tick-763b10)

ACCEPTANCE CRITERIA:
- hookCleanupInterval == 10 * time.Second declared as a named constant.
- maybeRunHookCleanup(deps) returns without calling runHookStaleCleanup when time.Since(lastCleanup) < interval, leaving lastCleanup unchanged.
- When interval elapsed, calls runHookStaleCleanup with the pinned args exactly once and resets lastCleanup to ~now.
- Four call args pinned: lister=deps.Client, store=deps.HookStore, swallowListError=true, onRemoved=nil.
- A cleanup error is logged WARN under the daemon logger and swallowed — never returned, never panics/exits.
- No new audit event; the EmitCleanStaleSummary breadcrumb (via hooks.Store.CleanStale) remains the sole audit record.
- go build passes; go test ./cmd/... green; golangci-lint clean.

STATUS: Complete

SPEC CONTEXT:
Spec § Daemon-Owned Hooks Cleanup → Operational contract. Hooks cleanup is re-homed from the (removed) bootstrap step 11 onto the _portal-saver daemon. The daemon must (a) reuse runHookStaleCleanup verbatim (mass-deletion guard + EmitCleanStaleSummary breadcrumb, no new audit event), (b) throttle to ~10s via a cheap time.Since(lastCleanup) check so the 1s capture tick stays light, (c) use lister=Client, store=loadHookStore()-built *hooks.Store, swallowListError=true (portal-clean posture), onRemoved=nil, and (d) log-WARN-and-swallow all cleanup errors, never crash. This task delivers the standalone gate + unit tests; tick placement is task 3-3.

IMPLEMENTATION:
- Status: Implemented (with a benign downstream signature evolution — see note below)
- Location:
  - const hookCleanupInterval = 10 * time.Second — cmd/state_daemon.go:216 (documented per DO).
  - func maybeRunHookCleanup(deps *daemonDeps) — cmd/state_daemon.go:422-433. Throttle no-op (426-428), pinned call (429), WARN-swallow (429-431), reset-after-body (432).
  - daemonDeps.HookStore / lastCleanup fields — cmd/state_daemon.go:44,50; wired at startup HookStore via loadHookStore(), lastCleanup=daemon-start time — cmd/state_daemon.go:715-730.
  - Gate placed on tick idle branch (task 3-3) — cmd/state_daemon.go:381.
- Notes:
  - Signature evolution: the task pins a 5-arg call runHookStaleCleanup(Client, HookStore, Logger, true, nil). The current helper (cmd/run_hook_stale_cleanup.go:83-88) is 4-arg — the swallowListError bool was removed in a later refactor because "neither live caller wants a propagated ListAllPanes error"; the swallow-ListAllPanes-error behaviour is now hardcoded (helper always Warn-and-continues, returns nil). The gate correctly calls the 4-arg form (state_daemon.go:429). The "swallowListError=true" acceptance intent (log WARN + retry next cadence, never escalate on a list-panes failure) is fully satisfied structurally and is behaviourally verified by TestMaybeRunHookCleanup_ListPanesErrorSwallowedNoReap. Not a defect — a consistent cross-task evolution of the shared dependency.
  - The nil-HookStore early guard (state_daemon.go:423-425) is a documented task-4-2 addition (loadHookStore failure disables cleanup for the daemon's lifetime, capture undisturbed). Beyond 3-2's literal scope but correct and covered by TestMaybeRunHookCleanup_NilStoreNoOps.
  - reset-after-body is correctly placed unconditionally after the cleanup call (line 432), so a swallowed/failed cleanup still advances the throttle (retry next cadence, not every tick) — matches the AMBIGUITY NOTE resolution and stays consistent with 3-3.

TESTS:
- Status: Adequate
- Coverage (cmd/state_daemon_hook_cleanup_test.go, no t.Parallel, drives lastCleanup to controlled instants, no sleeps):
  - DoesNotRunBeforeInterval (36): asserts stale entry survives, no list-panes call recorded, lastCleanup unchanged — the below-interval no-op.
  - RunsAndResetsOnceIntervalElapsed (69): stale reaped, live retained, lastCleanup advanced.
  - FiresAtIntervalBoundary (100): lastCleanup = now - interval proves the >= boundary is inclusive.
  - LogsWarnAndSwallowsCleanupError (124): store points at a directory to force store.Load error; asserts no panic, gate WARN "hooks stale-cleanup failed" under daemon component, lastCleanup still advanced.
  - ListPanesErrorSwallowedNoReap (158): panesErr set; asserts no reap, NO gate WARN (swallowed inside helper), lastCleanup advanced — proves swallowListError=true posture and pinned lister.
  - NilStoreNoOps (200): nil store + elapsed interval; no list-panes work, lastCleanup untouched, no WARN.
  - ReusesMassDeletionGuard (231): elapsed + non-empty hooks + zero live panes; asserts nothing reaped and hazard WARN logged — proves the shared guard fires through the gate.
- Notes:
  - Every task-listed test case is present, plus the task-4-2 nil-store case. Tests exercise real behaviour (store contents, recorded tmux calls, log sink) not implementation details. No redundant assertions; the boundary test and the run/reset test overlap on "fires" but pin distinct properties (inclusive >= vs reap+reset).
  - onRemoved=nil is not asserted by an explicit "no per-removal output" check; it is a literal nil in the call site and is exercised without panic through the reap path (documented in the test comment at line 161-163). The daemon path has no stdout writer, so a stronger assertion is not meaningfully constructible at the gate level. Acceptable de-facto pin, not under-tested.

CODE QUALITY:
- Project conventions: Followed. Small helper, package-level constant beside selfSupervisionHysteresisTicks, DI via daemonDeps, tests avoid t.Parallel and reuse the existing daemonFakeCommander/newTempHooksStore/newCaptureLoggerForComponent helpers per the cmd-package mutable-state rule. Error handled with log-and-swallow mirroring the existing "tick failed" WARN.
- SOLID principles: Good. Single responsibility (throttle + delegate), reuses the shared cleanup body rather than duplicating the guard/audit logic.
- Complexity: Low. Two guard returns + one delegated call.
- Modern idioms: Yes. time.Since throttle, idiomatic error-swallow.
- Readability: Good. Thorough doc comment covering the reset-after-body rationale and nil-store semantics.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] cmd/state_daemon.go:421 — the maybeRunHookCleanup doc comment ends "Task 3-3 places this on the tick's idle branch; here it is standalone." Task 3-3 has shipped (the gate is wired at state_daemon.go:381), so "here it is standalone" is now stale. Reword to reflect that placement is complete (e.g. "Placed on the tick's idle branch by task 3-3; independently unit-tested here.").
