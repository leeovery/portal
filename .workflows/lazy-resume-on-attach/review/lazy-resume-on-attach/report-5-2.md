TASK: lazy-resume-on-attach-5-2 (tick-bc0022) — The Confirmation Is a Screen the Chain Can Draw

ACCEPTANCE CRITERIA:
- `--screen discard` paints bytes byte-identical to `tui.RenderResumeDiscardConfirm` called with the same `ResumeScreen`; the waiting panel's bytes are still byte-identical to `tui.RenderResumePanel`.
- An absent `--screen`, an empty one, and an unrecognised value (`"panel"`, `"DISCARD"`, `"x"`) each paint the waiting panel, so no chain argv composed before this task changes meaning.
- `resumeChainArgv` for a payload naming the waiting panel is byte-identical to the argv it composed before this task, and the discard payload's argv differs only by `--screen discard`.
- A draw of either screen produces the same seam-call sequence — one `Size`, one `ResolveTheme`, one alternate-screen entry write, one paint, one `ExecSelf`.
- The `resume-wait` argv the draw execs carries `--screen discard` when the confirmation was drawn, and carries no `--screen` when the panel was.
- A non-empty `--report` renders on whichever screen is drawn, on that screen's own report row.
- At a pane size below the card's, the confirmation degrades to the plain stack exactly as the panel does — the size ladder is the renderer's and this task adds no second decision.
- `log.ResolveProcessRole` is unchanged: no subcommand name is added, so the closed role space gains no member.
- `stateResumeWaitCmd` accepts `--screen` and carries it onto its payload.

STATUS: complete

SPEC CONTEXT: The discard confirmation is the picker's kill modal retitled, built through the shared destructive-confirm builder, carrying its own single report row and degrading to a plain stack below the card's size exactly as the waiting panel does. Every screen the pane shows is a fresh draw that hands off to a fresh wait (a resize, `d`, Escape and a reported failure all take the same handover). The three hidden `state` subcommands map to the existing hydrate process role, and the closed role space does not grow. The phase's structural decision is a screen selector on the existing chain commands rather than a fourth subcommand.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_chain.go:25 — `resumeFlagScreen = "screen"` added to the flag-name constants
  - cmd/state_resume_chain.go:32-35 — `resumeScreenPanel = ""`, `resumeScreenDiscard = "discard"`
  - cmd/state_resume_chain.go:54 — `Screen string` on `resumeChainPayload`
  - cmd/state_resume_chain.go:83-85 — `--screen discard` emitted only when `p.Screen == resumeScreenDiscard`; panel and unrecognised values emit nothing, so pre-task argvs are unchanged (the same shape the older `TestResumeChainArgv` in cmd/state_resume_chain_test.go:19-50 pins)
  - cmd/state_resume_draw.go:75-80 — `renderResumeScreen` selects `tui.RenderResumeDiscardConfirm` for `"discard"` and `tui.RenderResumePanel` for every other value
  - cmd/state_resume_draw.go:43-72 — body order kept (size → theme → alternate-screen entry → cursor home → paint via `renderResumeScreen` → hand-off, whose `exec` INFO sits immediately before the exec in `resumeHandOff`, cmd/state_resume_chain.go:124-125); the screen rides the payload into the `resume-wait` argv
  - cmd/state_resume_draw.go:147,157,182 and cmd/state_resume_wait.go:460,471,500 — `--screen` registered on both commands (default `resumeScreenPanel`) and populated onto the payload each builds
  - internal/log/process_role.go:27 — role table unchanged; the flag adds no argv name, and `subcommandPath` skips flag tokens so `path[1]` still resolves to `resume-draw`/`resume-wait`
- Notes: The draw calls the production renderers directly with the payload's `ResumeScreen`, so there is no wrapper layout that could drift, and the card-or-stack decision stays in `renderPaneScreen` (internal/tui/resume_pane_canvas.go:20-28), which both renderers route through. An unrecognised screen is treated consistently end to end: the draw paints the panel, `resumeChainArgv` drops the value, and the waiter dispatches panel keys (`resumeKeysFor`, cmd/state_resume_wait.go:135-147), so offered and dispatched keys cannot disagree. Later-task code on the same path (the `DropInput` clear and the forced-panel fallback when the input drop fails, cmd/state_resume_draw.go:45-58) sits outside this task and does not disturb its criteria.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/state_resume_draw_screen_test.go:48-59 — confirmation bytes equal `RenderResumeDiscardConfirm`, and differ from the panel's
  - :61-83 — table over `""` (twice), `"panel"`, `"DISCARD"`, `"x"`, each painting `RenderResumePanel`; the true absent-flag parse (flag default) is exercised by `TestStateResumeDrawCommand` (cmd/state_resume_draw_test.go:352-385), whose payload carries no screen and expects `Screen == ""`
  - :85-122 — exact seam-call sequence `size, theme, write:alt-screen, write:cursor-home, write:paint, exec` for the panel, and equality of the confirmation's sequence with it
  - :124-156 — `--screen discard` present on the exec'd `resume-wait` argv for the confirmation, absent for the panel
  - :158-183 — report present and bytes matching each screen's renderer, for both screens
  - :185-210 — confirmation at 100x30 and 80x24 framed, at 30x12 (below the 58-column card) the plain stack, bytes matching the renderer at every size
  - :213-265 — panel argv hardcoded against the pre-task shape for both `resume-draw` and `resume-wait`; discard argv equals the panel's plus `--screen discard`; `resume-recover` carries no screen
  - :270-314 — both commands driven end to end through Cobra, proving each registers and reads `--screen`
  - internal/log/process_role_test.go:19-20 — `resume-draw --screen discard` and `resume-wait --screen discard` resolve to `hydrate`
  - cmd/state_resume_wait_resize_test.go:298-317 — a settled resize on the confirmation redraws the confirmation (edge case carried from the task)
- Notes: Each test would fail if the selector were ignored, inverted, or dropped from either command's registration or from the argv. The `"absent"` and `"empty"` rows at lines 66-67 are the same value, but that is a redundant row rather than a gap, since absence is covered at the parse layer.

CODE QUALITY:
- Project conventions: Followed (function seams staged through `withFuncSeam`; `resetRootCmd` resets state-child flags, so the `--screen discard` left on the shared command instances by `TestStateResumeCommands_Screen` does not leak into later parse tests; no `t.Parallel`)
- SOLID principles: Good — one selection point, and both screens share one draw and one hand-off path
- Complexity: Low
- Modern idioms: Yes
- Readability: Good — the constants' comment (cmd/state_resume_chain.go:29-31) states the default and the unrecognised-value rule, and it matches the code
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
