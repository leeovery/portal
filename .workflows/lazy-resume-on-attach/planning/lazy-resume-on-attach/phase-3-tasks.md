# Phase 3: The resume panel and its discard confirmation — 6 tasks

## lazy-resume-on-attach-3-1

### Task 3.1: The command block and the report row both screens share

**Problem**: Both screens a waiting pane can show render the same two variable pieces — the registered command, and a reason when an answer could not be carried out — and both must render them identically. The command is arbitrary user-authored text: it can be hundreds of characters (a realistic one carries a directory and a session identifier), it can hold a double-width or combining rune, and it can hold control bytes a hand edit or a copy-paste put there. Nothing in `internal/tui` renders a multi-line wrapped block at a pinned width today: `destructiveNameRow` renders one row and `deleteModalPathRow` truncates to one. A command rendered naively would move the card's width with its own length, overflow the frame `renderJoinedPanel` builds around it, or — with an escape sequence in it — repaint the rest of the card.

**Solution**: One shared parts file holding the pinned card content width, a command block that sanitises then wraps to at most three rows marked `…` beyond, and a single-row report, each row padded to the pinned width so the card's geometry is a constant rather than a function of its content. The colour token and emphasis are the caller's, because the waiting panel renders the command in `text.primary` under its label and the confirmation renders it in `state.destructive`.

**Outcome**: A command of any length, rune width or byte content produces exactly the same card width, over at most three rows; a report is one row or no row at all; nothing in either block can widen, lengthen or repaint the card.

**Do**:
- Add `internal/tui/resume_panel_parts.go` with `const resumeCardContentWidth = destructiveBodyWidth` — one pinned content width both screens' card geometry is built on, taken off the destructive builder's existing wrap target so the two cards match.
- Add `sanitiseCommandText(s string) string`: replace every rune below `0x20` and `0x7f` with a single space (which neutralises `\n`, `\t` and the `ESC` that opens an escape sequence in one rule, leaving the rest of that sequence as literal text), then collapse nothing else — the command is shown as it is stored.
- Add `resumeCommandLines(command string, width int) []string`: sanitise, `ansi.Wrap(command, width, "")` (word-wrap that hard-breaks a word longer than the limit), split on `\n`; return at most three lines, and when there are more, replace the third with `ansi.Truncate(strings.Join(lines[2:], " "), width, "…")`. An empty command returns one empty line.
- Add `resumeCommandRows(command string, width int, tok theme.Token, bold bool, th theme.Theme, colourless bool) []string`: each line from `resumeCommandLines(command, width)` rendered through `headerStyle(tok, th, colourless)` (with `.Bold(true)` when `bold`) and padded to `width` with `headerPadRight`. A card call passes `resumeCardContentWidth`, so the widest row of the card is always the pinned width; a degraded stack passes the pane's width, so the rows it stacks are wrapped and `…`-marked at the width they will actually be shown at rather than cut mid-word by the canvas.
- Add `resumeReportRow(report string, width int, th theme.Theme, colourless bool) (string, bool)`: `("", false)` for an empty report; otherwise one row — sanitised, `ansi.Truncate(report, width, "…")`, rendered in `accent.attention` and padded to `width`. Never wrapped. The width is the caller's for the same reason the command block's is.
- Cover both in `internal/tui/resume_panel_parts_test.go` over `testDarkTheme(t)` / `testLightTheme(t)` and both colourless values, asserting on `lipgloss.Width` of the rendered rows and on `ansi.Strip`ped text.

**Acceptance Criteria**:
- [ ] Every row `resumeCommandRows` returns has `lipgloss.Width == resumeCardContentWidth` when it is called at that width, for a one-character command, a command that wraps to exactly three lines, and a command ten times longer than three lines.
- [ ] Called at a width narrower than the card's — the width a degraded stack passes — both helpers return rows at exactly that width, wrapped and `…`-marked to it, so neither ever hands the canvas a row it has to cut.
- [ ] A command that wraps beyond three lines returns exactly three rows whose third ends in `…`, and that third row's display width is the pinned width.
- [ ] A single unbroken token longer than the width is broken at the width rather than overflowing: three rows, each at the pinned width, the third ending in `…`.
- [ ] A command carrying double-width runes (CJK) or combining marks produces rows whose display width is still exactly the pinned width — the measurement is display width, not byte or rune count.
- [ ] A command carrying `\n`, `\t`, `\x1b[31m` or `\x07` produces the same row count and the same row widths as the same command with those bytes replaced by spaces, and the stripped output carries no control character.
- [ ] `resumeReportRow` returns `ok == false` and an empty row for an empty report, and for a non-empty one returns exactly one row at the pinned width, truncated with `…` rather than wrapped, whatever its length.
- [ ] An empty command renders one empty row at the pinned width and does not panic.
- [ ] Both helpers render through `headerStyle` / `headerPadRight`, so `internal/tui`'s colour-literal guard passes with no exemption added.

**Tests**:
- `"it renders a short command as one row at the pinned width"`
- `"it wraps a long command over at most three rows"`
- `"it marks the third row with … when the command runs past it"`
- `"it breaks a single unbroken token at the inner width"`
- `"it keeps the pinned width for double-width and combining runes"`
- `"it neutralises control characters without changing the geometry"` (table: `\n`, `\t`, `\x1b[31m`, `\x07`)
- `"it renders the same width whatever the command's length"` (table over 1, 40, 52, 53, 500 characters)
- `"it renders a report as one truncated row"`
- `"it renders both blocks at the width its caller chose"` (table: the card's pinned width, a narrower pane's)
- `"it reports no row at all for an empty report"`
- `"it renders an empty command as one empty row"`
- `"it renders the command in the token and emphasis its caller chose"` (table: `text.primary` unbolded, `state.destructive` bolded)

**Edge Cases**:
- A command longer than three wrapped lines is marked `…` on the third — the remainder is joined into the third line before truncation so the `…` replaces content rather than being appended to a line that already fits.
- A single unbroken token wraps at the inner width rather than overflowing the card: `ansi.Wordwrap` alone leaves an over-long word intact, which is why the wrap is `ansi.Wrap`.
- Wrapping is on display width, so a double-width or combining rune does not widen the card — every assertion measures with `lipgloss.Width` / `ansi.StringWidth`, never `len`.
- Control characters in a stored command cannot move the card's geometry: the command is arbitrary user-authored text and reaches the panel from `hooks.json`, where a hand edit can put anything; sanitising before the wrap is what keeps an `ESC` from repainting the card around it.
- The card's width is the same whatever the command's length — the rows are padded to the pinned width, so `renderJoinedPanel`'s max-row-width sizing cannot be moved by content.
- A report is one row truncated with `…` rather than wrapped: the specification gives the report a single line, and a wrapping report would change the card's height between screens that are otherwise identical.
- No report row at all when there is nothing to report — the `ok` return is what lets the two screens carry exactly their three parts in the ordinary case.
- An empty command renders an empty row rather than panicking: an object-form entry carrying an empty command is a lookup miss rather than a registration, so this path should not be reached, but a renderer must not be the thing that fails when it is.

**Context**:
> A command longer than the card wraps rather than being cut. The registered command is the only thing on the panel that says which piece of work the pane is holding, and a realistic one carries a directory and an identifier. It wraps within the card's inner width over at most three lines, with anything beyond marked `…`; the card's width is unchanged. The discard confirmation renders the command the same way.
>
> A card with something to report carries one more row. When an answer cannot be carried out, the reason is stated on a single line between the command and the key hints, and it stays there until the next key is pressed rather than timing out: a report the user can miss leaves them believing the thing they asked for happened. The row is present only when there is something to say, and a panel with nothing to report carries exactly the three parts above.
>
> Every colour is a theme token, as everywhere else in Portal — the panel holds no raw hex. `internal/tui`'s colour-literal guard enumerates every non-test file in the package with no exemption, which is what makes that structural rather than policed by review.
>
> The report row's own colour token is not named by the specification. `accent.attention` is what this task renders it in — the token the panel already gives its `● PAUSED` badge and the picker gives its attention state — and it is a question the Phase 3 visual gate can settle; nothing downstream depends on which token it is.
>
> The report text itself is supplied by the caller and is Phase 4's and Phase 5's to compose. This task renders whatever string it is handed and invents none.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §5.2, §5.3, §5.4

## lazy-resume-on-attach-3-2

### Task 3.2: A card when it fits, a plain stack when it does not, on a full-pane canvas

**Problem**: Both screens are drawn into a pane, and a pane is whatever size the restored geometry gave it — it can be narrower or shorter than the card, down to sizes no picker ever runs at. The picker's own canvas fill is a `Model` method carrying the global content gutter and the theme-panel composite, so it cannot serve a pane that fills edge to edge, and there is no size ladder anywhere in `internal/tui` — every modal today is placed on a terminal assumed big enough for it. A pane too small for the card must still say what it is: a waiting pane swallows every key it is not answered with, so one that drew nothing would read as an ordinary restored pane with a dead keyboard, and a clipped frame would hide the very hints that say which keys act.

**Solution**: Two free functions — a full-pane canvas fill with no gutter inset, and a size ladder that builds the card first, measures the card it just built, and falls back to a plain stack of the same parts when the pane cannot hold it. The ladder takes a parts builder called at the width it decides, so the card's dimensions are never restated.

**Outcome**: Every cell of the pane is painted in the active theme; a pane that fits the card gets the card centred on that canvas; a pane one column or one row short gets the same parts stacked plainly; the smallest pane a restore can produce still draws something.

**Do**:
- Add `internal/tui/resume_pane_canvas.go` with `fillPaneCanvas(view string, w, h int, th theme.Theme, colourless bool) string`: clamp to `h` rows, pad every row to `w` through `padLineToCanvasWidth` over a canvas-background style, backfill mid-line gaps through `backfillCanvasBackground` / `canvasBgParams` as the picker's fill does, and pad short renders with blank canvas rows — with no gutter inset and no theme-panel composite. The colourless path routes through `fillColourless(view, w, h)` and paints no background.
- Add `renderPaneScreen(parts paneScreenParts, w, h int, th theme.Theme, colourless bool) string` in the same file, where `paneScreenParts` is `struct { card func() string; stack func(width int) []string }`: resolve non-positive dimensions to `fallbackTermWidth` / `fallbackTermHeight`; call `parts.card()`; if `lipgloss.Width(card) <= w && lipgloss.Height(card) <= h`, place it centred with `placeModalOnClearedCanvas(card, w, h)`; otherwise join `parts.stack(w)` vertically. Wrap either result in `fillPaneCanvas`.
- Clamp the plain stack to the pane's rows by keeping its first row and its last row and dropping from the end of the rows between them, and to its columns, so a pane too short loses body rows rather than the row its screen put last, and never overflows the pane and scrolls the transcript underneath it. A one-row pane renders the first row alone.
- Cover both in `internal/tui/resume_pane_canvas_test.go` with fake parts builders (a fixed-size card, a stack that records the width it was called at) so the ladder is tested independently of either screen's content, plus a table over pane sizes including `1x1`, one-short-in-each-dimension, and exactly-fits.

**Acceptance Criteria**:
- [ ] A pane exactly the card's width and height renders the card; a pane one column narrower, and a pane one row shorter, each render the plain stack instead — the threshold is read off the built card, so a card width change moves it with no edit here.
- [ ] The stack builder is called with the pane's width, and is not called at all when the card fits.
- [ ] Every line of the result is exactly `w` display cells wide and there are exactly `h` lines, for every size in the table including `1x1`.
- [ ] Under a non-colourless render every cell carries the theme's `canvas` background — no line contains a background reset that is not immediately re-set.
- [ ] Under `colourless` the `ansi.Strip`ped output is byte-identical to the non-colourless render's stripped output, and the colourless output carries no SGR background parameter at all.
- [ ] A non-positive width or height renders at the fallback dimensions rather than panicking or returning an empty string.
- [ ] Content taller or wider than the pane is clamped rather than overflowing, and the clamp keeps the stack's first and last rows: a twenty-row stack in a five-row pane renders the first row, the three after it and the last row; a six-row stack in a three-row pane renders the first row, the one after it and the last row; a two-row pane renders the first and the last; a one-row pane renders the first alone.

**Tests**:
- `"it renders the card when the pane fits it"`
- `"it renders the plain stack when the pane is one column short"`
- `"it renders the plain stack when the pane is one row short"`
- `"it measures the threshold off the card it just built"` (card builder returns a wider card; the same pane flips to the stack)
- `"it calls the stack builder at the pane's width"`
- `"it paints every cell of the pane"` (table over sizes)
- `"it clamps content taller than the pane"`
- `"it keeps the stack's first and last rows when it clamps"` (table: twenty rows in five, six rows in three, six rows in two, six rows in one)
- `"it renders at the fallback size for a non-positive dimension"` (table: `0x0`, `-1x10`, `10x-1`)
- `"it keeps the layout and paints no canvas under NO_COLOR"`

**Edge Cases**:
- The threshold is measured off the card just built rather than a restated dimension — the card's content width is pinned in task 3.1, and a later change to it must move the fallback point with it rather than leaving a pane that renders a clipped frame.
- A pane one column or one row short of the card gets the plain stack rather than a clipped frame: a frame missing its right edge reads as a rendering fault, and a frame missing its bottom rows hides the key hints.
- The plain stack keeps every part its screen's builder hands it, including the report row — which parts a screen stacks is the screen's own decision, and which of them survive is decided by the pane's rows, not by the ladder.
- Content is clamped to the pane's rows and columns rather than overflowing: a render taller than the pane would scroll the pane's primary buffer, which is where the user's replayed transcript is sitting. What goes is taken from the middle, because both screens stack their key hints last and a pane that swallows every key it is not answered with must never be the one that hides which keys answer it — the same failure a clipped frame produces, and the reason the degraded form exists at all.
- The smallest pane a restore can produce still draws the title, the command and the key hints — asserted at a realistically small pane, at a three-row pane holding a command that wraps past one row, and at the degenerate `1x1`, where the single row must be the title.
- A non-positive width or height falls back to a bounded render rather than panicking: the drawing process reads the pane's size from its environment and can be handed a zero.
- Every cell of the pane is painted so no terminal background shows through — the panel is the pane's own content over the alternate screen, and an unpainted cell would show the terminal's own background through the canvas.
- Under `NO_COLOR` the layout is unchanged and no canvas is painted: coverage does not depend on the fill, because the panel sits on the alternate screen, so the transcript stays hidden whether or not a colour is painted over it.

**Context**:
> The overlay fills the pane, painted in the active Portal theme so nothing behind it shows through, and the decision sits in a compact bordered card in the middle. A box stretched to near-full size with a few lines in the middle would look lost on a large terminal; the canvas-plus-card shape is what Portal already uses everywhere else for exactly this reason.
>
> A pane too small for the card still says what it is. Below the size the card needs, the panel degrades instead of disappearing: the canvas is painted as always, and the title, the command and the key hints stack plainly without the card frame, down to the smallest pane a restore can produce. Enter and `d` act at every size. A waiting pane swallows every other key, so one that drew nothing would read as an ordinary restored pane with a dead keyboard.
>
> `NO_COLOR` is the same carve-out here as everywhere else in Portal. No canvas is painted, no appearance detection runs at all, and the panel renders colourless on the terminal's native foreground and background. Coverage does not depend on the fill: the panel sits on the pane's alternate screen, so the transcript stays hidden underneath it whether or not a colour is painted over the pane.
>
> The full-pane canvas fill is a new free function rather than a change to the picker's own fill, which carries the picker's gutter inset and the theme-panel composite and is a model method; the pane panel fills edge to edge.
>
> Entering the alternate screen, writing the result into the pane, and reading a key are Phase 4's. This task produces a string of exactly the pane's dimensions and touches no terminal.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §5.1, §5.2, §5.4

## lazy-resume-on-attach-3-3

### Task 3.3: The waiting panel

**Problem**: The waiting panel is the feature's whole user surface — it is what a restored pane holds instead of a running process, and it is the only thing that says which piece of work the pane is holding and which keys answer it. Nothing renders it today, and the one thing it must not be is a new visual grammar: it has to read as Portal rather than as a tmux dialog, which means the rename modal's header-with-badge slot, the same joined-panel frame, and the same key-hint footer the picker's modals use. The badge helper those modals share hardcodes `◉ EDIT MODE` as both the badge text and the width the hidden-badge blank is cut to, so a second badge cannot reach that slot without it.

**Solution**: `RenderResumePanel` — the exported entry point the drawing process calls, taking the command, the report and the pane's size and returning a full-pane string: the card assembled from the shared parts and the existing panel machinery when the pane holds it, the same parts stacked plainly when it does not, on the full-pane canvas. `renderHeaderWithBadge` gains a badge-text parameter, with its two existing call sites passing `editModeIndicator` so the rename and edit modals render byte-identically.

**Outcome**: A pane can be handed a command and a size and get back exactly the specified panel — `Resume session` with a `● PAUSED` badge, `ON RESUME` over the command, `⏎ resume` and `d discard` — at any size, in any theme, with no copy on it that names any particular tool.

**Do**:
- Add `internal/tui/resume_panel.go` with the constants `resumePanelTitle = "Resume session"`, `resumePausedBadge = "● PAUSED"`, `resumeCommandLabel = "ON RESUME"`, `resumeKeyResume = "⏎"`, `resumeLabelResume = "resume"`, `resumeKeyDiscard = "d"`, `resumeLabelDiscard = "discard"` — the whole of the screen's fixed copy, each verbatim from the specification.
- Declare the exported render input beside them: `type ResumeScreen struct { Command, Report string; Width, Height int; Theme theme.Theme; Colourless bool }`, and `func RenderResumePanel(s ResumeScreen) string` routing through `renderPaneScreen` with the card and stack builders below.
- Build the card through `renderJoinedPanel` with three compartments: header `renderHeaderWithBadge(title, resumeCardContentWidth, true, resumePausedBadge, …)` with the title in `text.primary` bold; body `resumeCommandLabel` in `accent.primary`, then `resumeCommandRows(s.Command, resumeCardContentWidth, th.TextPrimary, false, …)`, then the report row from `resumeReportRow` when it reports one; footer `renderConfirmCancelFooter(resumeKeyResume, resumeLabelResume, resumeKeyDiscard, resumeLabelDiscard, …)`.
- Build the plain stack from the same pieces at the pane's width: the title row alone, the command rows wrapped to the pane's width, the report row when present, then the key-hint row. Neither the `● PAUSED` badge nor the `ON RESUME` label is stacked — both are card parts, the badge occupying a header slot a frameless screen does not have, and the small-pane form is the title, the command and the key hints. The title row is the stack's first row and the key-hint row its last, which is what the pane's clamp keeps.
- Add the badge-text parameter to `renderHeaderWithBadge` in `internal/tui/edit_modal.go`, cutting the hidden-badge blank to the passed badge's width, and pass `editModeIndicator` from `editModalHeaderRow` and `renameModalHeaderRow`.
- Cover the panel in `internal/tui/resume_panel_test.go` over `testDarkTheme(t)` / `testLightTheme(t)` and both colourless values, asserting on stripped text, on SGR parameter runs for the token roles (as `rename_modal_test.go` does for the badge), and on `lipgloss.Width` for the geometry; re-run the rename and edit modal suites unchanged.
- Add one clause to the `tui` row of CLAUDE.md's package table naming `resume_panel.go` / `resume_panel_parts.go` / `resume_pane_canvas.go` as the pane-drawn resume panel's renderers — exported pure functions over the shared card grammar, not a Bubble Tea component.

**Acceptance Criteria**:
- [ ] The card renders `Resume session` on the left of its header row and `● PAUSED` on the right, the badge in `accent.attention`, in the slot `renderHeaderWithBadge` gives `◉ EDIT MODE` and pinned to `resumeCardContentWidth`.
- [ ] The body renders `ON RESUME` in `accent.primary` on its own row with the command beneath it in `text.primary`, and the footer renders `⏎ resume` and `d discard` through the shared confirm/cancel footer.
- [ ] With a non-empty report the card carries exactly one extra row, between the command and the key-hint footer; with an empty report the card carries exactly its three compartments and no blank filler row.
- [ ] The card's rendered width is identical for a one-character command, a three-line command, a long report and an empty report — no content moves the frame.
- [ ] The rename modal and the edit modal render byte-identically to before the badge parameter (their existing suites and the kill/delete byte-exact goldens all pass unchanged).
- [ ] Under `colourless` the stripped render is identical to the coloured one, so `●`, `PAUSED`, `ON RESUME`, `⏎` and `d` carry the state with no hue.
- [ ] Below the card's size the panel renders the plain stack carrying the title, the command, the report when present and the key hints, and never an empty screen; neither the `ON RESUME` label nor the `● PAUSED` badge appears on it.
- [ ] A four-row pane holding a command that wraps to two rows renders the title, both command rows and the key hints — the stack spends no row on anything the small-pane form does not carry.
- [ ] Stripping the fixed constants and the caller's command and report from the rendered screen leaves no alphabetic text — the screen names no tool and carries no copy beyond what this task declares.
- [ ] `internal/tui`'s colour-literal guard passes with no exemption added.

**Tests**:
- `"it renders the title and the PAUSED badge in the header slot"`
- `"it renders the badge in accent.attention"`
- `"it renders the ON RESUME label in accent.primary over the command"`
- `"it renders the resume and discard key hints"`
- `"it carries exactly three parts with nothing to report"`
- `"it carries the report row between the command and the key hints"`
- `"it renders the same card width whatever the command the report or the badge"` (table)
- `"it renders the plain stack below the card's size"`
- `"it drops the ON RESUME label and the PAUSED badge from the plain stack"`
- `"it renders the title the command and the key hints in a four-row pane"` (command wrapping to two rows)
- `"it draws the title the command and the key hints at the smallest pane"`
- `"it carries no copy beyond its own constants and the caller's text"`
- `"it renders every state through glyphs and words under NO_COLOR"`
- `"it leaves the rename and edit modal headers byte-identical"`

**Edge Cases**:
- The `● PAUSED` badge takes the rename modal's badge slot so the header pins to the card's content width — the badge is right-anchored against the pinned width rather than against the card's widest row, which is what keeps the header aligned with the body when the command is short.
- The report row sits between the command and the key hints and nowhere else: it is the last body row, so an answer that could not be carried out is read immediately above the keys that retry it.
- A panel with nothing to report carries exactly three parts — the row is absent rather than blank, so the card's height does not change when there is nothing to say.
- Every string is tool-agnostic and says nothing about what the command is: Portal's resume machinery runs whatever a registration holds, and the panel states the command without interpreting it.
- Under `NO_COLOR` the paused state reads from its glyph and its word rather than its hue — Portal's rule is that state stays glyph-backed and never colour-only.
- The card's width does not move with the command, the report or the badge: the command rows and the report row are padded to the pinned width in task 3.1, and the header is pinned to the same width.
- No raw hex at any call site: every colour is a token on the passed theme, which the package's colour-literal guard enforces over every non-test file with no exemption.

**Context**:
> Header — `Resume session` on the left; a `● PAUSED` badge on the right in `accent.attention`, occupying the slot the rename modal gives `◉ EDIT MODE`. Body — an `ON RESUME` label in `accent.primary`, the token the rename modal gives `NEW NAME`, with the registered command beneath it. Footer — `⏎ resume` and `d discard`. It carries this, and carries nothing else.
>
> The card is the shape of Portal's existing rename modal: a header row carrying the title and a state badge, a body, and a footer row of key hints, assembled through the same joined-panel frame the picker's modals use.
>
> The reuse is at the presentation layer, not the code path. The picker's modals are Bubble Tea components rendering into its own model, while this panel is drawn straight into a pane by the program that then waits there. What carries across is the theme tokens and the modal's visual grammar, which is what makes it read as Portal rather than as a tmux dialog.
>
> A meta line carrying the directory and how long the pane had been paused was drafted and cut. It was invented rather than decided, and on the page it added nothing the command and the badge did not already say.
>
> The design frame the specification names for this screen — **Resume panel — waiting (Nord)** — is committed at `testdata/vhs/reference/resume-panel-waiting-nord.png` and is the design reference for what this screen looks like. It is read at the phase's visual gate, not copied from: the panel is built from Portal's own existing modal grammar — the kill modal, the rename modal's badge slot, the shared destructive-confirm builder and the joined-panel frame — which is what the frame was itself built by duplicating, so its card geometry is identical to the existing modals' rather than approximate.
>
> Reading the registration, deciding whether to draw at all, and dispatching Enter and `d` are Phase 4's. This task renders a string from a command, a report and a size.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §5.2, §5.3, §5.5

## lazy-resume-on-attach-3-4

### Task 3.4: The discard confirmation

**Problem**: `d` opens a confirmation over the panel, and what it destroys is the only copy of a user-authored command. It is the same act the picker's kill confirm performs, so it must be the same object — built through the shared destructive-confirm builder rather than assembled to look like it — or the two will drift and this one will be the one that drifts, because it is the one no picker test renders. Two things stand in the way: the builder renders its target as a single row (`destructiveNameRow`), where this screen's target is a command that wraps over up to three; and the builder returns a finished framed panel, with no way to reach its parts for the plain stack a small pane needs.

**Solution**: The builder's spec gains an optional pre-rendered target block and an optional trailing report block, and its compartment assembly is split out behind an accessor so both the frame and the plain stack are built from one set of parts. `RenderResumeDiscardConfirm` is then the kill modal with a different title, consequence and confirm label, and the command in place of the session name.

**Outcome**: The confirmation is the kill modal retitled — `▲ Discard resume?`, the command in `state.destructive`, the specified consequence line, `y discard   esc cancel` — with a report row when a discard could not be written, degrading with the pane exactly as the waiting panel does, while the kill and delete modals render byte-identically to today.

**Do**:
- In `internal/tui/destructive_confirm.go`: add `targetRows []string` and `reportRows []string` to `destructiveConfirmSpec` — `targetRows`, when non-empty, replaces the single `destructiveNameRow`; `reportRows` is appended after the consequence rows — and split the compartment assembly into `destructiveConfirmCompartments(spec destructiveConfirmSpec, wrapWidth int, th theme.Theme, colourless bool) [][]string`, which wraps the consequence at `wrapWidth` rather than at the package constant, with `renderDestructiveConfirm` reduced to `renderJoinedPanel(destructiveConfirmCompartments(spec, destructiveBodyWidth, th, colourless), th.Border, th, colourless)` — the framed path passes the width the builder wraps at today, so the kill and delete modals are untouched.
- Add `internal/tui/resume_discard_confirm.go` with the constants `discardConfirmTitle = "Discard resume?"`, `discardConfirmConsequence = "Removes this pane's resume command permanently. The session and its scrollback are untouched."`, `discardKeyConfirm = "y"`, `discardLabelConfirm = "discard"` — each verbatim from the specification — and `func RenderResumeDiscardConfirm(s ResumeScreen) string` routing through `renderPaneScreen` over the same `ResumeScreen` the waiting panel takes.
- Build the card's spec with `targetRows: resumeCommandRows(s.Command, resumeCardContentWidth, th.StateDestructive, true, …)`, `reportRows` from `resumeReportRow`, `title: discardConfirmTitle`, `consequence: discardConfirmConsequence`, `confirmKey: discardKeyConfirm`, `confirmLabel: discardLabelConfirm` — the `▲` glyph and the `esc cancel` half come from the builder unchanged.
- Build the plain stack by flattening `destructiveConfirmCompartments` at the pane's width — the same title, consequence, confirm key and label, with `targetRows` and `reportRows` rebuilt through the shared helpers at that width — so the degraded screen carries the title, the command, the consequence line, the report when present and the key hints without the frame, cannot drift from the card, and wraps the command exactly as the waiting panel's stack does. The consequence wraps at that same width: every part of the stack is built at the width it will be shown at, so the canvas is never handed a row it has to cut.
- Cover the confirmation in `internal/tui/resume_discard_confirm_test.go` over both test themes and both colourless values; extend `internal/tui/destructive_confirm_test.go` with the spec-level coverage of `targetRows` and `reportRows`, and leave `TestKillDeleteModalContent_ByteIdenticalGolden` untouched as the drift tripwire.

**Acceptance Criteria**:
- [ ] `TestKillDeleteModalContent_ByteIdenticalGolden` passes unchanged for all four of its cases, and the kill and delete modal suites pass with no edit — the compartments split and the two new spec fields change nothing for a spec that sets neither.
- [ ] The confirmation renders `▲ Discard resume?` in `state.destructive` bold in its header, exactly as the kill modal renders its own title.
- [ ] The command renders in `state.destructive` over up to three rows where the kill modal renders a one-line session name, wrapped and `…`-marked by the same shared block the waiting panel uses.
- [ ] The consequence line renders verbatim as `Removes this pane's resume command permanently. The session and its scrollback are untouched.`, word-wrapped by the builder at its existing width and in the builder's existing muted token.
- [ ] The footer renders `y discard   esc cancel` — the confirm key and label from this screen's constants, the cancel half from the builder.
- [ ] With a non-empty report the card carries exactly one extra row, on the same single row the waiting panel gives a report, and `y` remains offered in the footer beside it.
- [ ] Below the card's size the confirmation renders the plain stack carrying the title, the command, the consequence line, the report when present and `y discard   esc cancel`, and never an empty screen.
- [ ] No row of the plain stack is wider than the pane it was built for, the consequence rows included: at a pane narrower than the builder's own wrap width the consequence is re-wrapped to the pane, every word of it still present, rather than being cut mid-word by the canvas.
- [ ] Under `colourless` the `▲`, the title, the consequence and the key words all render, and the stripped output matches the coloured render's — the destructive signal survives where the token drops.
- [ ] The card's rendered width does not move with the command's length or the report's.
- [ ] Nothing structural differs from the kill modal: header compartment, body compartment, footer compartment, same frame, same inset, same divider rules.

**Tests**:
- `"it leaves the kill and delete modals byte-identical"` (the existing golden, re-run)
- `"it renders the discard title with the destructive glyph"`
- `"it renders the command in state.destructive over up to three rows"`
- `"it renders the consequence line verbatim"`
- `"it renders the y discard esc cancel footer"`
- `"it carries the report row on the confirmation itself"`
- `"it carries exactly the kill modal's compartments with nothing to report"`
- `"it degrades to the plain stack below the card's size"`
- `"it wraps the consequence to the pane in the plain stack"` (a pane narrower than the builder's wrap width: no row exceeds the pane and the sentence's words are all present)
- `"it keeps the destructive signal under NO_COLOR"`
- `"it renders the same card width whatever the command and the report"`
- `"it builds the plain stack from the same compartments as the card"` (at a pane width equal to the card's, the stripped stack rows are the stripped card's content rows, in order)

**Edge Cases**:
- The kill and delete modals render byte-identically after the builder's compartments are exposed — the split is a refactor with a golden already standing behind it, and that golden is the acceptance test for it.
- The command takes the multi-line block where the kill modal takes a one-line session name: a session name always fits a row and a command does not, which is why the spec carries pre-rendered rows rather than a second string field.
- A report row on the confirmation itself, on the same single row the waiting panel gives one: a confirmation that closed on a failed write would be indistinguishable from one the user backed out of, so the report has to land on this screen.
- The confirmation degrades with the pane exactly as the waiting panel does — below the card's size the frame goes and the parts stack plainly, because a confirmation that drew nothing would leave the user pressing the key the footer offered a moment earlier against a question they never saw.
- The consequence re-wraps with the pane in the degraded form. The card only fits a pane of roughly the builder's wrap width plus its frame, so the plain stack is what a two- or three-way split actually renders, and a consequence left at the builder's own width there is a row the canvas cuts mid-word — on the one screen whose job is to make an irreversible act deliberate. The framed path still passes the builder's own width, so the kill and delete modals and their byte-identical golden are untouched.
- Under `NO_COLOR` the `▲` and the words carry the destructive signal where the token drops, which is the builder's existing behaviour inherited rather than restated.
- The consequence line is plain language and names no tool — it is stated verbatim in the specification and is not paraphrased, improved or re-derived here.
- Nothing structural differs from the kill modal: it is the same act on the same object, and any structural difference is a defect rather than a variation.

**Context**:
> The discard confirmation is the kill modal, retitled. `▲ Discard resume?`, the command rendered in `state.destructive` where the kill modal puts the session name, the consequence line `Removes this pane's resume command permanently. The session and its scrollback are untouched.`, and `y discard   esc cancel`. Nothing structural differs, which is the point: it is the same act the picker's kill confirm performs, so it is the same object, built through the same shared destructive-confirm builder.
>
> The confirmation carries a report row too. A discard the store will not accept is reported on the confirmation itself, on the same single line the waiting panel's card gives a report, and `y` retries from there. The screen the user answered on stays in front of them.
>
> The confirmation degrades with the pane, as the waiting panel does. Below the size the card needs the frame goes and the parts stack plainly on the canvas — the `▲ Discard resume?` title, the command, the consequence line and `y discard   esc cancel` — and `y` and Escape act at every size, as Enter and `d` do.
>
> The confirm key is `y`, matching Portal's two existing destructive confirmations. Killing a session and deleting a project both take `y` with `esc` to cancel, through one shared builder.
>
> The specification places the waiting panel's report row "between the command and the key hints" and says the confirmation's sits "on the same single line" — which on a screen whose body is command-then-consequence leaves the row's exact position open. It is rendered as the last body row, immediately above the key hints, which satisfies both statements literally; nothing downstream depends on the choice and the visual gate can settle it.
>
> The committed frame for this screen — `testdata/vhs/reference/resume-panel-discard-confirm-nord.png` — predates the corrigendum that stated the consequence line verbatim, and it diverges from what this task builds in two visible ways. It carries the earlier drafted wording (`Removes this pane's resume command for good. It won't come back after a restart. Can't be undone.`) where the constant above is the one the specification now states; and it renders the command truncated to a single row with a ` · ~/Code/flowx` trailer in the kill modal's trailer slot, where this screen wraps the command over up to three rows and carries no directory — the meta that was drafted and cut. The specification's stated strings and this task's constants govern both. The frame is read at the phase's visual gate for card geometry, compartment structure and colour-role match — which is what it was built by duplicating the Nord kill modal for — and never for copy.
>
> Dispatching `d`, `y` and Escape, dropping input already in flight, and removing the registration are Phase 5's. This task renders the screen.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §5.4, §5.2, §6.2, §6.3

## lazy-resume-on-attach-3-5

### Task 3.5: The theme the panel draws in

**Problem**: The panel paints a canvas, so it has to know which palette is in force — and under an adaptive light/dark pair that is a question only the terminal can answer. The picker's answer to it is a Bubble Tea gate: `tea.RequestBackgroundColor` raced against `appearanceDetectTimeout`, resolved through `Model.Update`'s `tea.BackgroundColorMsg` arm. The process that draws this panel cannot be a Bubble Tea program — it paints the alternate screen and then execs a minimal waiter over itself, leaving the painted screen behind — so it has no program loop to deliver that message into, and no existing route to the answer. Getting it wrong is not cosmetic: a light palette painted on a dark terminal, or the reverse, is the whole pane.

**Solution**: A non-Bubble-Tea OSC 11 read placed beside the picker's gate in `internal/tui`, reusing that gate's own timeout constant and the picker's own reply classification so the two cannot disagree — write the query, read the reply under a deadline, classify, and select the nomination's member. A constant nomination skips it entirely and writes nothing.

**Outcome**: One call answers "which palette does this pane paint in" for a constant and for a pair alike, resolves dark by one route for every way the question can go unanswered, restores the terminal's mode on every path, and never writes an OSC 11 *set*.

**Do**:
- Add `internal/tui/pane_appearance.go` with the exported `ResolvePaneTheme(n theme.Nomination, colourless bool) theme.Theme`: return `n.Constant()` for a constant, `theme.Theme{}` for a zero nomination, and `n.Select(theme.MemberDark)` with no query at all under `colourless`; otherwise run the probe and return `n.Select(answer)`.
- Add the probe behind a struct of seams — `out io.Writer`, `openReader func() (paneReader, error)` where `paneReader` exposes `Read`, `SetReadDeadline` and `Close`, `isTerminal func() bool`, `makeRaw func() (restore func(), err error)`, and `timeout time.Duration` defaulted from `appearanceDetectTimeout` — with the production constructor binding `os.Stdout`, `term.IsTerminal(os.Stdin.Fd())` and `term.MakeRaw`/`term.Restore`, over `github.com/charmbracelet/x/term`. That package is already in the module graph as an indirect requirement of the Bubble Tea stack and carries the pointer-sized descriptor shape these calls are written against, so this edit promotes it to a direct requirement rather than adding a module; it is also where the chain's later terminal calls come from — the waiter's raw-mode entry and the draw's size read alike — so one package serves all of them.
- The production `openReader` is `os.OpenFile("/dev/tty", os.O_RDONLY, 0)` and **not** `os.Stdin`. Measured on darwin against a real pty slave, Go 1.27.1: `os.NewFile` over a blocking tty descriptor — which is how the runtime builds `os.Stdin` — refuses `SetReadDeadline` with `file type does not support deadline`, while the same tty opened through `os.OpenFile` takes the deadline and times the read out at the duration set. Binding `os.Stdin` would send every production probe down the deadline-unsupported branch, resolving dark on every draw whatever the terminal answered and leaving the reply in the pane's input queue for the waiter to read as keystrokes. For the process drawing into a pane, `/dev/tty` is that pane's own pty; an open that fails resolves dark without writing, by the same route a non-terminal does. Raw mode stays on stdin's descriptor: the mode is a property of the terminal rather than of a descriptor onto it, so the reader sees it either way and nothing needs a second `MakeRaw`.
- Probe body: return dark without writing when `isTerminal` is false, when `openReader` fails, or when `makeRaw` fails; `defer` the restore — which closes the reader as well as restoring the mode — so it runs on every path; write `ansi.RequestBackgroundColor`; set the read deadline and, if setting it is unsupported, return dark without reading rather than blocking; read until a `BEL` or `ST` terminator, a bounded byte cap, the deadline or an error.
- Classify through the picker's own rule: parse the OSC 11 payload with `ansi.XParseColor` and hand the result to `terminalReplyFrom(tea.BackgroundColorMsg{Color: c})`, so an unparseable payload arrives as a nil colour and classifies dark by the same route a missing answer does.
- Add `internal/tui/pane_appearance_guard_test.go`: a source guard over `internal/tui`'s non-test files asserting `ansi.SetBackgroundColor` is called from exactly one file and that file is `restore.go`, so no pane-draw path can ever acquire a background *set*.
- Cover the probe in `internal/tui/pane_appearance_test.go` driving the seams over an `os.Pipe` pair: a scripted dark reply, a light reply, no reply at all, a truncated reply, an unparseable payload, a read error, a non-terminal, a failed `openReader`, a failed `makeRaw`, a reader that refuses a deadline, and a reply arriving after the deadline — each asserting the answer, whether anything was written, and that the restore ran.
- Add `internal/tui/pane_appearance_realtty_test.go` (`//go:build darwin`) covering the production `openReader` alone against a real terminal: open a pty through `/dev/ptmx` plus the host's grant/unlock/name ioctls, open its slave the way the production constructor opens `/dev/tty`, and assert `SetReadDeadline` returns nil and a read of it returns a timeout at the duration set rather than blocking. A pipe takes a deadline whatever reader shape is bound, so it cannot fail this; the linux arm is the same `os.OpenFile` call and is covered through the seam.

**Acceptance Criteria**:
- [ ] A constant nomination returns its palette with nothing written to the writer and no reader touched; a zero nomination returns the zero theme the same way.
- [ ] `colourless` runs no detection and writes nothing, whatever the nomination's shape.
- [ ] An adaptive pair writes exactly `ansi.RequestBackgroundColor` — once — and returns the light member for a light reply and the dark member for a dark one.
- [ ] The timeout is read from `appearanceDetectTimeout`, the same constant the picker's gate uses, rather than a second copy; a probe whose reply never arrives returns dark once that duration has elapsed.
- [ ] A reply that arrives after the deadline never changes the answer already returned.
- [ ] No answer, a truncated reply, an unparseable payload, a read error, a stdin that is not a terminal, and a `makeRaw` failure all return the dark member — and the two that happen before the write make no write.
- [ ] The restore closure runs on every path that reached raw mode, including the timeout, the read error and the successful read, and it closes the reader it opened.
- [ ] The production reader takes a read deadline against a real terminal and times a read out at the duration set — asserted over a pty slave rather than a pipe, because a pipe takes a deadline whatever reader shape is bound.
- [ ] A reader whose `SetReadDeadline` is unsupported returns dark without reading and without blocking, and that branch is reachable only through a seam a test binds — no production path takes it.
- [ ] An `openReader` that fails returns the dark member and writes nothing, exactly as a non-terminal does.
- [ ] `internal/tui` calls `ansi.SetBackgroundColor` from exactly one non-test file, `restore.go`, with the guard failing on a second call site.

**Tests**:
- `"it paints a constant nomination with no query at all"`
- `"it returns the zero theme for a zero nomination"`
- `"it runs no detection and writes nothing under NO_COLOR"`
- `"it writes the background-colour query exactly once for a pair"`
- `"it selects the light member for a light reply"`
- `"it selects the dark member for a dark reply"`
- `"it resolves dark when the terminal never answers"`
- `"it resolves dark for a truncated or unparseable reply"` (table)
- `"it resolves dark for a read failure"`
- `"it resolves dark and writes nothing when stdin is not a terminal"`
- `"it resolves dark and writes nothing when raw mode cannot be entered"`
- `"it resolves dark and writes nothing when the terminal cannot be opened"`
- `"it resolves dark without blocking when the reader refuses a deadline"`
- `"it bounds a read against a real terminal"` (pty slave, darwin-tagged)
- `"it ignores a reply that arrives after the deadline"`
- `"it restores the terminal mode on every path"` (table over every outcome above)
- `"it calls SetBackgroundColor from restore.go alone"` (source guard)

**Edge Cases**:
- A constant nomination writes nothing to the terminal and paints from the first frame — the gate is never consulted, exactly as the picker's `newNominationGate` pins a constant resolved.
- The reply is read from a fresh `/dev/tty` open rather than from `os.Stdin`, because `os.Stdin` refuses a read deadline on a terminal — measured. A probe bound to it would answer dark on every draw whatever the terminal said, and would leave the terminal's reply in the pane's input queue, where the waiter reads it as keystrokes; on the discard confirmation the reply's leading `ESC` is the cancel key, so the confirmation would back itself out, and the input drop that guards that screen runs before the probe writes and so cannot catch it. The two reader shapes are indistinguishable over a pipe, which is why the deadline is pinned against a real terminal.
- The pair's query races the picker's own timeout constant rather than a second copy: two timeouts that could drift would make the panel and the picker disagree about how long a silent terminal is waited for.
- The first to resolve wins and a late reply never flips a resolved answer — the probe returns once and the reader is abandoned, so nothing can re-decide after the palette is chosen.
- No answer, an unparseable reply and a stdout that is not a terminal all resolve dark by the same route: `ansi.XParseColor` yields a nil colour, and the picker's nil-safe classification calls that dark, so there is one fallback rather than three.
- A pane drawn with no client attached resolves dark with no second rule — most panes are drawn at restore with nobody watching, so the no-answer path is the common one rather than an edge, and it must not cost more than the timeout.
- `NO_COLOR` runs no detection at all and writes no query: a query written under `NO_COLOR` would put bytes on a screen Portal has promised to leave alone.
- The terminal's mode is restored on every path including the timeout and a read failure — raw mode left on would break the pane's keyboard for the waiter that follows.
- No OSC 11 set is ever written from a pane draw: the picker owns the canvas of a whole terminal and sets its background back on exit; a pane owns one region of someone else's terminal and must never move the terminal's default background at all.

**Context**:
> The theme resolves as it does everywhere else in Portal. A named theme paints from the first frame with no gate at all. A light/dark pair runs the same detect-or-timeout appearance gate the picker runs — a query to the terminal raced against the same short timeout, resolving dark when there is no answer — in the process that draws. That process hands off before it waits, so the gate is paid once per draw and nothing of it stays resident while the pane waits. A pane drawn with no client attached to it gets no answer and resolves dark — that fallback reached by the ordinary route rather than a second rule.
>
> `NO_COLOR` is the same carve-out here as everywhere else in Portal. No canvas is painted, no appearance detection runs at all, and the panel renders colourless on the terminal's native foreground and background.
>
> The appearance query is a non-Bubble-Tea OSC 11 read placed beside the picker's existing gate, because the drawing process cannot be a Bubble Tea program — it must leave the alternate screen painted while it execs the waiter away — and that is where Portal's raw terminal-background I/O and its detect timeout already live.
>
> Loading the theme setting itself — reading `prefs.json` and the themes directory to produce the nomination this function is handed — is Phase 4's, on the same route `cmd/open.go` already loads it for the picker.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §5.2, §4.2

## lazy-resume-on-attach-3-6

### Task 3.6: Both screens on demand for the visual gate

**Problem**: The panel is the feature's whole user surface, and the only route to seeing a Portal screen before release is `cmd/capturetool` — a scratch build of Portal itself disturbs the running daemon and touches real state, and these two screens are worse than that: reaching one for real means registering a lazy hook, rebooting, and attaching to the pane it produced. Every screen under active work is reachable by name through that tool today, but every one of them is a picker fixture built through `tui.Build`, and these two are not a picker model at all. There is a second cost to leaving them out: `internal/capture`'s swap-and-diff completeness guard enumerates whatever the registry holds and asserts every token painted under one palette is painted under the other, so two screens outside the registry are two screens outside that guard.

**Solution**: Standalone named surfaces alongside the contrast swatch — each a tiny `tea.Model` rendering the production `RenderResumePanel` / `RenderResumeDiscardConfirm` at a pinned size — enumerated by `FixtureNames()` and resolved by `capturetool`, with the swap guard's "the swatch is the only skip" assertion widened to a named set and a palette-diff guard of their own so admitting a second kind of skip does not shrink the completeness guard.

**Outcome**: `go run ./cmd/capturetool --fixture resume-panel-waiting` (and its five siblings) renders the real screen at a chosen theme, including the report row and the degraded form, with no tmux server, no reboot and no registration.

**Do**:
- Add `internal/capture/resume_surfaces.go`: a `Surface` type holding the screen to draw, its `tui.ResumeScreen` seed and its pinned width and height, implementing `tea.Model` (`View` returns the production render with `AltScreen` set and **no** `BackgroundColor`, `Update` quits on `q`/`ctrl+c`/`esc` and records the window size), plus `SurfaceNames()` and `SurfaceByName(name) (*Surface, bool)`.
- Register six names, each one variable from its base: `resume-panel-waiting`, `resume-panel-waiting-report`, `resume-panel-waiting-degraded`, `resume-panel-discard`, `resume-panel-discard-report`, `resume-panel-discard-degraded` — the degraded pair pinned to a size below the card's, the report pair seeded with a report string, all six carrying a realistic long command (a directory plus an identifier) so the wrap is visible.
- Extend `FixtureNames()` to append `SurfaceNames()` beside `ContrastValidationFixture` before the sort, leaving `FixtureByName` resolving `*Fixture` only.
- In `cmd/capturetool/main.go`: route a surface name in `resolveProgram` before `resolveModel`, and extract the `NO_COLOR` env read into one helper both that branch and `resolveModel` use, so the two cannot drift; extend `renderSizeFilter` to pin a surface's size as it pins a fixture's.
- Widen `TestThemeSwapGuard_EnumeratesRegistry`'s second sub-test in `internal/capture/theme_swap_guard_test.go` from "the swatch is the only skip" to a named skip set — the swatch plus `SurfaceNames()` — asserting every member is enumerated by `FixtureNames()`, that none of them resolves through `FixtureByName`, and that the guarded fixtures plus the skip set are exactly `FixtureNames()`.
- Add `internal/capture/resume_surface_swap_guard_test.go`: for each surface, render under `themetest.SyntheticPair`'s palettes A and B and assert no theme-A parameter run survives in the B render, that the observed token-name sets under A and under B are equal, and that the set is non-empty — reusing `tokenForms`, `carriesRun` and `observedTokens` from the fixture guard rather than restating them.
- Add one clause to CLAUDE.md's "Visual capture harness" paragraph: `capturetool` also renders standalone named surfaces that are not picker fixtures — the contrast swatch and the resume panel's two screens — which `FixtureNames()` enumerates, `FixtureByName` does not resolve, and the swap guard skips by name in exchange for their own palette-diff guard.

**Acceptance Criteria**:
- [ ] All six names are listed by `capture.FixtureNames()`, resolve through `SurfaceByName`, and are rejected by `FixtureByName` with its existing unknown-fixture error.
- [ ] `resolveProgram` returns a model for each of the six at a built-in slug and at an explicit `.theme` path, and the model's view content is byte-identical to calling the production `tui.RenderResumePanel` / `tui.RenderResumeDiscardConfirm` with the same seed, theme and pinned size.
- [ ] The two degraded surfaces render the plain stack at their pinned size with no terminal resize involved, and the two report surfaces render their report row.
- [ ] A surface's `tea.View` carries `AltScreen` and leaves `BackgroundColor` unset, and the surface satisfies neither branch of `run`'s restore type switch — nothing sets or sets back a terminal background for these screens.
- [ ] `NO_COLOR` set to a non-empty value renders every surface colourless with no canvas painted, by the same env read `resolveModel` uses.
- [ ] The widened skip assertion fails if a surface name is enumerated but absent from the skip set, and fails if a name in the skip set resolves through `FixtureByName`.
- [ ] The surface palette-diff guard fails when a token painted under palette A is not painted under palette B on the same surface, and fails when a surface paints no token at all.
- [ ] `TestPortalBinaryDoesNotImportCapture` still passes — nothing added here reaches the production binary.
- [ ] Every existing fixture guard (`TestThemeSwapGuard_*`, the colourless and render-size suites) passes unchanged.

**Tests**:
- `"it enumerates every resume surface by name"`
- `"it resolves each surface to a model and no fixture"` (table over the six)
- `"it renders the production panel function byte-identically"` (table over the six)
- `"it renders the degraded stack at the pinned size"`
- `"it renders the report row on the report surfaces"`
- `"it sets no terminal background"`
- `"it renders colourless under NO_COLOR"`
- `"it skips exactly the named standalone surfaces"` (widened guard, plus a negative case)
- `"it paints the same token set under either palette"` (per surface)
- `"it keeps internal/capture out of the production binary"` (existing guard, re-run)

**Edge Cases**:
- The surfaces render the production functions rather than a copy of the layout — a capture surface that reimplemented the screen would sign off a design the pane never draws, which is the one failure a visual gate cannot catch.
- The swap guard's single-skip assertion widens to a named set rather than silently admitting a second unguarded surface: the assertion exists so a screen cannot drop out of the completeness guard unnoticed, and relaxing it to "some things are skipped" would remove exactly that property.
- The new surfaces carry their own palette diff so the completeness guard does not shrink — they are skipped by the fixture guard because they are not `*Fixture`, and the exchange for that skip is an equivalent guard of their own.
- A degraded form is reachable by name at a pinned size rather than by resizing the terminal: a human dragging a window to find the fallback point cannot reproduce it, and the fallback is where the screen is most likely to be wrong.
- The capture surface sets no terminal background so what a human sees is what a pane shows — the picker and the swatch both own the whole terminal and set OSC 11; a pane draw never does, and a surface that did would be showing a canvas the real screen does not paint.
- `NO_COLOR` is honoured on the surface as it is on a fixture, through the same env read rather than a second one.
- `internal/capture` stays out of the production binary — the existing import guard is the check, and nothing here may give the portal binary a path to it.

**Context**:
> `cmd/capturetool` is a separate offline program that renders a named deterministic fixture of the real TUI with every tmux seam faked; it is the only route to seeing a visual change before release, because a scratch build of Portal itself disturbs the running daemon and touches real state.
>
> The Go fixture definitions in `internal/capture` and the harness itself are permanent — the swap-and-diff completeness guard drives the fixture renderer and its coverage assertion enumerates whatever fixtures exist, so deleting one silently shrinks the guard rather than failing it.
>
> The capture surfaces are standalone named entries alongside the contrast swatch rather than picker fixtures, since a fixture builds a picker model through the shared constructor and these screens are not one. The swap guard's "the swatch is the only skip" assertion widens to a named set, and in exchange the panel surfaces get their own palette-diff guard, so enrolling a second skip does not silently shrink the completeness guard.
>
> The two design frames the specification names are committed at `testdata/vhs/reference/resume-panel-waiting-nord.png` and `testdata/vhs/reference/resume-panel-discard-confirm-nord.png`. These surfaces exist so each screen can be rendered live and held against its frame at the phase's visual gate, and against Portal's own existing modal grammar — the kill modal, the rename modal's badge slot and the shared panel frame — which is reachable through the picker fixtures the same tool already renders.
>
> The `.tape` files and rendered PNGs under `testdata/vhs/` are scaffolding rather than a durable asset: they are written as work proceeds and cleared after sign-off. Nothing in this task commits an image.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §5.2, §5.3, §5.4, §5.5
