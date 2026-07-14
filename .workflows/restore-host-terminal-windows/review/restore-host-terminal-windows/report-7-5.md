TASK: restore-host-terminal-windows-7-5 — Re-derive the marked set at burst decision time so a deferred N≥2 Enter cannot open a stale selection (tick-be06d5)

ACCEPTANCE CRITERIA:
- A mark toggle between a deferred Enter and terminalDetectedMsg is reflected in the spawned set: a session unmarked during the window is NOT opened; one newly marked IS opened.
- The already-resolved (non-deferred) path is behaviourally unchanged.
- pendingBurstOrdered is either removed or demonstrably no longer the source of the spawned set.

STATUS: Complete

SPEC CONTEXT:
This is a Phase 7 analysis-cycle correctness fix on the §6 picker spawn-burst. When an N≥2 Enter lands before async host-terminal detection (§6-1) resolves, beginBurst DEFERS (flags pendingBurstEnter) without engaging the burst input-lock (updateSessionList's `if m.burstPending` guard, model.go:3295) — so the picker stays live and an `m` toggle during the (tiny) defer window mutates selectedSessions. The prior implementation stashed an Enter-time snapshot (pendingBurstOrdered) and replayed it at terminalDetectedMsg, so a toggle in the window opened a stale set. The spec repeatedly warns against "correctness depends on caller/timing discipline"; the chosen fix makes the spawned set a pure function of the live selection at decision time.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:2481-2483 — terminalDetectedMsg arm now calls `m.decideBurst(m.orderedMarkedSessions())`, re-deriving live from selectedSessions instead of replaying a snapshot.
  - internal/tui/burst_progress.go:372-385 — beginBurst deferred branch sets only pendingBurstEnter (no snapshot), dispatches detection if not yet dispatched (maybeDispatchDetectionCmd), and branches immediately via decideBurst once resolved.
  - internal/tui/burst_progress.go:405-439 — decideBurst adds an empty-ordered guard (`if len(ordered) == 0 { return m, nil }`) preventing dispatchBurst from panicking on ordered[len-1] when the window empties the selection; comment documents §7-5.
  - internal/tui/model.go:3572 — direct N≥2 path (handleMultiSelectEnter → beginBurst(m.orderedMarkedSessions())) unchanged; resolved branch still routes decideBurst(ordered).
  - internal/tui/model.go:520-526 — pendingBurstEnter field doc updated; pendingBurstOrdered field fully removed.
- Notes:
  - AC1 satisfied by the model.go:2482 re-derive: a toggle in the defer window is now honoured.
  - AC2 satisfied: the synchronous resolved path is unchanged — live-at-Enter equals live-at-decision when there is no defer window, and decideBurst's body is otherwise untouched.
  - AC3 satisfied and exceeded: pendingBurstOrdered is REMOVED entirely (grep across internal/ and cmd/ returns zero references — struct, readers, and writers all gone), not merely bypassed.
  - Both entry points now converge on the identical derivation rule `decideBurst(m.orderedMarkedSessions())`, eliminating the dual source-of-truth. The empty-set guard is a sound defensive addition (all-unmarked window would otherwise hit dispatchBurst → SplitNetN on an empty slice).

TESTS:
- Status: Adequate
- Coverage:
  - internal/tui/burst_rederive_test.go:30 TestBurstDispatch_RederivesLiveMarkedSetOnDeferredResolve — the core AC1 test: detectDispatched=true (in-flight), mark alpha+bravo, Enter (asserts DEFERS: !BurstPending, zero adapter calls), then unmark alpha + mark charlie in the window, fire terminalDetectedMsg, and assert BurstTrigger=="charlie" and BurstExternal==["bravo"] (the live post-toggle set, not the stale [alpha]). Drains the batch and asserts the fake adapter opened exactly "bravo". This would fail against the pre-fix snapshot replay, so it genuinely guards the fix.
  - internal/tui/burst_rederive_test.go:94 TestBurstDispatch_AllUnmarkedDuringDefer_NoOp — pins the empty-set edge case (unmark everything in the window): asserts no dispatch, nil cmd, zero adapter calls, no panic. Exercises the decideBurst empty-ordered guard directly.
- Notes:
  - AC2 (already-resolved path unchanged) is delegated to the existing burst_dispatch_test.go suite (resolveDetection → Enter → dispatch), consistent with the tick's Regression note — no new test needed, and adding one here would duplicate that coverage.
  - Tests are white-box on the model's package-visible accessors (BurstPending/BurstTrigger/BurstExternal/SelectedSessionCount) and the fake adapter's recorded argv — they assert observable behaviour (which sessions open), not implementation internals. Setup reuses the shared wireBurstSeams/markRow/spawnedSession helpers, so no bespoke over-mocking.
  - Minor redundancy: lines 85-87 re-call spawnedSession on the same argv (adapter.Calls[0]) and assert got != "alpha"; line 82-84 already asserts got == "bravo", so on a single-element Calls the alpha check can never independently fire. It adds a distinct diagnostic message but is otherwise a tautological second assertion of the same fact.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel (per CLAUDE.md tui mock-injection rule); unit-lane test (no daemon/binary spawn, no integration tag needed); seams wired via package-level white-box fields consistent with the burst test surface.
- SOLID principles: Good. Single derivation rule (orderedMarkedSessions) now feeds both entry points; removing pendingBurstOrdered eliminates a redundant, drift-prone second representation of the marked set.
- Complexity: Low. Net change is a one-line re-derive at the resolution point, a field removal, and a guard clause.
- Modern idioms: Yes. Idiomatic Go; len-guard before slice indexing.
- Readability: Good. Comments at model.go:2473-2483, burst_progress.go:363-371 and 405-415, and the field doc at model.go:520-526 clearly explain the §7-5 rationale and the empty-set no-op.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/burst_rederive_test.go:85-87 — remove the tautological `spawnedSession(...) == "alpha"` re-check; the preceding assertion (line 82) that the opened session equals "bravo" already proves alpha did not open, so this second call to spawnedSession on the same argv asserts nothing new.
