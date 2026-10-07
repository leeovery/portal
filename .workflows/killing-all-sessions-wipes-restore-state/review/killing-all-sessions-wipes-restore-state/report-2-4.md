TASK: A SIGTERM Landing As The Draw Or The Waiter Starts Still Leaves The Pane Waiting (killing-all-sessions-wipes-restore-state-2-4, tick-c85754)

ACCEPTANCE CRITERIA:
- At restore, a SIGTERM reaches a waiting pane as its parked shell starts the draw, before the draw has installed its catch. The pane is left waiting, with its marker set and no recovery tail run.
- A SIGTERM reaches a waiting pane as its draw execs into the waiter, before the waiter has installed its catch. The pane is left waiting, with its marker set and no recovery tail run.
- A waiting pane is resized, and a SIGTERM reaches it as its waiter execs back into the draw, before the draw has installed its catch. The pane is left waiting, with its marker set and no recovery tail run.
- A waiting pane is answered on its panel after a SIGTERM landed in one of those windows. Its hook program and the user's shell start with SIGTERM at its default disposition, not inherited as ignored.

STATUS: complete

SPEC CONTEXT: Section 3.2 requires every process a waiting pane runs (the draw and the waiter it becomes) to outlast SIGTERM by a catch, never an ignore, so the pane is saved still waiting with its token-named transcript rather than recovered (marker cleared) while tmux still answers. The 2026-10-05 corrigendum extends this to the moment each process starts, before it installs its catch (catches reset across fork and exec, including every redraw): a SIGTERM there must still leave the pane waiting, with the marker set and no recovery tail run, and how the window is closed is the implementer's. Section 3.3 keeps SIGHUP uncaught so a kill or tmux's exit still ends the pane. Section 5.2, after the 2026-10-07 corrigendum, accepts as residue only the two shell hand-overs (waiter to hook shell, hydrate helper to parked chain), not the draw/waiter windows this task covers. Section 6.2 asks for a test of a SIGTERM landing as the draw or the waiter starts, at restore or at a redraw.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_hydrate.go:341-344 — parkedChainDraw: `while <draw>; [ $? -eq 143 ] && <paneStillPending>; do :; done`. It re-runs the draw when the draw, or the waiter or redraw it exec'd into (the same pid throughout), was ended by SIGTERM and the pane's @portal-resume-pending marker still reads pending.
  - cmd/state_hydrate.go:333 — sigtermStatus = 128 + SIGTERM.
  - cmd/state_hydrate.go:325-330 — paneStillPending: a plain format read of the marker, with a failed read counting as pending. The backstop's answered gate (cmd/state_hydrate.go:295) now shares this helper instead of keeping its own copy.
  - cmd/state_hydrate.go:346-351 — parkedResumeChain composes trap, then draw loop, then recover, then backstop.
  - The steady-state catches are unchanged: cmd/state_resume_chain.go:204-206 (catchSIGTERM, signal.Notify), called as the first statement of cmd/state_resume_draw.go:44 and cmd/state_resume_wait.go:103. Nothing in production code calls signal.Stop, signal.Reset or signal.Ignore, or uses an ignoring `trap ''`.
- Notes: The window is closed at the parked shell, not inside resumeHandOff. This is a sound departure from the task's "Do" pointer. The parent comment states the reason: a Go image cannot catch a signal before its runtime starts. The fix keeps to the caught-never-ignored rule: the parked shell's trap stays `trap : INT QUIT TERM`, and nothing is ignored, so an answered pane's hook and shell keep default SIGTERM handling. I traced each window and found no gap:
  - Parked shell forking the draw: the child resets the caught trap, dies with status 143, and the loop redraws.
  - Draw exec'ing into the waiter, and waiter exec'ing back into the draw at a redraw (the waiter restores the tty before the exec, cmd/state_resume_wait.go:397): the shared pid dies with status 143 and the loop redraws.
  - Group SIGTERM landing on the `$(tmux ...)` command substitution: the read fails, which counts as pending, so the loop redraws (the safe direction).
  - Answered pane: the marker reads back clear, so a SIGTERM-ended user shell falls through to the tail as before and never reaches the panel again.
  - INT/QUIT (statuses 130/131) and SIGHUP still reach the tail or end the shell as before.
  - A restarted draw takes the payload baked at hydrate time, so a pane SIGTERMed mid-confirmation comes back on the panel. That still meets "left waiting", and it happens only at shutdown.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/state_resume_handoff_term_test.go:23-57 defines the three windows: parked shell starting the draw, draw exec'ing into the waiter, and waiter exec'ing back into the draw after a resize. Each is held before its real catch by the harness hook at cmd/state_resume_term_test.go:181-200.
  - cmd/state_resume_handoff_term_test.go:67-80, for each window:
    - awaitEnded proves the SIGTERM actually ended the held process, so the window was reached.
    - A new waiter comes up.
    - assertStillWaiting (cmd/state_resume_term_test.go:412-428) checks the marker was never cleared and the recovery tail never ran.
    - The parked shell is still up.
    This covers AC1-AC3.
  - cmd/state_resume_handoff_term_test.go:82-99 covers AC4 for each window: after the pre-catch SIGTERM and a redraw, Enter hands the hook and the user's shell SIGTERM at its default disposition (assertEndsOnSIGTERM).
  - cmd/state_resume_handoff_term_test.go:102-125 guards the loop's marker gate: an answered pane's shell ended by SIGTERM is not redrawn.
  - The non-143 side of the gate is covered by existing suites, so a loop that dropped the status check would hang them:
    - cmd/state_resume_backstop_test.go: statusChainExe draw statuses 0/1, with the stub tmux reading the marker as set.
    - cmd/state_resume_signals_test.go:43-67: INT/QUIT still reach the tail.
  - The stub tmux added at cmd/state_resume_term_test.go:73-82 keeps the marker read off any real server.
  - Reverting parkedChainDraw to a single draw would fail every new subtest: the tail would run and no second waiter would come up.
- Notes: Each test is focused on one acceptance criterion. Running the answered case once per window matches AC4's wording ("one of those windows") and is not redundant setup.

CODE QUALITY:
- Project conventions: Followed. Shell fragments are built through shellquote and the pane target through tmux.PaneIDTarget. The `@portal-resume-pending` name comes from state.ResumePendingOption. The CLAUDE.md Resume-hooks section describes the draw loop accurately.
- SOLID principles: Good. paneStillPending is one rule shared by the draw loop and the backstop gate.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The comments on parkedChainDraw, paneStillPending and sigtermStatus are accurate against the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
