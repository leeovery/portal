# Analysis Tasks: killing-all-sessions-wipes-restore-state (Cycle 6)

## Task 1: Every Committer Logs An Unreadable sessions.json From The Cycle That Read It
severity: low
sources: architecture

**Problem**: `readPriorIndex` (`internal/state/commit.go:62-74`) turns every read or decode failure of `sessions.json` into nil. `RunCommitCycle` (`internal/state/commit_cycle.go:120-124`) then calls the caller's `LoadPrev` after the cause has already been dropped.

Suppose the file stops reading or decoding while the daemon runs, most likely because a hand edit broke the JSON. The daemon's next tick falls back to its in-memory index, and `commitOver`, handed a nil prior, overwrites the file unconditionally (`cmd/state_daemon.go:271`). That commit writes no drop lines, and portal.log holds only `capture: tick complete`. Nothing says the saved index was unreadable and has been replaced.

`commit-now` in the same state does log a WARN naming the cause. It gets that cause from a second read of the same file under the same lock, through the `CommitNowDeps.ReadIndex` seam (`cmd/state_commit_now.go:34`, `:157-169`), and it throws away the index that read returns. So whether a committer reports the condition depends on what its fallback callback happens to do. The daemon commits every tick, so it is the committer most likely to hit this, and it is the silent one.

**Solution**: `readPriorIndex` reports why it read nothing, telling an absent file apart from a read or decode failure and carrying that failure's cause. `RunCommitCycle` logs one WARN through the cycle's logger before it calls `LoadPrev`. The daemon and `commit-now` both log through the `daemon` component, so they report the condition in the same words under the same component.

The first half of each existing `commit-now` line stays as it reads: `sessions.json absent`, and `read sessions.json failed` with the cause in `error`. The `proceeding with zero-value PrevIndex` tail becomes `committing without the saved index`. The shared line must be true for the daemon too, and the daemon falls back to its in-memory index, not to a zero one. Both lines stay at WARN, so a fresh install's first daemon commit logs one `sessions.json absent` line.

`commit-now`'s lines fire in exactly the cycles they fire today, which keeps cycle 4 Task 2's settled direction. The change completes phase 6 Task 1, which reduced `loadPrevIndex` to a fallback, by moving the diagnosis to the read that found the problem.

After the change, `LoadPrev` only supplies the fallback. `commit-now`'s returns an empty index without reading anything. The following are removed: `loadPrevIndex`, the `CommitNowDeps.ReadIndex` seam with its fill line in `resolveCommitNowDeps`, and its seam-convention cases (`cmd/deps_merge_convention_test.go:117-118`, `:275-284`). `Commit` (`internal/state/commit.go:29`, exported for test fixtures only) keeps its current behaviour. The expected text in `TestStateCommitNow_WarnsOnlyWhenSessionsJSONCannotBeRead` changes with the line.

**Outcome**: A committing cycle that finds `sessions.json` absent or unreadable says so itself, once, before it falls back. Whichever committer ran the cycle (the daemon's tick, its shutdown flush, or `commit-now`), portal.log gets the same WARN under `daemon`. A daemon tick that replaces a hand-broken index now leaves a line naming the cause beside its `capture: tick complete`. `commit-now` reports the condition without a second read of the file, and without a seam whose only effect was that log line.

**Acceptance Criteria**:
- [ ] A hand edit leaves `sessions.json` undecodable while the daemon runs. The daemon's next tick logs `read sessions.json failed; committing without the saved index` once at WARN under component `daemon`, with the decode failure in `error`. It then commits from its in-memory index and replaces the file. (§4.2)
- [ ] With no `sessions.json` in the state directory, as on a fresh install, the daemon's tick logs `sessions.json absent; committing without the saved index` once at WARN under component `daemon`, and commits
- [ ] A `sessions.json` that is present but cannot be read gets the `read sessions.json failed; …` line with the read's error in `error`, never the absent line
- [ ] `portal state commit-now` logs those lines, in the same words under the same component, in exactly the cycles it warns in today: the absent line once over a missing file, the failure line once (cause in `error`) over an undecodable one, and neither over a readable one. Over a missing or undecodable file it commits from an empty previous index without reading `sessions.json` a second time
- [ ] Each cycle that finds the file absent or unreadable logs exactly one of the two lines before its capture, so a cycle that then stands down or fails before its commit has still logged it. A cycle over a readable `sessions.json`, from either committer, logs neither line and calls no `LoadPrev`
- [ ] `state.Commit` over an absent or undecodable `sessions.json` logs neither line and writes exactly as it does today
- [ ] `CommitNowDeps` has no `ReadIndex` seam and `loadPrevIndex` no longer exists. The seam-convention suite's every-field coverage guard passes over the fields that remain

**Do**:
- `internal/state/commit.go`, `readPriorIndex` (`:62-74`): it reports why it read nothing, telling an absent file apart from a read or decode failure and carrying the failure's cause. It fails at `os.ReadFile` (`:64`) and at `DecodeIndex` (`:68`). `state.ReadIndex` (`internal/state/index_reader.go:17-32`) fails at the same two calls, reporting `fs.ErrNotExist` as absent and anything else as a failure, so `commit-now`'s lines keep firing in the same cycles. `Commit` (`:28-30`) keeps its behaviour and logs neither line.
- `internal/state/commit_cycle.go`, `RunCommitCycle` (`:120-124`): when the read under the lock returns nothing, the cycle logs the one WARN through `cycle.Logger` before it calls `cycle.LoadPrev()`. The messages, verbatim, are `sessions.json absent; committing without the saved index` and `read sessions.json failed; committing without the saved index`. The second carries its cause in `error`, a key the closed log vocabulary already holds.
- `cmd/state_commit_now.go`: commit-now's `LoadPrev` (`:117-120`) returns an empty index without reading anything. Remove `loadPrevIndex` (`:157-169`), the `CommitNowDeps.ReadIndex` field (`:34`) and its fill line in `resolveCommitNowDeps` (`:58-60`).
- The seam's whole footprint is `rg -n 'deps\.ReadIndex|\{ReadIndex:|ReadIndex +func|field: +"ReadIndex"|\(\)\.ReadIndex|readIdx|loadPrevIndex' cmd`, which finds 18 lines across 3 files. All of it goes:
  - `cmd/state_commit_now.go`: 5 lines.
  - `cmd/deps_merge_convention_test.go`: 4 lines, in the fall-through check (`:117-118`) and the seam case (`:274-287`).
  - `cmd/state_commit_now_test.go`: 9 lines. They are the `commitNowFixture` fields (`:68-71`), the override in `installCommitNowDeps` (`:131-135`) and the override's one user, `TestStateCommitNow_LeavesSessionsJSONByteIdenticalWhenCommitFailsBeforeRename` (`:838-841`), whose comment explains an override that no longer exists.
- Both committers already hand the cycle `daemonLogger`: the daemon through `deps.Logger` (`cmd/state_daemon.go:415`, `:451`, passed to the cycle at `:274`), and `commit-now` at `cmd/state_commit_now.go:95` and `:121`. So the shared line lands under `daemon` for both with no change to either.
- Existing tests that encode the old behaviour:
  - `TestStateCommitNow_WarnsOnlyWhenSessionsJSONCannotBeRead` (`cmd/state_commit_prev_lag_test.go:142-194`) already runs the real cycle. Its expected messages (`:144-145`) change with the line.
  - `TestStateCommitNow_FallsBackToZeroPrevAndLogsWarnWhenSessionsJSONMissing` and `…OnCorruptSessionsJSON` (`cmd/state_commit_now_test.go:326-405`) assert a `daemon` WARN through the `installCommitNowDeps` stand-in (`:95-117`). That stand-in replaces `RunCommitCycle`, so once the line moves into the cycle it no longer produces it. Their zero-index assertions (`:348`, `:394`) still hold.
  - `TestDaemonTick_WarnsWhenReRaisingSaveRequestedAfterAFailedCycleFails` (`cmd/state_daemon_save_request_test.go:71-105`) wants exactly two WARNs (`:99`), and `TestDaemonTick_ConsumesSaveRequestedWhenTheCycleIsCancelled` (`:107-145`) wants none (`:139`). Both tick over a state directory with no `sessions.json`, so each of those cycles now also logs the absent line.
- Unchanged:
  - the daemon's `LoadPrev` (`cmd/state_daemon.go:271`);
  - the daemon's start-up read and its `ReadIndex failed` WARN (`:423-430`), with `TestDaemonStartup_HandlesMissingSessionsJSONAsNilPrev` and `TestDaemonStartup_LogsWarningOnUndecodableSessionsJSON` (`cmd/state_daemon_run_test.go:1166-1213`), which replace the run loop and run no cycle;
  - `state.ReadIndex` and its other callers;
  - `commitOver` (`internal/state/commit.go:34-60`).
