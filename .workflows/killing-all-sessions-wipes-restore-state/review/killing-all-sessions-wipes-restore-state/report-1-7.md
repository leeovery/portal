TASK: Kill Path Stays Final Under The Hardened Save Path (killing-all-sessions-wipes-restore-state-1-7, tick-aff4a9)

ACCEPTANCE CRITERIA:
- A live runtime holds several user sessions, each with scrollback and a resume hook. The sessions are killed one at a time. After each kill, that session is gone from `sessions.json` and its scrollback files are deleted, committed at that kill.
- The last user session is killed while tmux keeps running for `_portal-saver` and `_portal-bootstrap`. `sessions.json` then holds zero sessions, and no scrollback file remains.
- The next hook-staleness sweep after the kills removes the killed sessions' resume hooks from `hooks.json`.
- A capture whose session listing names only Portal's own sessions, and a capture in which every session vanishes mid-capture, each yield an empty index with no error. `commit-now` with zero live sessions writes a `sessions.json` holding zero sessions. The three tests listed in §5.1 that pin these pass with their assertions unchanged.

STATUS: complete

SPEC CONTEXT: §1.1 makes a kill final: the record, the scrollback and (through the hook-staleness sweep) the resume hooks all go, and killing every session ends in an empty restore state. §5.1 keeps the kill path unchanged. The `session-closed` hook still runs `commit-now` synchronously, now through the hardened cycle (listing probe plus own-server confirmation). The empty state is committed when the last user session closes while tmux keeps running for `_portal-saver` and `_portal-bootstrap`. §5.1 also names the three empty-save contract tests to leave untouched. §6.4 asks for regression coverage of kill-by-kill removal, the empty end state and the sweep's reaping. A `commit-now` that stands down and is then committed by the daemon's next tick is a separate task (1.4) and §5.2 residue.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_kill_path_integration_test.go:57-96: `TestKillPathStaysFinalUnderHardenedSavePath`. Three user sessions: alpha has two panes and two hook keys, bravo and charlie one each. alpha and bravo end through `tmux kill-session`. charlie ends by exiting its last program, the third kill form named in §1.1.
  - cmd/state_kill_path_integration_test.go:100-136: the fixture. tmux is kept alive by Portal's own `_portal-bootstrap` alone, with no extra anchor session. Each pane is stamped with its token, `hooks.json` is staged through `hookstest.StageStore`, and the real bootstrap runs via `portal list`.
  - cmd/state_kill_path_integration_test.go:183-198: `retireDaemon`. It respawns `_portal-saver` onto a placeholder and waits for the daemon's process to be gone. After that the only committer left is the hook's `commit-now`, so a commit seen after a kill is that kill's own commit and a stood-down `commit-now` cannot be hidden by a tick. I confirmed `commit-now` and the daemon are the only two production callers of `state.RunCommitCycle` (cmd/state_commit_now.go:109, cmd/state_daemon.go:267).
  - cmd/state_kill_path_integration_test.go:203-229: after each kill this poll needs two consecutive reads agreeing that the killed session is gone, every survivor is still named, and the killed session's saved scrollback files no longer exist. Lines 89-91 then check that the survivors' files still exist.
  - cmd/state_kill_path_integration_test.go:231-260: zero sessions in `sessions.json`, an empty scrollback directory, and live tmux sessions exactly `[_portal-bootstrap, _portal-saver]`.
  - cmd/state_kill_path_integration_test.go:262-288: runs `hooksweep.Run`, the shared cycle the daemon and `doctor --fix` both drive, and asserts it removed exactly the killed keys and left `hooks.json` empty.
- Notes: The kill path itself is untouched. The `session-closed` body is still the synchronous `run-shell ... portal state commit-now` (internal/tmux/hooks_register.go:84), and that file has no diff over the work unit. The three named contract tests are present, their bodies have no diff over the work unit, and their assertions are unchanged:
  - the "it returns an empty index with nil error when keep is empty after filtering" subtest (internal/state/capture_test.go:1677);
  - the "it proceeds with empty index when every session is natural churn" subtest (internal/state/capture_test.go:761);
  - `TestStateCommitNow_WritesEmptySessionsJSONWhenZeroLiveSessions` (cmd/state_commit_now_test.go:151).

  Only the shared fixture plumbing around them moved, under other tasks: the `captureMock` commander's `display-message` answer, the `failFastCaptureClient` method rename, and `installCommitNowDeps`.

TESTS:
- Status: Adequate
- Coverage: Kill-by-kill removal of the record and its scrollback, committed at that kill. Survivors' transcripts are not collected along the way, which is the defect class this work fixes. Two kill forms are covered: `kill-session` and exiting the last program. Also covered: the empty end state with Portal's two sessions still live, and the sweep reaping every killed key. The test discriminates. With the daemon retired, a `commit-now` that stood down (an unset own-server pid, or a refused confirmation) would leave `sessions.json` unchanged and time the poll out. Hooks reaped early, or left behind by the sweep, fail the exact `removed == killedKeys` assertion. Every file list the noneExist/assertFilesExist checks read is non-empty, because `waitForDumpedScrollback` requires one non-empty file per pane before the kills start.
- Notes: The test is integration-tagged because it builds and execs the binary. It uses `IsolateStateForTest` (which also sets the sandbox registry and HOME in the process env), `RegisterStateDirTeardownGuard` before `tmuxtest.New`, an explicit test socket, and no `t.Parallel`. It is not over-tested: each assertion maps to an acceptance criterion or to the precondition one names (tmux still running for Portal's own sessions).

CODE QUALITY:
- Project conventions: Followed (lane placement, isolation helpers, exact `=name:` targets, `portalbintest` staging, helpers reused from the symptom/reentrancy suites rather than redeclared, no identifier collisions in the integration build)
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (`slices.Sorted(slices.Values(...))`, `slices.Equal`, `strings.SplitSeq` in the shared helper)
- Readability: Good. Comments hold true against the code and reference no process artifacts.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "After each kill, that session is gone from `sessions.json` and its scrollback files are deleted, committed at that kill." To settle it, run `go test -tags integration -p 1 ./cmd -run TestKillPathStaysFinalUnderHardenedSavePath` and observe it pass, repeatedly, within the 1.5s per-kill budget. Whether the hook subprocess's hardened cycle confirms against its own server (via the `TMUX` tmux hands a `session-closed` `run-shell` job) can only be observed against real tmux.
- "`sessions.json` then holds zero sessions, and no scrollback file remains." Settled by the same integration run.
- "The next hook-staleness sweep after the kills removes the killed sessions' resume hooks from `hooks.json`." Settled by the same integration run.
- "The three tests listed in §5.1 that pin these pass with their assertions unchanged." Reading settled the "unchanged" half. Passing needs a unit-lane run of `go test ./internal/state -run 'TestCaptureStructurePreLoopFailFatal|TestCaptureStructurePerSessionLogAndContinue'` and `go test ./cmd -run TestStateCommitNow_WritesEmptySessionsJSONWhenZeroLiveSessions`.
