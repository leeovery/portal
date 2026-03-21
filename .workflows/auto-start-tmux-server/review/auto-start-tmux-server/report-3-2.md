TASK: Timing messages and transition logic

ACCEPTANCE CRITERIA:
- Init() returns tick commands when activePage is PageLoading
- SessionsMsg with sessions during loading + minWaitDone transitions to normal view
- SessionsMsg with no sessions during loading schedules re-fetch after DefaultPollInterval
- MinWaitElapsedMsg + sessionsReceived transitions to normal view
- MaxWaitElapsedMsg transitions unconditionally
- Ctrl+C during loading quits, other keys swallowed
- Polling stops after transition (orphaned polls handled safely)

STATUS: Complete

SPEC CONTEXT: The specification requires session-detection with min/max timing bounds: minimum 1 second (prevents jarring flash), maximum 6 seconds (proceed regardless). Poll interval is 500ms. The TUI path uses its existing refresh cycle plus tick commands. Both MinWaitElapsedMsg and MaxWaitElapsedMsg are integral to the Bubble Tea message loop approach. The spec says the TUI should remain responsive (Ctrl+C quits) during loading.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/model.go:90-94` — MinWaitElapsedMsg and MaxWaitElapsedMsg message types
  - `internal/tui/model.go:143-145` — serverStarted, minWaitDone, sessionsReceived boolean fields
  - `internal/tui/model.go:574-580` — pollSessionsCmd using tea.Tick with tmux.DefaultPollInterval
  - `internal/tui/model.go:582-589` — transitionFromLoading helper
  - `internal/tui/model.go:607-635` — Init() batches fetchSessions + minWaitTick + maxWaitTick when on PageLoading
  - `internal/tui/model.go:654-701` — Update() handles SessionsMsg (loading branch), MinWaitElapsedMsg, MaxWaitElapsedMsg
  - `internal/tui/model.go:724-728` — PageLoading key handler: Ctrl+C quits, all other keys return nil (swallowed)
  - `internal/tmux/wait.go:7-9` — DefaultMinWait=1s, DefaultMaxWait=6s, DefaultPollInterval=500ms constants
- Notes: Implementation closely follows the plan and spec. The transition logic correctly handles all four state combinations (minWaitDone x sessionsReceived). Orphaned messages are safely ignored via the `m.activePage != PageLoading` guard. The pollSessionsCmd correctly embeds the ListSessions call inside the tick callback so sessions are re-fetched, not just re-scheduled.

TESTS:
- Status: Adequate
- Coverage:
  - `model_test.go:7472-7489` — Init returns batch with tick commands when on PageLoading (verifies BatchMsg has >= 3 commands)
  - `model_test.go:7491-7508` — SessionsMsg with sessions + minWaitDone transitions
  - `model_test.go:7510-7524` — SessionsMsg with sessions before minWait does NOT transition
  - `model_test.go:7526-7537` — Empty SessionsMsg during loading returns non-nil command (re-poll)
  - `model_test.go:7539-7556` — MinWaitElapsedMsg with sessionsReceived transitions
  - `model_test.go:7558-7571` — MaxWaitElapsedMsg transitions unconditionally
  - `model_test.go:7573-7600` — Other keys during loading are swallowed (q, p, enter, esc tested)
  - `model_test.go:7457-7470` — Ctrl+C during loading quits
  - `model_test.go:7602-7619` — Orphaned MinWaitElapsedMsg after transition is harmless
  - `model_test.go:7621-7640` — Orphaned MaxWaitElapsedMsg after transition is harmless
  - `model_test.go:7642-7664` — Orphaned poll SessionsMsg after transition updates list normally
  - `model_test.go:7666-7691` — MaxWait with no sessions + projects loaded transitions to PageProjects
  - `model_test.go:7693-7722` — Transition with sessions + projects loaded stays on PageSessions
- Notes: All seven acceptance criteria are covered with dedicated tests. Edge cases (both ordering permutations of min-wait vs sessions-received, orphaned messages, no-sessions-to-projects-page fallback) are well tested. Tests verify behavior, not implementation details. Test count is proportional to the combinatorial complexity of the state machine — not over-tested.

CODE QUALITY:
- Project conventions: Followed — table-driven tests used where appropriate, functional options pattern, exported types documented, interfaces are small and focused
- SOLID principles: Good — the transition logic is cleanly separated into transitionFromLoading(), the state machine variables (minWaitDone, sessionsReceived) have clear single responsibilities, the polling command is extracted as pollSessionsCmd()
- Complexity: Low — the Update() switch handles each message type linearly; the loading branch in SessionsMsg has a simple 2-boolean state check; no deeply nested conditionals
- Modern idioms: Yes — proper use of tea.Tick for time-based messages, tea.Batch for Init() command composition, type switch for message dispatch
- Readability: Good — message types are clearly named, boolean fields self-document their purpose, comments explain intent (e.g., "No sessions yet -- schedule a re-poll")
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The re-poll test (line 7526) only checks `cmd != nil` but does not verify the returned command would produce a SessionsMsg after the poll interval. This is reasonable given tea.Tick is a framework function, but a more thorough test could execute the command and assert the message type. Minor — the current approach is pragmatic and avoids testing framework internals.
