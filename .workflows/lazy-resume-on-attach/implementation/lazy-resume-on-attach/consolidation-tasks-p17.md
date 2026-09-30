# Consolidation Tasks: Lazy Resume On Attach (Phase 17)

## Task 1: A Stray Paste Never Answers a Waiting Pane
placement: phase 17
severity: behaviour

**Problem**: The specification says a stray paste cannot answer the panel (§4.3), and its 2026-09-22 corrigendum names the failure: a stray paste spending the offer and unfreezing the pane's scrollback with nobody watching. The code does not hold that for Enter. On the waiting panel, CR and LF are the resume key (`cmd/state_resume_wait.go:146-150`), and tmux delivers a paste's line feeds as carriage returns (noted by the phase's own `pasteIntoSubject`, `internal/restore/lazy_resume_burst_integration_test.go:147-148`). So any stray paste containing a line break resumes a waiting pane, and its hook runs with nobody pressing Enter. That was true before this phase. Phase 17 adds a second route: when input is still arriving after the drain's one-second bound (`cmd/tty_drain.go:53-65`, `errInputKeptArriving`), the draw puts up the waiting panel instead of the confirmation (`cmd/state_resume_draw.go:55-58`), so the rest of a `d`-led multi-line burst lands on the one screen where its carriage returns act. The burst suite accepts that screen as a passing outcome (`lazy_resume_burst_integration_test.go:76-78`), so nothing notices a hook started by the burst.

**Solution**: A waiting pane ignores pasted input: only an Enter or a `d` typed at the pane answers it, and a paste — including the remainder of a burst the drain gave up on — neither resumes nor discards. Derived from §4.3, which names a stray paste among what cannot answer the panel and now states that a paste cannot answer with its line breaks either (corrected this pass), and from its 2026-09-22 corrigendum, which names the failure a stray paste spending the offer causes; the cost of honouring it falls on the implementation, never on the user, since no user answers a panel by pasting.
- **What stays.** The discard's burst guard (phase 17's drain and command match) and the keys a typed answer uses are unchanged. An errant `send-keys` is byte-for-byte a typed keystroke and answers as one; the specification now says so, and nothing here tries to tell the two apart.
- **Both routes are covered.** A paste arriving on a panel that is already up, and the remainder of a burst the drain gave up on and handed to the waiting panel with its report — whose opening the drain has already discarded — are both kept from answering.
- **The burst suite's accepted outcomes match.** Where it accepts the waiting panel carrying the drop's report, it also requires that the pane's hook did not run and the marker still stands.

**Outcome**: A paste of any size, with or without line breaks, leaves a waiting pane waiting: its hook does not run, its registration and marker stand, and a key typed afterwards answers it as before.

**Acceptance Criteria**:
- [ ] A pane showing the waiting panel receives a paste that carries line breaks and no `d`, delivered the way a terminal paste reaches it (through a tmux buffer, whose line feeds arrive as carriage returns). The pane stays on the waiting panel: its hook does not run, `hooks.json` is byte-identical, `@portal-resume-pending` stands, and the waiter still holds the pane.
- [ ] A paste carrying a `y` arrives while the discard confirmation is already on screen. It removes nothing: `hooks.json` is byte-identical, the marker stands, and the pane stays waiting.
- [ ] The discard's input drain gives up on a `d`-led multi-line paste that is still arriving past its one-second bound, and the waiting panel comes up reporting `can't clear pending input: …`. The rest of that paste, whose opening the drain has already discarded, neither resumes nor discards: the hook does not run, the registration and the marker stand, and the pane stays on the panel with its report.
- [ ] After any of those pastes, an Enter typed at the pane resumes it: the marker clears and the hook starts over the transcript. A `d` typed at the pane opens the discard confirmation, where a typed `y` discards and Escape backs out. Both behave exactly as on a pane that received no paste.
- [ ] A key tmux injects as if typed (`send-keys Enter`, `send-keys d`) answers the waiting panel exactly as the typed key does.
- [ ] The burst suite's three pastes (the 2,565- and 4,005-byte multi-line pastes and the 4,004-byte single-line burst) still never confirm. On every screen the suite accepts as a paste's outcome, the waiting panel reporting the drop included, it requires that the subject's hook did not run and that the marker still stands.

**Do**:
- Where the work lives:
  - The waiter's reading and key dispatch: `resumeKeysFor` and `resumeWaitLoop` (`cmd/state_resume_wait.go:139-202`) and `startResumeReader` (`:314-327`). The waiter reads the pane one byte at a time and acts on the first byte its screen answers to.
  - The draw's drop-refusal route, which puts the waiting panel up with the drop's report whenever the drain returns an error (`cmd/state_resume_draw.go:53-59`). An error includes `errInputKeptArriving` and `errUnselectableTTY` (`cmd/tty_drain.go:12-15`).
  - The burst suite, `internal/restore/lazy_resume_burst_integration_test.go`.
- What stays unchanged:
  - The discard's burst guard: the drain `resumeInputDrain` (`cmd/tty_drain.go:25-67`), run before the confirmation goes up, and the command match in `hooks.Store.Discard` (`internal/hooks/store.go:234-240`).
  - The bytes each screen answers to (`resumeKeysFor`). On the panel these are CR and LF for Enter, plus `resumekeys.Discard`. On the confirmation they are `resumekeys.Confirm` and a lone Escape.
  - Nothing tries to tell a `send-keys` apart from a typed key.
- The burst suite's post-paste checks (`:83-97`) already require the following for both screens it accepts today:
  - `hooks.json` is byte-identical.
  - The marker stands (`assertPending`).
  - No pasted line ran as a shell command (`burstSentinel`).
  - The waiter still holds the pane.

  They never look for the subject's own hook sentinel (`discardSubjectFired`, `internal/restore/lazy_resume_discard_integration_test.go:33`). The suite gains that check on every screen it accepts, and the screens it accepts match what a paste leaves up.

## Task 2: Corrections
placement: phase 17
severity: corrections

**Problem**: CLAUDE.md's "Resume hooks" section describes the panel's confirmed discard as removing "the pane's `on-resume` entry and nothing else" (CLAUDE.md:186). Phase 17 made the discard conditional: `hooks.Store.Discard` removes the entry only when its stored command is byte-for-byte the command the confirmation showed, under the store's lock, and never compares the resume mode (`internal/hooks/store.go:224-240`). An agent working from CLAUDE.md would read the command argument and the no-op on a mismatch as drift from the documented contract and could drop them, reopening the route by which a discard deletes a replacement registration the user never saw.

**Solution**: One edit, derived from the landed code and the specification's §6.2 as corrected this pass:
- CLAUDE.md:186, the discard sentence — state that the confirmed discard removes the pane's `on-resume` entry only when its stored command matches the command the confirmation showed byte-for-byte (the resume mode is not compared), and that an entry rewritten to a different command during the wait is left in place and answered as nothing to remove, with no write and no `op=discard` line.

**Outcome**: CLAUDE.md describes the panel's confirmed discard the way `hooks.Store.Discard` implements it. A future agent reading the paragraph therefore finds the command argument and the no-op on a mismatch documented, not apparently drifting from the documented contract.

**Acceptance Criteria**:
- [ ] CLAUDE.md's "Resume hooks" discard sentence (`:186`) states that the confirmed discard removes the pane's `on-resume` entry only when its stored command matches the command the confirmation showed byte-for-byte, and that the resume mode is not compared.
- [ ] The same sentence states that an entry rewritten to a different command during the wait is left in place and answered as nothing to remove, with no write and no `op=discard` line.
- [ ] The sentence's other claims stand as they read today:
  - The key's other events and every other entry stand.
  - No token is unstamped.
  - A removed command is logged as `value` under `op=discard`.
  - `rm`, `discard` and `clean-stale` stay greppable apart.
- [ ] No other CLAUDE.md text changes, and no source or test file changes.

**Do**:
- Where: `CLAUDE.md:186`, the sentence beginning "A lazy resume's waiting panel is a third removal route". It currently says the discard "removes the pane's `on-resume` entry and nothing else".
- The landed behaviour the edit states:
  - `hooks.Store.Discard` (`internal/hooks/store.go:227-240`) removes through `removeEntry` with `matches: r.Command == shown`.
  - `removeEntry` compares under the mutation lock it takes at `:259` (the load is at `:265`). It returns `false, nil` before the save and before the `op=discard` breadcrumb when the stored registration's command differs (`:274-277`).
  - `TestDiscard_OnlyTheShownCommand` (`internal/hooks/discard_test.go`) pins that the same command re-registered under another mode is still removed.
- The specification's §6.2 (corrected 2026-09-30) states the same rule.
