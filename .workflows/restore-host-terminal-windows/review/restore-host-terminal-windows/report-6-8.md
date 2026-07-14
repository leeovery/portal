TASK: restore-host-terminal-windows-6-8 — Cancellation: Ctrl-C/Esc mid-burst

ACCEPTANCE CRITERIA:
- Ctrl-C/Esc mid-burst calls burstCancel (cancels the goroutine context) and does NOT tea.Quit — the picker returns to multi-select mode.
- Cancelling before the first spawn opens nothing and leaves every marked session marked.
- Cancelling after some windows opened leaves those windows in place (no teardown), unmarks the opened/confirmed sessions, keeps the rest marked (retry re-opens only the missing set).
- The batch markers are self-cleaned on the cancel path (the goroutine's cleanBatch runs on its terminal step).
- Ctrl-C is live even while the picker is input-locked (the burst-pending guard intercepts before the normal quit mapping).
- After a full-success self-exec there is nothing to cancel (burstPending is false; the model is quitting).

STATUS: Complete

SPEC CONTEXT:
Spec §Burst & Partial-Failure Contract → In-picker execution model (Cancellation post-state) and §Cancellation (spec lines 202, 208-210): "Ctrl-C/Esc mid-burst returns to the picker in multi-select mode (it does not quit Portal), aborts the remaining spawns, leaves any already-opened windows in place, and self-cleans the batch markers. Selection follows the same rule as a partial failure: the sessions whose windows opened are unmarked, the rest stay marked, so a retry re-opens only what's missing." Self-exec being the last step keeps cancellation clean — before it aborts the remaining spawns and leaves opened windows in place (nothing torn down); after it there is nothing to cancel. The task explicitly requires cancellation and partial-failure to converge on the ONE applyBurstSelectionMutation path (no second mutation rule).

IMPLEMENTATION:
- Status: Implemented (fully, matches spec + plan)
- Location:
  - internal/tui/burst_progress.go:310-334 — cancelBurst(): calls m.burstCancel() (cancels goroutine ctx), sets m.burstCancelled=true, returns m, m.burstPipe.receiver() (never tea.Quit). Defensive nil-pipe guard returns m, nil.
  - internal/tui/model.go:3295-3300 — the §6-5 burst-pending input-lock guard, first thing in the KeyPressMsg case (before flash-clear, the normal Ctrl-C→tea.Quit at :3332, SettingFilter, and the rune switch); routes keyIsCtrlC || keyIsCode(Esc) to cancelBurst while pending, swallows every other key.
  - internal/tui/model.go:2519-2528 — spawnCompleteMsg arm: full-success self-attach is gated on `!m.burstCancelled`, so a cancel (even one racing an all-confirmed terminal event) routes to handleBurstPartialFailure instead of tea.Quit.
  - internal/tui/burst_partial_failure.go:34-79 — handleBurstPartialFailure: the shared leave-what-opened path cancellation converges on; applyBurstSelectionMutation(confirmed) unmarks confirmed, keeps the rest; the failed-window flash is suppressed under `!m.burstCancelled` (silent cancel).
  - internal/tui/burst_progress.go:188-206 — burstRunner.run calls ackChannel.Clean(batch) on ALL terminal paths (incl. cancel), strictly before the terminal event → self-clean on cancel.
  - internal/tui/burst_progress.go:128-137 — the terminal-vs-progress send split (naked terminal send, ctx-guarded progress).
  - internal/tui/burst_progress.go:261-268 — resetBurstState clears burstCancelled on every terminal path.
  - internal/spawn/burst.go:157-159, 199-206 — the ctx.Err() between-windows + ack-poll checks the cancel rides on (Task 6.3 mechanism, verified present and working end-to-end).
- Notes: Every AC is satisfied. The full-success "nothing to cancel" AC (#6) is a structural property (burstPending is false after §6-4's tea.Quit; the guard only fires while pending), guaranteed by construction rather than a positive assertion — inherently untestable as a positive case, covered at the §6-4 level.
- Notable strength / justified deviation from the plan's literal wording: the plan Do-section assumed the ctx-guarded send (from 6-3) would suffice to drain the terminal event. Implementers discovered (in review) a real concurrency defect — the ctx-guarded terminal send raced ctx.Done on cancel and DROPPED the terminal event ~50% of cancels, permanently wedging the picker in the input-lock. The fix (split: NAKED unconditional terminal send at burst_progress.go:129-131, ctx-guarded progress send at :133-136) is correct, well-reasoned (bounded 64-buffer + actively-consuming re-issued receiver → the terminal send never blocks/leaks), and consistent with the spec intent (return to picker, always clear pending). This is an improvement over the plan, not drift.

TESTS:
- Status: Adequate (exceeds the plan's listed set with two high-value additions)
- Location: internal/tui/burst_cancel_test.go
- Coverage:
  - AC1 (Ctrl-C/Esc → cancel, not quit, multi-select): TestBurstCancel_CtrlCReturnsToMultiSelectNotQuit, TestBurstCancel_EscReturnsToMultiSelectNotQuit — assert *cancelled fired, not isQuitCmd, multiSelectMode, burstCancelled, burstPending stays true, receiver returned.
  - AC2 (cancel before first spawn → all stay marked, silent): TestBurstCancel_BeforeFirstSpawnKeepsAllMarkedSilent (empty Results terminal event; count unchanged, no flash, no self-attach, pending cleared, burstCancelled reset).
  - AC3 (cancel after some opened → unmark confirmed, keep rest): TestBurstCancel_AfterSomeOpenedUnmarksConfirmedKeepsRest (alpha AckConfirmed unmarked; bravo AckTimeout, charlie un-attempted, delta trigger all stay marked; silent).
  - AC4 (self-clean on cancel path): TestBurstCancel_SelfCleansBatchMarkersOnCancelPath — drives a REAL cancelled burst goroutine, asserts ack.Cleaned==1.
  - AC5 (Ctrl-C live while input-locked): TestBurstCancel_CtrlCLiveWhileInputLockedCancelsNotQuits — a second Enter is swallowed (nil cmd, no cancel) while Ctrl-C still cancels.
  - Extra (justified, non-redundant): TestBurstCancel_AllConfirmedRaceDoesNotSelfAttach (cancel racing an all-confirmed terminal must still leave-what-opened, no self-attach — directly guards the model.go:2519 `!m.burstCancelled` gate); TestBurstCancel_TerminalEventAlwaysDeliveredAfterCancel (the concurrency-regression tripwire for the naked-terminal-send fix, driven through a real goroutine, run-under-race worthy via driveCancelToTerminal's burstChannelClosedMsg guard).
  - The tests would fail if the feature broke: e.g. dropping the naked send trips driveCancelToTerminal's burstChannelClosedMsg fatal; removing the `!m.burstCancelled` gate trips the AllConfirmedRace / return-to-multi-select tests; a teardown call would be caught by the "rest stay marked" assertions.
- Notes: No under-testing — every AC has a direct assertion (bar the structural AC6, appropriately). Mix of deterministic crafted-message injection (newPendingBurstModel + injectComplete) and real-goroutine drive (realCancellableBurst) is well-chosen — fast determinism for the mutation/suppression contract, real send-path exercise under -race for the concurrency contract.
- Over-testing: none material. TestBurstPartialFailureFlash_DegenerateEmptyFailedNoFlash and TestBurstPartialFailure_DegenerateEmptyFailedRendersNoBand are §6-6 (non-cancel) degenerate-flash guards co-located here; the placement is defensible (cancel-before-first-spawn produces the same empty-Results/no-flash shape, so contrasting cancel-silent vs non-cancel-degenerate-also-silent belongs together) — not redundant.

CODE QUALITY:
- Project conventions: Followed. Value-receiver Update/handler pattern (m Model), nil-tolerant seams, test accessors mirroring convention, single-chokepoint reset (resetBurstState), convergence on one applyBurstSelectionMutation path (no duplicated mutation rule — exactly as the task mandated). Mirrors cmd/bootstrap_progress.go's channel+goroutine+receiver pattern per the codebase standard.
- SOLID principles: Good. cancelBurst is single-responsibility (cancel + flag + drain); the terminal-vs-progress send split cleanly separates must-deliver from droppable; partial-failure and cancel share one mutation path (DRY).
- Complexity: Low. cancelBurst is 9 lines; the guard is a 5-line early-return.
- Modern idioms: Yes (context.CancelFunc, ctx.Err() checks, slices.Clone in run).
- Readability: Good. The send() and cancelBurst() doc comments explain the load-bearing naked-vs-guarded distinction and why burstPending is deliberately kept true — the exact subtleties a future reader needs.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/burst_progress.go:330-332 — the `if m.burstPipe == nil { return m, nil }` branch in cancelBurst has no comment; unlike the rest of the function it silently returns no receiver, which would leave burstPending latched if ever reached. Add a one-line comment noting it is defensive/unreachable in production (dispatchBurst always pairs burstPending with a live pipe; only the input-inert WithInitialBurstOpening capture harness sets pending without a pipe) so a future reader doesn't mistake it for a wedge path.
