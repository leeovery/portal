TASK: restore-host-terminal-windows-5-4 — Multi-select footer copy

ACCEPTANCE CRITERIA:
1. In multi-select mode (filter not focused) the footer entry row reads exactly `↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel`.
2. The multi-select footer has NO right-aligned `? help` anchor (matches the delivered frame).
3. While the filter input is focused within the mode, the input-active filter footer renders (not the multi-select footer).
4. The footer is two rows over the shared `border.footer` rule and is height-neutral (does not change the list's reserved height budget).
5. At a narrow width the left cluster degrades on one line (leading entries kept, trailing dropped with an ellipsis) and never wraps to a second row.
6. Under NO_COLOR the glyphs + labels render on the terminal's native fg/bg (hues + canvas drop) with no crash.

STATUS: Complete

SPEC CONTEXT:
Spec §Multi-Select Mode → Mode affordance (visual) (line 137) and §Design References → Sessions — Multi-Select (active) (line 503) both fix the footer copy EXACTLY as `↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel`, sourced from `design/sessions-multi-select-active.png` which shows five entries and NO `? help` on the right. §Filter as an inner sub-state (lines 125–128) requires the focused filter input to own `Enter`/`Esc` and time-share the single notice-band/footer slot: filter-focused → filter footer; otherwise → multi-select footer. "No new colour tokens" — violet reused as the selection accent, standard MV footer convention (blue keys / dim labels). Implementation matches the spec copy byte-for-byte.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/footer.go:111-196 (constants, entries, renderMultiSelectFooter, fitFilterCluster); internal/tui/model.go:4636-4647 (renderSessionsFooterForFilterState wiring)
- Notes:
  - Copy is authored as spec-exact per-entry constants (footer.go:122-133) plus a composed `multiSelectFooterText` single-source constant (footer.go:138-142), mirroring the `commandBandText` pattern the plan requested. Because both the render entries (multiSelectFooterEntries) and multiSelectFooterText are composed from the SAME per-entry constants, they cannot drift.
  - renderMultiSelectFooter (footer.go:170-176) reuses footerTopRule (shared 1px border.footer rule), fitFilterCluster→renderFilterCluster (per-glyph cluster body), and assembleRightAnchoredRow with an EMPTY right segment (rightSeg=="", 0) — which routes to headerPadRight, so no `? help` anchor renders. It deliberately does NOT route through renderFilterFooter/filterFooterRow (those hardcode the sessionsKeymap `? help` anchor), exactly as the plan directs.
  - Two-row shape via lipgloss.JoinVertical(rule, row) → height-neutral against renderSessionsFooter (also rule+row).
  - Resolver wiring: NoMatches/Empty guards kept; `FilterState()==list.Filtering` → renderFilteringFooter (filter focused, mode steps aside); then `m.multiSelectMode` → renderMultiSelectFooter (covers unfiltered AND FilterApplied-in-mode); then FilterApplied→renderFilterAppliedFooter; default→renderSessionsFooter. Branch ordering matches the plan precisely. No drift.

TESTS:
- Status: Adequate
- Coverage:
  - AC1 exact copy: TestMultiSelectFooter_ExactCopy (render == exact copy), TestMultiSelectFooter_CopyConstant (constant pin + render tied to constant), TestSessionsFooterResolver_MultiSelectMode (resolver path shows `m toggle`).
  - AC2 no `? help`: TestMultiSelectFooter_NoHelpAnchor (no `? help`, no `help`, no accent.violet `?` role SGR), TestSessionsFooterResolver_MultiSelectMode (no `? help`, no `switch view` leak).
  - AC3 filter-focused precedence: TestSessionsFooterResolver_FilteringOverridesMultiSelect (Filtering state → `browse results` filter footer, NOT `m toggle`).
  - AC4 height-neutral: TestMultiSelectFooter_HeightNeutral (==standard footer height AND ==2).
  - AC5 narrow degrade: TestMultiSelectFooter_NarrowDegradeOneLineEllipsis (widths 56/40/30/20/12 all exactly 2 rows + no width overflow; at 30 navigate survives, ellipsis present, trailing `esc cancel` dropped). Additionally TestFitFilterCluster_MatchesSharedHelperAcrossWidths (footer_fit_test.go) pins the exact fitter output byte-for-byte against a reference algorithm across wide/narrow/ellipsis-only/empty regimes.
  - AC6 NO_COLOR: TestMultiSelectFooter_NoColorKeepsGlyphsDropsHues (all five glyphs+labels present, no canvas bg SGR, no accent.blue/text.detail/border.footer fg SGR).
  - Extra convention coverage (not over-testing — from the "Do" list): TestMultiSelectFooter_TokenColours (blue keys/dim labels/footer rule in both modes), TestMultiSelectFooter_PaintsCanvasNoEdgeBleed (canvas painted in both modes).
- Notes:
  - Well-balanced. The ExactCopy + CopyConstant pair is not redundant: one asserts the render, the other pins the constant and ties render→constant (the requested drift guard). No excessive mocking; the two resolver tests drive the real Model resolver (NewModelWithSessions + real FilterState), verifying wiring, not just the pure render function.
  - One gap: the plan explicitly states the `m.multiSelectMode` branch "covers both the unfiltered and the FilterApplied-in-mode states", but no test exercises multiSelectMode=true + FilterState==list.FilterApplied → multi-select footer. Only the unfiltered case is asserted. A reorder regression (moving the FilterApplied switch above the multiSelectMode check) would not be caught. See non-blocking note.

CODE QUALITY:
- Project conventions: Followed. Reuses shared footer plumbing (footerTopRule, renderFilterCluster, assembleRightAnchoredRow, fitClusterToWidth) rather than duplicating; single-source copy constants per the commandBandText idiom; no raw hex (all colour via theme.MV tokens); no t.Parallel(); heavily-documented in the codebase house style.
- SOLID principles: Good. renderMultiSelectFooter is a small single-responsibility composition; fitFilterCluster owns only the per-glyph cluster+budget and delegates the shared narrow-degrade loop to fitClusterToWidth (shared with fitLeftCluster, so the two cannot drift).
- Complexity: Low. Linear composition; the only loop is the shared, well-tested fitter.
- Modern idioms: Yes. Idiomatic slice construction, lipgloss joins.
- Readability: Good. Intent is explicit and the comments state WHY (e.g. why it does not route through renderFilterFooter).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/multi_select_footer_test.go:200 — Add a resolver test for the FilterApplied-in-mode path (set multiSelectMode=true and sessionList.SetFilterState(list.FilterApplied), assert renderSessionsFooterForFilterState renders the multi-select footer — `m toggle`, no `? help`, not the `esc clear filter` FilterApplied footer). The plan calls this state out as explicitly covered by the multiSelectMode branch, but only the unfiltered case is currently tested; a branch-reorder regression would go uncaught.
