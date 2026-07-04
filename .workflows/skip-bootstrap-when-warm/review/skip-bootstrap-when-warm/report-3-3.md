TASK: 3-3 — Place the cleanup gate on the tick idle branch (skip-bootstrap-when-warm-3-3)

ACCEPTANCE CRITERIA:
- tick idle branch (!dirty && !gap) calls maybeRunHookCleanup(deps) then returns; the bare idle return is gone.
- maybeRunHookCleanup is placed AFTER the @portal-restoring early-return and AT the !dirty && !gap point — NOT after the captureAndCommit branch.
- A @portal-restoring-set tick returns before the idle branch -> no cleanup runs.
- A capture-pending tick (dirty || gap) runs captureAndCommit and never reaches the idle branch -> no cleanup runs that tick.
- An idle tick (!dirty && !gap, restoring unset) runs maybeRunHookCleanup then returns.
- The daemon (maybeRunHookCleanup) plus portal clean are the only callers of hooks-CleanStale; no bootstrap step calls it (guard test present/green).
- go build passes; go test ./cmd/... ./cmd/bootstrap/... green; golangci-lint clean.

STATUS: Complete

SPEC CONTEXT:
Spec § Daemon-Owned Hooks Cleanup → Operational contract → "Placement in the tick (load-bearing)" (specification.md:258) and "Priority / non-interference" (:260) pin the exact placement: cleanup lives on the idle branch, evaluated AFTER the @portal-restoring check but AT the !dirty && !gap point, replacing the bare idle return. Placing it after the idle return would gate cleanup behind capture work and it would never fire on a mostly-idle warm server (the weeks-long scenario the feature targets). Net behaviour: restoring set -> whole tick skipped (no cleanup); capture pending (dirty||gap) -> capture runs, cleanup skipped that tick (scrollback always wins); idle + throttle elapsed -> cleanup runs. § Test Strategy → Branch selection (:297) and → Daemon cleanup (:300) require asserting the daemon is the ONLY remaining automatic hooks-CleanStale caller after Phase 1 removed the orchestrator step.

IMPLEMENTATION:
- Status: Implemented (exactly to task/spec; no drift)
- Location: cmd/state_daemon.go:378-383 (idle branch); cmd/state_daemon.go:353-367 (updated tick doc comment); maybeRunHookCleanup helper at cmd/state_daemon.go:422-433.
- Notes:
  - The change is precisely the single insertion required: `if !dirty && !gap { maybeRunHookCleanup(deps); return }` (lines 380-383). The @portal-restoring early-return (374-376), the dirty/gap computation (378-379), and the captureAndCommit branch (385) are untouched — matches the task's "ONLY change" constraint.
  - Ordering is correct: restoring check precedes the idle branch (a restoring tick returns at 375 before reaching 380), and the idle branch precedes captureAndCommit (a dirty/gap tick skips 380-383 and hits 385). This satisfies all three branch ACs structurally.
  - The tick doc comment (353-367) was updated with the required "Cleanup lives HERE" rationale block, faithfully reproducing the task's mandated wording.
  - maybeRunHookCleanup itself (task 3-2, in scope only as a consumer here) correctly no-ops on nil HookStore and below-throttle, and advances lastCleanup after the body — the daemon's single-threaded tick loop guarantees cleanup never races a pending capture (mutually-exclusive branches).

TESTS:
- Status: Adequate
- Coverage:
  - cmd/state_daemon_run_test.go:605 TestDaemonTick_RunsHookCleanupOnIdleTick — idle tick (!dirty && !gap, restoring unset, throttle elapsed): seeds stale + live hook entries, panesOut="live:0.0"; asserts stale reaped, live survives, AND list-sessions NOT called (proves cleanup fired on the idle fast-path with no capture). Verified genuine: ListAllPanes (tmux.go:799) parses panesOut into ["live:0.0"] as the live set, so the mass-deletion guard does not trip and the stale key is genuinely reaped — a broken placement would fail this.
  - cmd/state_daemon_run_test.go:649 TestDaemonTick_SkipsHookCleanupWhenRestoring — @portal-restoring set + throttle elapsed: asserts stale entry RETAINED and list-panes (cleanup's ListAllPanes) never invoked. Distinct, correct proxy for "whole tick skipped".
  - cmd/state_daemon_run_test.go:688 TestDaemonTick_SkipsHookCleanupOnDirtyCaptureTick — dirty tick: asserts list-sessions called (capture ran) AND stale entry RETAINED (idle branch not reached).
  - cmd/state_daemon_run_test.go:724 TestDaemonTick_SkipsHookCleanupOnMaxGapCaptureTick — gap tick: same assertions via the gap path.
  - cmd/hooks_cleanstale_single_caller_guard_test.go:30 TestHooksCleanStale_NoBootstrapStepIsAnAutomaticCaller — AC5 guard: scans cmd/bootstrap/*.go production source for `\bCleanStale\b|\brunHookStaleCleanup\b` and fails on any match; includes a scanned==0 vacuity guard. Word-boundary correctly excludes the legitimate CleanStaleMarkers (step 9) since "M" follows without a boundary.
- Notes:
  - The dirty and max-gap tests both assert "capture ran + stale retained"; not redundant — they exercise the two distinct branch conditions (dirty vs gap) that both route to captureAndCommit, and the task explicitly requested both. Retention (not a list-panes-absence assertion) is the correct proxy here because capture legitimately calls list-panes for pane enumeration.
  - No tick-level test for "idle but throttle NOT elapsed -> no cleanup" — correctly out of scope: the throttle gate is task 3-2's responsibility (covered in maybeRunHookCleanup's own unit tests) and re-asserting it here would be over-testing. Likewise the mass-deletion guard is covered by runHookStaleCleanup's own tests, so it is not re-exercised at the tick level (avoids over-testing).
  - Every test correctly omits t.Parallel() per the package's mutable-state constraint.

CODE QUALITY:
- Project conventions: Followed. Component-bound logging (deps.Logger), seam-based fakes (daemonFakeCommander), makeDeps DI helper, no t.Parallel(), reuse of the shared runHookStaleCleanup helper (no new import/cycle — same cmd package) — all consistent with the documented daemon/testing patterns.
- SOLID principles: Good. Single, minimal insertion on the correct branch; cleanup body delegated to the existing single-responsibility helper.
- Complexity: Low. tick gains one branch statement; no new nesting or paths beyond the required gate.
- Modern idioms: Yes. errors.Is on fs.ErrNotExist retained; idiomatic time.Since throttle in the helper.
- Readability: Good. The load-bearing placement rationale is captured in the tick doc comment and each test carries a spec-referencing header explaining why the branch matters.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (Note: go build / go test / golangci-lint could not be executed under the read-only review posture; the placement, branch ordering, doc comment, and all five tests were verified by reading, and the recent commit history records the implementation and five clean analysis cycles.)
