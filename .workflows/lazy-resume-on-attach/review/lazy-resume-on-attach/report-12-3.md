TASK: A Save Request Made During a Tick Survives That Tick (lazy-resume-on-attach-12-3, tick-5a1a81)

ACCEPTANCE CRITERIA:
- A dirty tick during whose cycle a save request lands — the fake commander touching `save.requested` as it serves a `capture-pane`, standing in for a `commit-now` that timed out mid-dump — commits, and after it returns `save.requested` still exists
- The tick after that one, with the 30s gap not elapsed, runs a fresh capture and commits it, and leaves `save.requested` absent
- A dirty tick whose cycle commits with no request landing during it leaves `save.requested` absent and advances the last-save time, as today
- A dirty tick whose cycle fails — a capture error, or another committer holding `commit.lock` past the bound — logs its `tick failed` WARN, leaves `save.requested` set and does not advance the last-save time; once the lock frees, the next tick commits and leaves the flag absent (`TestDaemonTick_StandsDownWhileAnotherCommitterHoldsTheCycle` and `TestDaemonTick_PreservesSaveRequestedOnError` stay green unchanged)
- A failed cycle whose re-touch of `save.requested` also fails logs one WARN for the failed touch under the `daemon` component beside the `tick failed` WARN, and the tick returns normally
- A tick whose cycle is cancelled mid-dump emits no WARN and leaves `save.requested` consumed rather than re-raised
- A tick suppressed by `@portal-restoring` leaves `save.requested` in place, as today

STATUS: complete

SPEC CONTEXT: An analysis-cycle task rather than a spec section. Phase 12's bounded commit lock gives a `commit-now` that times out one way to recover: it touches `save.requested` so the daemon's next tick retries. The only committer that holds the lock for seconds is the daemon's own tick, and that tick used to delete the flag after its commit, so it always erased the retry request. A session killed during a long tick then stayed in `sessions.json` until the next dirty event or the 30s MaxGap tick. That breaks the spec's wider rule that every committing capture reflects the live server promptly. CLAUDE.md's `state` row now states the new rule: the tick consumes `save.requested` before its cycle and re-touches it when the cycle fails.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_daemon.go:174-209 — `tick`
  - cmd/state_daemon.go:175-181 — restoring suppression, unchanged
  - cmd/state_daemon.go:183-189 — dirty/gap decision, unchanged
  - cmd/state_daemon.go:191-198 — the pre-cycle removal, keeping its not-exist-tolerant `remove save.requested failed` WARN
  - cmd/state_daemon.go:200-206 — the failure branch: `tick failed` WARN, re-touch through `state.TouchSaveRequested`, a `touch save.requested failed` WARN on `deps.Logger`, and a return that leaves `LastSaveAt` alone
  - cmd/state_daemon.go:208 — `LastSaveAt` advances on success
  - The post-commit removal is gone. `defaultShutdownFlush` (cmd/state_daemon.go:371-390) and the daemon-start clear (cmd/state_daemon.go:412-414) are unchanged.
  - cmd/state_commit_now_daemon_merge_integration_test.go:116-132 — `waitForForcedTickSettled`. It waits for the forced tick to consume the flag, touches the flag again, and waits for that second touch to be consumed. Ticks run one after another in the daemon goroutine, and the forced tick never removes the flag a second time, so a later tick has started and the forced one has returned. `:69-75` then fails the test if `daemon: tick failed` is in the log. A failed cycle re-touches the flag, so the second wait still completes, and the log check catches the failure. The test therefore reads `sessions.json` only after the forced tick has committed.
- Notes:
  - Traced every ordering case against the code:
    - A touch between the dirty check and the removal is erased, but the capture follows it.
    - A touch between the removal and the capture costs one redundant commit.
    - A touch during the cycle survives, because the removal happens only once, at the start.
    - A failed cycle restores the flag.
    - A cancel before or during the cycle returns nil through `captureAndCommit` (`:248-252`, `:265-267`), so nothing is re-raised and no WARN is logged.
  - The comment on `tick` (`:170-173`) and the new block comment (`:191-195`) both hold against the code.
  - `failCommitNow`'s claim that the touch makes the next tick retry (cmd/state_commit_now.go:138-147) is now true even while a tick holds the lock.
  - No other test relies on the flag disappearing as proof that a commit landed. I checked every `*_test.go` under cmd/, cmd/bootstrap/ and internal/ that references `save.requested`. The only waiter was the integration test, which was updated.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1 — `TestDaemonTick_KeepsASaveRequestThatLandsDuringTheCycle` (cmd/state_daemon_save_request_test.go:17). The fake's `dispatchHook` fires on `capture-pane`, empties the fake's session list and touches the flag. The test asserts that the first tick committed one session and that the flag still exists. It would fail against the old post-commit removal.
  - Criterion 2 — same test. `LastSaveAt` starts at now with the 30s MaxGap from `makeDeps`, so the next tick fires on the flag alone. The test asserts the `list-sessions` count grew, the index now holds zero sessions (a structural change, so `Commit` writes it — internal/state/commit.go:31), and the flag is absent.
  - Criterion 3 — `TestDaemonTick_RemovesSaveRequestedAfterSuccess` (cmd/state_daemon_run_test.go:361), extended to assert that `LastSaveAt` advances.
  - Criterion 4, lock held — `TestDaemonTick_StandsDownWhileAnotherCommitterHoldsTheCycle` (cmd/state_commit_cycle_lock_test.go:116), unchanged: the WARN carries `ErrCommitLockHeld`, the flag is kept, and after release the next tick commits and the flag is absent.
  - Criterion 4, capture error — `TestDaemonTick_PreservesSaveRequestedOnError` (cmd/state_daemon_run_test.go:381), unchanged. It now exercises the re-touch.
  - Criterion 4, remaining assertions — the capture-error `tick failed` WARN and the unchanged `LastSaveAt` are asserted in `TestDaemonTick_WarnsWhenReRaisingSaveRequestedAfterAFailedCycleFails`, which reaches the same return.
  - Criterion 5 — `TestDaemonTick_WarnsWhenReRaisingSaveRequestedAfterAFailedCycleFails` (cmd/state_daemon_save_request_test.go:71). A directory is created where `save.requested` belongs during `list-panes`, which runs after the pre-cycle removal, so `OpenFile(O_WRONLY)` fails. The test asserts exactly one `tick failed` WARN, exactly one `touch save.requested failed` WARN under `daemon`, two WARNs in total, and an unchanged `LastSaveAt`.
  - Criterion 6 — `TestDaemonTick_ConsumesSaveRequestedWhenTheCycleIsCancelled` (cmd/state_daemon_save_request_test.go:107). It uses two panes, so the cancel is seen at the second pane's check. It asserts that no `sessions.json` is written, `capture-pane` runs fewer than two times, there is no WARN, and the flag is absent. The first fix round's vacuous single-pane fixture has been replaced.
  - Criterion 7 — `TestDaemonTick_PreservesSaveRequestedWhenRestoring` (cmd/state_daemon_run_test.go:345), plus the unchanged early return.
  - The integration test's ordering change is covered by reading, as described under Implementation.
- Notes:
  - Not over-tested. The three new tests each pin a different branch: the surviving touch, the failed re-raise, and the cancelled consumption. The existing `captureAndCommit` cancel tests (cmd/state_daemon_run_test.go:1250, :1291, :1353) call `captureAndCommit` directly and never observe the flag, so they do not overlap.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel`. The capture logger is used through `newCaptureLoggerForComponent`. Queries go through the `logtest` `Records().Matching(...).AtExactLevel(...).Only(...)` chain. The integration test stays under `//go:build integration` and uses `portaltest.ReadPortalLogSafe`.
- SOLID principles: Good.
- Complexity: Low. The tick gained one pre-cycle statement and one nested error branch.
- Modern idioms: Yes.
- Readability: Good. The comments state why the flag is consumed before the cycle, and they carry no references to process artifacts.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
