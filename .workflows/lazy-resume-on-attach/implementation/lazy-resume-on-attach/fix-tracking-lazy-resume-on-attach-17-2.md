## Attempt 1

ISSUES:
- A key the user types right after an answer is now eaten by the waiter instead of reaching the hook, the shell or the next draw. The inherited-bytes guarantee planned for the waiter (phase-4-tasks.md:128, :383) no longer holds.
  - **How it happens.** Once a key's byte is read, the loop head asks for another read at once (`cmd/state_resume_wait.go:199-203`). That read sits blocked on stdin while the quiet window runs. When the window elapses (`:220-223`), the answer runs with the read still pending. That covers the marker clear, the alternate-screen release (tmux polls with 50ms pauses), the store lookup or the discard write, and then the exec.
  - **What is lost.** The first byte typed in that window (tens to over a hundred ms) is consumed by the dead waiter and thrown away at exec. Before this change, an answer left no read pending.
  - **Confirmed.** The reviewer ran a probe on a scratch copy that writes a byte from inside `ClearMarker` during an Enter answer. On the current tree the byte is consumed (3/3 runs). On HEAD's waiter it stays unread for the next program (3/3 runs).
  - **Impact.** Enter followed by typing into the resumed hook drops the first character, and so does `y` followed by typing at the shell. Escape followed by a quick key loses that key. The `d` route is unaffected because its draw drains input anyway.
  - **Secondary effect.** When a byte lands just as the window elapses, `select` picks between the read result and the quiet timer at random. A key that did have a follower can then answer, and the follower is lost.
  - **Stale comment.** The comment on `startResumeReader` (`:273-275`) still claims the next program inherits queued input.
  FIX: Make listening past a key non-consuming.
  1. Give the reader goroutine a second kind of request. While an arrival is open, it waits up to `resumeInputQuiet` for stdin to become readable, using a `select` on the fd. `awaitTTYInput` in `cmd/tty_drain.go:71` already does exactly this. It reads the byte only if one is ready, and otherwise reports "quiet". With no arrival open, it keeps today's blocking read.
  2. Drive the quiet branch from that "quiet" result rather than the `cfg.Settle(resumeInputQuiet)` timer. A lone key is then judged with nothing reading the tty, and bytes typed during the answer stay queued for the exec'd program. This also removes the `select` race.
  3. Put the readiness wait behind a seam on `resumeWaitConfig`: stdin's fd via `awaitTTYInput` in production, a fake in the unit tests.
  4. Put the resize test's read count back to 1, and drop `quietOnceListening`.
  5. Add a test that writes a byte from inside `ClearMarker` (Enter) and from inside `DiscardRegistration` (`y`), and asserts the byte is still unread after the hand-off.
  ALTERNATIVE: Keep the pending read and write the narrowing down instead. Rewrite `startResumeReader`'s comment so it no longer promises inheritance, and record the dropped-keystroke window through the corrigendum path. This is much cheaper. The cost is that Enter→hook and `y`→shell keep losing a keystroke typed just after the answer, which the earlier design avoided on purpose. The reviewer recommends the fix.
  CONFIDENCE: medium

COMMENT_CORRECTIONS:
- cmd/state_resume_wait_paste_test.go:151 — the claim is false for the list's "line feeds as they are" case (`"echo one\necho two\n"`).
  OLD: // Every paste below arrives as terminal input reaches the pane through a tmux
// buffer: its line feeds as carriage returns.
  NEW: // A paste through a tmux buffer reaches the pane with its line feeds as
// carriage returns; one case keeps them as line feeds regardless.

NOTES:
- Pre-existing display defect, now persistent: on the route where the drain gives up, the drain clears only ICANON and leaves ECHO on (`cmd/tty_drain.go:38-39`). Between the draw's paint and the waiter entering raw mode, the tty is back in its found mode with echo on. Echoed stream lines scroll the panel's title off screen, and nothing repaints until a resize. Before this task the stream's carriage returns resumed the pane and hid this; now the pane stays on the corrupted panel. The integration check was narrowed to the report row (`internal/restore/lazy_resume_burst_integration_test.go:243`). A fix could stay inside the waiter: hand the pane to a fresh draw once a carried arrival ends. The route is rare (a lone `d` followed by more than 1s of continuous input).
- Limits of a timing rule, not defects: a one-byte paste (a bare line feed) cannot be told apart from a keystroke; a paste stalled for more than 50ms mid-delivery whose final chunk is exactly one answering byte would still answer (pastes up to 1 MB measured arriving well inside that).
- Coverage of the carried-input flag: the drain-gives-up integration subtest would pass without the `carried` flag, because every streamed chunk is several bytes. The flag's own effect is pinned by `TestRunResumeWait_InputStillArriving` and `TestResumeInputArriving_Draw`, which is adequate.
- Latency: every answer now waits 50ms of quiet before acting. Two keys typed less than 50ms apart (for example `d` then `y`) answer nothing; the outcome matches the old drain behaviour.
- AC3's literal scenario (a `d`-led paste reaching the drain) can no longer happen, since the `d` leading a paste never arrives alone; the integration subtest reaches the drain's give-up with a typed `d` followed by a 2.5s stream instead.
