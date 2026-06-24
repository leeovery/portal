TASK: spectrum-tui-design-2-4 — Condensed footer + right-aligned `? help`: single row of core keys from the keymap descriptor (tick-f3737e)

ACCEPTANCE CRITERIA:
- Sessions footer is a single row showing exactly: ↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch view · x projects + right-aligned `? help`; n/r/k/q/paging NOT in the footer.
- Key glyphs render accent.blue, labels text.detail, `?` glyph accent.violet; a 1px border.footer top rule above the row.
- Footer rendered FROM the task 2-1 keymap descriptor (single source of truth) — not a second hand-authored list.
- s switch view + x projects appear on ALL session views incl. Flat.
- Single-row footer height folded into the list size budget so pagination stays exact (with task 2-2 header), verified at every Sessions SetSize site.
- Narrow truncation: row truncates gracefully below width without wrapping/overflow; `? help` survives as long as possible.
- Behaviour parity: display-only swap; omitted keys (k/n/r/q/paging) still dispatch.
- VISUAL VERIFICATION: vhs tape captures Sessions flat footer matching the MV v2 / Light reference.

STATUS: Complete

SPEC CONTEXT:
§3.4 specifies exactly `↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch view · x projects` + right-aligned `? help` over a 1px border.footer rule; glyphs accent.blue, labels text.detail, `?` accent.violet; n/r/k/q/paging are help-only (§8.5); s switch view + x projects on all session views incl. Flat. §12.1 lists the full Sessions keymap; §12.2 de-overloads x (Sessions⟷Projects) and pins s as Sessions-only. The glyph forms (↑↓/⏎/␣) match the §3.4 reference frame and the post task 8-2 glyph switch. Implementation matches the spec verbatim.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/footer.go — full condensed-footer renderer. renderSessionsFooter (64) → renderCondensedFooter (118) shared by Sessions+Projects, driven by sessionsKeymap()/projectsKeymap(). footerTopRule (129) draws the 1px border.footer rule (▔ upper-eighth glyph, distinct from the 2px header border.separator). footerKeyRow (140) splits Core vs right anchor, renders the violet `? help` first, fits the left cluster around it, and assembles the right-anchored row. splitFooterEntries (186) partitions Core/RightAligned. fitLeftCluster (209) implements the §2.7 priority-ordered narrow degrade with ellipsis. renderFooterCluster (261) dot-joins entries with accent.blue glyphs / text.detail labels+separators. renderFooterEntry (280) routes through the shared renderKeyHint primitive.
  - internal/tui/keymap.go — sessionsKeymap() (86) is the single descriptor; Core flags + the sole RightAligned `?` entry drive the footer.
  - internal/tui/model.go — applySessionListSize (1299) folds sessionFooterHeight (1324) into the reserved budget alongside header + notice band; viewSessionList (4106) composes header/list/footer; renderSessionsFooterForFilterState (4117) swaps to the contextual filter / no-match / empty footers (all two rows, height-neutral).
- Notes: The three-column path (renderKeymapFooter / sessionFooterBindings / chunkBindingsIntoThreeColumns) is FULLY removed from the codebase — zero production OR test references remain (only two historical mentions in test comments). The Projects footer was migrated to the SAME descriptor-driven machinery (renderProjectsFooter) rather than left on the legacy path, which is cleaner than the task's "leave Projects on its existing path" minimum and matches the later Phase-3 direction. Token discipline is clean: every colour via theme.MV.* tokens, no literal hex; leaf .Background(canvas) painted on every cell incl. the flex spacer (no terminal-bg island). NO_COLOR carve-out handled via headerStyle/headerCanvasBg.

TESTS:
- Status: Adequate
- Coverage: footer_test.go covers single-row + exact Core key set + right-aligned `? help` (SingleRowCoreKeysWithRightAlignedHelp), token colours in dark+light (TokenColours), the `?`-glyph-is-violet SGR-run assertion (HelpGlyphIsViolet), omission of n/r/k/q/page/Ctrl (OmitsHelpOnlyKeys), descriptor-as-source-of-truth incl. splitFooterEntries partition (SourcedFromKeymapDescriptor), NO_COLOR drop (ColourlessDropsHueAndCanvas), canvas paint (PaintsCanvasNoEdgeBleed), narrow truncation no-wrap across 6 widths (NarrowTruncationNoWrap), priority-ordered drop + ellipsis (NarrowTruncationKeepsHighestPriority), height-folded-into-budget at the construction+resize+rebuild sites (SubtractedFromListBudget, CountedAtEverySizeApplySite), and omitted-keys-still-dispatchable (OmittedKeysStillDispatchable). sessions_footer_switch_view_test.go covers switch view / projects at all session counts incl. zero and on the Flat view explicitly, plus the negative case on Projects/command-pending. keymap_test.go locks the descriptor shape; keymap_dispatch_guard_test.go independently guards descriptor↔dispatch parity for every key (the strongest behaviour-parity backstop).
- Notes: Coverage is comprehensive and well-targeted — not over-tested. The behaviour-parity claim is double-covered (footer_test + dispatch-guard) but each tests a distinct contract (the footer swap vs the descriptor/dispatch link), so this is justified, not redundant. The SGR-run assertion for the violet `?` (locating the last \x1b[ before the `?` byte) is a precise, non-brittle way to pin glyph-specific colour.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(); leaf .Background(canvas) paint mirrors header.go/section_header.go; token-only colours; shared primitives (renderKeyHint, headerStyle, headerCanvasBg, headerPadRight, headerWidthOrFallback) reused rather than re-authored.
- SOLID principles: Good. Clear single-responsibility decomposition (split / fit / cluster / assemble / detail). assembleRightAnchoredRow is correctly factored as the single owner of the right-anchor geometry shared by the standard and contextual filter footers, so the degrade rule lives in one place.
- Complexity: Low. fitLeftCluster's loop is bounded by the fixed ≤6 Core entries; control flow is linear and well-commented.
- Modern idioms: Yes. Pointer-return for the optional right anchor (*keymapEntry), slice prealloc with the right capacity, idiomatic lipgloss joins.
- Readability: Good — arguably above average. Doc comments are dense but accurate (every cross-reference verified: renderFilterCluster, footerKeyLabelGap, helpKeyGlyph all exist and are used as described).
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/footer.go:236 — fitLeftCluster re-renders the cluster from scratch for each prefix length in the narrow-degrade loop (O(n²) glyph renders). n is fixed at ≤6 Core entries and this only runs below the truncation width, so the cost is negligible today; flagging only as a future consideration if the Core set ever grows materially. No action needed now.
