TASK: 5.2 — `●` selection markers on session rows (restore-host-terminal-windows-5-2 / tick-daede7)

ACCEPTANCE CRITERIA:
- A marked SessionItem row renders `●` in the left-bar column in accent.violet (dark canvas); an unmarked row does not.
- In By-Tag mode a multi-tag session that is marked shows `●` on every one of its rows (all share Session.Name).
- A row that is both cursor and marked renders the bg.selection band AND the `●` (`●` in the left-bar column, band spans the row).
- HeaderItem rows never render a `●`.
- With multiSelectMode == false no row renders a `●` (gated on MultiSelect).
- Under NO_COLOR (Colourless == true) the `●` glyph renders with no violet hue and no canvas/selection background.
- The row remains exactly one delegate line and name/count/attached alignment is byte-unchanged from the non-marked row.

STATUS: Complete

SPEC CONTEXT: Multi-Select Mode → Mode affordance (visual): "Selected rows carry a glyph marker + the mode colour, never colour-only" (MV NO_COLOR rule); violet reused as the selection accent, `●` marker on selected rows incl. the cursor row; no new colour tokens. Design Reference `design/sessions-multi-select-active.png` shows `●` at the far-left 2-cell column on three rows including the banded cursor row `fab-flowx-explore`, so the name's left edge is unchanged.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/session_item.go:34 — `multiSelectMarker = "●"` const near `selectorBar`.
  - internal/tui/session_item.go:210-216 — `SessionDelegate.MultiSelect bool` + `Selected map[string]struct{}` (nil-tolerant).
  - internal/tui/session_item.go:229-232 — `isSelected(set, name)` nil-safe membership helper.
  - internal/tui/session_item.go:392-403 — `renderMarkedLeftBarColumn` (violet `●` in the shared 2-cell left-bar via `renderLeftBarGlyphColumn`).
  - internal/tui/session_item.go:482-492 — `marked := d.MultiSelect && isSelected(d.Selected, it.Session.Name)`; switch renders gone > marked > default(selector), so `●` supersedes `▌`; marker style routed through `d.rowToken(..., AccentViolet, selected)` so it carries bg.selection tint on a banded row and drops hue under NO_COLOR.
  - internal/tui/model.go:1281-1289 — `sessionDelegate()` single chokepoint sets MultiSelect + Selected from model state.
  - internal/tui/model.go:1297-1299 — `refreshSessionDelegate()` narrow re-set path.
  - internal/tui/model.go:3510,3528,3541 — enter/toggle/exit all call `refreshSessionDelegate()` so the marker tracks the set live.
- Notes: Matches the plan exactly. The plan text names helpers under "internal/tui/row_style_helpers"; the shared free functions (rowBgStyle/rowTokenStyle/renderLeftBarColumn/renderLeftBarGlyphColumn) actually live in session_item.go — only the test file row_style_helpers_test.go carries that name. Behaviourally correct; a minor naming mismatch (see notes). GoneFlagged precedence (Phase 6.7) sits above `marked` in the switch, which does not affect any 5-2 criterion.

TESTS:
- Status: Adequate
- Coverage (internal/tui/multi_select_marker_test.go):
  - TestSessionRow_MarkedShowsVioletBulletInLeftBar — `●` at col 0 in accent.violet on a marked row; no `●` on an unmarked/unattached row. (criterion 1)
  - TestSessionRow_ByTagMarkedBulletOnEveryRow — `●` on every By-Tag row of a multi-tag session (same name). (criterion 2)
  - TestSessionRow_CursorRowMarkedShowsBandAndBullet — band + `●`, `●` supersedes `▌`, violet carried on the tint. (criterion 3)
  - TestSessionRow_HeaderNeverRendersBullet — HeaderItem carries no `●`. (criterion 4)
  - TestSessionRow_NoBulletWhenMultiSelectFalse — populated set + MultiSelect==false renders no `●`. (criterion 5)
  - TestSessionRow_MarkedColourlessGlyphSurvivesNoHue — glyph survives, no violet fg, no canvas, no selection tint. (criterion 6)
  - TestSessionRow_MarkedAlignmentByteUnchanged — name/window/attached columns and total width unchanged vs unmarked. (criterion 7)
  - TestMultiSelectMarkerReflectsSetLive — model-level: `m`-toggle renders `●`, Esc clears it (proves applyCanvasMode/refreshSessionDelegate wiring).
  - internal/tui/row_style_helpers_test.go — pins renderLeftBarGlyphColumn (incl. the `●` glyph) byte-for-byte against pre-refactor goldens across mode × selected × colourless.
- Notes: One test per acceptance criterion plus live-wiring and alignment gates — focused, no redundant assertions. Tests assert rendered-row behaviour (glyph column, fg SGR, bg params, width), not implementation internals. All shared helpers (renderRow, visibleColOf, flatItems, tokenFgSeq, selectionBgParams, lineHasBgParams, canvasSeq, escSeq, pressM, pressSession) are defined in the package test files. Would fail if the feature broke (glyph absence, wrong column, hue leak, width shift all asserted). Neither under- nor over-tested.

CODE QUALITY:
- Project conventions: Followed. No raw hex at call sites (AccentViolet token via rowToken); NO_COLOR carve-out inherited through the shared rowToken/rowBg free functions; no t.Parallel(); heavy intent-documenting comments per house style.
- SOLID principles: Good. Delegate keeps single render responsibility; marker precedence is a small explicit switch; `isSelected` is a focused nil-safe predicate.
- Complexity: Low. The gone>marked>default switch is linear and clear.
- Modern idioms: Yes. map-set membership, thin composable helpers.
- Readability: Good. The left-bar precedence and the "use `selected` not literal true so the tint/canvas/NO_COLOR rules apply" reasoning is documented in-source.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/row_style_helpers_test.go:1 / internal/tui/session_item.go — the test file `row_style_helpers_test.go` tests free functions (rowBgStyle, rowTokenStyle, renderLeftBarColumn, renderLeftBarGlyphColumn, renderMarkedLeftBarColumn) that live in session_item.go; no `row_style_helpers.go` source file exists. Align the naming: either extract those shared row-style free functions into a new `internal/tui/row_style_helpers.go`, or rename the test to `session_item_helpers_test.go`. Cosmetic only; a future reader looking for the named source finds only the test.
