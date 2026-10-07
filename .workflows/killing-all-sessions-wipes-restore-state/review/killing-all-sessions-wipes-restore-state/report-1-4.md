TASK: Each Committer Reports A Stand-Down Through Its Existing Failure Route With A Back-Off Line (killing-all-sessions-wipes-restore-state-1-4, tick-88c076)

ACCEPTANCE CRITERIA:
1. The daemon's tick stands down, once because its session listing fails and once because its confirmation is refused. Each time, the log holds a line at INFO or above saying the save backed off because tmux stopped answering, with the cause in `error`, and `save.requested` is present afterwards.
2. `commit-now` stands down, once because its session listing fails and once because its confirmation is refused. Each time, the log holds a line at INFO or above saying the save backed off because tmux stopped answering, with the cause in `error`. `save.requested` is touched and the command exits non-zero.
3. The daemon's shutdown flush stands down, once because its session listing fails and once because its confirmation is refused. Each time, the log holds a line at INFO or above saying the save backed off because tmux stopped answering, with the cause in `error`, and the `shutdown` line reports `flush_completed=false`.
4. One tick stands down on a transient listing failure on a healthy server, and the next tick's reads are all answered. That next tick commits, so the save is delayed by one tick.
5. A live session is killed, and the `commit-now` its `session-closed` hook runs stands down. The daemon's next tick, with tmux answering, removes the killed session from `sessions.json` and deletes its scrollback files.
6. The restore-in-progress marker read fails in the tick, in the flush and in `commit-now`. Each logs its existing WARN line for that failure, unchanged, with the cause in `error`.
7. The restore-in-progress marker reads as set in the tick, in the flush and in `commit-now`. None of them logs a line it does not log today.
8. Every back-off line uses an existing log component and existing attribute keys only. No new log component or attribute key is introduced.

STATUS: complete

SPEC CONTEXT: §2.5 says a cycle that stands down (on a failed session listing, §2.1, or a refused confirmation, §2.2) writes no commit, runs no housekeeping pass, and ends as a failed cycle through each committer's existing route: the tick's WARN plus a re-touch of `save.requested`, `failCommitNow` (log, touch, non-zero exit), and the flush's failure line plus `flush_completed=false`. A transient failure therefore delays a save by one tick. §4.2 requires a default-level line saying the save backed off because tmux stopped answering, with the cause in `error`, using only existing components and attribute keys. The three restore-marker read-failure WARNs stay unchanged, and a marker that reads as set logs nothing new. §5.1 relies on the daemon's next tick to commit a kill whose `commit-now` stood down.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/scrollback.go:254 — `ErrTmuxStoppedAnswering` sentinel. Stand-down sites wrap it: internal/state/scrollback.go:367 (refused or not-own-server confirmation), internal/state/capture.go:102 (failed session listing; an unparseable listing is left for the confirmation to classify at capture.go:99-101, which a later task added), internal/state/capture.go:111 (pane listing), plus scrollback.go:359 and scrollback.go:384-386 (also added by later tasks).
  - cmd/state_daemon.go:244-249 — `cycleFailureMessage` renders `<stage> backed off: tmux stopped answering` when the error wraps the sentinel, and `<stage> failed` otherwise.
  - cmd/state_daemon.go:203-208 — the tick's WARN goes through `cycleFailureMessage("tick", err)` with `error`, followed by `state.TouchSaveRequested`. The single touch site is unchanged.
  - cmd/state_commit_now.go:143-149 — `failCommitNow` logs ERROR through `cycleFailureMessage(stage, cause)` with `error`, touches `save.requested`, and returns `errCommitNowFailed`. main.go:62-76 maps that to exit code 1 with stderr suppressed.
  - cmd/state_daemon.go:389-393 — the flush logs WARN through `cycleFailureMessage("final flush", ...)` with `error`, then the `shutdown` INFO line with `flush_completed` set to `flushErr == nil`, so a stand-down reports `false`.
  - The marker-read lines are untouched: cmd/state_daemon.go:180 (`read @portal-restoring failed`), cmd/state_daemon.go:380 (`read @portal-restoring at shutdown failed; skipping final flush`), and cmd/state_commit_now.go:101 (`isRestoring query failed; ...`). The marker-set paths are unchanged too (tick silent; flush DEBUG plus `shutdown`; commit-now's existing INFO skip line).
- Notes: Every back-off line goes out under the existing `daemon` component (`daemonLogger` / `deps.Logger`) and carries only the existing `error` key. The stage-keyed message keeps the single per-committer failure line rather than adding a second one, so no committer logs two lines for one failure. The CLAUDE.md wording ("its line reading `<stage> backed off: tmux stopped answering` where any other failure reads `<stage> failed`") matches the code. The code has moved past the original commit (cdd357797): tasks 1-9, 3-2 and 4-2 widened the sentinel to refused marker and pane reads, classified failed captures through the confirmation, and exempted the unparseable listing. These changes are consistent with the spec and do not weaken any criterion of this task.

TESTS:
- Status: Adequate
- Coverage:
  - Criteria 1-3: cmd/state_commit_backoff_test.go:143 (`TestDaemonTickReportsAStandDownAsABackOff`), :164 (`TestCommitNowReportsAStandDownAsABackOff`) and :191 (`TestShutdownFlushReportsAStandDownAsABackOff`) each run over `standDowns` (:35-73), which includes "a failed session listing" and "a refused confirmation". `assertBackOffLine` (:116-127) requires exactly one matching `daemon` line at INFO or above, with keys exactly `[component, error]` (criterion 8), and a cause that unwraps to the tmux `CommandError` carrying the injected stderr. Each test also asserts that the plain `<stage> failed` line is absent. The tick and commit-now tests assert `save.requested` is present. commit-now asserts `errCommitNowFailed`, the non-zero exit. The flush test asserts `flush_completed=false`. All three assert sessions.json and scrollback are byte-identical.
  - Criterion 4: cmd/state_commit_backoff_test.go:217 (`TestDaemonTickCommitsOnTheTickAfterAStandDown`). It heals the fake and sets `LastSaveAt = time.Now()` before the second tick, so the gap rule cannot be what runs that tick and the commit can only come from the re-touched `save.requested`. The test would fail if the touch were dropped.
  - Criterion 5: cmd/state_commit_backoff_test.go:242 (`TestDaemonTickCommitsAKillWhoseCommitNowStoodDown`). commit-now stands down on a failed listing, then a tick with `LastSaveAt = now` commits "work" alone, and `assertWorkAloneCommitted` (:267-280) checks that "notes" is gone from sessions.json and its transcript deleted. This also proves commit-now's touch is what drives the retry.
  - Criterion 6: cmd/state_commit_backoff_test.go:332 (`TestCommittersKeepTheirRestoreMarkerReadFailureLines`) checks the exact pre-existing message per committer at WARN, keys `[component, error]`, the cause, and that no back-off line appears.
  - Criterion 7: cmd/state_commit_backoff_test.go:359 (`TestCommittersLogNothingNewWhileARestoreIsInProgress`) pins the exact list of daemon-component messages per committer.
  - Classification at the state layer: internal/state/commit_cycle_confirm_test.go:394 (`TestRunCommitCycleClassifiesAStandDownAsTmuxStoppedAnswering`).
  - Non-stand-down failures still read `<stage> failed`: cmd/state_commit_backoff_test.go:394, plus cmd/state_daemon_lifecycle_log_test.go:175 and cmd/state_commit_cycle_lock_test.go:155.
- Notes: The table-driven stand-down set also carries the later tasks' refused marker, pane and environment reads. That shares one fixture rather than duplicating tests, so it is not over-testing. Every test reaches each committer through its real entry point (`tick`, `defaultShutdownFlush`, the `state commit-now` Cobra body) with only the tmux commander faked, so the tests observe the real behaviour.

CODE QUALITY:
- Project conventions: Followed (closed log vocabulary respected; seams staged through `withCommitNowDeps` / `withOwnTmuxServer`; no `t.Parallel()`)
- SOLID principles: Good (classification lives in `internal/state` via the sentinel; rendering lives in one `cmd` helper shared by all three committers)
- Complexity: Low
- Modern idioms: Yes (`errors.Is`, multi-`%w` wrapping, `errors.AsType` in tests)
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
