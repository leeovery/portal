TASK: killed-sessions-resurrect-on-restart-2-5 — Unit test: runHydrate timeout fall-through preserves the 100 ms settle-sleep, marker-unset ordering, and FIFO-unlink tolerance

ACCEPTANCE CRITERIA:
- Unit tests pin the timing/ordering contract documented in spec § Fix 2 → Specific Changes #4 (100ms settle-sleep preserved on the timeout path) and the recovery contract (marker-unset before exec; FIFO-unlink tolerance).
- Edge cases: elapsed time on timeout handler stays well under hydrateSettleSleep (handler does not own the sleep); os.Remove(cfg.FIFO) tolerates missing FIFO silently; marker-unset call ordered before exec fall-through.

STATUS: Issues Found

SPEC CONTEXT: Spec § Fix 2 → Specific Changes #1 (handler unsets marker via unsetSkeletonMarkerOrLog) and #4 (100ms settle-sleep preserved before exec, "same posture as the success path"). Spec assigns sleep ownership to runHydrate, not the handler. FIFO-unlink tolerance follows from in-code rationale at cmd/state_hydrate.go:265-267.

IMPLEMENTATION:
- Status: Implemented (test-only task; production landed in 2-1/2-2/2-3)
- Location:
  - cmd/state_hydrate.go:98-190 (runHydrate timeout branch: settle-sleep at line 111 before execShellOrHookAndExit at line 115)
  - cmd/state_hydrate.go:260-277 (handleHydrateTimeout: preamble → os.Remove (ignored err) → warn log → unsetSkeletonMarkerOrLog → return nil; no time.Sleep in handler)
  - cmd/state_hydrate.go:322-326 (unsetSkeletonMarkerOrLog wraps state.UnsetSkeletonMarkerForFIFO)

TESTS:
- Status: Adequate (with one minor gap)
- Coverage:
  - Settle-sleep preserved on runHydrate timeout path: cmd/state_hydrate_test.go:1050-1067 (TestHydrate_Timeout_PreservesSettleSleepBeforeExec) asserts elapsed >= hydrateSettleSleep.
  - FIFO-unlink tolerance at runHydrate level: cmd/state_hydrate_test.go:1190-1201 (TestHydrate_TimeoutToleratesMissingFIFOSilently).
  - Marker-unset ordered before exec fall-through + FIFO-unlink tolerance at handler level: cmd/state_hydrate_test.go:1212-1253 (TestHydrate_TimeoutHandler_OrderingAndTimingInvariants).
  - Supplementary: TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU (line 1083) asserts argv emitted exactly once.
- Gap: Plan edge case "elapsed time on timeout handler stays well under hydrateSettleSleep (handler does not own the sleep)" is not pinned. handleHydrateFileMissing has the symmetric upper-bound assertion at cmd/state_hydrate_test.go:693-695. A regression that moves time.Sleep(hydrateSettleSleep) from runHydrate into the handler would keep all current tests green.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [bug] TestHydrate_TimeoutHandler_OrderingAndTimingInvariants (state_hydrate_test.go:1212-1253) does not pin "elapsed time on handler stays well under hydrateSettleSleep". The symmetric file-missing handler check at lines 693-695 demonstrates the pattern. Suggested addition: `start := time.Now()` / `elapsed := time.Since(start)` and assert `elapsed < hydrateSettleSleep`.
- [quickfix] state_hydrate_test.go:1099-1117 argv-equality loop could collapse to reflect.DeepEqual for consistency with lines 1244-1249.
- [idea] The runHydrate-level lower-bound timing test and handler-level direct test could be co-located as adjacent subtests in a TestHydrate_Timeout_SleepOwnership table.
