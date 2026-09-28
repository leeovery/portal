TASK: lazy-resume-on-attach-5-7 — A Two-Byte Escape Ends at Two Bytes (the waiter's escape resolver branches on the introducer, so an Alt chord, an Option-arrow or a fast Escape pair is consumed at two bytes and the next key acts)

ACCEPTANCE CRITERIA:
- On the waiting panel, `ESC b` (an Option-← or Alt-b chord) followed by Enter resumes the pane on that Enter: the marker clears and the pane is handed to its registered command, with the chord itself having acted on nothing
- On the discard confirmation, `ESC x` followed by `y` confirms the discard on that `y`: the pane's registration is removed once and the pane is handed to a plain shell
- On the discard confirmation, `ESC ESC` followed — after the follow window has elapsed — by a lone Escape backs out to the waiting panel, with the store and the marker untouched
- A two-byte chord on its own acts on neither screen: `ESC y` on the confirmation and `ESC d` on the waiting panel are swallowed — no hand-off, no store or marker touched — and the wait goes on reading
- A pane resize that arrives after a two-byte chord, with no further key pressed, hands the pane to a fresh draw once the resize settles
- A sequence opened by `P`, `X`, `^` or `_` is swallowed through its BEL or ST terminator, within the OSC's 64-byte cap, even when its payload carries `y` or `d`; the key after the terminator acts on the screen that is up
- CSI (`[`), SS3 (`O`) and OSC (`]`) sequences are swallowed exactly as before, to their final byte, terminator or cap — every existing case in `cmd/state_resume_screens_test.go` passes unmodified

STATUS: issues_found

SPEC CONTEXT: Spec 4.3 makes the waiting pane swallow every byte except the keys the screen in front of the user offers (Enter and `d` on the panel; `y` and Escape on the confirmation), as a safety property: nothing the user did not send may answer the panel. 5.2/5.4 require those keys to act at every size — a pane whose named key does nothing reads as a dead keyboard. 6.3 makes Escape inert on the panel and a back-out on the confirmation. The spec's corrigendum on the appearance probe establishes that the pane's one terminal query is OSC 11, whose late reply arrives as input opening with ESC and must be swallowed whole.

IMPLEMENTATION:
- Status: Implemented, with one deliberate and sound drift from criterion 6
- Location: cmd/state_resume_wait.go:28-40 (constant block restating the three-way rule; `resumeCSIIntroducer`/`resumeSS3Introducer` added), cmd/state_resume_wait.go:204-220 (`resolveResumeEscape`, unchanged), cmd/state_resume_wait.go:225-246 (`consumeResumeSequence`: `[`/`O` take the CSI final-byte rule under `resumeEscapeSequenceCap`, `]` the BEL/ST rule under `resumeOSCSequenceCap`, and the `default` arm returns having read nothing further)
- Notes:
  - Criteria 1-5 hold by reading. A byte other than `[`, `O` or `]` returns from `consumeResumeSequence` at :233-234 with no further read, so `resolveResumeEscape` returns `(false, false, nil)` and the loop at :151-198 dispatches the next byte as a key. It also goes back to selecting on `cfg.Winch`, so a resize is serviced after a chord. The chord's second byte never reaches `keys.bytes`, so Alt-y and Alt-d act on nothing. A second ESC completes the pair, and the next lone ESC opens its own follow window.
  - Criterion 6 is superseded on purpose. `P`, `X`, `^` and `_` fall to the two-byte `default` arm instead of the terminator rule. The implementation reviewer's attempt-1 fix tracking records the reason. Those four bytes are what the Alt-Shift-P, Alt-Shift-X, Alt-^ and Alt-_ chords produce. Under the terminator rule each chord would swallow up to 62 following keys (the loop at :236 reads `limit-2` bytes), which is the dead keyboard this task exists to remove and contradicts its Outcome. Nothing is lost against intent. The pane's only terminal query is OSC 11 (internal/tui/pane_appearance.go:103), whose reply keeps the `]` terminator arm, and no other query appears in the draw path. A DCS/SOS/PM/APC string therefore reaches a waiting pane only as a chord, a paste or a send-keys, and the waiter already treats those as keystrokes. The code is sound and the plan's criterion 6 text is stale. That is not a finding.
  - Criterion 7 holds by reading. The `[`, `O` and `]` arms get the same limit and terminator they had before (the diff in 0f00d2d5a replaced `limit, ended := resumeEscapeSequenceCap, isCSIFinal` plus an `if introducer == ']'` override with a switch that assigns the same pairs). The task commit only added lines to cmd/state_resume_screens_test.go, and no later commit touches that file.
  - The constant-block comment (:28-31) and the `consumeResumeSequence` comment (:222-224) match the code.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1: cmd/state_resume_screens_test.go:442-457 sends `"\x1bb\r"` and asserts one clear, one store read for the hook key, and exactly one hook hand-off (`assertExec` fatals on any other exec count). The old CSI rule would have read `\r` as a non-final byte and ended at EOF, so the test fails if the fix regresses.
  - Criterion 2: :459-463 sends `"\x1bxy"` and asserts one discard for the hook key plus a shell hand-off.
  - Criterion 3: :465-479 uses `pipedKeysHarness` (:191-275) to order the pair, the lone ESC and the follow-window elapse against the loop. It asserts a back-out to the panel with no state touched. Under the old rule the second follow window is never armed, so `awaitWindow` fatals.
  - Criterion 4: :481-495 runs `ESC y` on the confirmation and `ESC d` on the panel through `swallowed`, which checks no hand-off, no state touched, and a read after the chord.
  - Criterion 5: :497-511 presses `\x1bb`, waits for the follow window, delivers SIGWINCH, elapses the settle window and asserts the redraw hand-off. Under the old rule the resolver blocks in `reader.next()`, so no settle window is ever armed and the test fails.
  - Criterion 6 as implemented: :513-535 pins the two-byte rule for `P` (the Enter after it resumes) and for `_` and `X` (the `y` after them confirms).
  - Criterion 7: the existing cases (:319-440) are unedited. The added :346-363 closes the gap where removing `O` from the CSI arm went unnoticed: `"\x1bOy"` and `"\x1bOd"` must be swallowed, and a following `y` must confirm.
- Notes: The criterion-4 cases alone would also pass under the old resolver, which likewise reads the chord and then EOF. The "goes on reading" half of that criterion is proved by the criteria 1, 2 and 5 cases, where the next key or resize acts. No redundant or over-mocked cases.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel, seams injected through `newResumeWaitConfig`, named constants for every byte, pipe harness cleans up its writer)
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None beyond the finding below

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [spreading] cmd/state_resume_wait.go:229-232 — The Alt chords whose second byte is `[`, `O` or `]` (Alt-[, Alt-Shift-O, Alt-]) still enter the blocking consumption loop at :236-244. tmux forwards them as `ESC [`, `ESC O` and `ESC ]`, the same ESC-plus-key form the task gives for every Alt chord, so they cost the user more than the chord. The task's Outcome promises that an Alt chord "costs the user nothing beyond the chord itself — the next key they press acts on whichever screen is up". These three chords do not meet that; the other chords now do. Criterion 7 froze this behaviour, so the implementation met its criteria, and the residual was recorded only in this task's fix tracking and never carried forward. The fix has more than one defensible shape. One is to put every byte after the introducer under the follow window, since a real sequence arrives in one write. Another is to keep servicing `Winch` while a sequence is consumed. Either changes the CSI/OSC behaviour that criterion 7 pins, and needs tests that drive a window between bytes. — FAILS: after Alt-[ on the waiting panel, the next 14 Enter presses are swallowed (0x0d is outside 0x40-0x7E and the loop reads `resumeEscapeSequenceCap-2` bytes), so the Enter the footer names does nothing. After Alt-] the same happens for 62 presses, and on the confirmation repeated Escape presses cannot back out, because an ESC followed by ESC is neither BEL nor ST. A resize is not serviced while the loop blocks in `reader.next()`.

UNSETTLED:
- "CSI (`[`), SS3 (`O`) and OSC (`]`) sequences are swallowed exactly as before, to their final byte, terminator or cap — every existing case in `cmd/state_resume_screens_test.go` passes unmodified" — reading settles "exactly as before", since the three arms keep their limits and terminators and the existing cases are unedited. Settling "passes" needs a suite run: `go test ./cmd -run TestRunResumeWait_Screens`.
