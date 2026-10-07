TASK: Every Committer Logs An Unreadable sessions.json From The Cycle That Read It (killing-all-sessions-wipes-restore-state-8-1, tick-7d66a5)

ACCEPTANCE CRITERIA:
- A hand edit leaves `sessions.json` undecodable while the daemon runs. The daemon's next tick logs `read sessions.json failed; committing without the saved index` once at WARN under component `daemon`, with the decode failure in `error`. It then commits from its in-memory index and replaces the file.
- With no `sessions.json` in the state directory, the daemon's tick logs `sessions.json absent; committing without the saved index` once at WARN under component `daemon`, and commits.
- A `sessions.json` that is present but cannot be read gets the `read sessions.json failed; …` line with the read's error in `error`, never the absent line.
- `portal state commit-now` logs those lines, in the same words under the same component, in exactly the cycles it warns in today; over a missing or undecodable file it commits from an empty previous index without reading `sessions.json` a second time.
- Each cycle that finds the file absent or unreadable logs exactly one of the two lines before its capture (so a cycle that then stands down or fails has still logged it). A cycle over a readable `sessions.json`, from either committer, logs neither line and calls no `LoadPrev`.
- `state.Commit` over an absent or undecodable `sessions.json` logs neither line and writes exactly as it does today.
- `CommitNowDeps` has no `ReadIndex` seam and `loadPrevIndex` no longer exists. The seam-convention suite's every-field coverage guard passes over the fields that remain.

STATUS: complete

SPEC CONTEXT: Spec section 4 makes a wipe or a backed-off save visible in portal.log at the production default level, using only the closed attr vocabulary (`error` for a cause). This task closes a gap in that observability: a daemon tick that replaced an unreadable `sessions.json` left no trace. The fix moves the diagnosis into the commit cycle's own read under the commit lock, so the daemon tick, the shutdown flush and `commit-now` all report it in the same words under `daemon`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/commit.go:63-75 — `readPriorIndex` now returns `(prior, absent, err)`. It delegates to `ReadIndex` (internal/state/index_reader.go:17-32), which fails at the same `os.ReadFile`/`DecodeIndex` calls and classifies `fs.ErrNotExist` as absent, so the conditions match those `commit-now`'s old second read used.
  - internal/state/commit.go:28-31 — `Commit` discards the reason (`prior, _, _ :=`), keeping its write behaviour and logging neither line.
  - internal/state/commit_cycle.go:121-126 — when the read under the lock returns nil, `logUnreadIndex` runs before `cycle.LoadPrev()` and before `captureAndRefile` (:127).
  - internal/state/commit_cycle.go:156-163 — `logUnreadIndex` logs the two verbatim messages at WARN. The failure line carries the cause under `error`, and a nil logger is tolerated through `loggerOrDiscard`.
  - internal/state/commit_cycle.go:39-42 — the `LoadPrev` doc now covers the absent case and the prior WARN.
  - cmd/state_commit_now.go:33-43, 109-116 — the `ReadIndex` field, its fill line and `loadPrevIndex` are gone. `LoadPrev` returns `&state.Index{}` without reading anything.
  - Both committers hand the cycle `daemonLogger` (`log.For("daemon")`, cmd/state_common.go:8): commit-now at cmd/state_commit_now.go:91/115, and the daemon through `deps.Logger` (cmd/state_daemon.go:274, 415, 450).
  - Seam footprint: the task's own `rg` pattern now returns no matches in `cmd` or `internal`. The old `proceeding with zero-value PrevIndex` text is gone from the tree.
- Notes: The classification order (err checked before skip) reproduces the old `loadPrevIndex` precedence exactly, so commit-now warns in the same cycles as before. `Commit`'s write path is unchanged: a nil prior still writes unconditionally and skips drop logging. Task 8-2 later changed the daemon start-up read (cmd/state_daemon.go:423-430 no longer warns). That is outside this task.

TESTS:
- Status: Adequate
- Coverage:
  - internal/state/commit_cycle_unread_index_test.go:39-100 — absent, undecodable, and present-but-unreadable (`sessions.json` staged as a directory). Each case checks the line count per message (exactly one, never the other), that `LoadPrev` runs exactly once and only after the line was logged, a non-nil `error` cause, and the commit for the two committable cases.
  - :102-121 — the decode failure as the cause (`errors.Is(..., ErrCorruptIndex)`).
  - :123-142 — the line is logged on a cycle whose capture fails (stand-down).
  - :144-166 — a readable file logs neither line and makes zero `LoadPrev` calls.
  - :168-192 — `state.Commit` over an absent or undecodable file logs neither line and writes.
  - cmd/state_daemon_unread_index_test.go:18-64 — a real daemon `captureAndCommit` over absent and hand-broken files. Checks component `daemon` at WARN with the decode failure in `error`, that the file is replaced with the captured index, and that the other line is absent. :66-86 covers the readable case.
  - cmd/state_commit_prev_lag_test.go:142-197 — real commit-now cycle: absent, undecodable and readable. Checks one line under `daemon` per failing case, none for readable, and a total WARN count that rules out a duplicate line.
  - cmd/state_commit_now_test.go:317-373 — the zero previous index on the commit-now fallback, through the stand-in.
  - Tests updated for the extra absent WARN: cmd/state_daemon_save_request_test.go:96-103 and :140-143, and cmd/state_daemon_capture_logging_test.go:118-125.
  - Seam-convention table, cmd/deps_merge_convention_test.go:268-335 — four cases (RunCommitCycle, NewClient, IsRestoring, TouchSaveRequested) against the four remaining exported fields. `assertSeamCasesCoverFields` (:343-364) therefore matches.
- Notes:
  - `TestDaemonTick_ReportsASessionsJSONItCouldNotReadAndReplacesIt` stages `deps.PrevIndex = sentinelIndex("remembered")` but never checks that the commit used it. The daemon's `LoadPrev` is unchanged code, so this is below the finding bar.
  - The commit-now real-cycle test does not check the `error` attr on its undecodable case. Both committers share the same line from `RunCommitCycle`, and the state- and daemon-level tests pin the attr, so this is not a gap.

CODE QUALITY:
- Project conventions: Followed. Logging uses the existing `daemon` binding and the closed `error` key. No new component or attr. No new seam, and one seam removed.
- SOLID principles: Good. The diagnosis now sits with the read that found the condition, and `LoadPrev` is reduced to a pure fallback.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The doc comments on `readPriorIndex`, `commitOver`, `CommitCycle.LoadPrev` and commit-now's `LoadPrev` all hold against the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
