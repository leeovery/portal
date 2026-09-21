# Consolidation Tasks: Lazy Resume On Attach (Phase 3)

## Task 1: The Wrap Re-Flows Its Overshoot Instead of Dropping It
placement: phase 3
severity: behaviour

**Problem**: `ansi.Wrap` over-packs a run of hyphens past the width it was given, at ordinary widths rather than only degenerate ones, and `clampToWidth` then truncates that overshoot permanently with no `…` marking the loss. Verified against the pinned `github.com/charmbracelet/x/ansi v0.11.7`: at width 12, `ansi.Wrap("a-b-c-d-e-f -- --port=3000 --resume", 12, "")` returns a first line of display width 15, and the clamp renders `"a-b-c-d-e-f "` — the standalone `--` argument gone from the screen entirely rather than shortened. The same happens at width 6 on the second row. Measured across realistic commands, 0.67% lose content at the card's width and 1.97% at a degraded pane's, up to six cells. The command is the only thing on either screen that says which piece of work the pane is holding, so a user approves or discards on a misread of it, with nothing on screen saying anything was cut. A second, independent cost of the same defect: it is why the stronger invariant "no line `resumeCommandLines` returns begins or ends with a space" cannot be asserted today — four such lines over 15 commands × widths 1-80, two of them the over-pack (`d-e-f ` at width 6, `a-b-c-d-e-f ` at width 12), two on degenerate widths. Adding that assertion now would mean writing carve-outs that encode the defect as expected behaviour.

**Solution**: Make the overshoot re-flow rather than disappear. When `ansi.Wrap` returns a line wider than the width, carry the excess into the following line and re-clamp, so the only text lost is text that ran past `resumeCommandMaxLines`, where `resumeEllipsis` already marks it. Truncating with `resumeEllipsis` instead of `""` is the smaller alternative and buys correctness of the signal but not of the text; re-flow buys both. This lands inside `wrappedLines`, which is where task 2 routes the two older sites — so this task before that one means one row-text change rather than two. It changes row text the phase's own suite pins at widths 1-3 (`internal/tui/resume_panel_parts_test.go`), which is why it was held out of that task's five fix rounds rather than because it was judged small; the user was shown it at the escalation gate and it was explicitly scoped out. With re-flow the blocked property assertion becomes true and is worth adding in the same task. The explanatory comment at `resume_panel_parts.go:40-46` was already corrected at this boundary to name the over-pack as the reason the clamp exists; it must be kept accurate with whatever shape lands.

**Outcome**: A command rendered on either screen is the command that will run, up to an `…` that says where it was cut — and the no-edge-space invariant becomes assertable.

**Do**:
- Replace the clamp-in-place inside `wrappedLines` (`internal/tui/resume_panel_parts.go:46-52`) with a re-flow: walk the `ansi.Wrap` output in order; trim each line's edge spaces; where the trimmed line is still wider than `width`, keep `ansi.Truncate(line, width, "")` as the row and carry `ansi.TruncateLeft(line, width, "")` onto the front of the next line, appending it as a new final line when the over-wide line was the last; trim again after the carry so no row ends on a space the split exposed. The two halves are lossless across a straddling wide grapheme — `ansi.Truncate` drops it and `ansi.TruncateLeft` keeps it — so the carry neither duplicates nor loses a cell.
- Bound the loop on the one input that cannot advance: a grapheme wider than `width`, which `ansi.Truncate` drops and `ansi.TruncateLeft` returns whole, so a naive carry re-presents the same line forever. Emit the empty row and drop that grapheme from the carry, which is what keeps the pinned `{"", "", "…"}` at width 1 for a double-width command.
- Leave `clampToWidth` itself unchanged and leave `clampStackToPane`'s use of it alone (`internal/tui/resume_pane_canvas.go:34-47`): those rows are already styled and their count is budgeted against the pane's height, so a re-flow there would add rows the pane never reserved.
- Re-measure and re-pin, from the code rather than by eye, every expectation the re-flow moves in `internal/tui/resume_panel_parts_test.go` — the degenerate-width tables at widths 1-4 and any hyphen-dense case; leave no comment claiming the clamp cuts the overshoot away, including the explanatory one at `resume_panel_parts.go:40-46`.
- Add the property the over-pack blocked: over the suite's commands at every width from 1 to 80, no line `resumeCommandLines` returns begins or ends with a space.

**Acceptance Criteria**:
- [ ] `resumeCommandLines("a-b-c-d-e-f -- --port=3000 --resume", 12)` renders the standalone `--` argument on the following row instead of dropping it, and the same command at width 6 keeps it too.
- [ ] Every line is still no wider than the width asked for, at every width from 1 to 80 across the suite's commands — the geometry invariant the clamp exists for is unchanged.
- [ ] No line `resumeCommandLines` returns begins or ends with a space, at any width from 1 to 80.
- [ ] The only text a render loses is text past `resumeCommandMaxLines`, and that row carries `resumeEllipsis`: a command that fits three re-flowed rows carries every character it was given, in order.
- [ ] A grapheme wider than the width terminates rather than looping: a double-width command renders `{"", "", "…"}` at width 1, and the widths 1-4 tables pass at their re-measured values.
- [ ] The degraded stack is untouched: `clampStackToPane` still truncates its styled rows and still renders no more rows than the pane's height.
- [ ] `RenderResumePanel` and `RenderResumeDiscardConfirm` still return exactly Width × Height cells at every size their suites drive, card and plain stack alike.

**Tests**:
- `"it re-flows a hyphen run past the width onto the next row"` (table: the worked command at widths 12 and 6)
- `"it carries every character of a command that fits three rows"`
- `"it marks the third row with … when the command runs past it"` (re-measured)
- `"it returns no line beginning or ending with a space"` (table: the suite's commands × widths 1-80)
- `"it renders an empty row for a grapheme wider than the width"` (double-width command at width 1)
- `"it returns no line wider than the width asked for"` (widths 1-80)
- `"it truncates rather than re-flows the degraded stack"` (`clampStackToPane` at a pane narrower than its rows)

## Task 2: One Wrap-And-Cap Helper for the Three Sites That Restate It
placement: phase 3
severity: behaviour

**Problem**: `internal/tui/notice_band.go:123` and `internal/tui/theme_panel_message.go:127-139` hand `ansi.Wrap`'s output straight into fixed-width frames, and `internal/tui/resume_panel_parts.go:30-60` is the same five-step rule — wrap, split, keep N-1, join the remainder, truncate with an ellipsis — plus the trim and clamp this phase added. The overflow chain was traced end to end: an over-wide row reaches `padRightWithStyle` (`header.go:119-121`), which returns it unchanged, then `padLineToCanvasWidth` (`model.go:3287-3293`), which explicitly never truncates, then the terminal wraps it and the picker paints one more line than `pageBandHeight` counted. That is the frame-overflow class that once pushed the picker's title and cursor off the top of the screen. It is latent at today's copy — measured, no shipped band message reproduces it — and goes live the moment a band message quotes a path, a bundle id like `dev.warp.Warp-Stable`, or a user-chosen session name. Nothing cross-checks the three copies: each pins its own behaviour, so all three stay green while one carries a correction the others do not.

**Solution**: One shared wrap-and-cap helper the three sites call, carrying the trim and the clamp. This is classed as behaviour rather than duplication deliberately: routing the two older sites through the clamp changes what they render, which the bar forbids folding into a refactor. Ordered after task 1, so the re-flow lands inside the helper before the other two sites are routed to it — otherwise the same row-text change is made twice.

**Outcome**: One implementation of the rule, so a correction to it reaches every screen that wraps into a frame, and no site can hand an over-wide row into a fixed-width frame.

**Do**:
- Land this after task 1, so the kernel the other two sites adopt is already the re-flowing one. Move `wrappedLines` and `clampToWidth` out of `resume_panel_parts.go` into a neutral `internal/tui/text_wrap.go`, and add the cap beside them: `wrapCapped(text string, width, maxRows int, ellipsis string) []string` — `wrappedLines`, then keep `maxRows-1` rows, join the remainder with a space, and truncate that last row with `ellipsis`. `resumeCommandLines` becomes `wrapCapped(sanitiseCommandText(command), width, resumeCommandMaxLines, resumeEllipsis)` and keeps its own sanitiser.
- Route `themePanelMessageText` (`internal/tui/theme_panel_message.go:127-139`) through it: keep the `!wrap || width == 0` truncate arm exactly as it is, and replace the five-step body with `strings.Join(wrapCapped(message, width, themePanelMessageWrapRows, themeRowEllipsis), "\n")`.
- Route `renderNoticeBand` (`internal/tui/notice_band.go:123`) through `wrappedLines(message, avail)` in place of the bare `strings.Split(ansi.Wrap(message, avail, ""), "\n")`. The band has no row cap — a caller budgets rows by measuring the rendered height — so it takes the wrap-trim-clamp core alone and still returns as many rows as the message needs, one more than today where a re-flow splits an over-packed row.
- Leave `destructiveConsequenceRows`'s `ansi.Wordwrap` (`internal/tui/destructive_confirm.go:83-92`) alone: it is a different rule over fixed copy, not a fourth copy of this one.
- Assert, once per adopted site, the property none of the three checks today — no rendered row is wider than the width it was given: drive the real `renderNoticeBand` with a path-bearing and a bundle-id-bearing message (`dev.warp.Warp-Stable`) over widths 10-120, and `themePanelMessageText` with a slug-bearing message over inner widths 20-47. Re-measure and re-pin whatever the trim moves in `internal/tui/notice_band_test.go` and `internal/tui/theme_panel_message_test.go`.

**Acceptance Criteria**:
- [ ] `ansi.Wrap` is called in exactly one non-test file in `internal/tui`, and all three sites reach it through that one helper.
- [ ] No row `renderNoticeBand` renders is wider than the band width, for a message carrying a long unbroken path and one carrying a hyphen-dense bundle id, at every width from 10 to 120 — the widths the finding reproduced off-width rows at (10, 15, 17, 21, 26, 29, 38, 51, 54, 58, 67, 74, 83, 95, 111, 113) all hold.
- [ ] No row `themePanelMessageText` returns is wider than the `inner` it was given, for a slug-bearing message at every inner width from 20 to 47, and the slot still costs exactly `themePanelMessageWrapRows` rows when it wraps.
- [ ] Today's shipped band and panel copy renders the same visible text at the same row widths as before the change, at widths 20-120 — the latent defect is fixed without moving any copy that ships.
- [ ] A band message that over-packs renders one more row rather than a truncated one, and `pageBandHeight` / the page's own height budget both count that row, so the composed page still fits the terminal height.
- [ ] `resumeCommandLines` renders identically to task 1's landed behaviour — routing it through the extracted `wrapCapped` moves no row of either resume screen.
- [ ] The theme panel's non-wrapping arm is untouched: at `width == 0` or with `wrap` false the slot is still one `ansi.Truncate` carrying `themeRowEllipsis`.

**Tests**:
- `"it renders no band row wider than the band"` (table: path-bearing and bundle-id-bearing messages × widths 10-120)
- `"it renders no panel message row wider than the inner width"` (table: slug-bearing message × inner 20-47)
- `"it leaves today's shipped copy reading the same"` (table: the spawn unsupported and no-op messages, the by-tag signpost, the panel's confirm and commit-failed copy)
- `"it costs one more row for a message that over-packs"` (band height measured off the render)
- `"it truncates rather than wraps below the wrap threshold"` (`themePanelMessageText` with `wrap` false and at `inner` 0)
- `"it wraps through one implementation"` (source assertion: one non-test `ansi.Wrap` call site in the package)

## Task 3: The Canvas Fill Is One Kernel, Not Two
placement: phase 3
severity: duplication

**Problem**: `internal/tui/model.go:3005-3022` and `internal/tui/resume_pane_canvas.go:55-69` are the same eleven lines — build the canvas style, parse its background params, loop to the height, backfill mid-line gaps, pad each row to the width, then pad blank rows. The colourless halves already delegate to one `fillColourless`; the coloured halves do not. Neither suite cross-checks the other, so a later correction to the per-cell background rule — a new SGR form that drops the background to default, a change in how trailing padding is emitted — lands on one copy and leaves the other, with both suites green. The symptom reaches the user as a stripe of their own terminal background showing through either the picker's canvas or the resume panel on a restored pane: exactly the bleed class the backfill was introduced to fix.

**Solution**: `fillCanvas` calls `fillPaneCanvas(view, contentW, contentH, m.themeState.active, m.colourless)` and applies `overlayThemePanelOnContent` and the inset (`insetColourless` / `insetCanvasCanvas`) on top, keeping the gutter and the panel composite exactly where they are. Behaviour-preserving; the picker's existing canvas-paint suite is what proves it.

**Outcome**: One canvas-painting kernel, so a correction to the per-cell background rule cannot reach one screen and miss the other.

**Do**:
- Delete the coloured arm's eleven lines from `fillCanvas` (`internal/tui/model.go:3005-3023`) and have both arms call `fillPaneCanvas(view, contentW, contentH, m.themeState.active, m.colourless)` — the kernel already branches on `colourless` to `fillColourless`, which is what the picker's own colourless arm calls today.
- Apply `overlayThemePanelOnContent` and then the inset on top of the kernel's result, exactly where they are now and in the order `model.go:3028-3031` records as load-bearing: `insetColourless(content, w, h, contentW, contentH)` under `m.colourless`, `insetCanvasCanvas(strings.Split(content, "\n"), w, h, contentW, canvas)` otherwise. The gutter and the panel composite do not move.
- Take the `lipgloss.Style` the coloured inset needs from the same one-line helper the kernel builds it with rather than constructing `lipgloss.NewStyle().Background(…)` a second time in `fillCanvas` — that line is the last thing the two copies still share.
- Move `fillPaneCanvas` (and its cases from `internal/tui/resume_pane_canvas_test.go:321-347`) out of the pane-screen file into `internal/tui/canvas_fill.go` / `canvas_fill_test.go`, keeping the name and signature; it is no longer the pane's alone, so leave no comment at `resume_pane_canvas.go:49-51` saying that it is.
- Extend that one suite to drive the kernel through both callers' shapes — the picker's (a view shorter than the region, a view taller than it, a view narrower than the width, a mid-line SGR reset, colourless) as well as the pane's — so one suite covers the shared rule instead of each caller pinning its own copy.

**Acceptance Criteria**:
- [ ] `backfillCanvasBackground` and `padLineToCanvasWidth` are called from exactly one non-test file in `internal/tui`, in one loop — the kernel; a source assertion holds it there so the second copy cannot come back.
- [ ] The picker's composed frame is byte-identical before and after, across the sessions page with and without a notice band, the projects page, a modal, the theme panel open, and colourless.
- [ ] The theme panel composite still lands after the fill and before the gutter inset, and the panel's own cells are neither backfilled nor padded by the fill.
- [ ] The gutter is unchanged: `insetCanvasCanvas` still paints canvas gutters and `insetColourless` still paints SGR-free ones, at the same geometry `gutterPadding` computes.
- [ ] Content taller than the content region is still clamped to it, and a region taller than the content is still padded with blank canvas rows — for both callers, through the one kernel.
- [ ] The resume panel and discard confirmation render byte-identically to their current output at every size their suites drive.
- [ ] Every existing canvas suite passes untouched: `TestCanvasCellBackground_EveryInGridCellIsCanvas`, `TestCanvasCellBackground_TitleAndFooterGaps`, `TestOuterFill_*`, `TestContentInset_GutterPaintedCanvas`, `TestColourless_FillEmitsNoCanvasBackground`.

**Tests**:
- `"it composes the frame the inline fill composed"` (golden captured from the current code before the change; table: sessions with band, sessions without, projects, modal, theme panel open, colourless)
- `"it fills both callers' shapes"` (table over the picker's and the pane's dimensions: short view, tall view, narrow view, mid-line SGR reset, colourless)
- `"it paints the per-cell background in one place"` (source assertion over `backfillCanvasBackground` / `padLineToCanvasWidth` call sites)

## Task 4: The Appearance Query's Own Reply Cannot Answer the Panel
placement: phase 3
severity: behaviour

**Problem**: The pane's appearance probe reads the terminal's reply under a deadline and closes, which it must — there is no program loop to resolve into, and a post-timeout drain might swallow a real keystroke. A reply arriving after that deadline therefore stays in the pane's input queue for whatever holds the pane next. That reply is not inert: its body is `]11;rgb:RRRR/GGGG/BBBB`, so it opens with `ESC` — the discard confirmation's cancel key — and on a terminal whose background carries a hex `d` (`#0d1117`, `#2d2d2d` and many others) it contains the waiting panel's discard key too. So a restored pane can come up already showing `▲ Discard resume?` with nobody having pressed anything, and a confirmation the user did open can back itself out. Nothing is destroyed by a reply alone, because `y` is not a hex digit. The picker's own gate has no such exposure: it resolves inside a program that owns stdin for its whole life, so a late answer can never be read as input. The evidence is `internal/tui/pane_appearance.go:90-101` (query written, deadline set, `readBackgroundReply` returning `("", false)` with no drain) and `:82` (the reader closed, which discards no queued input), against `internal/tui/resume_discard_confirm.go:18-24` over `internal/tui/destructive_confirm.go:16` — `destructiveKeyCancel = "esc"`. What this task can close is the reply that arrives while the probe is running or between its deadline and the drop; a reply arriving after the drop is bounded by the waiter's own escape-sequence handling and is not this task's to close. The same reversed ordering also sits in phase 4's task 4-1 (`.workflows/lazy-resume-on-attach/planning/lazy-resume-on-attach/phase-4-tasks.md:33`), whose acceptance criterion has the probe running "after any input drop", and whose production wiring calls `tui.ResolvePaneTheme(resolution.Nomination, colourless)` — a call the signature change below stops compiling, which is what puts the corrected ordering in front of whoever executes 4-1 rather than leaving it to be remembered.

**Solution**: The ordering is the fix, and it is now stated in the specification (corrigendum 2026-09-21, §5.2): the input drop runs after the appearance query, never before — a drop taken first cannot drop the query's own reply. This task makes the panel-drawing path hold that ordering and pins it, so the protection §4.3 already describes covers a byte source it did not enumerate because this route did not exist when it was written. **Planning note carried with it:** phase 5's task 5-4 currently states the opposite ordering in an acceptance criterion, reading the probe as harmless because it "runs after the drop" — the timeout bounds how long the probe waits, not when the reply arrives. That criterion is what makes this failure live, and it must be corrected when 5-4 is authored rather than implemented as written.

**Outcome**: No byte the user did not send can open, cancel or answer either screen — including the terminal's answer to a question Portal itself asked.

**Do**:
- Make the ordering structural at the one seam the pane draw resolves its palette through, so it cannot be a matter of call-site discipline: `func ResolvePaneTheme(n theme.Nomination, colourless bool, dropInput func() error) (theme.Theme, error)` in `internal/tui/pane_appearance.go:60`, with `dropInput` threaded through the unexported `resolvePaneTheme(n, colourless, dropInput, p)` so the probe stays injectable. The exported function has no production caller yet — `internal/capture` and `cmd/capturetool` build their themes directly — so the signature change ripples into `internal/tui/pane_appearance_test.go` and nothing else.
- Run `dropInput` once, after the appearance query has resolved and before the theme is returned, on every arm — constant nomination, zero nomination, `colourless`, and the probe's, whether it read a reply, hit the deadline, found no terminal, or failed to open one. A nil `dropInput` is a no-op, so a draw that owes no drop (every waiting-panel draw) is unchanged.
- Return the resolved theme on every path, including one where `dropInput` errors, and return that error beside it as it came: the caller decides what to paint and what to report, and nothing here logs it, retries it or swallows it.
- Pin the ordering with a call recorder shared by the probe harness's `out` writer, its reader and the drop (`internal/tui/pane_appearance_test.go:63-118` already carries the harness): the drop's entry sits after the query write and after the reader's `Close`.
- Pin the single route with a source assertion in the package: `newPaneAppearanceProbe` has exactly one call site outside its own declaration and the package's tests, so no second entry point can resolve a pane's palette with the drop somewhere else.

**Acceptance Criteria**:
- [ ] `ResolvePaneTheme` takes the drop and runs it after the appearance query resolves — on every nomination arm and every probe outcome, exactly once per resolution.
- [ ] The drop is never entered before the query is written, and never before the probe's reader is closed.
- [ ] A nil drop resolves the same palette today's code resolves for every arm, and returns a nil error — the constant and zero arms still write nothing to the terminal and still open it zero times.
- [ ] A drop that returns an error still returns the resolved palette, and returns that error unchanged, so the caller can paint the fallback screen and report the reason.
- [ ] A `colourless` or constant resolution writes no query and opens no terminal, and still runs the drop exactly once.
- [ ] `newPaneAppearanceProbe` has exactly one call site in the tree outside its declaration and the package's own tests.
- [ ] The probe's existing guards still pass unchanged: `TestPaneAppearance_TakesThePickerTimeout`, `TestPaneAppearance_OpensNothingButThePanesTerminal`, `TestBackgroundSet_ConfinedToRestore`.

**Tests**:
- `"it runs the drop after the appearance query resolves"` (ordering recorder; table: reply before the deadline, reply never sent, reader that fails to open)
- `"it runs the drop exactly once for every nomination"` (table: constant, zero, adaptive pair, colourless)
- `"it resolves the same palette with no drop supplied"` (table over the same arms)
- `"it returns the resolved palette when the drop fails"`
- `"it returns the drop's own error"`
- `"it reaches the probe from one place alone"` (source assertion)
