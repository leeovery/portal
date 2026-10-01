TASK: Keep the Waiting Pane's Terminal From Echoing Between the Paint and the Wait (lazy-resume-on-attach-19-1, tick-acd4ec, commit 748933678)

ACCEPTANCE CRITERIA:
- A waiting pane where `d` is pressed and input then streams in past the discard drain's one-second bound comes up on the waiting panel with both its `Resume session` title and its `can't clear pending input: …` report row on screen, with no streamed line printed over the card. It stays that way once the stream ends, with no resize or answer needed. The hook has not run, `hooks.json` is byte-identical, `@portal-resume-pending` stands, and the waiter holds the pane.
- On every screen the chain draws (first draw, discard confirmation, Escape back to the panel, a report, a resize redraw) input reaching the pane after the draw's echo step puts nothing on screen: a stream the drain gave up on, keys between the paint and the waiter's raw mode, and a late reply to the appearance query alike.
- The draw's echo step turns echo alone off; a terminal already without echo comes out unchanged.
- A draw whose echo step is refused logs one WARN under the `hydrate` component, and still paints and hands the pane to the waiter.
- Enter and `y` hand the hook or shell exactly the modes they hand on today (the pre-first-draw modes with signal generation back on); the answer's echo step turns echo alone back on and its signal step still turns signal generation alone back on.
- An answer whose echo step is refused logs the refusal and still hands the pane on.
- An Enter or `y` whose marker clear is refused brings the panel back with its report and takes no echo step.
- A waiter that ends unanswered leaves the user's shell an echoing terminal, through the recovery tail's cooked modes or the backstop's `stty sane`, both as today.

STATUS: complete

SPEC CONTEXT: The specification's swallow rule says nothing the user did not send may answer a waiting pane. It also covers the panel's degraded rendering, and its corrigenda cover the appearance-query reply as input that the drop must clear (probe first, then drop). It does not mention terminal echo. This task is a cycle-3 review fix for a display defect. On the discard drop route, the tty was still echoing between the paint and the waiter's raw mode, so streamed input scrolled and overprinted the card. Task 17-2 had narrowed the burst suite to accept that result. The fix turns ECHO off at the start of each draw and turns it back on only where an answer hands the pane on, the same way the chain already handles ISIG.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_draw.go:27-29 — new `DisableEcho` seam, documented as running ahead of the appearance query and the drop.
  - cmd/state_resume_draw.go:44-47 — the step is the first thing `runResumeDraw` does after resolving the logger. It comes before `cfg.Size` and `cfg.ResolveTheme` (which runs the probe and the drop). A refusal logs a WARN with `hook_key`/`pane_key`/`error` through `cfg.Logger`, and the draw continues.
  - cmd/state_resume_draw.go:178 — wired to `clearStdinEcho`.
  - cmd/tty_signals.go:22-30, 64-70 — `clearTTYEcho`/`setTTYEcho` and their stdin forms sit beside the ISIG siblings. They go through `updateTTYModes`, which writes with TIOCSETA/TCSETS (cmd/tty_termios_darwin.go, cmd/tty_termios_linux.go). Those are immediate writes, so no flush is added.
  - cmd/state_resume_wait.go:77-79 — new `EnableEcho` seam. It is called at :314 (`resumeAnswerEnter`) and :345 (`resumeAnswerDiscard`), after `cfg.restore()`, beside `enableTTYSignalsOrLog`, and before `handOffToHookOrShell`. Wired to `setStdinEcho` at :497.
  - cmd/state_resume_chain.go:213-217 — `enableTTYEchoOrLog` logs a refusal at WARN and returns, so the hand-on still runs.
  - internal/restore/lazy_resume_burst_integration_test.go:172, :238-256 — the drop-route subtest now requires `panelTitle` as well as `burstDropReport`. `assertPasteAnsweredNothing` takes variadic rows and also fails on any pasted line on screen. Its doc no longer says echoed input can scroll the title away.
- Notes:
  - Every guard condition in the task holds:
    - `ttyDrain.drain` is untouched (cmd/tty_drain.go:31-68). It restores the modes it found, which now have echo off.
    - Hydrate's `clearStdinSignals` and the waiter's ISIG-only `setStdinSignals` are unchanged.
    - The recovery tail's `cookStdin` (cmd/state_resume_recover.go:107) and the backstop's `stty sane` (cmd/state_hydrate.go:305) are unchanged.
    - Nothing was added to `resumeWaitLoop`, and the carried-arrival swallow is as it was.
    - No new flush, drop or exec site.
    - The waiter's file names none of the draw's route. `setStdinEcho` is not on `TestRunResumeWait_Waiting`'s forbidden list.
  - The redraw routes take no echo step: the refused-clear `resumeUnfreeze` → `resumeReport`, `resumeRedraw`, `resumeShowScreen` and `resumeOpenDiscardConfirm`.
  - I enumerated every `handOffToHookOrShell` call site. There are four in production: two answers (cmd/state_resume_wait.go:316, :347), the recovery tail (cmd/state_resume_recover.go:62, which cooks the tty) and hydrate's eager path (cmd/state_hydrate.go:251, which runs before any draw has touched echo). No route hands a pane on with echo still off.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/state_resume_echo_test.go `TestRunResumeDraw_Echo`:
    - The echo step runs exactly once, before the appearance query (`echoOffsAtResolve`), and before the exec on the first draw, the confirmation (with DropInput), a report and a resize redraw.
    - A refused step leaves exactly one WARN carrying pane_key/hook_key, a non-empty paint and one exec.
  - cmd/state_resume_draw_screen_test.go:117 pins the full order: echo-off, size, theme, alt-screen, cursor-home, paint, exec.
  - `TestResumeWait_EchoOnHandingThePaneOn`:
    - For Enter and `y`, echo goes on once, after one raw restore, before the exec, with signals also on once.
    - A refused echo-on still execs and logs a WARN carrying the pane keys.
  - `TestResumeWait_EchoStaysOffAcrossScreens` checks that each of these redraws once and takes no echo step: `d`, a refused clear on Enter, a refused clear on `y`, a refused discard, Escape on the confirmation, and a resize.
  - cmd/tty_modes_pty_test.go, on a real PTY:
    - Clearing echo flips ECHO alone (all of Lflag/Iflag/Oflag/Cflag/Cc compared), and is idempotent.
    - Setting echo flips ECHO alone.
    - With echo cleared, typed input displays nothing.
    - A `term.MakeRaw`/`Restore` round trip keeps echo off.
    - The production `DisableEcho`/`EnableEcho` wiring acts on stdin as specified.
    - Composed hydrate signal-off → draw echo-off → raw → restore → signals-on → echo-on equals the original modes with ISIG.
  - The burst integration subtest covers the end-to-end drop-route criterion.
  - The unanswered-waiter criterion is covered by the existing `TestCookTTY_RealPTY` ("typed input echoes once the terminal is cooked") and the existing backstop tests. Both routes are unchanged.
- Notes: Each test would fail if its behaviour broke: echo-off moved after the query, echo-on before the restore, echo-on added to a redraw, either seam wired to the wrong helper, or the helpers touching a mode other than ECHO. The draw-side table runs the same unconditional step four times, once for each screen the criterion lists. That is cheap and matches the criterion, not bloat.

CODE QUALITY:
- Project conventions: Followed. The new seams are injected through config structs and wired once in the cobra RunE. The helpers sit with their siblings. Log messages use the existing `hook_key`/`pane_key`/`error` attr keys under the `hydrate` component. No `t.Parallel`. Seam staging goes through `withFuncSeam`.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The new comments hold against the code, and none cite process artifacts.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "A waiting pane where `d` is pressed and input then streams in past the discard drain's one-second bound comes up on the waiting panel with both its `Resume session` title and its `can't clear pending input: …` report row on screen, with no streamed line printed over the card. It stays that way once the stream ends, with no resize or answer needed. The hook has not run, `hooks.json` is byte-identical, `@portal-resume-pending` stands, and the waiter holds the pane." — Settling this needs a run of `go test -tags integration -p 1 ./internal/restore -run TestLazyResumePaste_NeverAnswersAWaitingPane`, on real tmux with a streamed paste. Its subtest "the rest of a paste the drain gave up on answers nothing" must pass with the restored title assertion and the new no-pasted-line check.
