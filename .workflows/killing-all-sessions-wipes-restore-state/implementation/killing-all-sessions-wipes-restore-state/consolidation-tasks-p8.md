# Consolidation Tasks: Killing All Sessions Wipes Restore State (Phase 8)

## Task 1: The Daemon Reports An Unreadable sessions.json Once, From The Cycle
placement: phase 8
severity: behaviour

**Problem**: Phase 8 moved the report of an unreadable `sessions.json` into the commit cycle and deleted commit-now's pre-cycle report, but left the daemon's. Its startup read (`cmd/state_daemon.go:425-430`) still logs `ReadIndex failed` at WARN (`:429`) over a corrupt file, and about a second later its first tick logs `read sessions.json failed; committing without the saved index` (`internal/state/commit_cycle.go:156-163`) — two `daemon:` WARN lines carrying the same `error` for one condition. Over a missing file the two reads disagree the other way: the startup read is silent and the cycle warns. A reader of portal.log counting the condition, or grepping one wording to find it, gets a doubled or a split answer.
**Solution**: The daemon's startup read keeps seeding the previous index and drops its WARN branch (`if idx, skip, _ := state.ReadIndex(dir); !skip { prevIdx = &idx }`), so the cycle's line is the daemon's one report of the condition, as it already is for `commit-now`. Nothing is lost: the cycle logs its line before `LoadPrev` and before the capture, so even a first cycle that stands down has reported it, and restore keeps its own `ReadIndex failed` under `restore:`. No test pins the startup line; `TestDaemonStartup_LogsWarningOnUndecodableSessionsJSON` (`cmd/state_daemon_run_test.go`) is revised to assert the startup read logs nothing and seeds no index. Derived from phase 8 Task 1's direction, which put the report at the read that found the problem.
**Outcome**: A corrupt or missing `sessions.json` produces exactly one `daemon:` WARN per committing cycle that finds it, in the same words whichever committer ran.

**Acceptance Criteria**:
- [ ] Starting `portal state daemon` over a `sessions.json` that does not decode logs nothing about that file before the tick loop starts: no `ReadIndex failed` line, and no record carrying the decode error. The tick loop is handed no previous index.
- [ ] Starting the daemon over that file and running its first tick leaves exactly one line about the file in the log: `read sessions.json failed; committing without the saved index` at WARN under component `daemon`, its `error` wrapping `state.ErrCorruptIndex`. That tick commits its capture over the file, as today.
- [ ] Starting the daemon over that file, its first tick stands down because tmux stopped answering. The log still holds that one `read sessions.json failed; committing without the saved index` WARN, and no `ReadIndex failed` line under `daemon`.
- [ ] Unchanged: starting the daemon with no `sessions.json` logs nothing about the file and seeds no previous index, and its first tick logs `sessions.json absent; committing without the saved index` once at WARN under `daemon`. Starting it over a readable `sessions.json` seeds the previous index from that file.
- [ ] Unchanged: `portal state commit-now` over an absent or undecodable `sessions.json` logs the same one line, word for word, under `daemon`. Bootstrap's restore over an undecodable `sessions.json` still logs `ReadIndex failed` under `restore`.

**Do**:
- The change is the daemon's startup read in `stateDaemonCmd`'s `RunE` (`cmd/state_daemon.go:425-430`). The `else if err != nil { logger.Warn("ReadIndex failed", "error", err) }` branch (`:428-430`) goes, and the read stays as the previous-index seed: `if idx, skip, _ := state.ReadIndex(dir); !skip { prevIdx = &idx }`. `ReadIndex` returns `skip` set alongside every non-nil error (`internal/state/index_reader.go:17-32`), so the seed is the same on every path.
- The cycle's report stays as it is: `readPriorIndex` → `logUnreadIndex` in `RunCommitCycle` (`internal/state/commit_cycle.go:121-126`, `:156-163`), logged before `LoadPrev` and before `captureAndRefile`. So does restore's `ReadIndex failed` in `handleReadIndexSkip` (`internal/restore/restore.go:107`).
- `rg -n 'ReadIndex failed' --type go` finds only `cmd/state_daemon.go:429` and `internal/restore/restore.go:107`. The one test that reaches the startup line is `TestDaemonStartup_LogsWarningOnUndecodableSessionsJSON` (`cmd/state_daemon_run_test.go:1189-1213`). Because `withImmediateRun` stubs out the tick loop, its assertion that the log body carries `sessions.json corrupt` (`:1210-1212`) is met only by that WARN's `error`. It is revised to assert that the startup read logs nothing and seeds no index; its current name describes the warning it no longer sees.
- `TestDaemonStartup_LoadsPrevIndexFromSessionsJSON` (`:1125`), `TestDaemonStartup_HandlesMissingSessionsJSONAsNilPrev` (`:1166-1187`) and the cycle-line suites (`cmd/state_daemon_unread_index_test.go`, `TestStateCommitNow_WarnsOnlyWhenSessionsJSONCannotBeRead` in `cmd/state_commit_prev_lag_test.go:142`, `internal/state/commit_cycle_unread_index_test.go`) pass unchanged.

## Task 2: Corrections
placement: phase 8
severity: corrections

**Problem**: CLAUDE.md's `state` row says "`state commit-now` passes a zero-value fallback with a WARN naming why `sessions.json` could not be read". Since phase 8 Task 1, commit-now's fallback logs nothing: the commit cycle logs the WARN itself, for every committer, before it calls `LoadPrev`. An agent working from CLAUDE.md would look for the line in commit-now's fallback, or add one there, doubling it.
**Solution**: One prose edit to the `state` row in CLAUDE.md: replace "`state commit-now` passes a zero-value fallback with a WARN naming why `sessions.json` could not be read, discards the sets and dumps nothing." with "`state commit-now` passes an empty index as that fallback, discards the sets and dumps nothing. Whichever committer runs it, the cycle logs one WARN under `daemon` naming why `sessions.json` could not be read before it calls `LoadPrev`."
**Outcome**: CLAUDE.md's `state` row matches the tree. commit-now's fallback is an empty index that logs nothing, and the WARN for an unreadable `sessions.json` comes from the commit cycle, for every committer, before `LoadPrev` is called.

**Acceptance Criteria**:
- [ ] The `state` row (`CLAUDE.md:61`) holds the replacement text verbatim where the old sentence stood, between "The daemon's tick and shutdown flush dump, passing the daemon's in-memory index as that fallback." and "Those callers and the lazy-panel integration fixtures all enter through `RunCommitCycle`."
- [ ] `rg -F 'zero-value fallback with a WARN' CLAUDE.md` finds nothing (one hit today, on line 61)
- [ ] No other text in CLAUDE.md changes, and no Go source or test file changes

**Do**:
- The edit lands in `CLAUDE.md:61`, the `state` row of the Internal packages table, where the old sentence occurs exactly once today.
- The replacement matches the tree: commit-now's `LoadPrev` returns `&state.Index{}` and logs nothing (`cmd/state_commit_now.go:113-114`); the cycle calls `logUnreadIndex` before `cycle.LoadPrev()` (`internal/state/commit_cycle.go:121-126`); and both committers hand it `daemonLogger`, which is `log.For("daemon")` (`cmd/state_daemon.go:415`, `cmd/state_commit_now.go:91`, `cmd/state_common.go:8`).
