# Implementation Review: Saver Kill-Respawn Loop Leaks Daemons

**Plan**: saver-kill-respawn-loop-leaks-daemons
**QA Verdict**: Approve

## Summary

All 23 plan tasks are implemented, tested, and aligned with the specification. Changes 1, 2 and 3 from the spec land cleanly: `EnsurePortalSaverVersion` gates the kill decision on `BootstrapAliveCheck` before the version predicate (with a co-located defensive `WriteVersionFile` on the alive+absent branch); `captureAndCommit` is ctx-aware at three observation points; and `state.WriteVersionFile` emits the contract-prefixed DEBUG breadcrumb at every call. Phases 3–6 add disciplined refactoring (predicate collapse, helper extraction, package-scope correction) with zero behavioural drift. Every verifier returned 0 blocking issues. Non-blocking notes are all minor — naming polish, doc-comment cross-references, optional-test gaps. Recommend approve.

## QA Verification

### Specification Compliance

Implementation aligns with the specification. All ten Acceptance Criteria are reachable via the landed code/tests:

- AC #1 (no-kill WARN absence), #2 (`_portal-saver` survives), #4 (defensive `daemon.version` repair) pinned by `TestEnsurePortalSaverVersion_AliveAndVersionAbsent_NoKill` (integration).
- AC #6 (real version-mismatch still kills) preserved by the unchanged `kill-respawn-under-explicit-version-mismatch` integration test plus the new ordering tests (`TestEnsurePortalSaverVersion_Alive_VersionsMismatch_Kills`).
- AC #7 (SIGHUP-to-exit bounded) pinned by `TestDaemon_MidTickSIGHUP_ExitsWithinBoundedWindow` with anchored-threshold measurement.
- AC #8 (no partial commits on cancellation) pinned by `TestCaptureAndCommit_*` cancel-at-three-points unit tests using `assertNoCommit`.
- AC #9 (DEBUG breadcrumb) pinned by `TestWriteVersionFile_EmitsBreadcrumb*` plus the bootstrap-wrapper breadcrumb regression test.
- AC #10 (prior bugfix regression) preserved — `daemon.lock` flock and `killSaverAndWaitForDaemon` polling loop untouched; verified by full kill-respawn integration suite still green.
- AC #3 (single daemon, no orphans) and #5 (~520ms reclaimed) are implicit / informational per the spec; logical reachability holds via PID-unchanged and WARN-absence assertions.

### Plan Completion

- [x] Phase 1 acceptance criteria met (alive-check gating + defensive write + breadcrumb)
- [x] Phase 2 acceptance criteria met (ctx threading + three observation points + cancel-at-each test + SIGHUP integration test + cascade fault-injection)
- [x] Phase 3 acceptance criteria met (predicate collapse, logger threading, four extraction tasks)
- [x] Phase 4 acceptance criteria met (stale-reference doc fixes, restoretest package split into portalbintest, install* seam helper collapse)
- [x] Phase 5 acceptance criteria met (version-scenario and barrier-count helpers extracted)
- [x] Phase 6 acceptance criteria met (five inline daemonRunFunc holders migrated to withImmediateRun)
- [x] All 23 plan tasks completed (manifest `completed_tasks` matches)
- [x] No scope creep — every change traces back to the spec or to an analysis-cycle finding

### Code Quality

No issues found. Project conventions (no `t.Parallel()` in cmd-package tests, seam-as-package-var idiom, generic helpers via Go 1.18+ type parameters, t.Helper() propagation, error wrapping via `fmt.Errorf` `%w`, sentinel comparison via `errors.Is`) are followed throughout. The decision-matrix godoc at `internal/tmux/portal_saver.go:282-319` is exemplary — it documents the post-fix contract as an ASCII table and explicitly names the prior bug.

Minor literal-vs-spirit deviations exist (e.g. `WriteVersionFile`'s signature now takes a `logger *Logger` parameter, `portalSaverVersionMismatch` was renamed/collapsed rather than preserved as the spec named it). All such deviations are correct — they match the codebase's actual idiom and were anticipated by later analysis cycles. The plan ACs would benefit from updating to acknowledge these post-cycle shifts.

### Test Quality

Tests adequately verify requirements. The test pyramid is well-shaped: predicate-level table tests at the leaf (`TestShouldKillSaverOnVersionDecision_PredicateMatrix`), ordering-and-side-effect tests at the decision layer (nine `TestEnsurePortalSaverVersion_*` cases), three ctx-cancellation unit tests (one per observation point) plus a happy-path regression, and three integration tests on real-tmux fixtures (alive+absent survival, mid-tick SIGHUP latency, lock-contention cascade fault injection).

Helper extractions (Phase 3 cycle) consolidate the test scaffolding without losing diagnostic context. `assertNoCommit` / `assertCommitReplacedPrev` / `assertKillBeforeNew` / `swapSeam[T]` / `recordBarrierCalls` / `PollUntil` / `StagePortalBinary` all carry contract docstrings and have meta-tests where the helper itself is non-trivial.

No over-testing flagged. Minor gaps captured as non-blocking ideas: optional `recycle-induced sweep pressure` test from Task 2-5 not implemented (flagged optional in the task brief); explicit pgrep count assertion missing from Task 1-6 (covered transitively by PID-unchanged + WARN-absence).

### Required Changes (if any)

None.

## Recommendations

### Quick-fixes

1. Refresh the file-header comment block at `internal/tmux/portal_saver_integration_test.go:3-47` to enumerate all three integration tests (currently only describes the singleton-invariant test).
2. Tighten `anchorThreshold` docstring at `cmd/state_daemon_integration_test.go` to read "minimum threshold 2s, scaling to 2 × singlePaneWallTime when measurement is large".

### Ideas

3. The original Task 1-1 AC references the predicate by its old name (`portalSaverVersionMismatch`) and pins absent→true. The implementation evolved (Task 3-1 collapsed the parallel predicates). Future plan documents could note when task ACs are superseded by later-phase refactors.
4. The `state.WriteVersionFile` signature gained a `logger *Logger` parameter, technically violating the "signature unchanged" AC bullet for Task 1-2. The implementation correctly chose the package's parameter-passing pattern over a global-logger seam. Worth a planning-side memo for future tasks: when "existing logger pattern" conflicts with "signature unchanged", which wins.
5. `versionWriterLogger` in `internal/tmux/portal_saver.go` defaults to nil and is installed via `SetVersionWriterLogger` from `bootstrapadapter`. If a future code path invokes `portalSaverWriteVersionFile` before the bootstrap adapter wiring runs, the breadcrumb is silently dropped on that branch. Currently safe (only producer is `EnsurePortalSaverVersion` post-wiring), but a comment near call sites describing the wiring-order invariant would help.
6. `EnsurePortalSaverVersion` uses a two-arm `switch { case alive && X: ... case alive && Y: ... }` with implicit fall-through to `BootstrapPortalSaver`. An `if/else if` chain would read more idiomatically since there's no scrutinee. Cosmetic only.
7. The `shouldKillSaverOnVersionDecision` doc comment cross-references `EnsurePortalSaverVersion` but lacks an explicit "see X for the full kill-decision matrix" forward-pointer. The predicate's call site reads `alive && shouldKillSaverOnVersionDecision(...)`; renaming to e.g. `shouldKillSaverOnVersionDecisionAliveDaemon` or moving the alive-gate inside the predicate would make accidental misuse harder.
8. Task 1-6's integration test could include an explicit `pgrep -f "portal state daemon"` count assertion to directly pin the spec's structural "exactly one daemon" wording rather than via transitive logic (PID-unchanged + DaemonAlive + WARN-absence).
9. `assertNoForbiddenLogSubstrings` reads `portal.log` only; a rotated log (`state.PortalLogOld`) could in principle hide forbidden substrings. Unlikely in this short-lived test, but worth a brief comment acknowledging the single-file design.
10. The optional "recycle-induced sweep pressure does not block cancellation" test from Task 2-5's Tests section is unimplemented. Adding it would explicitly pin spec §Defect 2 self-amplifying property by looping `os.WriteFile(state.SaveRequested(...))` during the tick window.
11. `tickStartDelay` (Task 2-5) is a static 1.2s sleep against the 1s ticker. On very slow CI cold-start, SIGHUP could land in the idle outer select rather than mid-tick. A deterministic gate (e.g. polling for the first scrollback file appearing) would be more robust.
12. On a capture-pane-fast host where aggregate < 2s, Task 2-5's test only logs a WARNING instead of `t.Skip` — could pass trivially without exercising the load-bearing scenario.
13. `killBarrierTimeoutCeiling` in Task 2-5 is mirrored locally as `5 * time.Second` rather than imported from `internal/tmux`. A future production drop in the value would silently desync the test's assertion. Exporting the constant and importing it would close the gap.
14. The cancel-mid-loop test (Task 2-4) uses `< 3` for the capture-call count (allows 1 or 2). In practice the dispatch-hook timing yields exactly 1 call. A tighter `== 1` assertion would more precisely pin the contract.
15. The Task 2-4 mid-loop test does not include an explicit filesystem assertion that per-pane scrollback files MAY remain on disk for completed iterations. The non-rollback invariant is captured in prose but not pinned by an assertion.
16. The Task 2-6 sentinel goroutine writes `sentinelErr`/`sentinelFile` without a mutex, relying on the `close(ready)` happens-before edge. Correct under Go's memory model, but a one-line comment noting "memory ordering established by close(ready)" would help future readers spot the synchronisation primitive.
17. Regression-watch suites listed in the Task 2-6 plan are not enumerated in a comment near the new test. Adding a brief reference comment naming the three packages (`multiple-state-daemons-running-concurrently`, `daemon-merge-reintroduces-dead-sessions`, `killed-sessions-resurrect-on-restart`) would aid discoverability.
18. The Task 2-6 `_cascade-holder` session uses `sleep 60` but the doc comment mentions `sleep infinity`. Switching to `sleep infinity` would be marginally more defensive against any hypothetical kill-server cleanup race.
19. `assertCommitReplacedPrev` (Task 3-5) accepts `stateDir` for signature parity with `assertNoCommit` and discards it (`_ = stateDir`). If no on-disk assertion lands in a subsequent cycle, drop the parameter — symmetry alone is weak justification.
20. `assertKillBeforeNew` (Task 3-6) combined error message surfaces `killIdx=-1` when kill is missing — slightly less descriptive than per-mode messages. The unified message keeps the helper compact and is acceptable; flagged for completeness.
21. `cmd/state_daemon_integration_test.go:182-185` repeats `exec.LookPath("portal")` + `t.Skipf` after `StagePortalBinary`. A `StagePortalBinaryWithPath(t) (binDir, absPath string)` variant would skip the redundant lookup.
22. The eight `install*` wrappers in Task 4-3 could be removed in a follow-up by inlining `swapSeam(t, tmux.<X>Seam(), v)` at call sites. Keeping the wrappers preserves call-site readability — current state is the better trade-off; noted for completeness.
23. The Task 4-2 `restoretest`/`portalbintest` split is accurate today but has no automated sync check — future helper additions/removals risk drift. Worth recording the convention that "test plumbing that is portal-binary-adjacent but not domain-bound" lives in `portalbintest`.
24. Two additional inline `daemonRunFunc` holders match the Task 6-1 shape but were excluded because they sit inside table-test `t.Run` loops: `cmd/state_test.go:179-181` and `cmd/version_guard_test.go:149-151`. Flag for a future cleanup pass.
25. The wrapper indirection (`portalSaverWriteVersionFile` var) for Task 3-2 is a seam-as-implementation idiom — defensible because Task 1-4 needed it for stub injection, but worth revisiting if the wrapper grows.
