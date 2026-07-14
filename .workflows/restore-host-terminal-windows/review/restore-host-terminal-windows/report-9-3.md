TASK: restore-host-terminal-windows-9-3 — Extract the left-bar single-glyph column renderer (renderLeftBarGlyphColumn)

ACCEPTANCE CRITERIA:
1. The `glyphStyle.Render(glyph) + bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(glyph)))` shape appears exactly once (inside renderLeftBarGlyphColumn); the marked, gone, and selected-selector columns all delegate to it.
2. The rendered left-bar column for a marked (●), gone (⚠), and selected (▌) row is byte-identical to today, including the fixed 2-cell width and the name's left edge.
3. The gone → marked → selector precedence in renderSessionRow is unchanged.

STATUS: Complete

SPEC CONTEXT: A duplication-driven analysis-cycle (Phase 9) refactor, not a spec-feature task. The work unit added two copies of the "render one glyph in the fixed 2-cell left-bar column + pad the remainder" shape across earlier tasks — the marked-row ● column (task 5-2) and the gone-row ⚠ column (task 6-7) — alongside the pre-existing ▌ selector branch, hitting Rule of Three. The shared invariant is the leftBarColumnWidth (2-cell) geometry (§3.3/§3.5/§4.1) that keeps the name's left edge fixed regardless of which glyph occupies col 0; three independent copies meant a width/pad change had to be mirrored three ways or the marked/gone/selected rows drift out of alignment.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/session_item.go:387-390 (renderLeftBarGlyphColumn); delegations at :372 (selector branch of renderLeftBarColumn), :402 (renderMarkedLeftBarColumn), :416 (renderGoneLeftBarColumn); precedence switch at :484-492 (renderSessionRow).
- Notes:
  - AC1 verified: grep of the production tree shows the shape `padTo("", leftBarColumnWidth-lipgloss.Width(...))` occurs exactly once outside tests (session_item.go:389). The two remaining occurrences (row_style_helpers_test.go:57, :134) are intentional pre-refactor golden reproductions (preLeftBar / preGlyphColumn), consistent with this file's established consolidation-gate idiom. All three call sites delegate to the helper as prescribed.
  - AC2 verified structurally: each caller passes the identical (glyph, glyphStyle, bg) args the pre-refactor inline copies used, and the helper body is the verbatim shape — so output is byte-identical by construction. flashWarningGlyph resolves to "⚠" (notice_band.go:39), multiSelectMarker "●", selectorBar "▌".
  - AC3 verified: renderSessionRow's switch is goneRow → marked → default(selector), unchanged; renderLeftBarColumn's unselected branch left as-is (bg.Render(padTo("", leftBarColumnWidth))). No behavioural change to callers.

TESTS:
- Status: Adequate
- Coverage:
  - row_style_helpers_test.go:144 TestRenderLeftBarGlyphColumn_MatchesPreRefactorGolden — asserts the helper reproduces the pre-refactor glyph-column block for all three glyphs (●, ⚠, ▌) × 2 modes × selected/unselected × colourless on/off, using representative role tokens (AccentViolet, StateRed).
  - Row-level byte goldens: TestRenderSessionRow_ByteIdenticalAcrossRefactor (captured bytes) anchor the selector/unselected cases. Marked and gone columns are anchored by existing suites: multi_select_marker_test.go (● at col 0, precedence over ▌, width/column byte-unchanged, By-Tag repeat, header-never-bullet) and burst_preflight_abort_test.go (⚠ at col 0, precedence over ●/▌, session-gone badge, width byte-unchanged, colourless survival). These would fail on any column-position / width / pad drift introduced by the extraction.
- Notes: Not under-tested — the three-glyph fold is directly pinned and the marked/gone/selected row-level regressions all still exercise the extracted path. Not over-tested — the new helper test is a single focused table; it is somewhat self-referential (preGlyphColumn re-declares the shape rather than using captured bytes), but that is exactly the anti-drift gate's purpose and matches the sibling preRowBg/preRowToken/preLeftBar pattern in this file, with the true byte anchors living in the captured row goldens and the marked/gone suites. No redundant assertions, no excess mocking.

CODE QUALITY:
- Project conventions: Followed. Small free-function helper with a role-token style seam (no raw hex at call sites; colour flows through rowToken). Doc comment names the §3.3/§3.5/§4.1 invariant it centralises.
- SOLID principles: Good — single-responsibility glyph-column renderer; callers own only glyph + role-token style + precedence.
- Complexity: Low (one expression, no branches).
- Modern idioms: Yes.
- Readability: Good — the helper's doc comment explicitly enumerates the three folded call sites and why the 2-cell width lives in one place.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
