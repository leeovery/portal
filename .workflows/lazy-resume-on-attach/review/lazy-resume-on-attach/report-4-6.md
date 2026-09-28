TASK: One Redraw Once the Size Has Settled (lazy-resume-on-attach-4-6)

ACCEPTANCE CRITERIA:
- A burst of ten `Winch` signals delivered before the settle window elapses produces exactly one `Size` call and exactly one hand-off.
- Each signal arriving while a window is open re-arms the timer — asserted by counting `Settle` calls and proving the first timer's fire is not acted on after a re-arm.
- A settled size equal to `--width`/`--height` produces no exec, no write and no terminal restore; the loop is still reading afterwards and a subsequent key still acts.
- A settled size differing in either dimension produces exactly one `resume-draw` exec carrying the same `--command`, `--report`, `--hook-key`, `--pane` and `--pane-key` the waiter was launched with.
- A non-empty `--report` survives the redraw unchanged, so a reported reason is still on the card after a resize.
- A key delivered while a settle window is open is dispatched immediately and produces its own hand-off; no second exec follows from the pending timer.
- A `Size` that errors at settle time produces a redraw rather than ending the wait, and the argv it produces is the one the draw resolves to its bounded fallback.
- Task 4.2's swallow table, terminal-restore table and non-terminal/raw-mode refusals all pass unchanged against the restructured loop.
- At most one read is outstanding at any moment, so the loop never reads ahead: a burst delivered after the byte the loop dispatched is still queued for the next process image.
- Nothing on the redraw path resolves a theme or renders — the redraw is a handover, so the resting process after it is a fresh wait.
- Task 4.2's signal guard passes unchanged with the resize and settle seams in place: the wait path's only `signal.Notify` names `syscall.SIGWINCH`, and SIGHUP, SIGTERM and SIGINT keep their default disposition.
- The settle timer's firing neither ends the wait nor clears the report, held by the two criteria above rather than by a guard.

STATUS: issues_found

SPEC CONTEXT: The spec says a resize is the same handover run backwards: the waiter replaces itself with a fresh draw, which draws at the new size and hands back to a fresh wait, so the resting state stays at the runtime floor. The redraw is taken once the size has settled, never once per change, so a drag costs one handover per pane rather than hundreds of launches a second. A pane too small for the card must still show a degraded panel with its key hints, because a pane that loses its hints reads as a restored pane with a dead keyboard. The report row stands until the next key press, and a resize is not a key press.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_wait.go:46: `resumeResizeSettle = 150 * time.Millisecond`
  - cmd/state_resume_wait.go:67-72: the `Winch` / `Settle` / `Size` seams
  - cmd/state_resume_wait.go:151-198: `resumeWaitLoop`. It selects over the read result, `Winch` (which re-arms `settled`, lines 187-188) and `settled` (lines 190-195, where the settled size either changes nothing or hands over).
  - cmd/state_resume_wait.go:268-271: `resumeSizeUnchanged`. A read error counts as changed, so it redraws.
  - cmd/state_resume_wait.go:298-311: `startResumeReader`, the request-driven one-byte reader. Only one read is ever outstanding.
  - cmd/state_resume_wait.go:397-400: `resumeRedraw`, which restores the terminal and then calls `resumeHandOff` with the payload unchanged.
  - cmd/state_resume_wait.go:425-429: `winchSignals`, whose only `signal.Notify` names `syscall.SIGWINCH`.
  - cmd/state_resume_wait.go:479-481: production wiring (`winchSignals()`, `time.After`, `paneSizeFromStdin`). `paneSizeFromStdin` (cmd/state_resume_draw.go:128-130) is the same `term.GetSize` read the draw takes.
- Notes:
  - Every step of the Do list is in place:
    - A `Winch` replaces whatever timer was armed and draws nothing itself.
    - A matching settle drops the window and `continue`s with the read still outstanding, so no second request is sent.
    - A differing or failed read restores the terminal, emits the `exec` INFO and execs `resume-draw` with `cfg.resumeChainPayload` unchanged.
    - Keys dispatch exactly as before through `resumeKeysFor`.
  - The size-error case hands over with the payload unchanged rather than "at the value it reported". That is sound: the draw registers `--width`/`--height` but never reads them (cmd/state_resume_draw.go:177-181). It measures the pane itself and resolves a failed read to the renderer's bounded fallback (cmd/state_resume_draw.go:40-43).
  - One defect in the handover boundary this task built is listed under FINDINGS: a size change landing while the redraw handover is in flight is never answered.

TESTS:
- Status: Adequate
- Coverage:
  - All the new tests are in cmd/state_resume_wait_resize_test.go:173-375 and drive `Winch`/`Settle`/`Size` as rendezvous channels, so each assertion is ordered against the loop rather than the clock:
    - burst of ten → armed 10, one `Size`, one hand-off (line 174)
    - re-arm, where the first timer's fire is proven not acted on because the loop is still taking a third resize with `sizes == 0` (line 196)
    - matching size → no exec, no stdout write, `restores == 0`, loop still arming (line 222)
    - key after a no-op settle still acts (line 241)
    - width-only / height-only / both differ → one `resume-draw` exec with `restoredAtExec == 1` (line 257)
    - report carried, compared against the whole argv (line 285)
    - key inside an open window → its own hand-off with `sizes == 0` (line 319)
    - `Size` error → redraw rather than an ended wait (line 335)
    - one read outstanding, with no overlap and `reads == 1` after the dispatched byte (line 349)
  - `assertHandOff` (cmd/state_resume_wait_test.go:150-178) compares the full argv, which pins `--command`, `--report`, `--hook-key`, `--pane` and `--pane-key`, and it checks the INFO `exec` marker.
  - Task 4.2's tables run against the restructured loop, with no settle seam needed because a nil `Winch` never selects:
    - swallow table: cmd/state_resume_screens_test.go:537-550
    - terminal-restore table and non-terminal/raw refusals: cmd/state_resume_wait_test.go:241-313
    - burst-left-unread test: cmd/state_resume_wait_test.go:221-238
    - signal guard: cmd/state_resume_wait_test.go:437-469
  - The idle-wait test carries the report across a settled resize (cmd/state_resume_wait_test.go:349-430).
- Notes:
  - No test drives a size change that lands between the draw's size read and the new waiter's `signal.Notify`. That is the gap the finding below describes.
  - No over-testing: the no-op-settle pair asserts distinct things (no side effects vs. the loop still acting).

CODE QUALITY:
- Project conventions: Followed. Seams are small function fields on the config, production defaults are wired only in `RunE`, tests use no `t.Parallel`, and the source guards are reused.
- SOLID principles: Good
- Complexity: Acceptable. The three-way select with the `outstanding` flag is clear, and the escape branch that shares it predates or postdates this task.
- Modern idioms: Yes (`sync.OnceFunc`, `for range 10`, buffered `signal.Notify` channel)
- Readability: Good
- Issues: None beyond the finding below.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [spreading] cmd/state_resume_wait.go:151-158 (with cmd/state_resume_draw.go:43) — What is wrong: the waiter compares the pane's size against `--width`/`--height` only when a settle window it armed on a `Winch` elapses. Nothing re-checks the size for changes delivered before the waiter's own `signal.Notify` (cmd/state_resume_wait.go:479). The draw reads the size first (state_resume_draw.go:43), then resolves the theme (the adaptive-pair appearance probe can take up to its timeout), paints and execs. The new waiter then starts its Go runtime and cobra before `winchSignals()` runs. A SIGWINCH delivered anywhere in that span goes to a process that is not watching for it and is dropped. The fresh waiter carries the pre-change size and waits for a signal that has already come and gone. The fix: once the waiter has installed its SIGWINCH watch, it compares the pane's size once and arms the settle window when a successful read differs from the drawn size. That check must not hand over when the read fails or the drawn size is non-positive; otherwise a draw that fell back to 0×0 and a waiter whose read also fails would hand the pane back and forth indefinitely. It must land with a harness case that makes `Size` differ at startup with no `Winch` delivered. — FAILS: a size change whose last signal lands during a redraw handover or a first draw goes unanswered. Examples: a second zoom toggle or `resize-pane` tap about 150-250 ms after the first, a drag resumed and stopped within the handover, or a client attaching (which resizes the window) while a pane is still drawing after a reboot. The pane then holds a card centred on a size that no longer exists, or clipped by a pane that shrank, with the key hints among the rows cut away. This is the exact state the task exists to remove, and it persists until some later, unrelated resize.

UNSETTLED:
- None
