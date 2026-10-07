TASK: The Daemon Reports An Unreadable sessions.json Once, From The Cycle (killing-all-sessions-wipes-restore-state-8-2, tick-600a86)

ACCEPTANCE CRITERIA:
- Starting `portal state daemon` over a `sessions.json` that does not decode logs nothing about that file before the tick loop starts: no `ReadIndex failed` line, and no record carrying the decode error. The tick loop is handed no previous index.
- Starting the daemon over that file and running its first tick leaves exactly one line about the file in the log: `read sessions.json failed; committing without the saved index` at WARN under component `daemon`, its `error` wrapping `state.ErrCorruptIndex`. That tick commits its capture over the file, as today.
- Starting the daemon over that file, its first tick stands down because tmux stopped answering. The log still holds that one `read sessions.json failed; committing without the saved index` WARN, and no `ReadIndex failed` line under `daemon`.
- Unchanged: starting the daemon with no `sessions.json` logs nothing about the file and seeds no previous index, and its first tick logs `sessions.json absent; committing without the saved index` once at WARN under `daemon`. Starting it over a readable `sessions.json` seeds the previous index from that file.
- Unchanged: `portal state commit-now` over an absent or undecodable `sessions.json` logs the same one line, word for word, under `daemon`. Bootstrap's restore over an undecodable `sessions.json` still logs `ReadIndex failed` under `restore`.

STATUS: complete

SPEC CONTEXT: The specification does not address this logging detail directly; the task derives from phase 8 Task 1's direction (report an unreadable sessions.json at the read that found the problem, i.e. inside the commit cycle) so one condition yields one `daemon:` WARN per committing cycle, worded the same whichever committer ran. It sits inside the spec's broader commit-cycle model (§2.x): the cycle reads sessions.json under the commit lock, falls back on the caller's LoadPrev only when that read fails, and a stand-down writes nothing.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_daemon.go:423-429 (startup read now `if idx, skip, _ := state.ReadIndex(dir); !skip { prevIdx = &idx }`, WARN branch removed); internal/state/commit_cycle.go:121-126 and :156-163 (the cycle's report, unchanged, logged before LoadPrev and before captureAndRefile); internal/restore/restore.go:103-113 (restore's own `ReadIndex failed`, unchanged).
- Notes: The commit (eabbfe97e) touches exactly cmd/state_daemon.go and cmd/state_daemon_run_test.go, matching the plan's Do list. ReadIndex (internal/state/index_reader.go:17-32) sets skip on every non-nil error, so the seed is identical on every path; PrevIndex's only production readers are LoadPrev (cmd/state_daemon.go:271) and the post-commit reassignment (:283), so nothing else depended on the dropped branch. `grep 'ReadIndex failed'` over Go sources now finds only internal/restore/restore.go:107 plus the two negative assertions in cmd/state_daemon_run_test.go. Because logUnreadIndex runs before captureAndRefile (where the own-server confirmation happens), a first tick that stands down has already reported the condition — AC3 holds structurally. The updated comment at cmd/state_daemon.go:423-425 is true against the code.

TESTS:
- Status: Adequate
- Coverage:
  - AC1: TestDaemonStartup_UndecodableSessionsJSONLogsNothingAndSeedsNoIndex (cmd/state_daemon_run_test.go:1199-1226) runs the real RunE with the tick loop stubbed, asserts PrevIndex nil, zero `ReadIndex failed` records, and no record whose `error` wraps state.ErrCorruptIndex. Both assertions would fail against the pre-change code.
  - AC2/AC3: TestDaemonStartup_FirstTickIsTheOneReportOfAnUndecodableSessionsJSON (cmd/state_daemon_run_test.go:1228-1301) drives startup then one captureAndCommit through the real cycle, in a commit case and a refused-confirmation stand-down case; asserts exactly one `daemon` WARN with the cycle wording and an ErrCorruptIndex-wrapping error, zero `ReadIndex failed` under `daemon`, exactly one record carrying the decode error, and either the capture committed over the file or the file left undecodable with the error wrapping ErrTmuxStoppedAnswering.
  - AC4: unchanged tests TestDaemonStartup_HandlesMissingSessionsJSONAsNilPrev (:1166), TestDaemonStartup_LoadsPrevIndexFromSessionsJSON (:1125) and the absent case of TestDaemonTick_ReportsASessionsJSONItCouldNotReadAndReplacesIt (cmd/state_daemon_unread_index_test.go:18).
  - AC5: unchanged TestStateCommitNow_WarnsOnlyWhenSessionsJSONCannotBeRead (cmd/state_commit_prev_lag_test.go:142) and the restore suite's corrupt-index tests (internal/restore/restore_test.go:98).
- Notes: The new end-to-end test is not redundant with the tick-only unread-index suite: its subject is the startup read and first tick together, which is what proves the count of one. No over-testing.

CODE QUALITY:
- Project conventions: Followed (seam staged via withFuncSeam, logtest query chain, no t.Parallel, closed log vocabulary untouched)
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
- None
