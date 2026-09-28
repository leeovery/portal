TASK: lazy-resume-on-attach-5-3 — The Discard Key Opens the Confirmation and Escape Backs Out

ACCEPTANCE CRITERIA:
- `d` on the waiting panel produces exactly one hand-off, to a `resume-draw` argv carrying `--screen discard`, the same `--command`, `--hook-key`, `--pane`, `--pane-key` and size, and no `--report`.
- Escape on the confirmation produces exactly one hand-off, to a `resume-draw` argv carrying no `--screen` and no `--report`.
- `\r` and `\n` on the confirmation produce no hand-off, no write and no state change, and the loop is still reading afterwards.
- `d` on the confirmation produces no hand-off.
- Uppercase `Y` on the confirmation produces no hand-off.
- Escape on the waiting panel is inert: no hand-off, no write, loop still reading.
- An escape sequence delivered to the confirmation is swallowed, the loop keeps reading, and a `y` after it still confirms (`\x1b[A`, `\x1b[3~`, `\x1bOD`).
- A sequence whose final byte is an acting key produces no hand-off on either screen (`\x1b[?1;2y` on the confirmation, `\x1b[5d` on the panel).
- A bare `\x1b` backs out of the confirmation and is still inert on the waiting panel.
- A sequence longer than its own byte cap (`resumeEscapeSequenceCap` for CSI/SS3, `resumeOSCSequenceCap` for OSC) is swallowed and the loop keeps reading.
- A terminal's OSC 11 reply is consumed whole on both screens, BEL- and ST-terminated alike.
- The follow window arms only after an `\x1b` has been read; task 4.2's idle report-holding criterion passes unchanged.
- `0x03`, `0x04`, `0x1a` and printable text produce no hand-off on either screen, and neither loop exits on them.
- Both hand-offs clear `Report`.
- The terminal is restored before every hand-off exec on both screens.
- Every dispatch behaves identically below the card's size and at non-positive sizes.
- `y` on the confirmation produces exactly one hand-off, to a fresh draw of the confirmation, and removes nothing.
- A settle redraw while the confirmation is up execs a `resume-draw` carrying `--screen discard` and the report it was launched with.

STATUS: issues_found

SPEC CONTEXT: The spec's sections on the waiting panel, the discard confirmation and answering the panel (4.3, 5.4, 6.2, 6.3) cover this task. On the waiting panel only Enter and `d` act. `d` opens a confirmation where only `y` (lowercase, matching the picker's kill and delete confirms) and Escape act. Everything else is swallowed on both screens, including Enter on the confirmation and the Ctrl-C/Ctrl-D/Ctrl-Z/Ctrl-\ bytes. Escape does nothing on the waiting panel. A report stays up until the next key press and never times out, and a resize is not a key press. Each screen is a fresh draw that hands back to a fresh wait. The confirmation still works at every pane size. The draw's appearance query can leave a late OSC 11 reply in the input queue, and 5.2 says that reply must never act as a key.

IMPLEMENTATION:
- Status: Implemented. Two changes drifted from the task text, and both are sound. One was expected by the task itself; see Notes.
- Location:
  - cmd/state_resume_wait.go:51-55 — `resumeEscapeFollow` (50ms), `resumeEscapeSequenceCap` (16), `resumeOSCSequenceCap` (64)
  - cmd/state_resume_wait.go:126-147 — `resumeKeys` / `resumeKeysFor`: each screen gets its own dispatch table. The panel has CR, LF and `resumekeys.Discard` with no escape handler. The confirmation has `resumekeys.Confirm` plus `resumeCancelDiscardConfirm` as its escape handler.
  - cmd/state_resume_wait.go:167-177 — the loop resolves an ESC before dispatching it. `pending` reports the read left outstanding when the window elapses, so the loop never requests a second read.
  - cmd/state_resume_wait.go:204-220 — `resolveResumeEscape`: races one requested byte against `cfg.Settle(resumeEscapeFollow)`. The window is armed only after an ESC has been read.
  - cmd/state_resume_wait.go:225-263 — `consumeResumeSequence` / `isCSIFinal` / `oscTerminator`. The introducer is taken unconditionally. CSI and SS3 run to a byte in 0x40–0x7e under the 16-byte cap. OSC runs to BEL or ESC-`\` under the 64-byte cap. Counting starts at 2 (ESC plus introducer), so each cap bounds the whole sequence, which is what the constant's comment says.
  - cmd/state_resume_wait.go:330-337 — `resumeOpenDiscardConfirm` / `resumeCancelDiscardConfirm` go through `resumeShowScreen` (385-390), which clears `Report` and sets `Screen`. That feeds `resumeRedraw` (397-400), which restores the terminal and then calls `resumeHandOff`, which emits the `exec` INFO immediately before `ExecSelf`.
  - cmd/state_resume_wait.go:190-195 — the settle redraw is unchanged: it carries the whole payload, so both the screen and the report survive a resize.
- Notes:
  - Criterion "`y` … hands over to a fresh draw of the confirmation and removes nothing" was superseded by task 5.5, as this task anticipated ("Task 5.5 replaces this body with the removal"). `resumeAnswerDiscard` (cmd/state_resume_wait.go:343-356) now performs the discard. Its test was correctly replaced by "it answers the confirmation's y with the discard" (cmd/state_resume_screens_test.go:660-664). This is not a finding.
  - `resumeOpenDiscardConfirm` also sets `DropInput`, and the `d` hand-off argv carries `--drop-input`. That comes from task 5.4. `--screen discard`, the unchanged identity flags and size, and the absence of `--report` all still hold.
  - Divergence from the task text: the task said every introducer other than `]` runs to a byte in the CSI final range. The implementation (comment at cmd/state_resume_wait.go:28-31, `default: return nil` at :233-234) ends any other introducer after two bytes: an Alt chord, `\x1bP`, `\x1b_`, `\x1bX`, or a second ESC. The task's rule would swallow the user's next real keypress as the "final byte" of an Alt chord (Alt-d followed by `y` would eat the `y`). The two-byte rule never lets an acting key be reached from a chord either (`\x1by` and `\x1bd` are swallowed, per the tests at cmd/state_resume_screens_test.go:481-495). Tests pin it (:442-463, :513-535). The divergence is sound, so it is not a finding.
  - The Escape hand-off never carries `--drop-input`: the waiter does not register that flag (cmd/state_resume_wait.go:492-503), so `cfg.DropInput` is always false there.

TESTS:
- Status: Adequate. One reliability defect, recorded under FINDINGS.
- Coverage: Every named test is present in cmd/state_resume_screens_test.go `TestRunResumeWait_Screens`, except the resize case, which is at cmd/state_resume_wait_resize_test.go:298-317:
  - opens on `d`, with the argv checked via `opened` and `--report` absent
  - backs out on Escape, with `--screen` and `--report` absent
  - Enter, `d` and `Y` swallowed on the confirmation
  - Escape inert on the panel
  - the escape-sequence table, followed by a `y` that still acts
  - final-byte `y`/`d` sequences, plus SS3 variants
  - the OSC 11 reply in both BEL and ST forms on both screens, including a `2d2d` reply whose `d` lies past the CSI cap, with an acting key afterwards to prove the loop kept reading
  - a bare Escape on both screens, driven by a controlled window
  - cap-length sequences for CSI, SS3 and OSC, followed by `y`, plus the acting-key-at-the-last-byte-inside-the-cap cases
  - task 4.2's swallow table run per screen
  - the report cleared on both hand-offs
  - the terminal restored before the four acting keys' execs
  - every acting key at four sizes, including 0x0 and negative
  - the confirmation redrawn on a settled resize, carrying `--screen discard` and the report
  The idle-arms-nothing criterion is asserted in cmd/state_resume_wait_test.go:349-430, which counts escape windows separately. The `swallowed` helper checks reads == len(input)+1, so "the loop is still reading afterwards" is observed rather than assumed.
- Notes: The extra chord, SS3 and string-introducer cases are coverage for the two-byte rule, not redundancy. The bare-Escape and chord-then-resize cases use rendezvous harnesses (`escapeHarness`, `pipedKeysHarness`) that fire the follow window when the test chooses, not by wall clock. The `swallowed` and `answered` helpers do not: they race a real 50ms `time.After` against the next byte (see FINDINGS).

CODE QUALITY:
- Project conventions: Followed. Seams go through the config struct. The single `resumeHandOff` exec site is preserved. Act keys come from the `resumekeys` leaf. No `t.Parallel`. The waiter file imports no rendering path.
- SOLID principles: Good. The per-screen key table keeps each screen's dispatch declarative, and the ESC resolution is isolated in its own helpers.
- Complexity: Acceptable. The loop's escape branch is small, and sequence consumption is a tidy two-rule switch.
- Modern idioms: Yes (`sync.OnceFunc`, closure-based terminator state).
- Readability: Good. The comments at cmd/state_resume_wait.go:28-31, :48-50, :200-203 and :222-224 hold against the code.
- Issues: None in production code. One test-reliability issue, below.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/state_resume_screens_test.go:40 and cmd/state_resume_screens_test.go:78 — `swallowed` and `answered` build their config through `newResumeWaitConfig`, whose default `Settle` is the real `time.After` (cmd/state_resume_wait_test.go:82). For ESC-led input, every one of these cases therefore races a real 50ms follow window against a byte that is already queued in a `strings.Reader`. This test file already orders every other window "against the loop rather than against the clock". Fix: in both helpers, bind the config to a variable and set `cfg.Settle = func(time.Duration) <-chan time.Time { return nil }` before `runResumeWait`. This is safe as written: `newResumeWaitConfig` leaves `Winch` nil, so neither helper can arm a resize settle, and every input they are given ends in EOF, so the resolver's read always returns. Every ESC-led confirmation case in this file goes through these two helpers; the direct `runResumeWait` calls at :446 and :518 run on the panel. — FAILS: if the test process stalls for 50ms or more between the window arming and the queued byte being received (a loaded machine, the condition CLAUDE.md warns manufactures flakes), the resolver takes the ESC as bare. On the confirmation that backs out: `swallowed` then gets a nil error and a hand-off where it wants EOF, and `answered(..., seq+"y")` gets a panel draw where it wants the discard. Both are spurious failures across the sequence, OSC, cap and chord tables.

UNSETTLED:
- None
