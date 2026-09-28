TASK: lazy-resume-on-attach-7-1 (tick-e33f05) — A Kill Key Pressed Between Two Screens Is Swallowed, Not Fatal

ACCEPTANCE CRITERIA:
- A pane just restored as waiting swallows a Ctrl-C, Ctrl-\ or Ctrl-Z pressed while its first panel is being drawn: the panel comes up and waits, and the pane and its session stay open
- Ctrl-C, Ctrl-\ or Ctrl-Z pressed while a waiting pane moves between screens — just after `d`, just after Escape on the confirmation, during a resize redraw, during a report redraw — ends and stops nothing: the next screen comes up and waits for its own keys
- A SIGINT or SIGQUIT delivered to a waiting pane's whole process group while its draw or waiter runs ends in the recovery tail: the pane leaves the panel's screen, its pending marker is cleared and it lands at the user's shell, instead of the pane (and a single-pane session) closing
- After Enter, after a confirmed discard, after the recovery tail hands a pane its shell, and on a pane that never waits (an eager registration, or one whose pending marker could not be written), a Ctrl-C interrupts the command running in the pane as usual
- An answer that cannot be carried out — a pending marker that refuses to clear on Enter or on a confirmed discard, or a discard the store refuses — redraws with its report and still swallows Ctrl-C, Ctrl-\ and Ctrl-Z
- With signal generation off, the panel and the confirmation still render every row from the pane's left edge, card and plain stack alike

STATUS: issues_found

SPEC CONTEXT: The waiting-pane section says the waiter swallows every byte but Enter and `d`, including Ctrl-C, Ctrl-\, Ctrl-Z and Ctrl-D, and that the pane survives a waiter that dies because the parked shell below it runs a recovery tail that clears the pending marker and hands the pane the user's shell. Every screen change (d, Escape, resize, report) is a fresh draw that hands back to a fresh wait, so the pane passes through a cooked gap on each one. This task makes the kill-key swallow hold across those gaps rather than only inside a waiter's raw mode, and makes the parked shell survive a group signal. The install is almost entirely single-pane sessions (43 of 44), so a closed pane usually means a closed session and a transcript dropped from sessions.json.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/tty_signals.go:14-20 — clearTTYSignals / setTTYSignals toggle Lflag ISIG alone through updateTTYModes (:33-40); stdin forms at :42-48. OPOST is untouched.
  - cmd/state_hydrate.go:262-274 — execResumeChainAndExit calls cfg.DisableTTYSignals before exec'ing the parked chain, logs a WARN on failure and still parks. Production wiring is at :422 (clearStdinSignals).
  - cmd/state_hydrate.go:276-280 — parkedChainTrap = "trap : INT QUIT; ", a caught no-op trap whose comment says it must stay caught. It prefixes the chain at :299-304.
  - cmd/state_resume_wait.go:315-326 (Enter) and :343-356 (confirmed discard) — cfg.restore() and then enableTTYSignalsOrLog before the hand-off. Wiring is setStdinSignals at :487.
  - cmd/state_resume_recover.go:56 — the tail turns signals back on before exec'ing the shell, wired to cookStdin at :94. The answered-pane early return at :39-41 correctly leaves the tty alone.
  - cmd/state_resume_wait.go:397-400 (resumeRedraw), cmd/state_resume_draw.go:37-73, cmd/state_resume_wait.go:435-442 and internal/tui/pane_appearance.go:97-101 / :165-172 — these did not change. Each one restores the termios it found, so signal generation stays off across every screen hand-off. Refused clears and refused discards route through resumeReport -> resumeRedraw (cmd/state_resume_wait.go:363-381, :344-346) and never turn signals on.
  - cmd/state_resume_chain.go:177-181 — enableTTYSignalsOrLog: a failure is logged and the pane is still handed on.
- Notes: The code matches the plan's Do list item for item. The eager path and the marker-refused path never reach execResumeChainAndExit (cmd/state_hydrate.go:239-252, :348-356), so their tty is never touched and criterion 4's never-waits cases hold. Order is correct on both answers: the restore runs before the enable, so the enable is not undone by a restore that puts back ISIG-off. Terminal-generated SIGTSTP is discarded anyway for the pane's orphaned process group (the tmux pane process is a session leader), so the Ctrl-Z coverage is belt-and-braces. This is correct, not a gap.

TESTS:
- Status: Adequate (one production wiring unobserved — see FINDINGS)
- Coverage:
  - cmd/tty_signals_pty_test.go:80-171 (darwin, real pty):
    - With ISIG cleared, Ctrl-C and Ctrl-\ do not end a foreground process on the pty, and Ctrl-C/Ctrl-\/Ctrl-Z reach the reader as bytes.
    - Positive control: a set restores Ctrl-C's interrupt.
    - A MakeRaw/Restore round trip leaves ISIG off.
    - Clearing ISIG leaves OPOST on.
  - cmd/tty_signals_pty_test.go:173-215 — the panel and the confirmation, each as a card (100x30) and as a plain stack (24x6), reach a pty with ISIG cleared with no bare LF. This covers criterion 6.
  - cmd/state_resume_signals_test.go:
    - :43-67 — a real /bin/sh group SIGINT and SIGQUIT sent from the draw ends in the recovery tail. The test also fails if the trap became an ignore, because the draw would survive.
    - :69-90 — ISIG is cleared exactly once, before the exec, on all three hydrate tails.
    - :92-101 — the literal trap prefix.
    - :103-119 — a disable failure still parks and logs a WARN.
    - :121-153 — never cleared for eager, no-registration and marker-refused panes.
    - :163-209 — Enter and the confirmed discard enable after the raw restore and before the exec, and still hand on when the enable fails.
    - :211-270 — d, Escape, a resize, a refused clear on Enter or discard, and a refused discard all redraw with no enable.
    - :302-354 — recovery-tail enable ordering and failure, and no enable on an answered pane.
  - cmd/tty_modes_pty_test.go:82-123 — the resume-wait and resume-recover command wirings on a real pty.
- Notes:
  - The "with signal generation cleared, Ctrl-Z ends nothing" subtest (cmd/tty_signals_pty_test.go:87-101 run with ttySuspendKey) cannot fail: the kernel discards SIGTSTP for the orphaned group, and a stopped process is not an ended one. The helper's own comment at :27-31 acknowledges this. Ctrl-Z coverage is carried by the byte subtest at :103-117, so no coverage is lost; this is noted only.
  - Otherwise focused, with no redundant bloat.

CODE QUALITY:
- Project conventions: Followed. Seams are injected as config fields with production wiring in the cobra RunE. Tests stage through hydrateCfg/withFuncSeam. There is no t.Parallel. The pty and /bin/sh subprocesses are ones the tests spawn themselves.
- SOLID principles: Good. updateTTYModes is the single termios read-modify-write; the clear, set and cook operations are thin closures over it.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The comments hold against the code (cmd/state_hydrate.go:56-58, :260-261, :276-279; cmd/state_resume_wait.go:84-87; cmd/tty_signals.go:11-13).
- Issues: None beyond the finding below

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/state_hydrate.go:422 — Criteria 1 and 2 rest entirely on the production wiring `DisableTTYSignals: clearStdinSignals`: every later hand-off only restores the termios this one call left. None of the suites that exercise this path reads it:
  - hydrateCfg defaults the seam to a no-op for every case that does not name it (cmd/state_hydrate_test.go:1032-1034).
  - The `state hydrate` Execute in cmd/state_test.go:151-153 stubs hydrateRunFunc without reading the config it is handed.
  - The two lazy-panel integration suites (internal/restore/lazy_resume_panel_integration_test.go, internal/restore/lazy_resume_discard_integration_test.go) send no kill key.
  The sibling wirings for resume-wait and resume-recover are guarded on a real pty (cmd/tty_modes_pty_test.go:82-123). Fix: add a third case beside them:
  1. Stub hydrateRunFunc (cmd/state_hydrate.go:395) through withFuncSeam to capture the config.
  2. Execute `state hydrate --fifo <tmp> --file <tmp>`.
  3. Point stdin at a pty slave with withStdin (cmd/tty_modes_pty_test.go:48).
  4. Call the captured DisableTTYSignals.
  5. Assert that Lflag equals its prior value with only ISIG cleared, and that Iflag and Oflag (OPOST included) are unchanged.
  — FAILS: suppose the wiring is changed to setStdinSignals, to cookStdin, to a no-op, or to a raw-mode helper that also clears OPOST. The whole suite still passes, but in production:
    - a Ctrl-C or Ctrl-\ pressed during the first draw or any screen change once more kills the draw;
    - the parked shell's trap then runs the recovery tail, which drops the pane off its panel, clears the pending marker and gives up the offered resume for that boot — criteria 1 and 2 broken;
    - or, with an OPOST-clearing helper, every row after the first stair-steps off the pane's left edge (criterion 6).

UNSETTLED:
- None
