TASK: lazy-resume-on-attach-9-3 (tick-b728bc) — Waiter Answers Resizes It Missed and Stops Swallowing Keys After Alt-[ and Alt-]

ACCEPTANCE CRITERIA:
1. A waiter starts over a pane whose size already differs from the size it was drawn at, and no SIGWINCH is delivered. Once the settle window elapses it hands off exactly once, to a `resume-draw` argv carrying its payload unchanged.
2. A size change arriving while that startup window is open restarts it, so a drag still in progress costs one redraw; a key pressed inside it acts at once, as it does inside any settle window.
3. A waiter whose startup size read fails, or which was drawn at a non-positive size, starts no settle window and hands nothing off; it goes on reading and a key the screen offers still acts. A size read that fails when a SIGWINCH's window settles still redraws, as today.
4. A waiter started at the size it was drawn at arms nothing and hands nothing off.
5. On the waiting panel, `ESC [`, `ESC O` and `ESC ]`, each followed by no byte within the follow window and then Enter, resume the pane on that Enter; the chord itself acts on nothing.
6. On the discard confirmation, `ESC ]` followed by no byte within the follow window and then a lone Escape backs out to the waiting panel with the store and the marker untouched; `ESC [` followed after the window by `y` confirms the discard.
7. A resize delivered after `ESC [`, with no further key pressed, hands the pane to a fresh draw once the resize settles.
8. A CSI, SS3 or OSC sequence that arrives in one write is still swallowed to its final byte, terminator or cap on both screens: the arrow and delete keys, the OSC 11 reply in BEL and ST forms, cap-length sequences and sequences whose final byte is an acting key act on nothing, and the key after them acts.
9. At most one read is outstanding at any moment, including across a sequence the window cut short, so input the loop did not dispatch stays queued for the next process image.
10. Every `swallowed` and `answered` case reaches its verdict with a follow window that never fires, so no queued byte is raced against a real clock.

STATUS: issues_found

SPEC CONTEXT: A resize is the draw/wait handover run backwards, and the redraw happens once the size has settled, not once per size change. Every screen change (panel, confirmation, report) is a fresh draw that hands back to a fresh wait. The swallow rule: nothing the user did not send may answer the panel. Only Enter and `d` act on the panel, and only `y` and Escape on the confirmation. Escape on the panel is inert. The spec says nothing about escape-sequence parsing. The task itself deliberately changes an earlier task's rule: a sequence whose next byte does not arrive within the follow window now ends there.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_wait.go:158 — the loop's `settled` is seeded from `resumeStartupSettle(cfg)` rather than nil.
  - cmd/state_resume_wait.go:206-215 — `resumeStartupSettle`. It arms nothing when the read fails, the drawn size is non-positive, or the sizes match. Otherwise it arms `cfg.Settle(resumeResizeSettle)`, and from there the existing settle → `resumeSizeUnchanged` → `resumeRedraw` path takes over (:192-197).
  - cmd/state_resume_wait.go:222-229 and :231-247 — `resolveResumeEscape` now reads through `followResumeByte` (one follow window per byte).
  - cmd/state_resume_wait.go:253-274 — `consumeResumeSequence` reads every byte after a CSI, SS3 or OSC introducer under its own follow window and reports `pending` when a window cuts it short. The blocking `resumeReader.next()` is removed; no reference to it remains in cmd/.
  - cmd/state_resume_wait.go:171 — `outstanding = pending` carries the cut-short read back to the loop.
- Notes:
  - Ordering holds in production. `Winch: winchSignals()` (cmd/state_resume_wait.go:493) is evaluated while the RunE builds the config, before `runResumeWait` runs, so the startup read at :207 happens after `signal.Notify`. A SIGWINCH landing between the watch and the read is buffered on the cap-1 channel and restarts the window, so the pane still gets exactly one redraw.
  - At most one read is outstanding: `followResumeByte` requests a read only when none is in flight, and every return with `pending=true` leaves exactly one in flight for the loop to adopt.
  - All the conditions the task names hold:
    - redraw only through `resumeRedraw`;
    - `signal.Notify` on SIGWINCH only;
    - no theme/tui/prefs import;
    - no `flushTTYInput`;
    - `DropInput` untouched.
  - The anti-bounce guard is sound. A failed draw-side read draws at (0,0), and the waiter then refuses to arm on a non-positive drawn size.

TESTS:
- Status: Adequate
- Coverage:
  - AC1: TestRunResumeWait_StartupSize "it redraws once for a size change it missed before it started" (cmd/state_resume_wait_resize_test.go:438), plus the confirmation variant at :456.
  - AC2: "it restarts the start-up window…" (:470) and "it dispatches a key pressed while the start-up window is open" (:496).
  - AC3:
    - failed startup read (:512);
    - non-positive drawn sizes, 4 variants (:543);
    - a failed startup read followed by a settled resize still redraws (:531);
    - a failed read at a SIGWINCH settle still redraws (:391).
  - AC4: "it arms nothing when started at the size it was drawn at" (:572), and the idle case in cmd/state_resume_wait_test.go:371.
  - AC5–AC7, AC9: TestRunResumeWait_IntroducerChords (cmd/state_resume_screens_test.go:742-828), driven through `pipedKeysHarness`:
    - its follow windows are unbuffered and fired through a bounded send, so a fire lands only while the loop is selecting on it;
    - the read count of 3 and the overlap count of 0 pin AC9.
  - AC8: TestRunResumeWait_Screens covers, on both screens:
    - arrow and delete keys;
    - OSC 11 replies in BEL and ST forms;
    - cap-length sequences;
    - final bytes that are acting keys;
    - an acting key just inside the cap.
  - AC10: `swallowed`/`answered` route through `queuedKeysConfig` (cmd/state_resume_screens_test.go:87-97). It returns a nil follow window, and it fails the test if a resize settle is ever armed.
  - `newResumeWaitConfig` gains `Size: fixedSize(payload.Width, payload.Height)` (cmd/state_resume_wait_test.go:82).
  - The resize harness answers the drawn size on its startup read (`startResumeResize`, resize_test.go:74-77). `paneResize` answers the drawn size until the resize is delivered.
- Notes:
  - The task asked for "the premise that no resize settle can start in the nil-settle helpers" to be re-checked. It was, and the doc comment on `queuedKeysConfig` restates it.
  - Nothing is over-tested.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel`; seams are injected through config fields.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: one stale test comment (see FINDINGS)

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/state_resume_screens_test.go:291-292 — the doc comment on `pipedKeysHarness.awaitWindow` still reads "failing when it arms none: a wait still consuming a sequence arms nothing". That was true while `consumeResumeSequence` read through the blocking `reader.next()`. This task made every byte after an introducer arm its own follow window (`followResumeByte` → `cfg.Settle(resumeEscapeFollow)`, cmd/state_resume_wait.go:232, :265). The same file's `chord` helper (:739) now awaits exactly such a window ("follow window for the byte after the introducer"). Fix: cut the clause after the colon, leaving "awaitWindow takes the next window of a kind the wait armed, failing when it arms none." — or narrow it to "a wait still consuming a sequence arms no resize settle". — FAILS: a contributor writing the next sequence test takes the comment at its word. They expect `awaitWindow(h.follows, …)` to fail while a sequence is being consumed. In fact it hands back a live follow window, which the loop blocks on until it is fired.

UNSETTLED:
- None
