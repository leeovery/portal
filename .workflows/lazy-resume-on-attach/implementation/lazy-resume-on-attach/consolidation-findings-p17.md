# Consolidation Findings: lazy-resume-on-attach (Phase 17)

## Findings

None. The phase is one task (Tlazy-resume-on-attach-17-1, commit 4d6f37796) and the bank is empty. Candidates that did not clear the bar:
- `awaitTTYInput` (`cmd/tty_drain.go:70-89`) repeats the select-with-EINTR-retry loop of `selectBoundedReader.awaitReadable` (`internal/tui/pane_appearance.go:172-192`), and `errUnselectableTTY` repeats `errUnselectable`. The copies agree today, and drift would show up. A drain whose select broke would put a "can't clear pending input" row on the pane, and a missing FD_SETSIZE guard makes `FdSet.Set` panic. So this fails the duplication floor.
- `TestFlushTTYInput_TouchesTheInputQueueInExactlyOnePlace` counts `flushTTYInput` references, so a second caller of `resumeInputDrain.drain` would pass it. That gap is older than this phase: a second caller of `dropStdinInputQueue` passed it before. The phase moved the one site and did not open the hole.
- `resumeInputDrain.settle` is tied to `resumeEscapeFollow`, and `tty_drain.go:23-24` says so on purpose. This is a naming question, not a failure.

## Comment Corrections

- cmd/state_resume_report_test.go:353 — a drop's cause can now be Portal's own (`errInputKeptArriving`, `errUnselectableTTY`), not only the operating system's
  OLD: // A drop's cause is the operating system's, and can run past the card.
  NEW: // A drop's cause can run past the card.

- cmd/state_resume_drop_input_test.go:449-450 — one drop now flushes repeatedly by design (`cmd/tty_drain.go:53-65`), so the hazard is a second flush site, not a second flush
  OLD: // Two flushes would eat a keystroke the user meant after the confirmation went
       // up, so the queue is touched from one place in the chain.
  NEW: // A second flush site would eat a keystroke the user meant after the
       // confirmation went up, so the queue is touched from one place in the chain.

## Spec Defects

### S1: §6.2 says a confirmed discard removes the registration, but the landed discard removes only the command that was shown
- **Claim**: §6.2 (line 247): "A confirmed discard removes the pane's resume registration, permanently, and nothing else." Line 249 counts an entry "replaced by a re-registration, or hand-edited away while the pane waited" as already gone.
- **Observed**: `hooks.Store.Discard` (`internal/hooks/store.go:224-240`) removes the entry only when `r.Command == shown`, and it never compares the resume mode.
  - An entry rewritten to another command is left in place and answered as absent (removed=false, no write, no record). Even a command that differs only by a trailing space counts as another command.
  - The same command re-registered under another mode is still removed (`internal/hooks/discard_test.go` `TestDiscard_OnlyTheShownCommand`).
  - So line 249's "replaced by a re-registration" is only nothing-to-remove when the command changed. A mode-only re-registration is destroyed.
  - The match rule is stated nowhere in the spec. CLAUDE.md's resume-hooks paragraph still describes the discard as removing "the pane's `on-resume` entry and nothing else".
- **Read**: Spec stale. The landed rule makes two decisions the spec never made: the command is matched byte-for-byte, and the mode is not compared. §6.2 should state both.

### S2: §7.2 says a declined discard followed by a refused clear leaves nothing in the store behind the redrawn card
- **Claim**: §7.2 (line 302): after a discard, "a marker that then refuses to clear brings the card back with nothing in the store behind it … Enter reads the store again, finds nothing, and drops the pane through to a plain shell … which is where the discard was going."
- **Observed**: Suppose the entry was rewritten while the pane waited. Then `resumeAnswerDiscard` (`cmd/state_resume_wait.go:359-372`) gets (false, nil) from the discard, and the entry survives.
  - A refused `ClearMarker` then redraws the panel from the payload (`resumeReport`, `:394-400`). The redrawn card shows the old command.
  - Enter (`resumeAnswerEnter`, `:331-342`) looks the store up again, finds the rewritten registration, and runs it.
  - The user confirmed a discard and then pressed Enter on a card showing the discarded command. What runs is a command neither screen showed.
  - Before this phase, the discard removed whatever the key held, so the sentence held in every case.
- **Read**: Genuinely open.
  - If §6.1's rule (run what the store holds now) governs, the spec is stale and line 302 needs to say the store may still hold a registration the card does not show.
  - If the phase's own principle governs (act only on what was on screen), the code is wrong for this path: the redrawn card should not resume a command it never showed.
  - It is only reachable when a rewrite during the wait coincides with a refused tmux marker clear.

### S3: §4.3 says a stray paste cannot answer the panel, but a paste containing a line feed answers it with Enter
- **Claim**: §4.3 (line 131): "a stray paste, an errant `send-keys`, or a key pressed in the wrong window cannot *answer* the panel, because nothing but those two means anything to it."
- **Observed**: On the waiting panel, CR and LF are the resume key (`cmd/state_resume_wait.go:146-150`). tmux delivers a paste's line feeds as carriage returns, as the phase's own `pasteIntoSubject` comment notes (`internal/restore/lazy_resume_burst_integration_test.go:147-148`). So any stray paste containing a line feed resumes a waiting pane, and its hook runs with nobody pressing Enter.
  - This phase adds a second route to it. If input is still arriving past the one-second bound, the drain gives up (`cmd/tty_drain.go:53-65`, `errInputKeptArriving`).
  - The draw then shows the waiting panel instead of the confirmation (`cmd/state_resume_draw.go:55-58`). The rest of a `d`-led multi-line burst therefore lands on the one screen where its carriage returns act.
  - The burst suite accepts that report screen as a passing outcome (`lazy_resume_burst_integration_test.go:76-78`).
- **Read**: Genuinely open. The claim was already false for Enter before this phase. §4.3's burst rule covers only `d` followed by `y`.
  - Either the spec narrows the claim and accepts a resume from a paste (Enter is the non-destructive answer),
  - or the waiting panel needs a paste guard of its own, as the confirmation got. Otherwise the drain's give-up path hands a live burst to a screen that acts on it.
