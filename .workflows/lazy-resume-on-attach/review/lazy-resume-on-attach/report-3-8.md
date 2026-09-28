TASK: lazy-resume-on-attach-3-8 — One Wrap-And-Cap Helper for the Three Sites That Restate It

ACCEPTANCE CRITERIA:
- `ansi.Wrap` is called in exactly one non-test file in `internal/tui`, and all three sites reach it through that one helper.
- No row `renderNoticeBand` renders is wider than the band width, for a message carrying a long unbroken path and one carrying a hyphen-dense bundle id, at every width from 10 to 120 — the widths the finding reproduced off-width rows at (10, 15, 17, 21, 26, 29, 38, 51, 54, 58, 67, 74, 83, 95, 111, 113) all hold.
- No row `themePanelMessageText` returns is wider than the `inner` it was given, for a slug-bearing message at every inner width from 20 to 47, and the slot still costs exactly `themePanelMessageWrapRows` rows when it wraps.
- Today's shipped band and panel copy renders the same visible text at the same row widths as before the change, at widths 20-120 — the latent defect is fixed without moving any copy that ships.
- A band message that over-packs renders one more row rather than a truncated one, and `pageBandHeight` / the page's own height budget both count that row, so the composed page still fits the terminal height.
- `resumeCommandLines` renders identically to task 1's landed behaviour — routing it through the extracted `wrapCapped` moves no row of either resume screen.
- The theme panel's non-wrapping arm is untouched: at `width == 0` or with `wrap` false the slot is still one `ansi.Truncate` carrying `themeRowEllipsis`.

STATUS: complete

SPEC CONTEXT: Phase 3 builds the resume panel's command block, which wraps a user-authored command over at most three rows at a fixed card width, marking overflow with `…`, and must never widen the card. Task 3-7 made that wrap re-flow `ansi.Wrap`'s over-packed rows (hyphen runs) instead of dropping them. This task lifts that kernel into a neutral helper so the picker's notice band and the theme panel's message slot — which fed `ansi.Wrap` output straight into fixed-width frames — get the same no-over-width guarantee, closing the frame-overflow class that once pushed the picker's title off-screen.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/text_wrap.go:11-19 — `wrapCapped(text, width, maxRows, ellipsis)`: `wrappedLines`, keep `maxRows-1` rows (full slice expression, so the append cannot alias), join the remainder with a space, truncate with the ellipsis, trim.
  - internal/tui/text_wrap.go:30-47 — `wrappedLines` (moved here), holding the package's `ansi.Wrap` call at :31; helpers `droppedGap` :52-62, `rowAndCarry` :68-75, `splitAtWidth` :80-87.
  - internal/tui/text_wrap.go:89-94 — `clampToWidth` (moved here); its consumers are internal/tui/resume_panel.go:63 and internal/tui/resume_pane_canvas.go:36.
  - internal/tui/notice_band.go:122 — `renderNoticeBand` now takes `wrappedLines(message, avail)` with no row cap. The `ansi` import is gone from the file.
  - internal/tui/theme_panel_message.go:126-132 — the `!wrap || width == 0` arm (:128-130) is unchanged as a single `ansi.Truncate` with `themeRowEllipsis`. The wrapping arm is `strings.Join(wrapCapped(message, width, themePanelMessageWrapRows, themeRowEllipsis), "\n")` (:131).
  - internal/tui/resume_panel_parts.go:30-32 — `resumeCommandLines` is exactly `wrapCapped(sanitiseCommandText(command), width, resumeCommandMaxLines, resumeEllipsis)` and keeps its own sanitiser.
  - internal/tui/destructive_confirm.go:83-92 — `destructiveConsequenceRows` still uses `ansi.Wordwrap`, as the task required.
- Notes:
  - The no-over-width property holds by construction. Every row `rowAndCarry` returns is either the trimmed line when it fits, or `ansi.Truncate(line, width, "")` trimmed. `wrapCapped`'s last row is `ansi.Truncate(beyond, width, ellipsis)`. So neither the band nor the panel slot can hand an over-wide row to `padRightWithStyle` / `padLineToCanvasWidth`.
  - When no `ansi.Wrap` row overflows, `wrappedLines` returns the `ansi.Wrap` rows with only their edge spaces trimmed, and those spaces are invisible under the band tint and the canvas pad. So copy changes only where the old render overflowed.
  - The band budget counts the extra re-flow row by construction. `applySessionListSize` (internal/tui/model.go:1107-1110) reserves `sessionBandHeight`, which measures `renderSessionBandSlot` (model.go:1120-1126). `pageBandHeight` (model.go:3056-3061) delegates to the same function.
  - `resumeCommandLines`: `wrapCapped` is the five-step rule plus the trim that the task describes the old resume body as carrying. Because `wrappedLines` already bounds every row to the width, a per-row clamp would be a no-op. The exact-row pins in internal/tui/resume_panel_parts_test.go:119-417 fix both resume screens' rows at widths 1-4, 6, 12, 24 and 52.

TESTS:
- Status: Adequate
- Coverage:
  - "it wraps through one implementation" — internal/tui/text_wrap_guard_test.go:15-35. It is an AST scan of the package's non-test sources (`ParsePackageSources(t, ".", false)`, the same enumeration colour_literal_guard_test.go uses). It requires exactly one `ansi.Wrap` call, located in text_wrap.go.
  - "it renders no band row wider than the band" — internal/tui/notice_band_test.go:495-510. It renders the real `renderNoticeBand` for a path-bearing message and a `dev.warp.Warp-Stable-x86_64` message at every width from 10 to 120, and requires each row to be exactly the band width, so it catches an over-wide row.
  - "it leaves today's shipped copy reading the same" (band) — notice_band_test.go:512-527. It covers the by-tag signpost, the remote and named no-op messages, and the remote and named multi-select refusals at widths 20-120, compared against the trimmed bare `ansi.Wrap` rows.
  - "it costs one more row for a message that over-packs" — notice_band_test.go:554-567. It proves the over-pack exists at width 19 (rows == bare+1) and that no text is lost. "the page budget counts the row it costs" (:569-587) pins slot height == rendered rows+1 and `pageBandHeight` == slot.
  - Panel tests — internal/tui/theme_panel_message_test.go:482-530.
    - No row is wider than `inner` across inner 20-47, and the slot costs exactly `themePanelMessageWrapRows` rows when it wraps. The probe message over-packs under bare `ansi.Wrap` at inner 23 (traced through x/ansi v0.11.7 `wrap`), so the test would catch a regression.
    - Shipped confirm and commit-failed copy read the same at 20-47.
    - The truncate arm is exercised with wrap off and at inner 0.
  - `wrappedLines` / `wrapCapped` invariants (no edge space, never wider, at most three lines, reconstruction, reads the source back) stay in internal/tui/resume_panel_parts_test.go:645-756.
- Notes: The frame-height assertion at notice_band_test.go:584-586 cannot fail on its own, because `fillPaneCanvas` clamps and pads to the region height (internal/tui/canvas_fill.go:29-38). The load-bearing check is `pageBandHeight == slot` at :581, which goes through the same `sessionBandHeight` the list budget reserves. So the criterion is still observed.

CODE QUALITY:
- Project conventions: Followed. There is no `t.Parallel`, the guard is unit-lane and untagged, and there are no raw colour literals.
- SOLID principles: Good. There is one kernel. The band takes the uncapped core, and the two capped sites take `wrapCapped` with their own cap and ellipsis.
- Complexity: Acceptable. `wrappedLines` is split into small named helpers.
- Modern idioms: Yes. It uses `max`, a full slice expression to prevent append aliasing, and `strings.SplitSeq` in tests.
- Readability: Good. The comments in text_wrap.go describe what the code does; none cite process artifacts.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`ansi.Wrap` is called in exactly one non-test file in `internal/tui`, and all three sites reach it through that one helper." — Reading confirmed that the three sites route through text_wrap.go:31. It did not enumerate every non-test file in the package for other `ansi.Wrap` calls. Running `TestWrapsThroughOneImplementation` (internal/tui/text_wrap_guard_test.go) settles the package-wide count.
- "Today's shipped band and panel copy renders the same visible text at the same row widths as before the change, at widths 20-120 — the latent defect is fixed without moving any copy that ships." — By construction, the rendering differs from bare `ansi.Wrap` only where a row overflowed. Whether any shipped message overflows at some width in 20-120 depends on `ansi.Wrap`'s actual output. Running the two "it leaves today's shipped copy reading the same" subtests (notice_band_test.go:512, theme_panel_message_test.go:501) settles it.
