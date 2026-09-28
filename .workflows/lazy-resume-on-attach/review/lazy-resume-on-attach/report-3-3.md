TASK: lazy-resume-on-attach-3-3 — The Waiting Panel (RenderResumePanel over ResumeScreen; renderHeaderWithBadge gains a badge-text parameter)

ACCEPTANCE CRITERIA:
- The card renders `Resume session` on the left of its header row and `● PAUSED` on the right, the badge in `accent.attention`, in the slot `renderHeaderWithBadge` gives `◉ EDIT MODE` and pinned to `resumeCardContentWidth`.
- The body renders `ON RESUME` in `accent.primary` on its own row with the command beneath it in `text.primary`, and the footer renders `⏎ resume` and `d discard` through the shared confirm/cancel footer.
- With a non-empty report the card carries exactly one extra row, between the command and the key-hint footer; with an empty report the card carries exactly its three compartments and no blank filler row.
- The card's rendered width is identical for a one-character command, a three-line command, a long report and an empty report — no content moves the frame.
- The rename modal and the edit modal render byte-identically to before the badge parameter (their existing suites and the kill/delete byte-exact goldens all pass unchanged).
- Under `colourless` the stripped render is identical to the coloured one, so `●`, `PAUSED`, `ON RESUME`, `⏎` and `d` carry the state with no hue.
- Below the card's size the panel renders the plain stack carrying the title, the command, the report when present and the key hints, and never an empty screen; neither the `ON RESUME` label nor the `● PAUSED` badge appears on it.
- A four-row pane holding a command that wraps to two rows renders the title, both command rows and the key hints — the stack spends no row on anything the small-pane form does not carry.
- Stripping the fixed constants and the caller's command and report from the rendered screen leaves no alphabetic text — the screen names no tool and carries no copy beyond what this task declares.
- `internal/tui`'s colour-literal guard passes with no exemption added.

STATUS: complete

SPEC CONTEXT: Spec §5.2 fixes the shape (a full-pane canvas in the active theme with a compact card centred on it, built from the rename modal's grammar through `renderJoinedPanel`; below the card's size the title, the command and the key hints stack plainly without the frame; every colour is a token; NO_COLOR keeps state glyph-backed; every string is tool-agnostic). Spec §5.3 fixes the content exactly: header `Resume session` plus a `● PAUSED` badge in `accent.attention` in the slot the rename modal gives `◉ EDIT MODE`; body `ON RESUME` in `accent.primary` over the command; footer `⏎ resume` / `d discard`; one report row between the command and the key hints only when there is something to report. Spec §5.5 names the Nord reference frame `testdata/vhs/reference/resume-panel-waiting-nord.png`. I compared the implementation against that frame for structure and colour roles, and they match.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/resume_panel.go:8-16 — fixed copy constants, verbatim from the spec
  - internal/tui/resume_panel.go:20-27 — `ResumeScreen {Command, Report, Width, Height, Theme, Colourless}`
  - internal/tui/resume_panel.go:32-38 — `RenderResumePanel` routes through `renderPaneScreen` with card and stack builders
  - internal/tui/resume_panel.go:40-45 — card built from three compartments through `renderJoinedPanel`
  - internal/tui/resume_panel.go:47-50 — header: title in `text.primary` bold, then `renderHeaderWithBadge(title, resumeCardContentWidth, true, resumePausedBadge, …)`
  - internal/tui/resume_panel.go:52-57 — body: `ON RESUME` in `accent.primary`, command rows in `text.primary` unbolded, then the report row when present
  - internal/tui/resume_panel.go:62-68 — plain stack: title, command wrapped to the pane width, report, key hints. There is no badge and no label, and the title is first and the hints last.
  - internal/tui/resume_panel.go:77-79 — key hints through the shared `renderConfirmCancelFooter`
  - internal/tui/edit_modal.go:157-170 — `renderHeaderWithBadge` takes `badgeText`, and the hidden-badge blank is cut to its width
  - internal/tui/edit_modal.go:153 and internal/tui/rename_modal.go:35 — both existing callers pass `editModeIndicator`
  - CLAUDE.md, `tui` row — the requested clause about `resume_panel.go` / `resume_panel_parts.go` / `resume_pane_canvas.go` is present
- Notes:
  - The `resumeKeyDiscard = "d"` constant the task lists is not declared. The key hint instead renders `string(resumekeys.Discard)` (internal/tui/resume_panel.go:78), which comes from the leaf package the waiter also dispatches on. This is a sound divergence: the key the footer shows and the key that acts come from one declaration, so they cannot drift apart. The rendered text is still `d discard`, and the test at internal/tui/resume_panel_test.go:120-127 pins that literal.
  - With `badgeText == editModeIndicator`, the `renderHeaderWithBadge` body is token-for-token the pre-parameter logic. The rename and edit headers therefore stay byte-identical, and neither modal's test file was touched by this work.
  - The card's width is fixed. The header is pinned to `resumeCardContentWidth` (52). The command and report rows are padded to the same width (task 3-1). The footer's natural width is smaller. So `renderJoinedPanel`'s widest-row width is always 52, and the card is 58 wide whatever the content.
  - In a very narrow pane (under about 20 columns) the single key-hint row is truncated by `clampStackToPane` rather than re-flowed. This is the plan's deliberate row-budget choice (task 3-7), not a defect of this task.

TESTS:
- Status: Adequate
- Coverage: All 14 planned tests are present in internal/tui/resume_panel_test.go:
  - Header slot golden (82-90).
  - Full-card goldens with no report and with a report (92-118). These pin exactly two dividers and one extra report row. Blank rows inside the card keep their frame glyphs, so `nonEmptyRows` cannot hide a filler row.
  - Width table (144-164): one character, three wrapped lines, long report, empty report, empty command.
  - Token-role SGR assertions over dark and light (168-210): badge in `accent.attention`, title in `text.primary`, label in `accent.primary` with the command on the next row in `text.primary`, keys in `accent.key` and labels in `text.muted`.
  - Title bold on both the card and the stack (129-142).
  - Plain stack below card size, with the label, badge and frame absent (229-243).
  - Four-row pane with a two-row command (245-249).
  - Smallest panes: 3, 2 and 1 rows (251-266).
  - No-own-copy check over both the card and the stack (285-303).
  - NO_COLOR stripped-parity check with no token fg/bg painted and glyphs and words kept (306-338).
  - Rename and edit headers pinned byte-for-byte in colour and colourless, across hidden and shown badge states (356-406).
  - The card and stack suites run over dark, light, dark-colourless and light-colourless (`forEachResumePartsRender`).
- Notes: The single header-slot test overlaps with the full-card golden, but both were planned and each is small. Every test would fail if the behaviour it names broke.

CODE QUALITY:
- Project conventions: Followed. The new code reuses the shared card grammar (`renderJoinedPanel`, `renderHeaderWithBadge`, `renderConfirmCancelFooter`, `headerStyle`), uses theme tokens only with no hex (so the colour-literal guard holds; the guard file was not modified), and is a pure function with no model state, as the plan intended.
- SOLID principles: Good. The card and stack builders are separate small functions, and the size decision stays in `renderPaneScreen`.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The comments state why, not what.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
