TASK: A Capture That Fails Because Tmux Stopped Answering Logs As A Back-Off Whichever Read Failed (killing-all-sessions-wipes-restore-state-3-2, tick-b26f2c)

ACCEPTANCE CRITERIA:
- A saved sessions.json and its scrollback files are on disk; the session and pane listings name a live session; tmux then refuses every session's environment read ("server exited unexpectedly") and the confirmation sent after them — during the daemon's tick, its shutdown flush and commit-now. Each logs its back-off line at INFO or above, with tmux's own words in `error` and no `… failed` line beside it (`tick backed off: tmux stopped answering`, `final flush backed off: tmux stopped answering`, `commit cycle backed off: tmux stopped answering`). sessions.json and every scrollback file are unchanged.
- After that back-off, the tick leaves save.requested present and its next tick commits once every read is answered; commit-now touches save.requested and exits non-zero; the shutdown flush's `shutdown` line reports flush_completed=false.
- A committing cycle's capture fails on something not already classed as a refused read, and its confirmation is then refused, answered naming no server, or answered by another server: the cycle returns an error matching state.ErrTmuxStoppedAnswering with both the capture's error and the confirmation's cause reachable from it.
- With the committer's own server answering the confirmation, three ticks each log `tick failed` and no back-off line: an unparseable pane listing, a carry onto a captured session's name, and every session's environment read failing anomalously.
- A committing cycle whose own server is unknown, and whose capture fails, sends no confirmation read and returns the capture's error unchanged.
- When tmux refuses a capture's session listing or pane listing, the cycle ends at that read and sends no confirmation after it.

STATUS: complete

SPEC CONTEXT: §2.2 says a commit is written only after a read, sent after the last capture read, is answered by the committer's own tmux server. An answer from that server proves every earlier read was answered before tmux began exiting, and a refused or foreign answer confirms nothing. §2.5 says a stand-down writes no commit and goes through each committer's existing failure route (the tick re-touches save.requested, commit-now runs failCommitNow and exits non-zero, the flush logs flush_completed=false). §4.2 asks for a back-off line, not a `… failed` line, whenever a committer backs off because tmux stopped answering mid-save. §4.3 relies on those lines in a reboot log to show that a save was in flight as tmux went down. The gap this task closes: the per-session show-environment reads were not classified as refused reads. If tmux began exiting after the pane listing, the "all N sessions failed" aggregate returned before the confirmation was sent, so it logged as a genuine fault.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/scrollback.go:363-365 — a failed captureStructure now returns `classifyFailedCapture(c, ownServer, err)` instead of the bare error.
  - internal/state/scrollback.go:377-388 — `classifyFailedCapture` returns the error unchanged when it already matches ErrTmuxStoppedAnswering (the refused session/pane listings, internal/state/capture.go:102 and :111) or when ownServer is unknown (no read sent). Otherwise it sends the own-server confirmation through `confirmOwnServer` (:267-282). On refusal, a silent answer or another server's answer, it wraps as `fmt.Errorf("%w: %w: %w", ErrTmuxStoppedAnswering, captureErr, err)`, so the sentinel, the capture's error and the confirmation's cause all stay reachable. When ownServer answers, the capture's error is returned unchanged.
  - internal/state/scrollback.go:248-254 (ErrTmuxStoppedAnswering doc) and :344-355 (captureAndRefile doc) are widened as the task asked.
  - Unchanged as required: cycleFailureMessage (cmd/state_daemon.go:244-249), the tick route (cmd/state_daemon.go:203-208), the flush route (cmd/state_daemon.go:389-393), commit-now's failCommitNow (cmd/state_commit_now.go:144), and the already-wrapped listing errors (internal/state/capture.go:95-112).
- Notes: The classification needs no change in any committer and takes the narrow AnsweringConfirmer interface. RunCommitCycle (internal/state/commit_cycle.go:127-130) is the only caller of captureAndRefile, and it returns before Dump/Commit on any error, so writes, retries, save.requested and flush_completed are untouched; only the logged message moves. The explicit `ownServer <= 0` short-circuit is needed: confirmOwnServer would otherwise turn an unknown own server into an ErrNotOwnServer refusal and wrongly reclassify the failure, which the fifth criterion forbids. The code later grew an unparseable session-listing path (internal/state/capture.go:97-101). It returns that error unwrapped so this same classifier settles it, which is consistent with this task's principle. The behaviour matches §2.2/§2.5/§4.2.

TESTS:
- Status: Adequate
- Coverage:
  - Criteria 1 and 2: a new `standDowns` entry, "every environment read and the confirmation refused" (cmd/state_commit_backoff_test.go:58-66, fixture :91-100). It feeds the existing table-driven tests:
    - TestDaemonTickReportsAStandDownAsABackOff (:143): back-off line, no `tick failed`, save.requested present, state unchanged.
    - TestCommitNowReportsAStandDownAsABackOff (:164): errCommitNowFailed, back-off line, save.requested present.
    - TestShutdownFlushReportsAStandDownAsABackOff (:191): flush_completed=false.
    - TestDaemonTickCommitsOnTheTickAfterAStandDown (:217): the healed next tick commits.
    assertBackOffLine (:116-127) checks the line's keys and that tmux's "server exited unexpectedly" CommandError is reachable from `error`.
  - Criterion 3: TestRunCommitCycleClassifiesAFailedCaptureByItsConfirmation (internal/state/commit_cycle_confirm_test.go:474-521) covers the refused, silent and other-server confirmations. Each case asserts ErrTmuxStoppedAnswering, the capture's error (errEnvironmentRead), the confirmation's cause, that the confirmation was the last read, and that disk is unchanged.
  - Criterion 4: TestDaemonTickReportsAFailureThatIsNoRefusedReadAsFailed (cmd/state_commit_backoff_test.go:394-436) is left as it was. makeDeps wires OwnServer to fakeOwnServerPID (cmd/state_daemon_run_test.go:201), and the fake answers display-message with that pid, so the confirmation really is answered by the own server.
  - Criterion 5: TestRunCommitCycleWithNoOwnServerReturnsAFailedCaptureUnconfirmed (internal/state/commit_cycle_confirm_test.go:523-544) checks the error is unchanged, no confirmation read is sent and disk is unchanged.
  - Criterion 6: TestRunCommitCycleSendsNoConfirmationAfterARefusedListing (internal/state/commit_cycle_confirm_test.go:546-574) covers both listings.
  - Each test would fail if `classifyFailedCapture` were reverted to `return capture, err`: the cmd back-off tests would see `tick failed`, and the state test would see no ErrTmuxStoppedAnswering.
- Notes: Minor overlap: the pane-listing subcase of TestRunCommitCycleSendsNoConfirmationAfterARefusedListing uses the same server and the same last-read assertion as TestRunCommitCycleClassifiesARefusedCaptureReadAsTmuxStoppedAnswering's pane-listing case (:429/:452). The session-listing subcase is new coverage. Not raised as a finding: the duplication is one table row and causes no failure.

CODE QUALITY:
- Project conventions: Followed (narrow interface parameter, %w multi-wrapping, unexported helper doc in the package's existing style, no t.Parallel, tests through commandertest.FromFunc and the shared daemonFakeCommander)
- SOLID principles: Good — the classification is its own small function. Committers keep only their rendering of the outcome.
- Complexity: Low
- Modern idioms: Yes (multi-%w wrapping, errors.AsType in tests on Go 1.26)
- Readability: Good — the docs on ErrTmuxStoppedAnswering, captureAndRefile and classifyFailedCapture hold true against the code, including the unknown-own-server case.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
