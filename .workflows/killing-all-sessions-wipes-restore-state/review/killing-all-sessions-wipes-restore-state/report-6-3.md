TASK: Commit-Now's Previous-Index Loader Is Reduced To The Fallback It Now Is (killing-all-sessions-wipes-restore-state-6-3, tick-ed8613)

ACCEPTANCE CRITERIA:
- With no sessions.json, commit-now commits from a zero-value previous index and logs `sessions.json absent; proceeding with zero-value PrevIndex` once at WARN under `daemon`, text unchanged
- With an undecodable sessions.json, commit-now commits from a zero-value previous index and logs `read sessions.json failed; proceeding with zero-value PrevIndex` once at WARN under `daemon`, carrying the read's error, text unchanged
- With a readable sessions.json, commit-now runs the real cycle with no LoadPrev call and logs neither WARN; the `readable` row of TestStateCommitNow_WarnsOnlyWhenSessionsJSONCannotBeRead and the `commit-now` row of TestRunCommitCycleReadsNoCallerIndexOverAReadableSessionsJSON pass unchanged
- loadPrevIndex returns state.Index{} on every path; no path returns the index its reader read
- Through installCommitNowDeps, the stand-in cycle makes no LoadPrev call over a sessions.json that reads and decodes, and exactly one over an absent or undecodable one, as RunCommitCycle does
- TestStateCommitNow_PassesPrevIndexFromDiskToCaptureAndRefile no longer exists; every other test in cmd and internal/state passes unchanged, including the missing/corrupt fallback tests and TestStateCommitNow_LeavesSessionsJSONByteIdenticalWhenCommitFailsBeforeRename

STATUS: complete

SPEC CONTEXT: The spec's committing-cycle rules (every committer runs through one locked cycle; commit-now dumps nothing and stands down through failCommitNow) bound this task. The spec says nothing about the fallback WARN wording or about loadPrevIndex. The task finishes phase 6 Task 2: RunCommitCycle reads sessions.json under the commit lock and calls the caller's LoadPrev only when that read fails, so commit-now's `return idx` branch could no longer run.

IMPLEMENTATION:
- Status: Implemented. A later planned task (8-1) then deliberately took the work further.
- Location: cmd/state_commit_now.go:113-114 (commit-now's LoadPrev, now `func() *state.Index { return &state.Index{} }`); internal/state/commit_cycle.go:121-126 (the cycle reads under the lock, logs the WARN, then calls LoadPrev); internal/state/commit.go:64-74 (readPriorIndex now delegates to state.ReadIndex)
- Notes: Commit 5252efa7b did exactly what the task asked. It dropped `return idx`, kept the two WARN branches and returned state.Index{} on every path. It also made the stand-in's LoadPrev call conditional, rewrote the stand-in's comment, and deleted the test. Task 8-1 (tick-7d66a5, commit c40f8965e) then moved the WARN into RunCommitCycle and changed its tail to `committing without the saved index`, so one line is true for both the daemon and commit-now. It also removed loadPrevIndex and the CommitNowDeps.ReadIndex seam. The two `text unchanged` clauses and the loadPrevIndex clause in this task's criteria are therefore superseded by a later deliberate plan decision, not lost. The intent behind them still holds in the code:
  - commit-now carries no branch production cannot reach. Its LoadPrev reads nothing and returns an empty index.
  - The absent and failed-read WARNs still fire once each under `daemon`. commit-now's logger is daemonLogger, bound at cmd/state_common.go:8.
  - No path hands the cycle a read index.
  Because readPriorIndex now calls state.ReadIndex, the stand-in's predicate is the cycle's own read, not a parallel copy.

TESTS:
- Status: Adequate
- Coverage:
  - The stand-in (cmd/state_commit_now_test.go:87-95) calls cycle.LoadPrev() only when `state.ReadIndex(cycle.Dir)` reports skip or error. Its comment says so accurately.
  - TestStateCommitNow_PassesPrevIndexFromDiskToCaptureAndRefile is gone; no reference to it remains under cmd or internal.
  - capturePrevs is read only by the missing- and corrupt-file tests (cmd/state_commit_now_test.go:337, :370), both over a file that does not read. A stand-in that skipped LoadPrev there would panic on the index and fail.
  - The readable path is pinned through the real cycle in two places: the `readable` row of TestStateCommitNow_WarnsOnlyWhenSessionsJSONCannotBeRead (cmd/state_commit_prev_lag_test.go:154-158), which asserts zero WARN records, and TestRunCommitCycleReadsNoCallerIndexOverAReadableSessionsJSON (internal/state/commit_cycle_prev_test.go:156-185), which asserts `loads == 0`.
  - TestStateCommitNow_LeavesSessionsJSONByteIdenticalWhenCommitFailsBeforeRename seeds `{"sentinel":"untouched"}`. DecodeIndex rejects it for its missing version (internal/state/schema.go:87-88), so the stand-in still takes the LoadPrev route there.
- Notes: The missing- and corrupt-file tests lost their WARN assertions and were renamed. That happened in 8-1, not here, because the WARN moved into the real cycle, which the stand-in replaces. The WARN is pinned through the real cycle by the absent and not-decodable rows of TestStateCommitNow_WarnsOnlyWhenSessionsJSONCannotBeRead.

CODE QUALITY:
- Project conventions: Followed. Seams are staged through withCommitNowDeps, and no t.Parallel is used.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "the `readable` row of `TestStateCommitNow_WarnsOnlyWhenSessionsJSONCannotBeRead` and the `commit-now` row of `TestRunCommitCycleReadsNoCallerIndexOverAReadableSessionsJSON` pass unchanged" — settled only by running `go test ./cmd -run TestStateCommitNow_WarnsOnlyWhenSessionsJSONCannotBeRead` and `go test ./internal/state -run TestRunCommitCycleReadsNoCallerIndexOverAReadableSessionsJSON`
- "every other test in `cmd` and `internal/state` passes unchanged, including `TestStateCommitNow_FallsBackToZeroPrevAndLogsWarnWhenSessionsJSONMissing`, `…OnCorruptSessionsJSON`, and `TestStateCommitNow_LeavesSessionsJSONByteIdenticalWhenCommitFailsBeforeRename`" — settled only by a run of `go test ./cmd ./internal/state` (the two fallback tests now carry the 8-1 names TestStateCommitNow_FallsBackToZeroPrevWhenSessionsJSONMissing / …OnCorruptSessionsJSON)
