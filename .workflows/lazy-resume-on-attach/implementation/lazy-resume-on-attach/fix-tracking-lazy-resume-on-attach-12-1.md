## Attempt 1

ISSUES:
- `internal/state/commit_cycle_test.go:236-283` (and the matching holds at `:300-303` and `:333-336`): the ordering tests prove only that commit-now makes no tmux read while the tick is held inside its dump. Nothing reliably checks that the lock is still held through `Commit` and its housekeeping pass. That later part is what prevents the loss: a hold that ended before `Commit` would let commit-now file `pane-<token>.bin`, and then the tick's older index would delete it in its housekeeping pass. I tested this on a scratch copy by adding `_ = lock.Close()` just before the `Commit` call in `RunCommitCycle`. `TestRunCommitCycleSerialisesOverlappingCommitters` then passed 4 of 5 runs. A refactor that shortens the hold, for example to "just the capture and dump", would bring back the silent transcript loss and still pass most runs. Nobody would notice until a waiting pane came back empty after a reboot.
  FIX: Add a test in `internal/state/commit_cycle_test.go` that checks the lock from inside the end of the housekeeping pass:
    - Create `scrollback` as a regular file in a fresh temp state dir. `gcOrphanScrollback`'s `ReadDir` then fails, and `Commit` logs its `gc orphan scrollback failed` WARN as the last thing it does in the pass.
    - Run `RunCommitCycle` with `Dump` returning `true`, so `Commit` writes and runs the pass.
    - Give it a `Logger` over a small `slog.Handler`. On that one message, the handler opens `state.CommitLock(dir)` on a fresh descriptor, tries `LOCK_EX|LOCK_NB`, and records whether it got `EWOULDBLOCK`.
    - Assert that the record was seen and that the lock was held.
    - Name the handler's reason in one line. It probes at the moment of emission, which a `logtest.Sink` cannot do; `orchestrationSeqHandler` is the precedent.
  I built this in the scratch copy: it failed every time with the hold ended before `Commit` and passed against the current code.
  ALTERNATIVE: In the first scenario, assert inside the commit-now's `LoadPrev` or first tmux read that the tick's commit is already on disk and `closed__0.0.bin` is gone. This is cheaper and reads like the criterion, but it is still left to chance: the gap between the lock release and the tick's `Commit` is microseconds, against a 5ms poll. The probe is the better choice.
  CONFIDENCE: medium

COMMENT_CORRECTIONS:
- `internal/state/commit_cycle.go:20-22` — the real `portal.log` disproves the claim about how long a tick takes. September's ticks on this 40-session / 41-pane install were typically ~0.6s, but about 7.5k took 1s or more, 382 took 5s or more, and the longest took 18s.
  OLD: // commitLockTimeout bounds the acquire. An unbounded acquire would park the
// daemon's tick loop behind a holder that is alive but stuck; the bound sits
// well above a whole tick's capture, dump and commit on a large server.
  NEW: // commitLockTimeout bounds the acquire. An unbounded acquire would park the
// daemon's tick loop behind a holder that is alive but stuck.
- `cmd/state_daemon.go:276-277` — the comment says every live pane's scrollback is written, but `run` skips skeleton and pending panes.
  OLD: // scrollbackDump is the daemon's part of a committing cycle: every live pane's
// scrollback written through the hash map, tallied for the cycle summary.
  NEW: // scrollbackDump is the daemon's part of a committing cycle, tallied for the
// cycle summary.
- `cmd/state_commit_now.go:32-33` — the doc on the `CommitNowDeps` type now only restates what `RunE` does (it passes no `Dump`), away from the call it describes.
  OLD: // The cycle commit-now runs carries no dump: commit-now writes no scrollback
// bytes.
  NEW: (empty — delete the comment)

NOTES:
- CLAUDE.md's `state` row is now misleading; the executor flagged this too. It still says "the daemon, `state commit-now` (which discards both sets and dumps nothing) and the lazy-panel integration fixture all enter through it" (`CaptureAndRefile`), and it never mentions `RunCommitCycle` or `commit.lock`. Someone adding a committer by following that text would call `CaptureAndRefile` plus `Commit` outside the lock and bring the loss back. Suggested replacement for that clause: "every committer — the daemon's tick and shutdown flush, `state commit-now` (which dumps nothing) and the lazy-panel integration fixtures — enters through `RunCommitCycle`, which runs it, the caller's dump and `Commit` under an exclusive flock on the `commit.lock` sidecar in the state directory (bounded acquire; `ErrCommitLockHeld` on timeout)". It is outside this diff, so it is not listed as a correction.
- The bound in practice, from the ticks measured above: a commit-now that arrives during a tick of 5s or more times out. It then touches `save.requested`, which is the designed route. commit-now also runs under a plain `run-shell` (no `-b`) in the session-closed hook, so while it waits on the lock it holds up tmux's global notification queue for up to the bound, delaying other hooks such as signal-hydrate on client-attached. That cost comes with the serialisation the task asks for, and I found no deadlock: the lock holder only issues client-queue tmux commands. No change asked.
- `scrollbackDump` keeps `ctx` in a struct, which the golang-context skill forbids. It is created per call and thrown away, so nothing breaks. `Dump: func(c state.CaptureCycle) (bool, error) { return dump.run(ctx, c) }` would follow the rule at no cost.
- The fourth criterion is checked against `commitNowCycle`, a copy of commit-now's `LoadPrev` written in the test, not the real closure at `cmd/state_commit_now.go:119-122`. If someone moved `loadPrevIndex` out of that closure, the tests would still pass. A previous index that lags behind is still safe, because the re-file and the link both adopt an existing token-named file, so I do not count this as an issue.
- `commitLockName` and `CommitLock` are in `commit_cycle.go`, not in `paths.go`'s block of state-directory file names next to `daemon.lock`.
- `failCommitNow`'s `stage` parameter now only ever gets "commit cycle". The ERROR text changed from `capture failed` / `commit sessions.json failed` to `commit cycle failed`; a grep found no test, doc or spec that pins the old wording.
- `cmd/bootstrap/daemon_tick_test_helpers_test.go:27-100` (`runDaemonTick`) still builds a tick by hand from `CaptureStructure` and `Commit`, outside the lock. It predates this task, the task does not name it, it runs single-threaded, and it deliberately differs from production (the `withoutSkipGuard` option).
- Verified: unit lane for `internal/state`, `cmd` and `internal/restore` green; new tests green under `-race`; the integration tests behind the two converted fixtures green; `go vet -tags integration`, gofmt and `golangci-lint` clean.
- The 1-minute load average was 104 during the review. Timing-sensitive results from this session, including the executor's `cmd/bootstrap` integration failures, should be judged with that in mind.
