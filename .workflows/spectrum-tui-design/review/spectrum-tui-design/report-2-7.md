TASK: spectrum-tui-design-2-7 — Sessions grouped reskin: heading `··· N` (text.detail heading + text.dim count) + indented rows (cursor col 2 / name col 4) for By Project & By Tag, pure Lipgloss. (tick-9181ba)

ACCEPTANCE CRITERIA:
- Heading rows render `heading ··· N` with the heading in text.detail and the `··· N` count in text.dim — two separately styled runs, not one faint run; no literal hex / Faint(true) at the call site.
- Grouped session rows nest one indent level further than flat (cursor col 2 / name col 4); flat rows remain flush at col 2.
- Cursor never lands on a header row on initial selection or any navigation (existing cursor-skip preserved, not reimplemented).
- Catch-all (Unknown / Untagged) headings use the same heading style as resolvable groups.
- Grouping is pure Lipgloss in the delegate — no lipgloss/tree (§14.1).
- Grouping machinery (HeaderItem model, Pattern A/B, catch-alls, dir resolution, mode persistence, flatten-on-filter) behaviourally identical; pagination stays exact (one delegate line per row).
- No-tags signpost path behaviourally intact and NOT restyled here (Phase 4).
- Visual verification: by-Project / by-Tag captures match the MV Paper references.
- Behaviour parity traced against the pre-reskin grouping render.

STATUS: Complete

SPEC CONTEXT:
§5.1 (render-layer grouping, the key invariant): headings are real non-selectable HeaderItem rows; heading in text.detail with `··· N` in text.dim (dimmer); session rows nest one indent level further (cursor col 2, name col 4) while flat rows sit flush at col 2; cursor skips headers; every row exactly one delegate line so pagination stays exact; grouping is pure Lipgloss in the existing delegate — NOT lipgloss/tree (§14.1). §2.9 tokens: text.detail (group headings), text.dim (group `··· N` counts). §5.2/§5.3 Pattern A/B + Unknown/Untagged catch-alls. §5.4/§5.5 dir resolution + tag anchoring (preserve). §11.3 no-tags signpost is Phase-4, out of scope.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/session_item.go
  - Heading render split into two token runs: SessionDelegate.Render HeaderItem arm at session_item.go:256-265 — `heading := d.tokenStyle(lipgloss.Style{}, theme.MV.TextDetail).Render(it.headingText())` and `count := d.tokenStyle(lipgloss.Style{}, theme.MV.TextDim).Render(it.countText())`, composed as `bg.Render(groupHeaderIndent) + heading + count`. No Faint(true), no literal hex at the call site — both colours flow from §2.9 tokens.
  - headingText() (session_item.go:157-159) and countText() (session_item.go:164-166) split the `Heading` from the `··· N` count (groupSeparator = "···", U+00B7 ×3, session_item.go:66), keeping the "Heading ··· N" shape.
  - Indent: groupHeaderIndent (col 2, session_item.go:72) for the header; groupRowIndent (session_item.go:78) applied in renderSessionRow gated on `it.GroupKey != ""` (session_item.go:384-388) — rendered BEFORE the left-bar column so cursor/▌ lands at col 2 and name at col 4; flat rows (empty GroupKey) render flush (bar col 0, name col 2). The indent is folded into the width budget (`used` at session_item.go:415 adds lipgloss.Width(indent)), so it shrinks the flex name rather than pushing the row wide.
  - Cursor-skip preserved (not reimplemented) in model.go: ensureSessionRowSelected (model.go:1589) and skipHeaderRow (model.go:1602), unchanged by this task.
  - Grouping machinery untouched: grouping.go last changed at task 1-2 / earlier grouping fixes (git log), predating all reskin commits — buildByProject/buildByTag/assembleGroups/injectGroupHeaders/orderedSessionItems and the catch-alls are byte-for-byte preserved.
- Notes: Catch-all headings (Unknown / Untagged) flow through the identical HeaderItem arm, so they get the same two-run style automatically. The signpost path (byTagSignpost, model.go:1544/1548; byTagSignpostText, model.go:4261) is untouched. No drift from the plan.

TESTS:
- Status: Adequate
- Coverage: internal/tui/sessions_grouped_reskin_test.go is the dedicated gate and maps 1:1 onto the acceptance criteria:
  - text.detail heading + text.dim count, two distinct runs, exact mode-resolved SGR, both Dark+Light: TestGroupHeading_TextDetailHeadingWithTextDimCount.
  - per-run split (detail run precedes dim run; count digit under the dim run): TestGroupHeading_HeadingRunCarriesDetailCountRunCarriesDim.
  - no surviving faint param at the call site (leading `[2;38`/`[2;48`/`[2m` cross-check): TestGroupHeading_NoFaintAttributeAtCallSite.
  - grouped row indent cursor col 2 / name col 4 vs flat flush col 2: TestGroupedRow_NestsOneLevelFurtherThanFlat; flat-flush negative: TestFlatRow_StaysFlushAtColTwo; indent-is-layout-not-selection: TestGroupedRow_UnselectedAlsoIndents; header at col 2: TestGroupHeading_IndentsToColTwo.
  - catch-all same style + same indent: TestCatchAllHeadings_UseSameHeadingStyle, TestCatchAllRow_IndentsLikeResolvableGroupRow.
  - one delegate line each (pagination exactness): TestGroupedRow_OneDelegateLine; plus the regression guard TestGroupedViewDoesNotOverflowViewport (grouped_viewport_overflow_test.go) — 12 projects × 2 sessions never overflows the viewport.
  - no-overflow at narrow widths (indent folded into budget): TestGroupedRow_NeverOverflowsAtNarrowWidths (widths 1..80, both modes).
  - §14.1 no lipgloss/tree: TestSessionsTuiNoLipglossTree (AST source-walk over all production .go files).
  - machinery parity (items/order/Pattern A & B repeat/catch-alls): TestGroupingMachineryPreserved.
  - signpost path intact: TestNoTagsSignpostPathUnchanged (asserts zero HeaderItems injected and the signpost text renders).
  - Cursor-skip edges (initial + nav across boundaries/paging) are covered by the pre-existing grouping/model tests, correct since this task did not touch that logic.
- Notes: Not over-tested — assertions are role/behaviour-focused (token SGR, visible column, line count, builder shape), not brittle internal-state checks. Each test pins a distinct criterion; no redundant happy-path duplicates. The SGR-level pinning is justified (catches a silent token swap a glyph-presence check would miss), consistent with the row/footer/header test convention in this package.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (package-level mock convention). Token-by-name, no literal hex at call sites (§2.9). Two-part render reuses the existing tokenStyle/canvasBg leaf-paint helpers (single source of the canvas/NO_COLOR carve-out) rather than re-deriving. Comments are dense but accurate and load-bearing, matching the file's established style.
- SOLID principles: Good. Render-only change confined to the delegate; the build-layer grouping machinery is untouched (single-responsibility boundary respected). headingText()/countText() cleanly separate the two runs.
- Complexity: Low. The HeaderItem arm is three lines; the row indent is a single gated string prepend folded into the existing width budget.
- Modern idioms: Yes. Idiomatic Lipgloss composition; ansi.Truncate safety clamp preserved.
- Readability: Good. Intent is explicit in comments (why the indent sits before the left-bar column; why the count is a separate run).
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/session_item.go:263-264 — The heading arm hard-references theme.MV.TextDetail / theme.MV.TextDim directly, while the SessionItem row arm routes colour selection through local token vars (nameTok/countTok). Consider extracting a tiny headingTokens helper (or named consts) so the two heading runs read symmetrically with the row path and a future token reassignment has one edit site. Decide whether the symmetry is worth the indirection — current form is clear and correct.
- [idea] internal/tui/theme/theme.go:140,142 — Phase-1 token definition (NOT this task): the §2.9 spec table lists text.detail Light = #5A6296 and text.dim Light = #7C84AA, but theme.go pins Light = #586093 / #767DA2. This task correctly consumes the tokens by name, so the grouped reskin is unaffected; flagging for a Phase-1 token-vs-spec reconciliation (decide which value is canonical). Light captures were not produced for this task (visual gate was dark-mode), so the discrepancy is not visually validated either way.
