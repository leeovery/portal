TASK: The Confirmation Counts Only From The Committer's Own Server (killing-all-sessions-wipes-restore-state-1-3, tick-5c459d)

ACCEPTANCE CRITERIA:
- The daemon runs in `_portal-saver` on a server holding the user's sessions, and `portal uninstall` kills `_portal-saver` while the server keeps running. The daemon's shutdown flush commits, and its `shutdown` line reports `flush_completed=true`.
- The daemon's tick, with its own server answering every capture read and the confirmation, commits as before.
- The committer's own server exits, and a new server is started on the same socket before its restore has run. A save whose capture reads and confirmation are all answered by the new server writes nothing: `sessions.json` and every scrollback file are unchanged.
- A save's first capture reads are answered by its own server, which then exits. Its remaining capture reads, or only its confirmation, are answered by a new server started on the same socket. Nothing is written.
- A server's `session-closed` hook runs `commit-now`, and that server answers its capture reads and its confirmation. The commit is written as before.
- A server's `session-closed` hook runs `commit-now`, and the hook's server exits before `commit-now` reads. Its reads reach a new server started on the same socket. Nothing is written.

STATUS: issues_found

SPEC CONTEXT: Spec §2.2 makes every committing cycle confirm tmux is still answering with a read sent after the last capture read. It narrows that confirmation to the committer's own server: for the daemon, the server its `_portal-saver` pane runs in, which stays its own after that pane is destroyed (so the flush that `portal uninstall` triggers still commits); for `commit-now`, the server whose `session-closed` hook ran it. A new server started on the same socket before its restore holds none of the user's sessions, so its answers confirm nothing. §2.5 routes a stand-down through each committer's existing failure route. §6.1 asks for tests of the uninstall flush and of a save reaching a different server on the same socket. The implementation-phase corrigendum to §2.2 says an exit-0 empty answer names no server and stands the cycle down.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmux/tmux.go:92 — `ConfirmAnswering() (int, error)`: `display-message -p '#{pid}'`. It returns the answering server's pid, 0 for an empty exit-0 answer, and an error for a failed or unparseable read.
  - internal/tmux/detect.go:17 — `ServerPIDFromEnv`. It reads the pid from the second-to-last field of `TMUX`, which tolerates a comma in the socket path, and rejects a pid of 0 or below.
  - internal/state/scrollback.go:267 — `confirmOwnServer`. It refuses when the own server is unknown (no read is sent), when the answer names no server, and when the answering pid differs from the own server. The refusal is wrapped in `ErrNotOwnServer`.
  - internal/state/scrollback.go:366 — the confirmation runs after `captureStructure` (the last capture read) and before any link or re-file. A refusal is wrapped in `ErrTmuxStoppedAnswering`, which drives the "backed off" line.
  - internal/state/scrollback.go:380 — `classifyFailedCapture` applies the same own-server rule to a capture that failed.
  - internal/state/commit_cycle.go:37 — `CommitCycle.OwnServer`, passed into `captureAndRefile` at :127 and into the dump's writer at :139.
  - cmd/state_daemon.go:32, :452 — the daemon reads `OwnServer` from `TMUX` once, at startup, through `ownTmuxServer()`. That is its `_portal-saver` pane's server, and it keeps that value after the pane is destroyed. `captureAndCommit` (:269) passes it to both the tick and `defaultShutdownFlush`.
  - cmd/state_commit_now.go:111, :128 — `commit-now` takes its own server from the `TMUX` its `run-shell` job inherits. The hook body (internal/tmux/hooks_register.go:84) is a plain `run-shell`, so nothing strips `TMUX`.
- Notes:
  - Every tmux read the committed index is built from comes before the confirmation. Everything `captureAndRefile` and `RunCommitCycle` do after it, up to the dump, is filesystem work only.
  - The flush after an uninstall still commits: the daemon never re-derives its server from the pane, and `#{pid}` is a server-level format that needs no live pane.
  - Orphan daemons spawned by `SpawnIsolatedDaemon` carry `TMUX=…,0,0`, so `OwnServer` is 0 and they never commit. That is harmless: those fixtures (cmd/bootstrap/*) only test that orphans are swept. The integration fixtures that do need a committing daemon or `commit-now` were moved to the live server pid in the same commit.

TESTS:
- Status: Adequate
- Coverage:
  - AC1: cmd/state_commit_own_server_realtmux_test.go:94 kills `_portal-saver` on a running real server and runs `defaultShutdownFlush`. It asserts `flush_completed=true` and that the index was committed. cmd/state_commit_own_server_test.go:104 pins the daemon's `OwnServer` to its `TMUX`.
  - AC2: the "the daemon's tick" subtest at realtmux :112 commits against its own real server. The existing tick suites were moved onto `fakeOwnServerPID`.
  - AC3: three tests cover it:
    - realtmux :135 — tick, flush (also `flush_completed=false`) and `commit-now`, each against a real replacement server on the same socket;
    - internal/state/commit_cycle_confirm_test.go:339, first subtest;
    - cmd/state_commit_own_server_test.go:30 — all three committers with a fake.
  - AC4: internal/state/commit_cycle_confirm_test.go:352-383 splits the capture across the two servers after each capture read in turn. The split after the last read leaves only the confirmation to the new server. A guard (:378) fails if the new server answered nothing.
  - AC5: the "commit-now run by its server's session-closed hook" subtest at realtmux :123. The real hook route is exercised by the integration-lane cmd/state_kill_path_integration_test.go:57, which retires the daemon so that only hook-run `commit-now` can commit each kill.
  - AC6: the "commit-now whose hook's server exited before it read" subtest at realtmux :164.
  - Edge cases:
    - a confirmation naming no server (commit_cycle_confirm_test.go:306);
    - no own server known (:320, and cmd :85 — `silentConfirm` there catches a naive `0 == 0` comparison);
    - stand-down classification (:394);
    - `ServerPIDFromEnv` parsing, including a comma in the socket path and the `-1` session id of a hook job (internal/tmux/own_server_test.go:11);
    - `ConfirmAnswering` against a real replacement server (internal/tmux/confirm_answering_realtmux_test.go:12).
  - Every negative test asserts `sessions.json` and the scrollback are byte-unchanged. That check would fail if the own-server rule were removed, because each fixture's capture, committed, would drop a saved session.
- Notes:
  - The "a different server answers everything" case is covered at three layers: state with a fake, cmd with a fake, and cmd against real tmux. Each layer adds something (the cycle's logic, committer wiring, real tmux pid behaviour), so the overlap is not a defect.

CODE QUALITY:
- Project conventions: Followed. Small interface (`AnsweringConfirmer`), sentinel errors wrapped with `%w`, no `t.Parallel`, real-tmux tests on `tmuxtest` sockets in the unit lane, TMUX poison kept, CLAUDE.md updated for `ConfirmAnswering` and `ServerPIDFromEnv`.
- SOLID principles: Good. A single decision point (`confirmOwnServer`) is shared by the capture confirmation, the failed-capture classification and the dump's empty-capture check.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: One stale field comment (see FINDINGS).

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/state/commit_cycle.go:35 — The `CommitCycle.OwnServer` comment says "zero stands every cycle down". The code contradicts this for a capture that fails for a reason other than a refused read:
  - `classifyFailedCapture` (internal/state/scrollback.go:381-382) returns that capture error unclassified when `ownServer <= 0`;
  - `TestRunCommitCycleWithNoOwnServerReturnsAFailedCaptureUnconfirmed` (internal/state/commit_cycle_confirm_test.go:537) asserts that such an error does not wrap `ErrTmuxStoppedAnswering`.

  Fix: replace the comment's second sentence with "The cycle commits only on a confirmation that server answered; zero commits nothing, and sends no confirmation." — FAILS: a maintainer reading the field doc expects every zero-`OwnServer` cycle to carry `ErrTmuxStoppedAnswering`, and with it the "backed off" log line. A zero-`OwnServer` cycle whose capture fails returns a plain failure and logs "… failed".

UNSETTLED:
- "A server's `session-closed` hook runs `commit-now`, and that server answers its capture reads and its confirmation. The commit is written as before." — The cmd-level tests set `TMUX` by hand (`withOwnTmuxServer`). The criterion also depends on tmux's `run-shell` job environment carrying `TMUX` with the real server pid. Settle it by running `go test -tags integration -p 1 ./cmd -run TestKillPathStaysFinalUnderHardenedSavePath`, which retires the daemon so that every kill must be committed by the real hook's `commit-now`.
