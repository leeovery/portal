TASK: A Stray Paste Never Answers a Waiting Pane (tick-219d19, lazy-resume-on-attach-17-2)

ACCEPTANCE CRITERIA:
1. A pane showing the waiting panel receives a paste that carries line breaks and no `d`, delivered through a tmux buffer (line feeds arrive as carriage returns). The pane stays on the waiting panel: its hook does not run, `hooks.json` is byte-identical, `@portal-resume-pending` stands, and the waiter still holds the pane.
2. A paste carrying a `y` arrives while the discard confirmation is already on screen. It removes nothing: `hooks.json` is byte-identical, the marker stands, and the pane stays waiting.
3. The discard's input drain gives up on a `d`-led multi-line paste still arriving past its one-second bound, and the waiting panel comes up reporting `can't clear pending input: …`. The rest of that paste neither resumes nor discards: the hook does not run, the registration and the marker stand, and the pane stays on the panel with its report.
4. After any of those pastes, an Enter typed at the pane resumes it (marker clears, hook starts over the transcript); a typed `d` opens the confirmation, where a typed `y` discards and Escape backs out — exactly as on a pane that received no paste.
5. A key tmux injects as if typed (`send-keys Enter`, `send-keys d`) answers the waiting panel exactly as the typed key does.
6. The burst suite's three pastes (2,565- and 4,005-byte multi-line, 4,004-byte single-line) still never confirm, and on every screen the suite accepts as a paste's outcome, the drop-report panel included, it requires that the subject's hook did not run and the marker still stands.

STATUS: issues_found

SPEC CONTEXT: Section 4.3 (as corrected 2026-09-30): nothing the user did not send may answer the panel; a stray paste cannot answer it, including with the line breaks it carries, since tmux delivers a pasted line feed as the carriage return Enter sends; an errant send-keys is byte-for-byte a keystroke and answers as one. The 2026-09-22 corrigendum names the failure being guarded against: a stray paste spending the offer and unfreezing the pane's scrollback with nobody watching. Section 4.3's burst paragraph keeps the discard's guard: input already in flight when the confirmation opens is dropped, never read as agreement. Section 5.2's late OSC 11 reply is input the same rule must swallow.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_wait.go:34-38 (resumeInputQuiet, 50ms)
  - cmd/state_resume_wait.go:139-179 (answerTo, resumeArrival add/open/end)
  - cmd/state_resume_wait.go:185-233 (resumeWaitLoop: arrival accounting, carried arrival, redraw hand-on)
  - cmd/state_resume_wait.go:265-298 (resumeReadRequest, startResumeReader with a non-consuming listen)
  - cmd/state_resume_wait.go:424-426 (awaitStdinInput), :458/:471/:482/:505 (flag and seam wiring)
  - cmd/state_resume_draw.go:53-61 (the drop-refusal route marks the waiter's input as arriving), :152/:163/:189
  - cmd/state_resume_chain.go:62-65, :116-118 (InputArriving payload field and argv flag)
  - cmd/tty_drain.go:23-26 (drain settle now named resumeInputQuiet; value unchanged at 50ms)
- Notes:
  - Mechanism. A key answers only when it is the only byte of an arrival: one byte followed by resumeInputQuiet of silence. A non-consuming select listen (awaitTTYInput) judges the silence, so no read is outstanding when an answer runs, and input typed during the answer is inherited by the exec'd program (the fix round's concern is resolved). Any multi-byte arrival answers nothing, whatever it carries. InputArriving carries an open arrival across the drop-refusal draw and across a resize redraw, so the rest of input the drain gave up on never starts a fresh arrival that could answer. resumeKeysFor's byte map, the drain and hooks.Store.Discard's command match are unchanged (the drain constant was renamed; its value is the same).
  - The removed escape-sequence parser is subsumed by the same rule: CSI/SS3/OSC sequences and Alt chords arrive as multi-byte arrivals and answer nothing, and a lone ESC still backs out of the confirmation. A late OSC 11 reply (Section 5.2) is swallowed the same way.
  - A sound divergence on AC3. A `d`-led paste can no longer open the confirmation at all, because its `d` never arrives alone. So the drain's give-up is reachable only by a typed `d` followed by input that keeps arriving, and the integration subtest drives it that way. This is stronger than the criterion's wording, not a loss.
  - Limits of the timing rule, recorded rather than reported:
    - A one-byte paste ("\r", "\n", "d", "y") is byte-identical to a keystroke on the paste-buffer route and answers. The plan's Outcome says "any size"; the criteria's multi-byte pastes are all honoured.
    - Two keys typed less than 50ms apart answer nothing.
    - An Enter whose quiet window coincides with a settled size change is carried into the redraw and swallowed. A user re-pressing the key recovers in each case.

TESTS:
- Status: Adequate
- Coverage:
  - Unit (cmd/state_resume_wait_paste_test.go). The arrivalHarness drives the AwaitInput seam so the test decides which bytes arrive together, never the clock. Covered:
    - Eight panel pastes (CR, LF, d-led, escape-led) answer nothing, and five confirm-key pastes on the confirmation answer nothing.
    - Enter and `d` typed after every panel paste answer as normal, and `y` and Escape typed after a paste on the confirmation answer as normal.
    - A key answers only after the pane goes quiet, and one read is outstanding at a time.
    - The InputArriving matrix: five keys on two screens are swallowed; a key typed once the pane is quiet answers; a redraw carries an open arrival on and does not carry a closed one.
    - The draw sets the flag for all three drain errors, omits it otherwise, and passes on a carried flag. Both subcommands parse the flag.
    - Keys typed during the Enter and `y` answers are left unread for the next program.
  - Unit (cmd/state_resume_screens_test.go). The escape and Alt-chord coverage was ported onto the arrival harness.
  - Integration (internal/restore/lazy_resume_burst_integration_test.go):
    - TestLazyResumePaste_NeverAnswersAWaitingPane covers AC1-AC5 on real tmux, using paste-buffer and send-keys.
    - TestLazyResumeDiscard_BurstNeverConfirms routes its three pastes through assertPasteAnsweredNothing -> assertUnanswered (:257-274). That check covers the subject's sentinel (discardSubjectFired), the marker, hooks.json byte-identity, the burst sentinel and the waiter.
- Notes:
  - The drop-route assertion was narrowed to the report row (:241-242, :248) to tolerate the echo defect in FINDINGS.
  - The paste matrix is wide, but each case is cheap and deterministic. Not over-tested.
  - Each test would fail if its behaviour broke: dropping the alone rule fails the paste tests, ignoring `carried` fails InputStillArriving, and the draw not setting the flag fails TestResumeInputArriving_Draw.

CODE QUALITY:
- Project conventions: Followed (behaviour behind config seams, withFuncSeam for command seams, no t.Parallel, harnesstest.PollUntil, rendezvous harness instead of sleeps)
- SOLID principles: Good — resumeArrival owns arrival accounting, answerTo owns key selection, the reader goroutine stays single-purpose
- Complexity: Acceptable
- Modern idioms: Yes
- Readability: Good
- Issues: None beyond the finding below. The changed comments hold against the code: runResumeWait, resumeWaitLoop, startResumeReader, resumeReadRequest, resumeArrival, the InputArriving field, the draw's drop-route comment and the drain settle comment.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [spreading] cmd/state_resume_draw.go:56 — On the route where the drain gives up, the draw paints the panel and execs the waiter while the tty is back in the chain's cooked mode.
  - Why the tty echoes: the drain restores the modes it found (cmd/tty_drain.go:47-51). The chain clears only ISIG (cmd/tty_signals.go:14-16), so ECHO, ICANON and ICRNL are on.
  - What happens: input still arriving between the paint and the waiter's MakeRaw is echoed onto the alternate screen, and each pasted CR echoes as a newline that scrolls the card. The waiter then swallows the carried arrival (cmd/state_resume_wait.go:203-207) and never repaints. The panel stays scrolled and overprinted until a resize or an answer.
  - Why it is this task's: before this task the stream's carriage returns resumed the pane, so the garble lasted only until then. This task makes it the route's resting state, and narrows the burst suite to the report row to accept it (internal/restore/lazy_resume_burst_integration_test.go:241-242, :248).
  - Remedy, which has more than one defensible form:
    - hand the pane to a fresh draw of its screen once a carried arrival ends unanswered, or
    - keep echo off across the draw-to-wait hand-over, without letting echo-off reach the hook or shell the answer hands to.
    - Either way, land it together with the drop-route subtest asserting the panel title is back on screen.
  — FAILS: a user who presses `d` and then has more than a second of input stream into the pane (the subtest's streamed paste-buffer) is left with a waiting panel whose title has scrolled away and whose card is overprinted with pasted lines such as "echo 0123456789 >> burst-ran". This reads as though those commands ran, and it stays until the pane is resized.

UNSETTLED:
- "The discard's input drain gives up on a `d`-led multi-line paste that is still arriving past its one-second bound, and the waiting panel comes up reporting `can't clear pending input: …`." — Reaching the give-up depends on real tmux delivery timing: a paste-buffer loop must keep the drain from seeing 50ms of quiet for its full one-second bound. To settle it, run `go test -tags integration -p 1 ./internal/restore -run TestLazyResumePaste_NeverAnswersAWaitingPane` and see the "the rest of a paste the drain gave up on answers nothing" subtest pass.
