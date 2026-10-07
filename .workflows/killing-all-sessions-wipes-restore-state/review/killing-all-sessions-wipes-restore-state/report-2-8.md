TASK: A Waiting Pane Outlasts A Shutdown SIGTERM And Comes Back Still Asking (killing-all-sessions-wipes-restore-state-2-8, tick-f1bea1)

ACCEPTANCE CRITERIA:
1. A restored lazy pane is waiting on its panel, with its transcript at its token-named path. SIGTERM reaches the pane's whole process tree alongside the daemon. The daemon's shutdown flush runs, `_portal-saver` closes, and the `commit-now` its `session-closed` hook runs commits while the server still answers. Throughout, the panel stays up, `@portal-resume-pending` stays set and no recovery tail runs.
2. The server is ended after that `commit-now`. Its exit ends the parked chain on SIGHUP before its recovery tail starts. Afterwards `sessions.json` names the pane's session and the pane's token-named transcript, and that transcript is present with the content it had before the shutdown.
3. At the next restore, the pane's token-named transcript is still referenced and present, and the pane comes back still asking: its panel is shown and its marker is set.

STATUS: complete

SPEC CONTEXT: §3.2 requires every process a waiting pane runs (the draw and the waiter it becomes) to outlast SIGTERM by catching it, so the marker stays set and no recovery tail clears it while tmux still answers. Otherwise a capture builds a record naming the positional path, the dump-less `commit-now` fired by `_portal-saver`'s close writes it, and its housekeeping pass deletes the token-named transcript. When tmux finally exits, the pty close delivers SIGHUP, which ends the parked shell before its tail runs. §2.2 says no commit can follow the server's exit. §6.2 bullet 4 names this end-to-end scenario, and bullet 6 names the SIGHUP leg. The 2026-10-06 corrigenda changed the re-file to a hard link and added the answered-pane hold. Because of the hold, sessions.json naming the token path no longer tells a set marker from a cleared one on its own, so the marker has to be asserted directly. The test does that.

IMPLEMENTATION:
- Status: Implemented (test-only task; the behaviour under test was built by Tasks 2.2–2.5)
- Location: cmd/state_shutdown_waiting_pane_integration_test.go:53-117 (`TestShutdown_WaitingPaneOutlastsSIGTERMAndComesBackStillAsking`). The only file in commit c08afb661, unchanged since.
- Notes:
  - Fixture (`newWaitingPaneRuntime`, :128-154):
    - Isolates state with `portaltest.IsolateStateForTest`. HOME is moved to a temp dir and XDG_CONFIG_HOME is cleared, so no `prefs.json` exists and the hook takes the shipped lazy default.
    - Seeds a one-pane session whose record carries `PortalPaneID` and names the positional transcript, plus a hooks.json entry keyed on that token (:156-179).
    - Bootstraps through the built binary (`portal list`) and waits for the panel and the marker.
  - `awaitWaitingPaneSaved` (:200-225) waits until sessions.json names the token-named path and the positional name has been removed, then pins the bytes. That sets up the starting state AC1 requires.
  - Subtest 1 (:67-93), AC1:
    - Signals: SIGTERM to each process of the pane's tree, then to the daemon. The tree is walked from the fixture pane's `pane_pid` and must settle to exactly two processes, the parked sh and `resume-wait` (:298-315).
    - Waits for: the daemon to exit, the saver to close, and a `commit-now` that exits with code 0. A commit-now that stood down exits non-zero through `failCommitNow` (cmd/state_commit_now.go:146-152), so code 0 is a real proxy for a commit. It then confirms the server still answers.
    - Asserts: `flush_completed=true`; every tree pid is still alive; the panel title is on screen; the marker is set; no `resume-recover` start line and no marker-clear WARN appear in the log; sessions.json names the token path and the bytes are unchanged.
    - The strings it matches exist in production: `Resume session` (internal/tui/resume_panel.go:9), `flush_completed` (cmd/state_daemon.go:393), `unset resume pending marker failed` (cmd/state_resume_recover.go:70), and the `args` attribute rendering (internal/log/init.go:46).
  - Subtest 2 (:95-108), AC2: SIGTERM to the server. It waits for the tree pids and the server to exit and for the state directory to settle. It then asserts no recovery-tail or marker-clear lines since its mark, and the same saved record and bytes.
  - Subtest 3 (:110-116), AC3: starts a new server on the same socket and runs a full bootstrap. It then waits for the panel and the marker and asserts the token path is still named and the bytes are unchanged.
  - Every signal goes to a pid the fixture started: the pane tree, the saver's daemon, and the server, each read from the test socket. The test is integration-tagged. This follows the CLAUDE.md lane rule and its process-isolation rule.
  - The task's Context pointed at `internal/restore` fixtures. The test lives in `cmd` instead, reusing `symptomFixture`. That is a sound choice: the scenario needs a real daemon, its saver close and the hook's `commit-now`, which the `cmd` integration fixtures already drive.

TESTS:
- Status: Adequate
- Coverage:
  - Each AC maps to one staged subtest, and each part of each AC has a matching assertion.
  - The per-pid liveness check in subtest 1 is stricter than "the panel stays up". It fails on a waiter that died and was redrawn by `parkedChainDraw`'s loop. That strictness matches §3.2's requirement that the waiter itself outlasts SIGTERM.
  - "On SIGHUP" (AC2) is observed by its effects only: the tree ends after the server's exit with no tail start line and no clear attempt. The terminating signal of a process the test did not fork cannot be read, so this is the observable form.
- Notes:
  - The subtests are sequential stages over one fixture, so subtest 3 cannot be run on its own. The same staged pattern is already established in internal/restore/lazy_resume_panel_integration_test.go, so this is not reported.
  - Nothing is over-tested: no redundant assertions and no mocking. Real tmux, a real daemon and the real hook drive everything.

CODE QUALITY:
- Project conventions: Followed. Integration tag, `IsolateStateForTest`, `RegisterStateDirTeardownGuard` placed before the socket, `portalbintest` staging, exact pane targets (`tmux.PaneTargetExact`), bounded `harnesstest.PollUntil` waits, no `t.Parallel`.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (`strings.FieldsSeq`, `slices.ContainsFunc`, `strings.CutPrefix`)
- Readability: Good. Helper names state the observation each one makes, and failures carry the state-dir, tmux and portal.log diagnostic.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "A restored lazy pane is waiting on its panel, with its transcript at its token-named path. SIGTERM reaches the pane's whole process tree alongside the daemon. The daemon's shutdown flush runs, `_portal-saver` closes, and the `commit-now` its `session-closed` hook runs commits while the server still answers. Throughout, the panel stays up, `@portal-resume-pending` stays set and no recovery tail runs." — run `go test -tags integration -p 1 ./cmd -run TestShutdown_WaitingPaneOutlastsSIGTERMAndComesBackStillAsking` (repeat with `-count=N` for timing stability) and observe subtest 1 pass
- "The server is ended after that `commit-now`. Its exit ends the parked chain on SIGHUP before its recovery tail starts. Afterwards `sessions.json` names the pane's session and the pane's token-named transcript, and that transcript is present with the content it had before the shutdown." — the same run; observe subtest 2 pass
- "At the next restore, the pane's token-named transcript is still referenced and present, and the pane comes back still asking: its panel is shown and its marker is set." — the same run; observe subtest 3 pass
