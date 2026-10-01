TASK: A Discard Removes Only a Registration the User Confirmed on Screen (lazy-resume-on-attach-17-1, tick-d95ec9)

ACCEPTANCE CRITERIA:
- A pane waiting on a real tmux server is sent a multi-line paste larger than its input queue, with a `d` first and `yes` last, at the measured 2,565- and 4,005-byte sizes. Nothing in the paste answers the confirmation: hooks.json still holds the registration, `@portal-resume-pending` is still set, the pane shows the confirmation or the waiting panel carrying the drop's report, and no shell runs any part of the paste
- The same holds for a 4,004-byte single-line burst carrying a `d` and a `y`
- A burst whose tail is an unterminated line still arriving when the drain starts is drained, not read as quiet: none of its bytes reach the waiter behind the confirmation
- When nothing arrives after the `d`, the confirmation goes up once the pane's input has been quiet for 50 ms, and a `y` pressed once it is on screen discards
- A terminal background reply that lands after the appearance probe's deadline is still dropped with the rest of the input, and it neither opens nor cancels a screen
- When input is still arriving after one second, the waiting panel comes up instead of the confirmation. Its report row reads `can't clear pending input: input kept arriving` and renders whole on the card, and one `clear pending input failed` WARN is logged. The registration and the marker both stand, and `d` can be pressed again
- However the drain ends, the pane's tty is left as the drain found it: canonical processing on, signal generation off
- A pane waiting on command X whose entry still holds X: `y` removes the entry, logs `op=discard` with `value` X, clears the marker and drops the pane to a shell, as it does today
- A pane waiting on X whose entry was rewritten to Y while it waited, by `hook set` aimed at the pane or by a hand edit: `y` leaves hooks.json byte-for-byte unchanged with Y in place and logs no `op=discard` line. The marker clears and the pane drops to a shell
- A pane waiting on X whose entry was rewritten to X under a different resume mode: `y` removes the entry and logs `op=discard` with `value` X
- Every other outcome is unchanged: an already-gone entry is nothing to remove (marker clears, shell, nothing written); an unavailable lock or unreadable store is reported on the confirmation with registration and marker standing; a refused flush is reported on the waiting panel in the OS's words; `portal hook rm` still removes whatever command the key holds

STATUS: complete

SPEC CONTEXT: The confirmation exists so a user-authored `on-resume` command (often its only copy) is destroyed only by a keystroke given to the confirmation on screen: "Input already in flight when the confirmation opened is dropped rather than read as agreement", which prevents "destroying the only copy of a user-authored command with nobody seeing either screen". An entry "replaced by a re-registration" is among those that have "already gone", so a discard over it finds nothing to remove. Refused removals are reported on the confirmation; a refused drop never puts the confirmation up over input it could not discard.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/tty_drain.go:12-68 — `ttyDrain.drain`: canonical off (VMIN=1/VTIME=0) for the whole drain, flush, then `awaitTTYInput` for the settle window, re-flushing on every arrival; returns nil once quiet, `errInputKeptArriving` ("input kept arriving") when an arrival lands past the one-second deadline; the deferred restore puts back the termios it found on every path after the mode switch
  - cmd/tty_drain.go:26 — `resumeInputDrain = ttyDrain{settle: resumeInputQuiet, bound: time.Second}`; `resumeInputQuiet` (cmd/state_resume_wait.go:38) is 50 ms. The task named `resumeEscapeFollow`; a later task replaced it with `resumeInputQuiet` at the same value and meaning — sound evolution
  - cmd/tty_drain.go:72-91 — `awaitTTYInput` (select, EINTR-retrying, FD_SETSIZE-guarded at :32)
  - cmd/state_resume_draw.go:122-127 — `dropStdinInputQueue` runs the drain and wraps its error; the refused-drop path at :56-61 logs the `clear pending input failed` WARN and swaps in the waiting panel with `resumeDropRefusal`
  - cmd/state_resume_report.go:80-82 + :88-107 — `causeWords` unwraps to `errInputKeptArriving`, so the row is `can't clear pending input: input kept arriving` (46 cells); an ioctl refusal still renders the errno's own words
  - internal/tui/pane_appearance.go:62-68 — the drop still runs once, after the appearance query
  - cmd/state_resume_wait.go:315-320 — `resumeOpenDiscardConfirm` comment reworded ("may still be arriving on the tty")
  - internal/hooks/store.go:227-240 — `Discard(key, event, shown, via)` with a `matches` predicate comparing `Command` only; :242-249 `removal`; :270-277 the comparison sits inside `removeEntry`'s locked load-mutate-save and answers `false, nil` with no write and no breadcrumb on a mismatch; `Remove` (:223-225) passes no predicate and is unchanged
  - cmd/state_resume_wait.go:330-333 and :407-414 — the waiter hands `cfg.Command` through `DiscardRegistration` → `discardResumeRegistration` → `Store.Discard`
- Notes: Phase 3's ordering and the single flush site are preserved — `flushTTYInput` is now referenced only from `ttyDrain.drain` (the task said `dropStdinInputQueue`; the drain is its production callee, and `TestFlushTTYInput_TouchesTheInputQueueInExactlyOnePlace` still enforces one site). A later task (17-2) made the waiter answer only a byte that arrives alone, so a `d`-led paste no longer opens the confirmation at all; the drain remains the guard for input arriving after a lone `d`. That evolution strengthens this task's outcome rather than losing any of it.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/tty_drain_pty_test.go (real pty, slave in the chain's canonical/no-ISIG state): queued burst discarded; a 20-line burst arriving over ~200 ms drained before return; an unterminated line fed one byte per 20 ms drained (this case fails if the drain ran canonical, since select would report quiet mid-line and `done` would still be false); a background reply landing 20 ms into the drain dropped; a quiet drain returns after at least the settle window; continuous input returns `errInputKeptArriving` reading exactly "input kept arriving" within ~1 s; modes restored (canonical on, signals off) for quiet, ending-burst and still-arriving cases
  - cmd/tty_drain_test.go — non-tty descriptor errors; settle/bound pinned to `resumeInputQuiet` and 1 s
  - cmd/state_resume_drop_input_test.go:296-317 — the kept-arriving refusal paints the waiting panel carrying the whole 46-cell row and logs exactly one `clear pending input failed` WARN; :268-294 the errno refusal still renders OS words; :111-123 `d` from a reported panel reopens the confirmation with the drop flag
  - internal/hooks/discard_test.go:320-404 — rewritten-by-Set, hand-edited and whitespace-only-different entries are left byte-identical with no records; same command under another mode is removed with `op=discard value=X`; `Remove` still removes any command; :156-185 and :406-467 keep the absent-entry, lock and read refusals
  - cmd/state_resume_discard_test.go:60-68 (the shown command reaches the store), :293-319 (rewritten entry left unchanged, no discard record, marker cleared, shell hand-off), :372-385 (`discardResumeRegistration` over a rewritten entry)
  - internal/restore/lazy_resume_burst_integration_test.go:53-123 — real chain via `paste-buffer` at 2,565 / 4,005 multi-line and 4,004 single-line: store byte-identical, marker standing, subject command unrun, no pasted line run, waiter still holding the pane; a `hook set`-equivalent rewrite then `d`/`y` leaves hooks.json byte-identical, logs no `op=discard`, clears the marker and reveals the transcript; :163-173 drain bound exceeded on the real chain shows the drop report with everything standing
  - internal/restore/lazy_resume_discard_integration_test.go:93-119 — the unchanged `d`/`y` happy path on the real chain
- Notes: The suite sits in its own `lazy_resume_burst_integration_test.go` beside the panel suite in the same package rather than inside `lazy_resume_panel_integration_test.go`; same package, fixtures and lane, so the placement intent holds. No redundant or implementation-detail tests worth flagging.

CODE QUALITY:
- Project conventions: Followed (seams via package vars, logtest/hookstest helpers, integration tag on the real-chain suite, no t.Parallel)
- SOLID principles: Good — the drain is a small value type with one job; the store's per-route `removal` keeps `Remove` and `Discard` on one locked path
- Complexity: Low
- Modern idioms: Yes
- Readability: Good; comments on the drain, `awaitTTYInput`, `Discard`, `removal` and `resumeOpenDiscardConfirm` hold true against the code
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "A pane waiting on a real tmux server is sent a multi-line paste larger than its input queue, with a `d` first and `yes` last, at the measured 2,565- and 4,005-byte sizes. Nothing in the paste answers the confirmation ..." — run `go test -tags integration -p 1 ./internal/restore -run TestLazyResumeDiscard_BurstNeverConfirms` against real tmux; whether the paste stays one unanswered arrival under tmux's queue refill is a timing property reading cannot observe
- "The same holds for a 4,004-byte single-line burst carrying a `d` and a `y`" — same integration run (the 4,004-byte single-line subtest)
