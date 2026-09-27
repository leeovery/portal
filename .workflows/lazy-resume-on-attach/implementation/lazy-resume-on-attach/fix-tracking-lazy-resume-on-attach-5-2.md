## Attempt 1

ISSUES:
- /Users/leeovery/Code/portal/cmd/state_resume_wait.go:204-207 (`resumeRedraw`) with /Users/leeovery/Code/portal/cmd/state_resume_wait_resize_test.go:284-295. No test covers the task's "a resize while the confirmation is up redraws the confirmation" edge case. Every resize test uses `drawnPayload()` with `Screen` empty. `assertHandOff` builds its expected argv from that same payload, so a redraw that dropped or reset `Screen` would still pass all of them. The risk is near: the next task edits exactly this area (Escape going back to the panel, a failed discard redrawing the confirmation with its report). If that work resets `Screen` on the shared redraw path, a user who resizes a pane while `▲ Discard resume?` is up gets the waiting panel instead, and every test stays green. It would only show up in a live pane.
  FIX: Add a subtest (`"it redraws the confirmation on a resize while it is up"`) next to `"it carries the command and the report across a redraw"` in /Users/leeovery/Code/portal/cmd/state_resume_wait_resize_test.go, or in the new screen test file, using the existing harness. Steps: `payload := drawnPayload(); payload.Screen = resumeScreenDiscard; h := startResumeResize(t, payload, resizedSize); h.elapse(h.resize())`, check `h.wait()` returns no error, then `assertHandOff(t, h.probe, payload)`. Also assert directly that `h.probe.execArgs` ends with `flagArg(resumeFlagScreen), resumeScreenDiscard`. That way the check does not depend only on an expectation built by the same `resumeChainArgv` it is testing.
  ALTERNATIVE: Turn the existing "carries the command and the report across a redraw" subtest into a table over {panel, discard}. It is fewer lines but rewrites a test from an earlier task; I recommend the separate subtest.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- /Users/leeovery/Code/portal/cmd/state_resume_chain.go:28-30 — "reads as it always has" is change history.
  OLD: // The screens one draw of the waiting pane can put up. The panel is the chain's
// default and names no flag, so every argv composed for it reads as it always
// has; any value the selector does not recognise draws the panel.
  NEW: // The screens one draw of the waiting pane can put up. The panel is the chain's
// default and names no flag; any value the selector does not recognise draws
// the panel.
- /Users/leeovery/Code/portal/cmd/state_resume_draw.go:58-61 — this comment has three problems: "the one point the pane's screens differ" is a claim that it is the only such place, which the waiter's per-screen key dispatch will make false; the drift sentence is the plan's reasoning; and the last sentence restates the two-line body. The unrecognised-draws-panel rule is already stated on the constants in state_resume_chain.go.
  OLD: // renderResumeScreen is the one point the pane's screens differ: everything
// around the paint is shared, so the confirmation cannot drift from the panel
// in the size read, the theme or the hand-off. Any selector but the discard
// confirmation's draws the waiting panel.
  NEW: (delete)
- /Users/leeovery/Code/portal/cmd/state_resume_draw.go:156-157 — the new `--screen` registration (line 160) now sits under "Registered but not read", which reads as covering `--screen`, and `--screen` is read. Both lines keep their single leading tab.
  OLD: 	// Registered but not read: a draw measures the pane itself, and a flag the
	// waiter hands back that this command did not register would fail its parse.
  NEW: 	// Width and height are registered but not read: a draw measures the pane
	// itself, and a flag the waiter hands back that this command did not
	// register would fail its parse.
- /Users/leeovery/Code/portal/cmd/state_resume_draw_screen_test.go:15 — restates the three-line body.
  OLD: // drawScreenPayload is the sample payload naming the given screen.
  NEW: (delete)
- /Users/leeovery/Code/portal/cmd/state_resume_draw_screen_test.go:22-23 — restates the function body.
  OLD: // drawScreen runs one draw of payload at w by h and answers the painted bytes
// with the alternate-screen entry and cursor home stripped, alongside the probe.
  NEW: (delete)
- /Users/leeovery/Code/portal/cmd/state_resume_draw_screen_test.go:319-320 — restates the switch below it.
  OLD: // labellingWriter records each write the draw makes to its stdout, naming the
// alternate-screen entry and the cursor home and calling anything else a paint.
  NEW: (delete)

NOTES:
- Production code never puts up the discard screen yet. `resumeAnswerDiscard` (/Users/leeovery/Code/portal/cmd/state_resume_wait.go:195-197) redraws with the payload unchanged, so `d` still redraws the panel. The waiter also does not act on `Screen` yet, so Enter on a confirmation would resume. Both belong to the next task's dispatch table, as this task's context says, and neither can be reached today.
- In the fallback table in state_resume_draw_screen_test.go:69-70, the "absent" and "empty" rows pass the same value to `runResumeDraw`. The case where the flag is absent from the command line is covered separately: `TestStateResumeDrawCommand` compares the whole parsed payload (`Screen` included), so an absent flag is shown to parse to `""`. Not a finding.
- The hidden commands' `Short` strings (state_resume_draw.go:120, state_resume_wait.go:246) still describe only the resume panel. They are not user-visible, and the diff did not touch them.
