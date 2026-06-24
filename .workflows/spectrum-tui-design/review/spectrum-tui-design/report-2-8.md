TASK: spectrum-tui-design-2-8 — Filtering input-active + list-active reskin (accent.orange query + `/` prefix, two contextual footers, flatten-on-filter, two-mode boundary clarity)

ACCEPTANCE CRITERIA:
- `/` opens an inline filter input in the section-header row; query in accent.orange with an accent.orange `/` prefix; list filters live as you type.
- Input-active: cursor at end-of-text, NO list row selected; footer `type to filter · ↵/↓ browse results · esc clear`.
- List-active: locked accent.orange query with no cursor, arrows move selection, ↵ attaches, no background tint; footer `↵ attach · ↑↓ navigate · esc clear filter`.
- Never both an input cursor AND a selected row simultaneously (§7.1).
- ↵ or ↓ commits input-active→list-active; Esc clears the filter from either mode (engine parity).
- Typing flattens grouped views (headings vanish via HeaderItem.FilterValue()=="" — preserved); no match-count shown.
- `s` remains a literal filter character while input-active.
- VISUAL VERIFICATION (mandatory): vhs captures match Filtering — input active (MV) / list-active (MV).
- Behaviour parity: bubbles/list filter engine unchanged.

STATUS: Complete

SPEC CONTEXT:
§7 / §7.1 / §7.2 require the MV reskin of bubbles/list's built-in filter: `/` opens an inline input in the section-header row; query + `/` prefix in accent.orange; two mutually-exclusive modes (input-active = typing, cursor at end, no selected row; list-active = locked cursor-less query, selectable rows, no input bg tint); two contextual footers; ↵/↓ commits input→list, Esc clears from either mode; §5.1 flatten-on-filter is free via HeaderItem.FilterValue()==""; no match-count. The filter engine is explicitly kept as-is (§14.1) — only styling + the two footers + the two-mode boundary clarity change.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:1219 styleFilterInput — restyles bubbles/list FilterInput: accent.orange `/ ` prompt + accent.orange query text + accent.orange non-blinking block cursor (input-active); NO .Background(canvas) leaf (no input tint, §7.1); colourless carve-out strips hues. Wired through applyCanvasMode (model.go:1130) which is re-applied on every canvas-gate transition (syncResolvedMode), so styling is in place before first paint.
  - internal/tui/section_header.go:155/169 filterPromptPrefix ("/ ") + renderFilterQueryHeader — the list-active LOCKED accent.orange `/ query` header (no cursor, no bg tint), single source of prompt wording shared by both modes.
  - internal/tui/model.go:4171 applySectionHeader — leaves first line untouched while Filtering (live input owns it); swaps in the locked-query header while FilterApplied. Projects mirror at model.go:3930 applyProjectsSectionHeader.
  - internal/tui/filter_footer.go — the two contextual footers (filteringFooterEntries / filterAppliedFooterEntries) + Projects variant (projectsFilterAppliedFooterEntries: `↵ new session` not `attach`). Per-glyph colours: accent.orange action word (`type` / `esc`), accent.blue nav glyphs, text.detail labels + separators. Reuses the shared right-anchored footer assembler (assembleRightAnchoredRow) so structure is byte-consistent with the §3.4 standard footer including the right-aligned `? help`.
  - internal/tui/model.go:4117 renderSessionsFooterForFilterState / model.go:3973 renderProjectsFooterForFilterState — mode-driven footer selection (Filtering→input-active, FilterApplied→list-active, default→standard).
  - Boundary (↵/↓ commit, Esc clear) is bubbles/list's built-in state machine — not reimplemented. model.go:3003 Esc progressive-back defers to the list while FilterApplied; model.go:2959 SettingFilter() guard keeps `s`/`?` literal while typing (the `s` case at model.go:3020 sits below the guard).
- Notes: No literal hex; all colours via §2.9 tokens (theme.MV.*). Cursor blink disabled for deterministic capture. No engine logic touched — the change is provably cosmetic + footer rendering.

TESTS:
- Status: Adequate
- Coverage (internal/tui/filtering_reskin_test.go): every acceptance criterion + every listed edge case has a dedicated test, both modes (Dark/Light) where colour-relevant:
  - InputActiveQueryOrange (orange `/`+query, exact mode-resolved SGR via tokenFgSeq)
  - InputActiveNoRowSelected (no bg.selection SGR anywhere + no selector bar on any fab* row — the never-both invariant)
  - InputActiveFooter + InputActiveFooterColours (copy + per-word orange/blue/detail SGR; asserts standard footer is replaced)
  - ListActiveLockedQueryOrange (locked `/ fab` orange, via renderFilterQueryHeader)
  - ListActiveSelectedRowNoInputTint (row IS selected via bg.selection present in frame, BUT the query header carries no bg.selection — the two-knob check)
  - ListActiveFooter + ListActiveFooterClearIsOrange
  - EnterOrDownCommitsInputToList (table: both keys → FilterApplied)
  - EscClearsFromEitherMode (both modes → Unfiltered)
  - TypingFlattensGroupedView (no HeaderItem in VisibleItems after a query; precondition-guarded)
  - SLiteralWhileInputActive (mode unchanged + `s` appended to FilterValue)
  - NoMatchCountShown (neither "filtered"/"matched" in either mode)
  Token assertions use exact SGR sequences (tokenFgSeq / selectionBgParams), not glyph-presence — a token swap is caught. The async bubbles/list FilterMatchesMsg is drained (drainFilterCmd) so the filtered visible set actually settles, matching how vhs drives keystrokes.
- Notes: Not over-tested — each test pins one distinct contract. Not under-tested — both modes covered, the never-both invariant tested from both directions, engine transitions verified (not reimplemented). VISUAL: both tapes (filtering-input-active.tape / filtering-list-active.tape) drive the correct states offline via capturetool (no tmux/no ~/.config touch); committed Paper references exist (reference/filtering-{input,list}-active-mv.png). Captures match the references for layout/structure/colour-role: orange query + cursor + no-row-selected (input-active); locked orange query + violet bar + bg.selection tint + untinted query header (list-active); correct footers with orange action word, blue glyphs, right-aligned `? help`.

CODE QUALITY:
- Project conventions: Followed — token-only colour (no call-site hex), leaf .Background(canvas) discipline (deliberately omitted for the filter input per §7.1, documented in-source), no t.Parallel(), small focused helpers, thorough doc comments tying each function to its spec section.
- SOLID principles: Good — single-source prompt wording (filterPromptPrefix), structural BrowseResults membership flag (no display-copy matching), shared right-anchored row assembler reused across all footer variants, Projects/Sessions footer copy correctly differentiated without leakage.
- Complexity: Low — flat helpers, mode resolved by a single switch per page.
- Modern idioms: Yes.
- Readability: Good — self-documenting; the input-active-vs-list-active split and the no-tint rationale are spelled out.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
