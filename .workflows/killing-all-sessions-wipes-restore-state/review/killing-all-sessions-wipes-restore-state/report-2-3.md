TASK: The Parked Chain Catches SIGTERM, So A Waiting Or Answered Lazy Pane Keeps Its Session (killing-all-sessions-wipes-restore-state-2-3, tick-2daa73)

ACCEPTANCE CRITERIA:
- A restored lazy pane is waiting, and the pane's top process (its parked shell) receives SIGTERM. The pane and its session stay up, the panel stays up, `@portal-resume-pending` stays set, and no recovery tail runs.
- A restored lazy pane is waiting, and SIGTERM reaches every process in the pane. The pane and its session stay up, the panel stays up, the marker stays set, and no recovery tail runs.
- A lazy pane has been answered on its panel and its hook program is still running. SIGTERM reaches every process in the pane. The parked shell and the shell running the hook both survive, the session is kept, and when the hook program ends the pane goes on to the user's shell.
- An answered lazy pane's hook program, and the user's shell it goes on to, start with SIGTERM at its default disposition, not inherited as ignored.

STATUS: complete

SPEC CONTEXT: Section 3.1 requires both non-interactive-shell panes Portal starts (the eager hook shell and the lazy parked chain) to survive SIGTERM through a caught trap, never an ignored disposition, so the hook program and the user's shell keep default SIGTERM handling; the pane's session then lasts until tmux itself exits, when no committer can reach the server (2.2). Section 3.2 adds that the draw and the waiter catch SIGTERM too (Task 2.2), and 3.3 keeps SIGHUP uncaught so a kill still ends the pane. Section 6.2 lists the tests: a SIGTERM to the top process; one reaching every process in a waiting pane; an answered pane whose hook is still running; and default SIGTERM for the hook program and the user's shell. The 2026-10-07 corrigendum lists the gap between the hydrate helper's exec and the parked chain's first command as accepted residue (5.2).

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_hydrate.go:276-281 (`parkedChainTrap` = "trap : INT QUIT TERM; "), composed into the chain by `parkedResumeChain` at cmd/state_hydrate.go:346-351 and exec'd by `execResumeChainAndExit` at cmd/state_hydrate.go:262-274
- Notes: This is a one-word change. TERM joins INT and QUIT in the parked shell's trap. The action is still `:`, so the trap is caught rather than ignored, and SIGHUP stays out of the list. With the draw and waiter catching SIGTERM (`catchSIGTERM`, cmd/state_resume_chain.go:204) and the hook shell's `trap : TERM;` (cmd/state_resume_chain.go:213), every process in a waiting or answered lazy pane now outlasts SIGTERM. The shell holds its foreground wait on the draw/waiter and runs the trap only once the child exits, so the waiter and the panel are not disturbed. Because the trap is caught, the parked shell's directly exec'd backstop shell (`exec "${SHELL:-/bin/sh}"`) still starts at default disposition. The doc comment on the constant matches the code. CLAUDE.md (Resume hooks section) already describes `trap : INT QUIT TERM`, and no other code still describes the old INT/QUIT-only trap.

TESTS:
- Status: Adequate
- Coverage: cmd/state_parked_chain_term_test.go:29-110 (`TestParkedResumeChain_SIGTERM`) runs the real parked chain from `parkedResumeChain` under /bin/sh. The draw and waiter are the test binary re-executed through the production `runResumeDraw` / `runResumeWait`, and the tmux and tty seams are stubbed.
  - Subtest at line 30 sends SIGTERM to the parked shell only (criterion 1). It asserts the chain is still up, the waiter is alive, no marker clear happened and no recovery ran.
  - Subtest at line 42 sends SIGTERM to the whole process group (criterion 2), with the same assertions.
  - Subtest at line 54 answers with Enter, waits for the hook program, then signals the group (criterion 3). It asserts the user's shell now runs at the waiter's pid, which proves the hook shell survived and exec'd it, the parked shell is up, the user's shell is alive and no recovery ran.
  - Subtest at line 76 sends SIGTERM to the parked shell, then answers, and asserts the hook program and the user's shell both end on SIGTERM (criterion 4).
  - Subtest at line 93 checks SIGHUP still ends the parked shell.
  - The caught (`:`) rather than ignored (`''`) form is pinned by the string assertion in `TestHydrateLazy_ParksTheChainBehindACaughtTrap` (cmd/state_resume_signals_test.go:92-101), updated in this task.
- Each of subtests 1-3 fails if TERM is dropped from the trap, and the SIGHUP subtest fails if SIGHUP is added to it.
- Notes: Nothing material. The existing INT/QUIT group-signal test (cmd/state_resume_signals_test.go:43-67) correctly leaves TERM out of its loop: its stub draw has default disposition, and under the redraw loop a TERM there would redraw rather than recover.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel; waits use harnesstest.PollUntil; cleanup kills the test's own process group only.
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
