# Consolidation Findings: lazy-resume-on-attach (Phase 12)

## Findings

### F1: CLAUDE.md's `state` row still names `CaptureAndRefile` as the committers' entry point and never mentions the commit lock
- **Class**: drift
- **Failure**: Every agent session loads CLAUDE.md as the architecture reference. After this phase, the `state` row contradicts the code in two ways:
  - It says "the daemon, `state commit-now` … and the lazy-panel integration fixture all enter through" `CaptureAndRefile`. They now enter through `state.RunCommitCycle`, and `CaptureAndRefile`'s only production caller is `RunCommitCycle` itself.
  - It never mentions `RunCommitCycle`, `commit.lock` or `ErrCommitLockHeld`. The row does list the state directory's other lock, `AcquireDaemonLock` / `ErrDaemonLockHeld`.

  Task 12-2 edited the same row in the same phase and left this sentence as it stood. An agent adding a committer from this text would call `CaptureAndRefile` and then `Commit` outside the lock. That brings back the interleaving this phase closed: one committer's housekeeping pass deletes a waiting pane's token-named transcript that the other committer has just filed. The pane then restores empty at the next reboot, which is the only point where anyone would notice.
- **Evidence**:
  - `CLAUDE.md:61`: the sentence quoted under OLD below.
  - `internal/state/commit_cycle.go:14-28`: `commitLockName`, `ErrCommitLockHeld`, the 5s `commitLockTimeout`, `CommitLock`.
  - `internal/state/commit_cycle.go:45-72`: `RunCommitCycle`. It holds the lock from the skeleton-marker read to the end of the housekeeping pass, and calls `LoadPrev` only after the acquire.
  - `cmd/state_daemon.go:249-256`: the tick and shutdown flush.
  - `cmd/state_commit_now.go:114-122`: commit-now.
  - `internal/restore/lazy_resume_panel_integration_test.go:412` and `internal/restore/lazy_resume_renumbered_restore_integration_test.go:170`: the two fixtures.
- **Proposed shape**: Replace the closing sentence of the `CaptureAndRefile` passage in the `state` row. If F2 lands, the timeout clause stays true as written.
  - OLD: A failed marker read returns its wrapped error before any capture is taken, and a failed capture returns the capture's error untouched with nothing linked or re-filed — so no committing caller can leave out the skeleton set or commit an index that still names a waiting pane's vacated positional path; the daemon, `state commit-now` (which discards both sets and dumps nothing) and the lazy-panel integration fixture all enter through it, and `CaptureStructure` stays exported for the structure-only callers.
  - NEW: A failed marker read returns its wrapped error before any capture is taken, and a failed capture returns the capture's error untouched with nothing linked or re-filed — so no committing caller can leave out the skeleton set or commit an index that still names a waiting pane's vacated positional path. No committer calls it directly: every committing cycle runs through `RunCommitCycle` (`internal/state/commit_cycle.go`). It holds an exclusive `flock` on the state directory's `commit.lock` sidecar (`CommitLock`, which is not `daemon.lock` and is never unlinked) from the skeleton-marker read through `CaptureAndRefile`, the caller's `Dump` and `Commit`, to the end of the housekeeping pass. So no two committers' captures, re-files and collections interleave, and neither can delete a token-named transcript the other has just filed. The caller's `LoadPrev` is called only once the lock is held. The acquire is bounded at 5s, and a timeout returns an error wrapping `ErrCommitLockHeld` with nothing read or written; on that timeout the tick leaves `save.requested` set and `commit-now` touches it, so the daemon's next tick retries. The daemon's tick and shutdown flush dump, passing the daemon's in-memory previous index. `state commit-now` reads `sessions.json` under the lock, discards both sets and dumps nothing. Those callers and the lazy-panel integration fixtures all enter through `RunCommitCycle`. Calling `CaptureAndRefile` and `Commit` directly reopens the interleaving. `CaptureStructure` stays exported for the structure-only callers.
- **Bank**: Reviewer (task 12-1): CLAUDE.md's `state` row still says capture is entered through `CaptureAndRefile` and never mentions `RunCommitCycle` or `commit.lock`.

### F2: A commit-now that times out behind the daemon's own tick has its retry request erased by that tick
- **Class**: behaviour
- **Failure**: This phase added a bounded acquire. When `commit-now` cannot take the lock, its recovery is `failCommitNow`'s touch of `save.requested`, "the touch is what makes the daemon's next tick retry". That recovery never works when the holder is the daemon's own tick, and the tick is the only committer that holds the lock for seconds, because `commit-now` dumps nothing. The sequence:
  - A session is closed while a tick is dumping. Its capture was taken before the close, and its dump runs past the 5s bound.
  - `commit-now` times out with `ErrCommitLockHeld`, logs its ERROR, touches `save.requested` and exits non-zero.
  - The tick then commits a `sessions.json` that still names the closed session.
  - The tick then removes `save.requested` unconditionally. The touch always comes before that removal: `commit-now` times out while the tick still holds the lock, and the tick removes the flag only after it releases the lock. So the erasure is deterministic, not a narrow race.
  - Nothing commits the removal until the next dirty event or the 30s `MaxGap` tick.
  - A reboot or tmux server loss inside that window brings the killed session back at the next bootstrap, scrollback included. The user notices when a session they killed reappears. `portal.log` shows only a `commit-now` ERROR that nothing followed up.

  Frequency: in this machine's retained `portal.log` files, 388 of 94,721 `capture: tick complete` lines report `took` ≥ 5s, with a maximum of 18.2s.

  This is not a regression. Before the phase, the long tick's commit overwrote `commit-now`'s correct result. But the phase's timeout route is what was supposed to make this case recoverable, and it does not. The same unconditional removal also erases touches from `hook set` and `portal state notify` that land after the tick's capture. Their new pane token or structure then waits for the next gap tick.
- **Evidence**:
  - `cmd/state_daemon.go:183`: the dirty check.
  - `cmd/state_daemon.go:191`: the cycle, which may hold the lock for seconds.
  - `cmd/state_daemon.go:196-200`: the unconditional `os.Remove(state.SaveRequested(...))` after a successful cycle.
  - `cmd/state_daemon.go:184` and `:449`: the 30s `MaxGap`.
  - `cmd/state_commit_now.go:123-124`: the timeout routes into `failCommitNow`.
  - `cmd/state_commit_now.go:138-147`: the touch, and its comment "The touch is what makes the daemon's next tick retry", which is false in this case.
  - `internal/state/commit_cycle.go:22` and `:76-98`: the 5s bound and the acquire.
  - `cmd/state_commit_cycle_lock_test.go:182-219`: asserts the touch, but no test covers the touch surviving a concurrent tick.
- **Proposed shape**: In `tick`, consume the flag before the cycle instead of after it.
  - After the restoring check and the dirty/gap decision, remove `save.requested`, then run `captureAndCommit`.
  - On a non-nil error, re-touch the flag with `state.TouchSaveRequested` and WARN if that touch fails. This keeps the "tick failed leaves the flag for the next tick" contract, so `TestDaemonTick_StandsDownWhileAnotherCommitterHoldsTheCycle` stays green.
  - Drop the post-commit removal.

  Any touch that lands during the cycle then survives to the next tick, which commits a capture taken after it. A touch that lands between the removal and the capture only costs one redundant commit. A cancelled cycle, which returns nil without committing, consumes the flag harmlessly: the shutdown flush follows, and daemon start clears the flag anyway.

  An alternative is to remove the flag only if its mtime has not changed since the dirty check. That depends on timestamp granularity, and removing first does not.

  Regression test: a daemon tick whose fake commander touches `save.requested` from its capture-pane dispatch, standing in for a `commit-now` that timed out mid-dump. After the tick returns, `save.requested` still exists, and the next tick commits.
- **Bank**: Reviewer (task 12-1): the tick deletes `save.requested` after every successful cycle, erasing a touch that arrives during that cycle, including a timed-out `commit-now`'s retry request.
