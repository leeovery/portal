# Analysis Tasks: Killing All Sessions Wipes Restore State (Cycle 3)

## Task 1: The Scrollback Writer Refuses A Pane The Cycle Skips
severity: low
sources: architecture

**Problem**: The `ScrollbackWriter` (`internal/state/commit_cycle.go:52-89`) is the only production route to a saved scrollback file, because the commit guard refuses `WriteScrollbackIfChanged` outside `internal/state`. It enforces the empty-capture rule itself, but `Write` accepts any pane key.

The cycle has a second rule for what a dump may write: skip every skeleton-marked (mid-restore), waiting or carried pane, as `CaptureCycle.SkipsScrollback` answers it (`internal/state/scrollback.go:331-338`). Nothing enforces that rule. It is left to each dump:
- the daemon's loop checks it (`cmd/state_daemon.go:315`);
- `CommitCycle.Dump`'s doc asks for it (`internal/state/commit_cycle.go:44-48`);
- the lazy-panel fixture's dump restates the check (`internal/restore/lazy_resume_panel_integration_test.go:508`).

A refactor of the daemon's loop, or a second dumper, could leave the check out. That dump then does damage in three ways:
- it writes a mid-restore pane's partial capture over the positional transcript that the pane's hydrate helper has not yet replayed;
- an empty capture there goes through too, because on a healthy server the committer's own server confirms it;
- it can write a live capture over a carried record's file.

The restored pane comes back without its history and nothing fails. The commit guard is satisfied because the write went through the sanctioned writer. The writer is built from the capture (`internal/state/commit_cycle.go:121-128`), and `heldTranscripts` already reads `capture.SkipsScrollback` while building it (`:172`). Even so, only the empty-capture rule moved inside the writer.

**Solution**: The writer carries the cycle's skip: the `CaptureCycle` it is built from. `Write` returns `(false, nil)` for any key that `SkipsScrollback` answers true for, before the confirmation, the dedup check or any write. For such a key no confirmation read is sent, and no dedup entry or file is touched.
- The daemon's dump keeps its own check, which now only saves the `capture-pane` read.
- `CommitCycle.Dump`'s doc says the writer refuses a skipped pane, instead of asking the dump to skip it.
- Behaviour on today's tree is unchanged, because the daemon never hands the writer a skipped key.

This extends the approved cycle 1 Task 1, under which the writer is the one production route to a scrollback file and carries the cycle's rules for what may be written there. This task adds the rule that guards a transcript nothing else holds.

The refusal returns `(false, nil)`, the same answer as a dedup hit, rather than an error. Leaving a skipped pane alone is the intended result. An error would log `write scrollback failed` and add to the cycle summary's anomalous tally for a pane that was rightly left alone.

**Outcome**: A dump that hands the writer a capture of a mid-restore, waiting or carried pane writes nothing and sends no confirmation, whatever the dump checks itself. The only production route to a saved scrollback file enforces both of the cycle's rules for what a dump may write.

**Acceptance Criteria**:
- [ ] A committing cycle's capture holds a pane that `SkipsScrollback` answers true for: one named by a skeleton marker, one carrying the resume pending marker, or one in a session carried forward from the previous index. The pane has a saved transcript. A dump hands the cycle's writer a capture for that pane. `Write` answers not-written with no error, no confirmation read is sent, the saved transcript keeps its bytes, and the cycle's dedup map holds the same entry for that key as before.
- [ ] The same holds when the capture handed over is empty and the committer's own server would confirm it.
- [ ] After such a cycle commits, `sessions.json` names for the skipped pane the same file as a cycle whose dump never handed it over, and that file is still on disk with its bytes.
- [ ] A pane the cycle does not skip is written exactly as before. The writer suites in `internal/state/empty_capture_test.go` and `internal/state/commit_cycle_answered_test.go` pass unchanged.
- [ ] The daemon's tick still sends no `capture-pane` for a skeleton-marked or waiting pane. The daemon suites pinning that skip (`TestDaemonTick_SkipsSkeletonMarkedPanesInScrollback` in `cmd/state_daemon_run_test.go`, `TestCaptureAndCommit_SkipsResumePendingPanes` in `cmd/state_daemon_resume_pending_test.go`) pass unchanged.

**Do**:
- `internal/state/commit_cycle.go`: `ScrollbackWriter` carries the `CaptureCycle` that `RunCommitCycle` builds it from (`:121-128`). `Write` (`:71-89`) refuses a key that `SkipsScrollback` answers true for as its first step, returning `(false, nil)`. That refusal comes ahead of the held-transcript lookup and its dedup delete, `confirmEmptyCapture` and `WriteScrollbackIfChanged`.
- `CommitCycle.Dump`'s doc (`:44-48`) no longer asks the dump to skip such a pane.
- The daemon dump's own check (`cmd/state_daemon.go:315`) stays as it is.
- The lazy-panel fixture's dump (`internal/restore/lazy_resume_panel_integration_test.go:502-526`) discards the writer and writes through `WriteScrollbackIfChanged` directly, which the commit guard allows in test files. This change does not reach it.

## Task 2: A Confirmation Naming No Server Is Refused In Its Own Right
severity: low
sources: architecture

**Problem**: `confirmOwnServer` (`internal/state/scrollback.go:266-276`) refuses tmux's shutdown answer to the confirmation (exit 0, no output) only by comparing it with the committer's own server. `ConfirmAnswering` returns pid 0 for that answer (`internal/tmux/tmux.go:92-105`). But a committer that does not know its own server also holds 0: `ownTmuxServer` returns 0 when `TMUX` is absent or unparseable (`cmd/state_commit_now.go:136-139`). For that committer, the early `ownServer <= 0` return is the only thing between the shutdown answer and a confirmation.

Next to `answered != ownServer` the guard looks redundant, and removing it leaves every test green. `TestRunCommitCycleStandsDownWithNoOwnServer` (`internal/state/commit_cycle_confirm_test.go:320`) and `TestCommitNowStandsDownOutsideAnyTmuxServer` (`cmd/state_commit_own_server_test.go:85`) both use fakes that answer pid 4242, which the mismatch refuses either way.

Without the guard, such a committer reads the shutdown answer as its own server confirming. The confirmation has three uses: after the capture, in classifying a failed capture, and before an empty scrollback write. Through any of them, the committer commits a capture taken during tmux's exit, which is the empty index and housekeeping wipe this fix exists to stop. Nothing shows it until the next restore brings nothing back.

§2.2's rule, that an answer counts only when it names the server that answered it, rests on a guard written for a different case.

**Solution**: `confirmOwnServer` refuses an answer naming no server (`answered <= 0`) with `ErrNotOwnServer`, before the comparison. The comparison then only ever sees a real pid.
- The unknown-own-server guard stays as a second check rather than the only one. `classifyFailedCapture`'s guard also stays, so a committer that does not know its own server still sends no confirmation (cycle 1 Task 2).
- `ConfirmAnswering` and the `AnsweringConfirmer` contract are unchanged: 0 for an answer naming none.
- The no-own-server stand-down is pinned against a confirmation that answers naming no server.

This is derived from the record:
- §2.2: the confirmation "counts as answered only when tmux returns exit status 0 with an answer that names the server that answered it";
- `ErrNotOwnServer`'s doc already lists an answer naming none among its refusals.

The finding's alternative was to have `ConfirmAnswering` return that answer as an error. It is not taken, for two reasons:
- CLAUDE.md documents the 0-for-no-server contract;
- the suites classify a silent answer as `ErrNotOwnServer`, separately from a refused read (`internal/state/commit_cycle_confirm_test.go:488` and `:617`). An error return would merge the two.

**Outcome**: The confirmation itself refuses an answer naming no server at all three of its uses, whatever the committer knows of its own server. The unknown-own-server guard no longer holds §2.2's rule alone. A test fails if both checks are ever removed.

**Acceptance Criteria**:
- [ ] A committing cycle that does not know its own server runs against a tmux that answers its confirmation with exit status 0 and no output. It stands down with an error wrapping `ErrNotOwnServer`, and `sessions.json` and every scrollback file are unchanged.
- [ ] `portal state commit-now` runs with `TMUX` empty against a tmux that answers its confirmation naming no server. It fails with `errCommitNowFailed`, and the saved state is unchanged.
- [ ] Each of those two scenarios still stands down with either check alone in place: the unknown-own-server guard, or the refusal of an answer naming no server. Its test fails only if both are removed.
- [ ] A committing cycle that knows its own server gets an answer naming no server, and refuses it with `ErrNotOwnServer` at each of the confirmation's three uses:
  - after the capture, where the cycle stands down;
  - after a failed capture, where the cycle stands down with the capture's error still reachable;
  - before an empty capture replaces a saved transcript, where the write is refused with `ErrUnconfirmedEmptyCapture` and the transcript is kept.
- [ ] A committer that does not know its own server and whose capture fails sends no confirmation read and returns the capture's error unchanged.
- [ ] `ConfirmAnswering` still answers pid 0 with no error for an answer carrying no output. The suites still classify a silent answer as `ErrNotOwnServer`, separately from a refused read (`internal/state/commit_cycle_confirm_test.go:488` and `:617`).

**Do**:
- `confirmOwnServer` (`internal/state/scrollback.go:266-278`) refuses `answered <= 0` with an error wrapping `ErrNotOwnServer`, ahead of the `answered != ownServer` comparison. Its `ownServer <= 0` early return stays.
- `classifyFailedCapture`'s `ownServer <= 0` guard (`internal/state/scrollback.go:377`) stays.
- `ConfirmAnswering` (`internal/tmux/tmux.go:87-105`) and the `AnsweringConfirmer` contract (`internal/state/scrollback.go:241-246`) are unchanged.
- The two no-own-server stand-down tests answer the confirmation naming no server:
  - `TestRunCommitCycleStandsDownWithNoOwnServer` (`internal/state/commit_cycle_confirm_test.go:320`). Its `onlyWorkLive()` client answers pid 4242 today; `exitingServer.shutdownAnswers` answers a read with exit 0 and no output.
  - `TestCommitNowStandsDownOutsideAnyTmuxServer` (`cmd/state_commit_own_server_test.go:85`). Its `workOnlyCommander(nil)` answers `fakeOwnServerPID` today; `daemonFakeCommander.silentConfirm` answers the confirmation with exit 0 and no output.
