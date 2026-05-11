# Implementation Review: Killed Sessions Resurrect on Restart

**Plan**: killed-sessions-resurrect-on-restart
**QA Verdict**: Comments Only

## Summary

The 38 implemented tasks across 10 phases cleanly land the three-pronged fix described in the specification: a new bootstrap `EagerSignalHydrate` step (Phase 1) that closes the per-session signaling gap; timeout-path corrections in `cmd/state_hydrate.go` (Phase 2) that unify the timeout and file-missing recovery paths through `execShellOrHookAndExit` with marker-unset; and the `sh -c` wrapper drop in `internal/restore/session.go` (Phase 3) that delivers AC5 (pane closes on first `exit`, no parked `sh` parent). Acceptance criteria AC1–AC5, AC7, and AC8 are pinned by dedicated automated tests (unit + integration); AC6 is observational-only per spec (Manual Verification Protocol). Seven analysis cycles (Phases 4–10) cleanly paid down architectural duplication and doc-staleness with strong scope discipline. No blocking issues were found across any task; one non-blocking `[bug]`-tagged finding (a missing handler-level upper-bound timing assertion on task 2-5) and one doc-staleness `[bug]` on task 5-4 (introduced by task 10-2) are flagged for follow-up. The cancelled task 3-4 (Manual Verification Protocol execution) is the only Definition-of-Done gate still open and is appropriately a PR-time activity rather than a code-level deliverable.

## QA Verification

### Specification Compliance

Implementation aligns with the specification. The three coordinated fix sites — Fix 1 (eager-signal step), Fix 2 (timeout-path corrections), Fix 3 (wrapper drop) — are all delivered. Several refactors collapsed the originally-planned shapes (the Phase-4-cycle-1 typed `state.FIFOSignaler` seam + no-seam helper supersedes the spec's two-method `EagerHydrateSignaler{ ListSkeletonMarkers; WriteFIFOSignal }` literal; production wiring moved from `internal/bootstrapadapter` to an inline literal at `cmd/bootstrap_production.go` mirroring `MarkerCleanupCore`). These deviations are documented in the analysis tracking files and are functionally equivalent to the spec's intent at a different module boundary.

### Plan Completion

- [x] Phase 1 acceptance criteria met (all 8 tasks complete; AC1 pinned by `TestPhase1Integration_EagerSignalHydrate_MultiSessionMarkersClearedWithin2s`, AC4 pinned by `TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4`, AC8 pinned by orchestrator-ordering test, CLAUDE.md updated).
- [x] Phase 2 acceptance criteria met (all 6 tasks; marker-unset, execShellOrHookAndExit routing, settle-sleep preserved, AC2 integration test landed).
- [x] Phase 3 acceptance criteria met for AC5 — pane closes on first `exit`, no parked `sh` wrapper (`TestNoParkedShWrapperPostRestore`); doc comment refreshed; manual verification (task 3-4) cancelled — DoD gate item 3 remains as a PR-time observational activity.
- [x] Phases 4–10 (analysis cycles): all 21 cleanup tasks complete; no scope creep observed.
- [x] No scope creep — every Phase 4–10 task traces back to a finding in the corresponding cycle's analysis files.

### Code Quality

No blocking issues. Strong adherence to project conventions across the board:
- Small DI seams via interfaces (typed `state.FIFOSignaler`, single-method `EagerHydrateSignaler`).
- `golang-pro` skill conventions followed (no `t.Parallel()` in cmd-package, `t.Helper()`/`t.Cleanup()` semantics preserved, error wrapping via `%w`, `errors.Is` for syscall sentinels).
- DRY wins consolidated via `internal/restoretest` (`SeedSessionsJSON*`, `WaitForFileExists`, `WaitForSkeletonMarkersCleared`, `OpenTestLogger`, `BuildPortalBinaryDir`) and `internal/statetest` (`RecordingFIFOSignaler`, `RecordingSleep`).
- Failure posture for the eager-signal step matches spec § Failure Posture exactly (soft WARN, never escalates).

### Test Quality

Tests adequately verify requirements. Coverage is balanced:
- Unit tests pin every plan-stated edge case for the marker-unset, hook-firing, settle-sleep, and bare-command-form contracts.
- Integration tests under `//go:build integration` validate against real tmux fixtures (AC1, AC2, AC4, AC5).
- Retry-ladder coverage anchored where the primitive lives (`internal/state/signal_hydrate_test.go`).
- Recording fakes centralised in `internal/statetest` with compile-time interface assertions.

One non-blocking test coverage gap: task 2-5's edge case "elapsed time on timeout handler stays well under hydrateSettleSleep (handler does not own the sleep)" is not pinned — a regression that moves the `time.Sleep` from `runHydrate` into `handleHydrateTimeout` would keep all current tests green. The symmetric file-missing handler check at `cmd/state_hydrate_test.go:693-695` demonstrates the pattern.

### Required Changes (if any)

None. No blocking issues identified.

## Recommendations

### Quick-fixes

1. `cmd/state_hydrate_test.go:1099-1117` (task 2-5): argv-equality loop reimplements inline deep-equality; sibling test uses `reflect.DeepEqual`. Could extract small `countArgvMatches` helper.
2. `internal/state/scrollback_tail.go` lines 73, 80 (surfaced during task 1-8 review): comment-leader typos use `/` instead of `//`. Unrelated to this task but worth folding into the next docs pass.
3. `cmd/bootstrap/orchestrator_builder_test.go:33-54` (task 4-3): `orchestratorOpts` comment still describes legacy "default-selection has one branch" logic; could trim to one-line pointer at `NewWithDefaults`.

### Ideas

4. `internal/state/signal_hydrate.go` (task 1-1): sibling symbols added alongside the strict relocation (`SendHydrateSignal`, `FIFOSignaler`, `DefaultFIFOSignaler`, `OpenFIFOForSignal`) widen the package's public surface beyond the literal "no public API surface added" wording. All justified by downstream tasks; a future plan-traceability reviewer may flag the wording mismatch.
5. `cmd/state_signal_hydrate.go:42-44` (task 1-2): no dedicated test for the list-markers (`show-options`) failure → soft-warn → return-nil arm. Coverage exists by analogy via `TestSignalHydrate_SoftFailsWhenSessionDoesNotExist`.
6. `cmd/state_signal_hydrate_test.go` (task 1-2): no multi-pane scenario asserting "pane A `SendSignal` fails, pane B still receives `SendSignal`". `statetest.RecordingFIFOSignaler.ErrOn` is purpose-built for path-keyed selective failure injection.
7. `cmd/bootstrap/eager_signal_hydrate.go` (task 1-3): the spec literal proposed a two-method `EagerHydrateSignaler { ListSkeletonMarkers; WriteFIFOSignal }` seam. Shipped shape is single-method (acknowledged outcome of task 4-1). No action needed; flagged for traceability.
8. `cmd/bootstrap_production.go` (task 1-3 / 1-5): production adapter wiring lives at `cmd/bootstrap_production.go` (inline literal) rather than as a named adapter type in `internal/bootstrapadapter`. Functionally equivalent; documented at `adapters.go:12-15`. A one-line addition noting `EagerSignalCore` is deliberately *not* in `internal/bootstrapadapter` would close a small documentation gap.
9. `cmd/bootstrap/eager_signal_hydrate_integration_test.go:240` (task 1-6): 2s timeout + 50ms tick hardcoded at call site. A named constant (e.g. `restoretest.AC1MarkerClearanceBudget`) would centralise the value.
10. `CLAUDE.md:78` (task 1-7): parenthetical bundles two distinct facts (production helper identity + `WriteFIFOSignal`-is-seam-only). Splitting could improve scanability.
11. `cmd/bootstrap/eager_signal_hydrate_integration_test.go` (task 1-8): plan's edge case "expose `state.RunCaptureOnce` as a test seam" was not taken; `runDaemonTick` mirrors production `captureAndCommit` byte-for-byte. Drift risk if production changes without mirroring the test helper.
12. `cmd/bootstrap/eager_signal_hydrate_integration_test.go` (task 1-8): consider adding an inline negative-control sub-test that wires `NoOpEagerHydrateSignaler{}` and asserts beta's scrollback file is absent.
13. `cmd/state_hydrate.go:260-277` (task 2-1): no explicit unit test forces the underlying `set-option -su` to return an error and asserts (i) WARN log line emitted, (ii) exec still proceeds on the timeout branch. Edge case (a) guaranteed architecturally but not pinned.
14. `cmd/state_hydrate_test.go:1517-1521, 1576-1580` (task 2-4): EISDIR-via-mkdir fixture recurs twice; could extract a `seedUnreadableHookStore(t, dir)` helper.
15. `cmd/state_hydrate_test.go:1504, 1565` (task 2-4): `TestHydrate_Timeout_LookupError_ExecsBareShellAndLogsWarning` and `TestHydrate_LookupErrorDegradesToBareShellAndLogsWarning` are near-identical except for the OpenFIFO seam; could be table-driven.
16. `cmd/state_hydrate_test.go` (task 2-5): runHydrate-level lower-bound timing test and handler-level direct test could be co-located as adjacent subtests in a `TestHydrate_Timeout_SleepOwnership` table.
17. `cmd/bootstrap/phase2_hook_fire_integration_test.go` (task 2-6): CI parallelism caveat documented at lines 49-56 — flake risk under default parallelism due to 500ms `WriteFIFOSignal` retry budget squeezed by concurrent `go build`. Mitigation documented but not enforced.
18. `cmd/bootstrap/phase2_hook_fire_integration_test.go` (task 2-6): test relies on `buildIntegrationOrchestrator`'s auto-default. Cross-reference comment to Phase 4 task 4-2 would aid future debugging.
19. `internal/restore/session.go:408-436` (task 3-2): doc comment conveys "absence of parked parent" only via past-tense rationale. A present-tense statement on the post-change invariant would more cleanly satisfy the AC bullet.
20. `internal/restore/exit_closes_pane_integration_test.go` (task 3-3): 2s `time.Sleep(exitClosesPaneBudget)` at line 229 is unconditional settle wait; could be poll-until-no-match loop. Deviation from plan's `pgrep -fa` to `-fl` is well-justified but a one-line note in planning doc would prevent future readers from flagging as drift.
21. `internal/state/signal_hydrate.go` (task 4-1): `FIFOSignaler` interface lives in `internal/state` rather than `cmd/bootstrap`. A one-line docstring note calling out the design decision would close planning-artifact-vs-implementation drift.
22. `cmd/bootstrap/eager_signal_hydrate.go` (task 4-1): `EagerSignalCore` has no defensive guard against nil `Markers` or nil `Signaler`. A one-line docstring note ("Markers and Signaler are mandatory; behaviour with either nil is undefined") would harden the contract.
23. `cmd/bootstrap/defaults.go` (task 4-3): hardcodes `state.DefaultFIFOSignaler{}` in the conditional real `EagerSignalCore` default. A future test wanting to inject a recording FIFOSignaler would need a new `With*` constructor.
24. `cmd/bootstrap/defaults.go:51-54` (task 4-3): ServerSeam union couples helper signature to `EagerSignalCore.Markers` shape; splitting into two positional args would scale better.
25. `cmd/state_hydrate_test.go` (task 4-5): optional `makeAndSignalFIFO(t, dir) string` companion not added; would extend the cleanup further.
26. `internal/restoretest/waitfor_file_exists.go` (task 4-6): deadline loop uses `time.Now().Before(deadline)` then `time.Sleep(tick)`; with very large tick relative to budget could overshoot by up to one tick. Fatalf diagnostic prefix is function name; including `t.Name()` would aid triage.
27. `internal/statetest/fifo_signaler_recorder.go`, `sleep_recorder.go` (task 5-1): both carry a duplicated "Concurrent invocation from multiple goroutines is NOT supported" paragraph. A short package-level doc-block would let each helper drop the paragraph.
28. `cmd/bootstrap/eager_signal_hydrate_integration_test.go:192-201` (task 5-2): slightly verbose explanatory block; AC4 sub-test at line 325 already cross-references back. Could tidy to single canonical comment.
29. `internal/bootstrapadapter/adapters.go` (task 6-2 / 9-1): after migrations land, `cmd/bootstrap_production.go` becomes the sole open-coded site. Worth re-reading constructor docstring. `orchestrator_builder_eager_default_test.go` still imports `internal/restore` for sentinels; future `bootstrapadapter.NewZeroRestoreAdapter()` could drop the last `restore` import.
30. `internal/restoretest/restoretest.go` (task 7-1): all seven call sites pass the same 50ms tick; if future cycles confirm consistency, helper could revert to `(t, client, timeout)` with documented 50ms default.
31. `internal/restore/session.go:423-430` (task 8-1): if a future change to `sanitizeSessionName` ever relaxes the filter, this docstring becomes the only signpost. Consider a test assertion in `panekey_test.go` pinning the filtered character set.
32. `internal/restore/*_test.go` (task 10-2): six other test files in `internal/restore` still carry inline `state.OpenLogger` preambles (integration_test.go, integration_full_test.go, session_test.go, session_markers_test.go, session_geometry_test.go, restore_test.go — 14 call sites total). Next obvious dedup batch since `OpenTestLogger` is already validated.

### Bugs

33. `cmd/state_hydrate_test.go:1212-1253` (task 2-5): `TestHydrate_TimeoutHandler_OrderingAndTimingInvariants` does not pin "elapsed time on handler stays well under hydrateSettleSleep" — a regression relocating `time.Sleep(hydrateSettleSleep)` from `runHydrate` into `handleHydrateTimeout` would keep all current tests green. The symmetric file-missing handler check at lines 693-695 demonstrates the pattern. Suggested addition: wrap the `handleHydrateTimeout` call with `start := time.Now()` / `elapsed := time.Since(start)` and assert `elapsed < hydrateSettleSleep`.
34. `internal/restoretest/doc.go` (task 5-4 / 10-2): `OpenTestLogger` (always-built, in `logger.go`) is not enumerated in the always-built block at `doc.go:9-14`. `logger.go` was added by task 10-2 AFTER 5-4 landed — doc-staleness regression introduced by 10-2. Fix: append `OpenTestLogger — *state.Logger opener that registers t.Cleanup, defined in logger.go.`
