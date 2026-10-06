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
