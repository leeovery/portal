# Review Tasks: Lazy Resume On Attach (Cycle 3)

## Task 1: Keep the waiting pane's terminal from echoing between the paint and the wait
severity: medium
sources: 17-2-1

**Problem**: Sometimes the discard's input drain gives up because input keeps arriving (`errInputKeptArriving`). When it does, `runResumeDraw` paints the panel with its `can't clear pending input` report row and execs the waiter (`cmd/state_resume_draw.go:56-75`) while the tty is in the chain's cooked mode. The drain puts back the modes it found (`cmd/tty_drain.go:47-51`), and the chain clears only ISIG (`cmd/tty_signals.go:14-16`), so ECHO, ICANON and ICRNL are on. Input that arrives after the paint and before the waiter's raw mode is echoed onto the alternate screen, and each pasted carriage return echoes as a newline that scrolls the card. The waiter then swallows the carried arrival (`cmd/state_resume_wait.go:203-207`) and never repaints, so the panel stays scrolled and overprinted until a resize or an answer.

Take a user who presses `d` while more than a second of input streams into the pane. They are left with a waiting panel whose title has scrolled away and whose card is overprinted with pasted lines such as `echo 0123456789 >> burst-ran`, which reads as though those commands ran. Task 17-2 made this the drop route's resting state, and narrowed the burst suite to the report row to accept it (`internal/restore/lazy_resume_burst_integration_test.go:241-242`, `:248`).

The cooked window is not specific to the drop route. The waiter's raw-mode restore puts back the echoing tty it found, so every draw-to-wait hand-over leaves the pane echoing from the paint until the next waiter's raw mode. That covers the first draw, the confirmation, cancel, a report and a resize redraw.

**Solution**: Turn echo off at the source instead of repainting after it. This is a judgment call between the two remedies the review named. A repaint once a carried arrival ends unanswered repairs only the drop route, and only after the stream has garbled the card for as long as it lasts; it also leaves every other hand-over's cooked window echoing. Echo-off closes the window on every route. Its cost is that echo must come back on wherever an answer hands the pane to a hook or a shell. That is two call sites, and it mirrors how the chain already treats ISIG, which is cleared for the wait and restored at the hand-on. The settled shape:

- **The draw step.** A new step in `runResumeDraw` clears ECHO alone on the pane's tty, ahead of the appearance query.
  - At that position, the probe's raw-mode restore and the drain's restore each put back echo-off, so nothing later in the draw reopens the window. A late OSC 11 reply that lands after the paint cannot echo either.
  - Anything echoed before the step is overwritten by the full-pane paint.
  - The step is best-effort. A failure logs a WARN under the `hydrate` component and the draw still paints, because a panel over an echoing tty is today's behaviour and a pane with no panel is worse.
- **Through the chain, with no further code.** The waiter's raw mode records echo-off as the mode it found, and its restore puts it back. Every redraw (confirmation, cancel, report, resize) therefore re-enters the draw with echo already off, and the draw step is idempotent there.
- **The answer step.** A new waiter seam sets ECHO alone back on.
  - It runs on the two paths that hand the pane on, `resumeAnswerEnter` and `resumeAnswerDiscard`. It runs after `cfg.restore()`, beside `enableTTYSignalsOrLog`.
  - A failure is logged and does not stop the hand-on, as the signal step's failure is handled.
  - The modes an answer hands to the hook or shell are therefore exactly the ones it hands on today: the chain's modes plus ISIG.
  - When a refused marker clear brings the panel back (`resumeUnfreeze` → `resumeReport`), that is a redraw, not a hand-on, so it takes no echo step.
  - The recovery tail (`cookTTY`, wired at `cmd/state_resume_recover.go:107`) and the shell backstop (`stty sane`) already restore echo and are unchanged.
- **No repaint.** Nothing is added to `resumeWaitLoop`, and the carried-arrival swallow stays as it is. The repaint guard (go through `resumeRedraw`, no `DropInput`, no second flush, per `TestResumeDropInput_HandOffs` and `TestFlushTTYInput_TouchesTheInputQueueInExactlyOnePlace`) is therefore moot, and no new flush or drop is introduced.
- **Guard conditions carried.**
  - Echo-off is a new draw step, never a change to the drain (`TestTTYDrain_RealPTY`) or to hydrate's ISIG-only `DisableTTYSignals`.
  - Echo is restored through its own seam, never by widening the waiter's ISIG-only `EnableTTYSignals` (`TestResumeHandOffs_TerminalModes_RealPTY`).
  - Exec stays behind `resumeHandOff`/`execHandOff` (`TestExecSeamsAreCalledOnlyByTheHandOffHelpers`).
  - `state_resume_wait.go` names none of the draw's route (`TestRunResumeWait_Waiting`), so the new tty helpers sit with their siblings in `cmd/tty_signals.go`.
  - Echo can be observed only on a real PTY, and `cmd/tty_modes_pty_test.go` already drives one.
- **The narrowed assertion.** The drop-route subtest ("the rest of a paste the drain gave up on answers nothing") goes back to requiring the panel title as well as the report row. `assertPasteAnsweredNothing`'s doc loses the clause saying echoed input can scroll the title away, which the fix makes false.

**Outcome**: A waiting pane's panel stays whole while input streams into it, on every route that draws it, the drop route's report panel included. The hook or shell an answer hands the pane to gets the same terminal modes it gets today.

**Acceptance Criteria**:
- [ ] A waiting pane where `d` is pressed and input then streams in past the discard drain's one-second bound comes up on the waiting panel with both its `Resume session` title and its `can't clear pending input: …` report row on screen, with no streamed line printed over the card. It stays that way once the stream ends, with no resize or answer needed. The hook has not run, `hooks.json` is byte-identical, `@portal-resume-pending` stands, and the waiter holds the pane.
- [ ] On every screen the chain draws — the first draw at restore, the discard confirmation, Escape back to the panel, a report, a resize redraw — input reaching the pane after the draw's echo step puts nothing on screen. That holds alike for a stream the drain gave up on, for keys arriving between the paint and the waiter's raw mode, and for a reply to the appearance query landing after the paint.
- [ ] The draw's echo step turns echo alone off: a terminal reaching it with echo on leaves with ECHO cleared and every other local, input and output mode as it arrived. A terminal reaching it with echo already off, as every redraw does, comes out unchanged.
- [ ] A draw whose echo step is refused logs one WARN under the `hydrate` component, and still paints its screen and hands the pane to the waiter.
- [ ] Enter on the waiting panel and `y` on the discard confirmation hand the hook or shell exactly the terminal modes those answers hand on today: the modes the pane's terminal held before its first draw, with signal generation turned back on. The answer's echo step turns echo alone back on, and its signal step still turns signal generation alone back on.
- [ ] An answer whose echo step is refused logs the refusal and still hands the pane to its hook or shell.
- [ ] An Enter or `y` whose marker clear is refused brings the panel back with its report and takes no echo step, so the redrawn panel stays whole under further input.
- [ ] A waiter that ends unanswered — killed while the pane waits — leaves the user's shell a terminal that echoes: through the recovery tail's cooked modes, or, where the tail cannot start, through the backstop's `stty sane`, both as they are today.

**Do**:
- `cmd/state_resume_draw.go`, `runResumeDraw`: add the echo step ahead of `cfg.ResolveTheme`, inside which both the appearance query and the input drop run. It clears ECHO alone on the pane's tty. A refusal logs a WARN through `cfg.Logger` (the `hydrate` component), and the draw goes on to paint and hand off.
- `cmd/state_resume_wait.go`: add a `resumeWaitConfig` seam that sets ECHO alone back on, wired in `stateResumeWaitCmd`. `resumeAnswerEnter` and `resumeAnswerDiscard` call it after `cfg.restore()`, beside `enableTTYSignalsOrLog` and before `handOffToHookOrShell`. A refusal is logged and the hand-on proceeds, as `enableTTYSignalsOrLog` treats the signal step. The refused-clear route (`resumeUnfreeze` → `resumeReport`), `resumeRedraw`, `resumeShowScreen` and `resumeOpenDiscardConfirm` take no echo step.
- `cmd/tty_signals.go`: the new echo-clear and echo-set tty helpers sit here beside `clearTTYSignals`/`setTTYSignals`, since `state_resume_wait.go` names none of the draw's route (`TestRunResumeWait_Waiting`).
- Leave unchanged:
  - `ttyDrain.drain` (`cmd/tty_drain.go`, `TestTTYDrain_RealPTY`);
  - hydrate's ISIG-only `DisableTTYSignals` (`clearStdinSignals`);
  - the waiter's ISIG-only `EnableTTYSignals` (`setStdinSignals`, pinned by `TestResumeHandOffs_TerminalModes_RealPTY`) — echo comes back through its own seam, never by widening this one;
  - the recovery tail's `cookTTY` (wired at `cmd/state_resume_recover.go:107`) and the shell backstop's `stty sane` (`cmd/state_hydrate.go:305`).
- Exec stays behind `resumeHandOff`/`execHandOff` (`TestExecSeamsAreCalledOnlyByTheHandOffHelpers`).
- No repaint. Nothing is added to `resumeWaitLoop`, the carried-arrival swallow (`cmd/state_resume_wait.go:203-207`) stays as it is, and no new flush or drop is introduced (`TestResumeDropInput_HandOffs`, `TestFlushTTYInput_TouchesTheInputQueueInExactlyOnePlace`).
- Echo is observable only on a real PTY, and `cmd/tty_modes_pty_test.go` already drives one.
- `internal/restore/lazy_resume_burst_integration_test.go`: the drop-route subtest "the rest of a paste the drain gave up on answers nothing" (:163-173) requires `panelTitle` as well as `burstDropReport`. `assertPasteAnsweredNothing`'s doc (:238-242) loses the clause saying input the tty echoed can scroll the panel's title away.
