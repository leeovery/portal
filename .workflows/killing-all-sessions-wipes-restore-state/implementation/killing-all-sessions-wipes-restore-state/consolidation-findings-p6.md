# Consolidation Findings: killing-all-sessions-wipes-restore-state (Phase 6)

## Findings

### F1: commit-now's previous-index loader keeps a readable-file branch production can no longer reach, and its test stand-in still models it
- **Class**: dead-code
- **Failure**: Task 6-2 made `RunCommitCycle` take its previous index from `sessions.json` as read under the lock, and call `LoadPrev` only when that read fails. Two things went stale with that change. First, the `return idx` branch of commit-now's `loadPrevIndex` can no longer run. Second, the cmd test stand-in for the cycle still calls `LoadPrev` on every run, and one test still pins that branch through the stand-in. As a result, a change to how commit-now builds its previous index from a readable file (filtering it, adjusting it, routing a merge through it) does nothing in production, while `TestStateCommitNow_PassesPrevIndexFromDiskToCaptureAndRefile` keeps passing and reports it working. The same goes for any later cmd test built on `commitNowFixture` that asserts on the previous index: it tests a contract the real command no longer has. Nobody notices until a real install behaves differently from the suite.
- **Evidence**:
  - `internal/state/commit_cycle.go:119-123`: `committed := readPriorIndex(cycle.Dir)`, and `LoadPrev` runs only when that is nil.
  - `internal/state/commit.go:63-74`: `readPriorIndex` returns nil exactly when `os.ReadFile` or `DecodeIndex` fails.
  - `internal/state/index_reader.go:17-32`: `ReadIndex` fails on the same two calls.
  - `internal/state/commit.go:47`: `commitOver` is the only production writer of `sessions.json` (`rg -n 'SessionsJSON\(' cmd internal -g '!*_test.go'`), and the commit lock shuts it out between the two reads. So when `LoadPrev` runs, `ReadIndex` fails too.
  - `cmd/state_commit_now.go:119-122` (the `LoadPrev` closure) and `:162-173` (`loadPrevIndex`): the `return idx` at `:172` is now unreachable. The function's only live job is to work out why the file could not be read, log the WARN, and return the zero index.
  - `cmd/state_commit_now_test.go:90-96`: the `commitNowFixture` stand-in calls `cycle.LoadPrev()` on every run. Its comment says the previous index is loaded on every cycle, "in the entry point's order".
  - `cmd/state_commit_now_test.go:285-341`: `TestStateCommitNow_PassesPrevIndexFromDiskToCaptureAndRefile` pins the unreachable readable branch through that stand-in.
- **Proposed shape**:
  - Reduce `loadPrevIndex` (`cmd/state_commit_now.go:159-173`) to the fallback it now is. It works out why `sessions.json` could not be read, logs the existing absent or corrupt WARN with its text unchanged, and returns the zero index on every path. Drop the `return idx` path.
  - Change the `commitNowFixture` stand-in so it calls `cycle.LoadPrev()` only when the `sessions.json` in `cycle.Dir` does not read and decode, mirroring `commit_cycle.go:119-123`. Rewrite its comment to match.
  - Delete `TestStateCommitNow_PassesPrevIndexFromDiskToCaptureAndRefile`. The readable path is already pinned through the real cycle in two places: the `readable` row of `TestStateCommitNow_WarnsOnlyWhenSessionsJSONCannotBeRead` (`cmd/state_commit_prev_lag_test.go:158-162`) and the `commit-now` row of `TestRunCommitCycleReadsNoCallerIndexOverAReadableSessionsJSON` (`internal/state/commit_cycle_prev_test.go:156-185`).
  - `TestStateCommitNow_FallsBackToZeroPrevAndLogsWarnWhenSessionsJSONMissing` and `…OnCorruptSessionsJSON` keep passing, because the file is unreadable in both, so the changed stand-in still calls `LoadPrev`.
- **Bank**: "The commitNowFixture stand-in cycle still calls LoadPrev on every run" (6-2, reviewer).

## Comment Corrections

- `cmd/state_commit_now.go:113-114`: this comment sits on the `LoadPrev` closure, which since 6-2 runs only when `sessions.json` cannot be read, and then returns the zero index. What the comment says about the cycle's previous index is `RunCommitCycle`'s documented contract, not something this call site does.
  OLD: // The previous index is read under the commit lock, so it is the
		// sessions.json the last committer to hold the lock left behind.
  NEW:

- `cmd/state_daemon.go:423-424`: the first capture's skeleton merge now reads `sessions.json` as it stands under the lock, not this startup read. This index is only the fallback for a cycle that cannot read the file (6-2, reviewer bank entry).
  OLD: // Skeleton-marked panes merge from this pre-boot state during the first
		// capture.
  NEW: // The previous index a cycle falls back on when it cannot read
		// sessions.json itself.

- `internal/state/commit_cycle_test.go:213-214`: the cycle now reads `sessions.json` on every run without recording it. `prevs` records only the fallback loads, which is what the serialising test's `len(secondPrevs) != 0` relies on.
  OLD: // commitNowCycle is the cycle `portal state commit-now` runs: the previous index
// read from disk, and no dump. prevs records each index it read.
  NEW: // commitNowCycle is the cycle `portal state commit-now` runs, with no dump.
// prevs records each index its LoadPrev fallback loads.

- `CLAUDE.md:61` (project guide prose, `state` row): this sentence omits the change from 6-2. The cycle's merges, carry and hold now read `sessions.json` under the lock, and `LoadPrev` is only the fallback (6-2, reviewer bank entries).
  OLD: The caller's `LoadPrev` is called only once the lock is held.
  NEW: The cycle's previous index (the one its skeleton merge, waiting-pane merge, carry and answered-pane hold all read) is `sessions.json` as read under the lock, so a committer whose own index lags another committer's commit merges from that commit. The caller's `LoadPrev` supplies the previous index, under the lock, only when that file cannot be read or decoded.

- `CLAUDE.md:61` (project guide prose, `state` row): the daemon's in-memory index is now only the fallback. Commit-now's own read now produces only the zero-value fallback, with its WARN (6-2, reviewer bank entries).
  OLD: The daemon's tick and shutdown flush dump, passing the daemon's in-memory previous index. `state commit-now` reads `sessions.json` under the lock, discards the sets and dumps nothing.
  NEW: The daemon's tick and shutdown flush dump, passing the daemon's in-memory index as that fallback. `state commit-now` passes a zero-value fallback with a WARN naming why `sessions.json` could not be read, discards the sets and dumps nothing.
