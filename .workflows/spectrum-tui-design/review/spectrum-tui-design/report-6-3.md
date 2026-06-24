TASK: spectrum-tui-design-6-3 — Extract shared row-style and left-bar-column helpers for the Session/Project list delegates (Phase 6 analysis-cycle DRY/consolidation).

ACCEPTANCE CRITERIA:
- rowBg/rowToken style logic exists in exactly one place; both delegates route through the shared free functions.
- The §3.3 left-bar selector-column render (both selected and unselected cases) exists in exactly one place; both renderSessionRow and renderRowLine call it.
- go build succeeds and the internal/tui test package passes.
- The rendered Session and Project list rows (selection background, accent.violet selector bar, 2-cell column width, indentation) are byte-identical to before in both light and dark modes and under the colourless carve-out, for selected and unselected rows.

STATUS: Complete

SPEC CONTEXT:
- §3.3 (selection thick violet left-bar): selected row = thick block ▌ in accent.violet pinned far-left as a full 2-cell column over a bg.selection tint; unselected = no bar, no tint. Single consistent selection signal across Sessions, grouped views, and Projects (Projects spans both lines). The implementation's selectorBar="▌", leftBarColumnWidth=2, and the violet-on-selection-tint composition match this exactly.
- §4.1 (Session row anatomy): name flex + fixed-width trailing slots; selected name in text.on-selection over bg.selection + violet bar. Confirmed.
- §6.2 (Project two-line rows): selected = full-height accent.violet left bar spanning both lines + bg.selection; name text.on-selection, path text.muted-bright. renderRowLine drives both lines through the same shared bar helper, so the full-height bar spans uniformly. Confirmed.
- §2.5 colourless carve-out: no canvas/selection bg, no foreground hue; structure + glyphs preserved. The shared helpers carry the colourless guard verbatim.

IMPLEMENTATION:
- Status: Implemented (clean, matches task intent and the later 9-3 delegation)
- Location:
  - internal/tui/session_item.go:292-336 — rowBgStyle (free func), rowTokenStyle (free func), renderLeftBarColumn (free func). These are the single home for the row-bg/token style logic and the §3.3 2-cell selector column.
  - internal/tui/session_item.go:341-350 — SessionDelegate.rowBg / rowToken now delegate to the free functions (terse method form retained for call sites).
  - internal/tui/session_item.go:394 — renderSessionRow calls renderLeftBarColumn.
  - internal/tui/project_item.go:77-88 — ProjectDelegate.rowBg / rowToken delegate to the same free functions.
  - internal/tui/project_item.go:141 — renderRowLine calls renderLeftBarColumn.
- Notes:
  - Consolidation is genuine: grep confirms rowBgStyle/rowTokenStyle/renderLeftBarColumn are each defined exactly once; both delegates' rowBg/rowToken bodies are now one-line delegations; both left-bar render sites route through renderLeftBarColumn. No surviving inline bg.selection-vs-canvas branch or duplicated 2-cell selector block in either delegate.
  - The 9-3 follow-up is present and correct: SessionDelegate.canvasBg (session_item.go:201-203) delegates to headerCanvasBg and tokenStyle (223-225) delegates to headerStyle (internal/tui/header.go:82-104). These are the header-leaf canonical helpers, orthogonal to the 6-3 row helpers — no conflict; the row helpers (rowBgStyle/rowTokenStyle) deliberately keep their own selection-vs-canvas branch because the selected case has no header analogue.
  - Other BgSelection.ColorFor / Canvas.ColorFor references in the tui package (edit_modal.go, header.go, model.go) are unrelated surfaces (modal chrome, header spacers, outer fill) — not row-delegate logic — so they are correctly out of scope for this extraction.

TESTS:
- Status: Adequate (well-targeted, not over- or under-tested)
- Coverage (internal/tui/row_style_helpers_test.go):
  - TestRowBgStyle_MatchesPreRefactorGolden — rowBgStyle vs a verbatim pre-refactor reproduction (preRowBg) across selected/unselected × both modes × colourless true/false.
  - TestRowTokenStyle_MatchesPreRefactorGolden — rowTokenStyle vs preRowToken across {zero, bold} base × {TextPrimary, AccentViolet, StateGreen} token × both modes × selected/unselected × colourless.
  - TestRenderLeftBarColumn_MatchesPreRefactorGolden — renderLeftBarColumn vs preLeftBar (verbatim original §3.3 block) for selected (bar + pad) and unselected (full-width pad), both modes, colourless.
  - TestRenderSessionRow_ByteIdenticalAcrossRefactor / TestRenderRowLine_ByteIdenticalAcrossRefactor — full delegate Render output vs captured raw-byte goldens (width 80) for selected + unselected rows across both modes and colourless on/off. The goldens carry the exact SGR sequences, so any drift in style composition, selector glyph, column width, or pad arithmetic is caught.
- Notes:
  - The byte-identical acceptance criterion is met directly: the goldens were captured from the pre-refactor inline-logic source (documented in the file header) and the post-refactor delegates must reproduce them byte-for-byte — this is exactly the regression the task's "Tests" section asks for.
  - The pre* reproductions (preRowBg/preRowToken/preLeftBar) duplicate the production logic, which is intentional and correct for a golden-pin: if the duplication and the production code drifted, the test would catch it (that is the test's job). Not over-testing.
  - Good edge coverage: the Dark==Light coincidence under colourless is explicitly noted as a pinned property; the unselected-passing-selectorStyle harmlessness is implicitly covered (unselected goldens never contain a violet selector run).

CODE QUALITY:
- Project conventions: Followed. Free functions homed in session_item.go per the task's stated home; both delegates already depend on the shared selectorBar/leftBarColumnWidth/padTo primitives there. No t.Parallel() (consistent with package-wide rule, noted in the test header). Naming (rowBgStyle/rowTokenStyle/renderLeftBarColumn) is idiomatic and role-named.
- SOLID principles: Good. Single source of truth for the selection-vs-canvas colour role and the 2-cell column; a future change to bg.selection, accent.violet, or the column width is now a single edit shared by both delegates — exactly the stated outcome.
- Complexity: Low. The free functions are flat guard-then-branch; renderLeftBarColumn is a two-arm conditional.
- Modern idioms: Yes. lipgloss.Style composition via Foreground/Background; ansi.Truncate for the safety clamp.
- Readability: Good. Doc comments on each free function explain the shared-home rationale and the delegation chain; the delegate methods document why the terse method form is retained.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/session_item.go:394, internal/tui/project_item.go:141 — both call sites always compute d.rowToken(lipgloss.Style{}, AccentViolet, true) and pass it as selectorStyle even on unselected rows, where renderLeftBarColumn never references it (only bg.Render is used). This is wasted style construction per unselected row (a hot path: every non-selected list row). Decide whether to defer the selectorStyle construction into renderLeftBarColumn's selected branch (e.g. pass the mode/colourless and build it lazily, or accept a thunk) — this preserves byte-identical output (the unselected path already ignores it) but avoids the per-row allocation. Tagged idea because it requires a small interface decision on renderLeftBarColumn's signature; not mechanical at a single known edit, and the current form is faithful to the pre-refactor logic.
