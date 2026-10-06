# Consolidation Tasks: Killing All Sessions Wipes Restore State (Phase 6)

## Task 1: Commit-Now's Previous-Index Loader Is Reduced To The Fallback It Now Is
placement: phase 6
severity: dead-code

**Problem**: Since phase 6 Task 2, `RunCommitCycle` takes its previous index from `sessions.json` as read under the commit lock, and calls the caller's `LoadPrev` only when that read fails (`internal/state/commit_cycle.go:119-123`). `readPriorIndex` (`internal/state/commit.go:63-74`) and `ReadIndex` (`internal/state/index_reader.go:17-32`) fail on the same two calls, `os.ReadFile` and `DecodeIndex`, and the commit lock shuts out `commitOver`, the only production writer of `sessions.json`, between them (`internal/state/commit.go:47`; `rg -n 'SessionsJSON\(' cmd internal -g '!*_test.go'` finds no other write outside the test-only `internal/restoretest` seeder). So when commit-now's `LoadPrev` (`cmd/state_commit_now.go:117-120`) runs, its `ReadIndex` fails too, and the `return idx` branch of `loadPrevIndex` (`cmd/state_commit_now.go:170`) can no longer run. The `commitNowFixture` stand-in for the cycle (`cmd/state_commit_now_test.go:90-96`) still calls `LoadPrev` on every run, and `TestStateCommitNow_PassesPrevIndexFromDiskToCaptureAndRefile` (`:285-341`) pins the unreachable branch through it. A change to how commit-now builds its previous index from a readable file would do nothing in production while that test keeps passing, and any later cmd test built on the stand-in asserts a contract the real command no longer has.
**Solution**: `loadPrevIndex` becomes the fallback it is: it works out why `sessions.json` could not be read, logs the existing absent or corrupt WARN with its text unchanged, and returns the zero index on every path. The `commitNowFixture` stand-in calls `LoadPrev` only when the `sessions.json` in the cycle's directory does not read and decode, mirroring `RunCommitCycle`, and its comment says so. `TestStateCommitNow_PassesPrevIndexFromDiskToCaptureAndRefile` is deleted: the readable path is already pinned through the real cycle by the `readable` row of `TestStateCommitNow_WarnsOnlyWhenSessionsJSONCannotBeRead` (`cmd/state_commit_prev_lag_test.go:158-162`) and the `commit-now` row of `TestRunCommitCycleReadsNoCallerIndexOverAReadableSessionsJSON` (`internal/state/commit_cycle_prev_test.go:156-185`). The missing- and corrupt-file fallback tests keep passing unchanged. Derived from phase 6 Task 2's settled direction, which this completes rather than reverses; it folds the banked entry about the stand-in.
**Outcome**: commit-now carries no branch production cannot reach, and its cmd tests model the cycle's real contract, so a cmd test asserting on the previous index tests what the command does.

**Acceptance Criteria**:
- [ ] With no `sessions.json` in the state directory, `portal state commit-now` commits from a zero-value previous index and logs `sessions.json absent; proceeding with zero-value PrevIndex` once at WARN under component `daemon`, its text unchanged
- [ ] With a `sessions.json` that does not decode, `portal state commit-now` commits from a zero-value previous index and logs `read sessions.json failed; proceeding with zero-value PrevIndex` once at WARN under component `daemon`, carrying the read's error, its text unchanged
- [ ] With a readable `sessions.json`, `portal state commit-now` runs the real cycle with no `LoadPrev` call and logs neither WARN: the `readable` row of `TestStateCommitNow_WarnsOnlyWhenSessionsJSONCannotBeRead` and the `commit-now` row of `TestRunCommitCycleReadsNoCallerIndexOverAReadableSessionsJSON` pass unchanged
- [ ] `loadPrevIndex` returns `state.Index{}` on every path; no path returns the index its reader read
- [ ] Run through `installCommitNowDeps` over a `sessions.json` in the cycle's directory that reads and decodes, the stand-in cycle makes no `LoadPrev` call; over an absent or undecodable one it makes exactly one, as `RunCommitCycle` does
- [ ] `TestStateCommitNow_PassesPrevIndexFromDiskToCaptureAndRefile` no longer exists, and every other test in `cmd` and `internal/state` passes unchanged, including `TestStateCommitNow_FallsBackToZeroPrevAndLogsWarnWhenSessionsJSONMissing`, `…OnCorruptSessionsJSON`, and `TestStateCommitNow_LeavesSessionsJSONByteIdenticalWhenCommitFailsBeforeRename`, whose `{"sentinel":"untouched"}` seed carries no version field, fails decode, and so still sends the stand-in through `LoadPrev` and its injected reader

**Do**:
- `cmd/state_commit_now.go`, `loadPrevIndex` (`:160-171`): drop the `return idx` at `:170`. The absent and corrupt WARN branches keep their messages and attrs, and every path returns `state.Index{}`.
- `cmd/state_commit_now_test.go`, `installCommitNowDeps` (`:94-96`): the stand-in calls `cycle.LoadPrev()` only when the `sessions.json` in `cycle.Dir` does not read and decode, mirroring `internal/state/commit_cycle.go:119-123`. Rewrite its comment at `:91-93`, which says the previous index is loaded on every cycle in the entry point's order, to match.
- Delete `TestStateCommitNow_PassesPrevIndexFromDiskToCaptureAndRefile` (`cmd/state_commit_now_test.go:285-341`). After it goes, the stand-in's recorded previous indexes are read only by the missing- and corrupt-file tests (`:403`, `:449`), both over a file that does not read.
- Pure refactor: no test is added, and no surviving test's assertions change.

## Task 2: Corrections
placement: phase 6
severity: corrections

**Problem**: CLAUDE.md's `state` row (`CLAUDE.md:61`) still describes the commit cycle as it stood before phase 6 Task 2. It says the caller's `LoadPrev` is called once the lock is held, and that the daemon's tick and shutdown flush pass its in-memory previous index while commit-now reads `sessions.json`. Since that task, the cycle's merges, carry and hold all read `sessions.json` under the lock, and `LoadPrev` is only the fallback for a file that cannot be read or decoded. An agent working from CLAUDE.md would reason from the lagging in-memory index, and might add a lag repair this task made unnecessary or route a merge back through that index, reopening the lag. Two banked entries from phase 6 Task 2's review carry this.
**Solution**: Two prose edits to `CLAUDE.md:61`, the `state` row:
- Replace "The caller's `LoadPrev` is called only once the lock is held." with "The cycle's previous index (the one its skeleton merge, waiting-pane merge, carry and answered-pane hold all read) is `sessions.json` as read under the lock, so a committer whose own index lags another committer's commit merges from that commit. The caller's `LoadPrev` supplies the previous index, under the lock, only when that file cannot be read or decoded."
- Replace "The daemon's tick and shutdown flush dump, passing the daemon's in-memory previous index. `state commit-now` reads `sessions.json` under the lock, discards the sets and dumps nothing." with "The daemon's tick and shutdown flush dump, passing the daemon's in-memory index as that fallback. `state commit-now` passes a zero-value fallback with a WARN naming why `sessions.json` could not be read, discards the sets and dumps nothing."

Both sentences state what holds once Task 1 lands, and the second already holds today in effect, since commit-now's readable branch cannot run.

**Outcome**: CLAUDE.md's `state` row describes the commit cycle as it stands: the previous index the cycle's merges, carry and hold read is `sessions.json` as read under the lock, `LoadPrev` is only the fallback for a file that cannot be read or decoded, the daemon's in-memory index is its fallback, and commit-now's is the zero value with a WARN.

**Acceptance Criteria**:
- [ ] The `state` row holds the first replacement text verbatim where "The caller's `LoadPrev` is called only once the lock is held." stood, between "…a token-named transcript the other has just filed." and "The acquire is bounded at 5s, …"
- [ ] The `state` row holds the second replacement text verbatim where "The daemon's tick and shutdown flush dump, passing the daemon's in-memory previous index. `state commit-now` reads `sessions.json` under the lock, discards the sets and dumps nothing." stood, between "…either way the daemon's next tick retries." and "Those callers and the lazy-panel integration fixtures all enter through `RunCommitCycle`."
- [ ] `rg -F 'is called only once the lock is held' CLAUDE.md` and `rg -F 'in-memory previous index' CLAUDE.md` each find nothing (one hit each today, both on line 61)
- [ ] No other text in CLAUDE.md changes, and no Go source or test file changes

**Do**:
- Both edits land in `CLAUDE.md:61`, the `state` row of the Internal packages table, each old sentence occurring exactly once there today.
