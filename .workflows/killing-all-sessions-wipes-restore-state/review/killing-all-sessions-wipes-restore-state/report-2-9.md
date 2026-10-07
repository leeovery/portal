TASK: Signalling Pane Programs And The Daemon Before The Server Preserves Hardened Sessions Against Real Tmux (killing-all-sessions-wipes-restore-state-2-9, tick-9431d8)

ACCEPTANCE CRITERIA:
- A live runtime holds sessions whose panes are interactive shells, restored eager resume panes with their hook programs running, waiting lazy panes, and answered lazy panes with their hook programs running, each with saved scrollback. The pane programs and the daemon are sent SIGTERM, then the server. Afterwards `sessions.json` names every one of those sessions, and every scrollback file it names is present and non-empty. (§3.1, §3.2, §6.5)
- In that run, each waiting lazy pane's record still names its token-named transcript. (§3.2, §6.5)
- Do: run in the integration lane, on real tmux with an isolated socket.

STATUS: complete

SPEC CONTEXT: §1.2 names sessions dying before tmux as the second root cause: the shutdown SIGTERM reaches pane programs independently of the server, and a session whose programs exit on it closes while tmux still answers, so the session-closed commit-now and the daemon flush drop it with its scrollback. §3.1 hardens the eager hook shell, the lazy parked chain and the hook shell under an answered lazy pane with a caught TERM trap. §3.2 makes the waiting pane's draw and waiter catch SIGTERM, so the pane is saved still waiting on its token-named transcript and no recovery tail runs. §5.2 accepts direct-program panes and fish-style shells as residue. §6.5's third ordering is this test: signal the pane programs and the daemon, then the server, and every interactive-shell or hardened-pane session is preserved.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_shutdown_hardened_panes_integration_test.go:1 — `//go:build integration` (integration lane)
  - cmd/state_shutdown_hardened_panes_integration_test.go:56-64 — the fixture: `shell` (no token, no hook), `eager` (Resume: Eager, hook `echo resumed; sleep 600`), `waiting` (lazy by default), `answered` (lazy, same running hook), each with a seeded transcript
  - cmd/state_shutdown_hardened_panes_integration_test.go:66-97 — the ordering: every non-saver pane process gets SIGTERM, then the daemon. The test waits for the daemon to exit, `_portal-saver` to close and the saver's commit-now to exit 0, and asserts every session is still live. Then the server gets SIGTERM, and the test waits for it to exit and for the state dir to settle
  - cmd/state_shutdown_hardened_panes_integration_test.go:107-137 — isolation: `IsolateStateForTest`, per-file env pins, `RegisterStateDirTeardownGuard` after isolate and before `tmuxtest.New`, and its own `-S` socket
  - cmd/state_shutdown_hardened_panes_integration_test.go:141-150 — pins `$SHELL` to zsh or bash, so the shell session and every `exec $SHELL` hand-off are shells that ignore SIGTERM
  - cmd/state_shutdown_hardened_panes_integration_test.go:217-245 — before any signal, waits until the eager and answered panes are running `sleep 600` (the hook program), the shell pane is a bare shell, and the waiting pane is down to parked shell plus waiter
  - cmd/state_shutdown_hardened_panes_integration_test.go:260-282 — waits for a saved commit with exactly one non-empty scrollback file per seeded session
  - cmd/state_shutdown_hardened_panes_integration_test.go:286-307 — walks pane trees only from `list-panes -a` on the fixture's own socket, so it signals only processes the test started
  - cmd/state_shutdown_hardened_panes_integration_test.go:322-347 — AC1 check: sessions.json names exactly the four seeded sessions, and every file it names is present and non-empty
  - cmd/state_shutdown_waiting_pane_integration_test.go:243-257 (`assertSavedWaiting`, reused at hardened :96) — AC2 check: the waiting pane's record names `state.PendingScrollbackFile(waitingPaneToken)`, and that file still holds the bytes captured before shutdown
- Notes: The production behaviour being exercised is the one CLAUDE.md and the spec describe: `hookShellTrap` at cmd/state_resume_chain.go:213, `catchSIGTERM` at cmd/state_resume_chain.go:204, `parkedChainTrap` at cmd/state_hydrate.go:281, and the answered pane's hand-off through `hookExecArgs` at cmd/state_resume_chain.go:178. The hooks.json body goes through `Registration.MarshalJSON` (the string form for no mode, the object form for eager) and is staged via `hookstest.StageStore`'s `Seed`, because the `Entries`/`Body` shapes cannot carry a resume mode. The file is hand-marshalled but not hand-written. The commit (7e617bd41) touches only this test file.

TESTS:
- Status: Adequate
- Coverage: The test is the deliverable. It would fail if the fix regressed:
  - Dropping the TERM trap from the hook shell or the parked chain closes that session while tmux answers. `assertSessionsLive` and the named-set check then fail.
  - Dropping the waiter's SIGTERM catch runs the recovery tail. Once a dump runs, the record moves off the token-named path and `assertSavedWaiting` fails.
  - Each session's scrollback presence is checked both before the shutdown (saved) and after it (named and non-empty).
  - The answered pane's hook-running state is established by Enter on its panel, followed by waiting for the marker to clear and `sleep 600` to appear.
- Notes: It reuses `waitingPaneRuntime`'s helpers by embedding instead of duplicating them. Nothing in it is over-tested: each assertion maps to one criterion or to a precondition the criterion names.

CODE QUALITY:
- Project conventions: Followed. Integration tag; `IsolateStateForTest`; teardown guard in the required order; isolated `-S` socket; `portalbintest.StagePortalBinary` for the binary; no `t.Parallel()`; only fixture-owned processes are signalled.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (`slices`, `maps.Keys`, `strings.SplitSeq`, range-over-int)
- Readability: Good. Comments match the code, and none reference task ids, phases or spec sections.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "A live runtime holds sessions whose panes are interactive shells, restored eager resume panes with their hook programs running, waiting lazy panes, and answered lazy panes with their hook programs running, each with saved scrollback. The pane programs and the daemon are sent SIGTERM, then the server. Afterwards `sessions.json` names every one of those sessions, and every scrollback file it names is present and non-empty." — Reading confirms the test builds this runtime and asserts this outcome. Whether the outcome holds needs the integration test run on real tmux: `go test -tags integration -p 1 ./cmd -run TestShutdown_PaneProgramsAndDaemonThenServerPreservesHardenedSessions`, ideally repeated for stability.
- "In that run, each waiting lazy pane's record still names its token-named transcript." — Same run: the `assertSavedWaiting` check at the end of that test has to pass.
