TASK: The Panel's Draw And Its Waiter Catch SIGTERM And Keep The Pane Waiting (killing-all-sessions-wipes-restore-state-2-2, tick-c4300a)

ACCEPTANCE CRITERIA:
- A lazy pane is waiting on its panel, and its waiter receives SIGTERM. The panel stays up, `@portal-resume-pending` stays set on the pane, and no recovery tail runs.
- A lazy pane's panel is still being drawn when its draw receives SIGTERM. The pane is left waiting on its panel, exactly as for a SIGTERM landing on the waiter: the marker stays set and no recovery tail runs.
- A waiting pane is resized, so its panel is redrawn. A SIGTERM to the redrawn panel's waiter leaves the pane waiting, with its marker set and no recovery tail run.
- A waiting pane's waiter has caught a SIGTERM, and the user then answers the panel. The hook program ends on SIGTERM with default handling, and the user's shell the pane goes on to starts with SIGTERM at its default disposition, not inherited as ignored.

STATUS: complete

SPEC CONTEXT: Section 3.2: while a lazy pane waits, its parked shell runs the draw, which execs into the waiter. If a reboot's SIGTERM ends the waiter, the parked chain runs its recovery tail and `resume-recover` clears `@portal-resume-pending` while tmux is still answering. A later capture then builds the pane a fresh record naming a positional scrollback path that no longer exists, a dump-less `commit-now` writes it, and housekeeping deletes the token-named transcript. Both the draw and the waiter must outlast SIGTERM by catching it, never by ignoring it, so the hook program and the user's shell keep default SIGTERM handling across exec. Section 3.3: SIGHUP is not caught, so a kill (pty close) still ends the pane. Section 6.2 lists the matching tests. The pre-catch window at process start belongs to Task 2.4, the parked shell's trap to Task 2.3, and the pty-close SIGHUP to Task 2.5. All three are out of scope here.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_chain.go:199-206: `catchSIGTERM` calls `signal.Notify(make(chan os.Signal, 1), syscall.SIGTERM)`. This is a real catch, so the kernel resets it to SIG_DFL on execve. The relay is never read, and Notify's non-blocking send drops further signals.
  - cmd/state_resume_draw.go:27 adds the `CatchSIGTERM` seam. cmd/state_resume_draw.go:44 makes it the first statement of `runResumeDraw`, and cmd/state_resume_draw.go:185 wires production to `catchSIGTERM`.
  - cmd/state_resume_wait.go:50 adds the `CatchSIGTERM` seam. cmd/state_resume_wait.go:103 makes it the first statement of `runResumeWait`, and cmd/state_resume_wait.go:502 wires production to `catchSIGTERM`.
  - SIGHUP stays at its default. `winchSignals` (cmd/state_resume_wait.go:427-431) still notifies on SIGWINCH alone, and nothing notifies, ignores or resets SIGHUP.
- Notes:
  - Every redraw is a fresh `resume-draw`, and every hand-on after it a fresh `resume-wait`, so each one reinstalls the catch. That covers the resize redraw, the discard-confirmation screens and the refusal reports.
  - An answered pane execs `/bin/sh -c 'trap : TERM; <hook>; exec <shell>'` through `handOffToHookOrShell`. That shell starts with SIGTERM at its default (a Go catch does not survive exec), and its own caught trap resets again for the hook program and the exec'd user shell.
  - A SIGTERM interrupting the waiter's select-based input wait cannot end the wait. `awaitTTYInput` (cmd/tty_drain.go:72-91) retries on EINTR, and so does the appearance probe's `selectBoundedReader`. A caught signal therefore never surfaces as a read error that would send the chain to its recovery tail.
  - Production code is unchanged since the task commit 99f2ea4b4. The one later edit to state_resume_chain.go is the `hookExecArgs` comment.

TESTS:
- Status: Adequate
- Coverage:
  - `TestResumeWaitingPane_SIGTERM` (cmd/state_resume_term_test.go:464-543) runs the real parked chain. The draw and the waiter are this test binary re-executed, with the tmux and terminal seams faked and events recorded to a file. Its subtests map one-to-one onto the criteria:
    - "a waiter keeps the pane waiting through a SIGTERM" covers criterion 1.
    - "a draw keeps the pane waiting through a SIGTERM landing mid-draw" covers criterion 2. It holds the draw inside `ResolveTheme`, after the catch, then releases it and checks that the waiter takes over the same pid and is still waiting.
    - "a waiter redrawn after a resize..." covers criterion 3, using SIGWINCH plus a size-file change and checking the redraw and the waiter keep the same pid.
    - "a pane answered after its waiter caught a SIGTERM..." covers criterion 4. The hook program ends on SIGTERM, and the user's shell runs as the pane's own pid and ends on SIGTERM.
    - The two "still ends on SIGHUP" subtests cover section 3.3.
  - `assertStillWaiting` (cmd/state_resume_term_test.go:412-428) checks four things: the chain has not exited, the holding pid is still alive, no "cleared" event (from either the waiter's ClearMarker or the stub tmux's `set-option`), and no "recovered" event.
  - `TestResumeWaitingPane_LeavesHangupAndInterruptAtTheirDefault` (cmd/state_resume_wait_test.go:479) is a source guard. It covers state_resume_wait.go, state_resume_draw.go and state_resume_chain.go, refuses `signal.Ignore`/`signal.Reset`, and allows Notify only on SIGWINCH and SIGTERM. This holds the "catch, never ignore" and "SIGHUP stays default" rules structurally.
  - internal/restore/lazy_resume_panel_integration_test.go:585-593 `endWaiterUnanswered` now uses SIGKILL. This is correct: a SIGTERM no longer ends the waiter, and a SIGKILL (status 137) still drives the recovery path that fixture tests.
- Notes:
  - Each behavioural subtest would fail if the catch were removed. The waiter or draw pid dies, so `processAlive` fails even when the later parked-chain redraw loop restarts the draw under a new pid. Without that loop the tail would also record "recovered".
  - Criterion 4's test would fail if the catch were turned into an ignore. The answered pane's `sh` would inherit SIGTERM as ignored and could not reset it, so the hook program would survive.
  - Not over-tested: six subtests, each bound to a distinct criterion or to section 3.3.

CODE QUALITY:
- Project conventions: Followed. The new seam follows the config's existing function-field pattern, every test config literal sets it (no nil-call risk), and the production wiring sits in the cobra RunE.
- SOLID principles: Good. One shared `catchSIGTERM` serves both processes and lives in the chain's shared file.
- Complexity: Low
- Modern idioms: Yes. The Notify channel is buffered, so the sigchanyzer vet check is satisfied.
- Readability: Good. The comments on `catchSIGTERM`, `runResumeWait` and `winchSignals` are accurate against the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
