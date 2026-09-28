TASK: The Command Block and the Report Row Both Screens Share (lazy-resume-on-attach-3-1, tick-567651)

ACCEPTANCE CRITERIA:
- Every row `resumeCommandRows` returns has `lipgloss.Width == resumeCardContentWidth` when it is called at that width, for a one-character command, a command that wraps to exactly three lines, and a command ten times longer than three lines.
- Called at a width narrower than the card's — the width a degraded stack passes — both helpers return rows at exactly that width, wrapped and `…`-marked to it, so neither ever hands the canvas a row it has to cut.
- A command that wraps beyond three lines returns exactly three rows whose third ends in `…`, and that third row's display width is the pinned width.
- A single unbroken token longer than the width is broken at the width rather than overflowing: three rows, each at the pinned width, the third ending in `…`.
- A command carrying double-width runes (CJK) or combining marks produces rows whose display width is still exactly the pinned width — the measurement is display width, not byte or rune count.
- A command carrying `\n`, `\t`, `\x1b[31m` or `\x07` produces the same row count and the same row widths as the same command with those bytes replaced by spaces, and the stripped output carries no control character.
- `resumeReportRow` returns `ok == false` and an empty row for an empty report, and for a non-empty one returns exactly one row at the pinned width, truncated with `…` rather than wrapped, whatever its length.
- An empty command renders one empty row at the pinned width and does not panic.
- Both helpers render through `headerStyle` / `headerPadRight`, so `internal/tui`'s colour-literal guard passes with no exemption added.

STATUS: complete

SPEC CONTEXT: The waiting panel (spec 5.3) and the discard confirmation (5.4) both render the registered command, wrapped within the card's inner width over at most three lines with anything beyond marked `…`, the card's width unchanged; the confirmation renders it in `state.destructive` where the kill modal puts the session name. Each screen carries one optional single-line report row between the command and the key hints, present only when there is something to say. Every colour is a theme token (5.2), enforced by the package's colour-literal guard; below the card's size both screens degrade to a plain stack at the pane's width (5.2, 5.4).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/resume_panel_parts.go:13 — `resumeCardContentWidth = destructiveBodyWidth` (52, internal/tui/destructive_confirm.go:15)
  - internal/tui/resume_panel_parts.go:21-28 — `sanitiseCommandText`: every rune < 0x20 and 0x7f mapped to a single space, nothing else collapsed
  - internal/tui/resume_panel_parts.go:30-32 — `resumeCommandLines`: sanitise, then `wrapCapped(..., resumeCommandMaxLines, "…")` (internal/tui/text_wrap.go:11-19), which joins the rows past the cap onto the third and truncates there with `…`
  - internal/tui/resume_panel_parts.go:34-45 — `resumeCommandRows`: `headerStyle(tok, …)` plus `.Bold(true)` when asked, each line padded to `width` via `resumePaddedRow` -> `headerPadRight` (internal/tui/header.go:127)
  - internal/tui/resume_panel_parts.go:49-56 — `resumeReportRow`: `("", false)` on empty; otherwise sanitised, `ansi.Truncate(…, width, "…")`, rendered in `accent.attention`, padded to `width`
  - Callers: internal/tui/resume_panel.go:55 (card, pinned width, text.primary), :65 (stack, pane width), :71 (report); internal/tui/resume_discard_confirm.go:43 (state.destructive bolded), :48 (report)
- Notes: The wrap no longer calls `ansi.Wrap` directly as this task's text describes. It goes through `wrappedLines` / `wrapCapped` in internal/tui/text_wrap.go, which later planned tasks 3-7 and 3-8 introduced to re-flow `ansi.Wrap`'s hyphen-run overshoot and to share the wrap-and-cap logic with the notice band and theme panel. That is a sound evolution that strengthens this task's guarantee, not a loss. The width invariant holds by construction: every line is either the trimmed `fits` (checked <= width), the head of `ansi.Truncate(line, width, "")`, or `ansi.Truncate(beyond, width, "…")`, and is then padded to exactly `width` by `headerPadRight`. The row count is capped at three structurally. An empty command reaches `rowAndCarry("", w)` -> `("", "")`, so it returns one empty line with no panic. No `lipgloss.Color` literal appears in the file, and internal/tui/colour_literal_guard_test.go carries no exemption list.

TESTS:
- Status: Adequate
- Coverage: All twelve named tests exist in internal/tui/resume_panel_parts_test.go, each run over dark/light x colour/colourless through `forEachResumePartsRender` (:29):
  - short command (:121)
  - exactly three wrapped rows, pinned (:127)
  - `…` on the third row for a command ten times longer than three lines, with the ellipsis replacing content (:165)
  - 500-char unbroken token -> 52/52/51+`…` (:211)
  - CJK and genuinely decomposed combining marks (`résumé`, :224) (:221)
  - `\n` / `\t` / `\x1b[31m` / `\x07` plus `\x7f` and `\x1f`, compared by `reflect.DeepEqual` against the spaced variant, with a raw-row check that `\x1b[31m` never reaches the output (:256)
  - lengths 1/40/52/53/500 (:397)
  - report truncation (:506)
  - both helpers at the card's and a narrower pane's width (:421)
  - empty report (:562)
  - empty command (:411)
  - token and emphasis for text.primary unbolded vs state.destructive bolded, discriminating the bold SGR prefix and the colourless no-escape case (:463)

  Narrow widths 1-4 are pinned for both helpers (:319, :532). Every assertion measures with `lipgloss.Width` / `ansi.StringWidth` and reads `ansi.Strip`ped text.
- Notes: Each assertion would fail if its behaviour broke. Removing the sanitiser breaks the spaced-equality check, removing the padding breaks `assertRowsAtWidth`, and removing the cap or the join-before-truncate breaks the pinned rows. Running the pure-geometry cases across four theme/colourless combinations is redundant in principle, but the task asked for exactly that coverage. The file's invariant sweeps (widths 1-80) belong to the later tasks 3-7 and 3-8 and are outside this task's assessment.

CODE QUALITY:
- Project conventions: Followed. Theme tokens only, rendered through the shared `headerStyle` / `headerPadRight` helpers. No `t.Parallel()`. Comments state why rather than restating code and name no process artefacts.
- SOLID principles: Good. Sanitise, wrap-and-cap, style and pad are separate steps, and colour and emphasis are the caller's choice.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
