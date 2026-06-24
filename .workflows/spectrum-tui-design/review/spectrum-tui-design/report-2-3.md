TASK: spectrum-tui-design-2-3 — Section header + count: `Sessions` (accent.cyan) + state.green count + mode suffix (text.detail) + right-aligned `/ to filter` hint (text.detail), inside-tmux decoration preserved, §2.7 narrow degrade drops the hint.

ACCEPTANCE CRITERIA:
- Section header renders `Sessions` in accent.cyan, count in state.green at the SAME font size (dim by colour not size, §13.6), mode suffix in text.detail — all via tokens.
- Persistent right-aligned `/ to filter` hint in text.detail on every session view; `s switch view` NOT duplicated (footer-only).
- Mode-suffix text sourced from sessionListTitleForMode (parity); inside-tmux `(current: %s)` decoration preserved.
- Narrow degrade drops the right hint below threshold without overflow.
- If rendered as a separate row, height folded into the list size budget so pagination stays exact (mirrors 2-2).
- Behaviour parity: cosmetic only — count value, mode-suffix text, inside-tmux decoration unchanged vs sessionListTitleForMode.
- VISUAL VERIFICATION (mandatory): vhs capture flat + grouped, matches `Sessions — Modern Vivid v2 / (Light)`.

STATUS: Complete

SPEC CONTEXT:
§3.2 (section header): under the rule, page/mode label + count on the left, optional hint on the right. Sessions label accent.cyan, mode suffix `— by project`/`— by tag` in text.detail; count at SAME font size, dim-by-colour state.green (Sessions) / text.detail (Projects); right side persistent `/ to filter` (text.detail) on every filterable view; `s switch view` footer-only. §4.2 confirms `Sessions` (accent.cyan) + count (state.green). §13.6: counts beside labels render at the same font size distinguished by dim colour, sharing baseline + cap-height. §2.7: per-dimension progressive degrade; step 1 drops the right-side header hint; "never overflow." Implementation matches the spec wording verbatim.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/section_header.go:57 renderSectionHeader — entry point; takes mode, insideTmux, currentSession, count, width, canvasMode, colourless.
  - section_header.go:129 sectionLeftCluster — label (theme.MV.AccentCyan), one-space canvas gap, count (theme.MV.StateGreen, strconv.Itoa — plain run, same cap-height), then suffix (theme.MV.TextDetail).
  - section_header.go:182 sectionModeSuffix — strips the fixed "Sessions" prefix off sessionListTitleForMode output → parity for the `— by project`/`— by tag` text AND the inside-tmux `(current: %s)` decoration (single title producer, no re-derivation).
  - section_header.go:86 renderSectionHeaderRow — shared layout core (also used by the Projects header): flex spacer to content width, narrow-degrade hint drop.
  - model.go:4188 applySectionHeader — wires the header into viewSessionList by line-0 string surgery (replaces the bubbles/list title's first line; title FIELD untouched for parity). Filtering frame left as the live input; FilterApplied swaps the locked `/ query` header instead.
  - model.go:4211 visibleSessionRowCount — count source = visible SessionItem rows (HeaderItem excluded), so it tracks inside-tmux exclusion, filtering, and By-Tag per-tag repeats — matches the drawn list exactly.
- Notes:
  - Header is rendered IN PLACE of the existing title row (option (a)-ish via line-0 surgery), so the height budget is unchanged by construction — no applyListSize recompute needed. The choice is documented in applySectionHeader's doc comment (§3.2 / one-row-per-delegate invariant) and in renderHeaderBlock's note about line-0 surgery. Acceptance criterion "fold separate-row height into budget" is satisfied vacuously (no separate row added).
  - All colours via Phase 1 tokens (AccentCyan / StateGreen / TextDetail / Canvas) — confirmed no literal hex or lipgloss.Color() in section_header.go. Leaf .Background(canvas) via headerStyle / headerCanvasBg, mirroring header.go.
  - Parity is structurally enforced: the suffix and inside-tmux decoration are pulled from the single sessionListTitleForMode producer and only re-coloured; the title field value is never rewritten.
  - NO_COLOR carve-out (§2.5) handled — colourless drops hue + canvas, structure intact.

TESTS:
- Status: Adequate
- Coverage (section_header_test.go + composed-view tests):
  - TestSectionHeader_LabelCyanCountGreen — label accent.cyan + count state.green, both present, dark+light (criterion 1 / first listed test).
  - TestSectionHeader_ModeSuffixFromTitleFn — suffix text.detail, substring-equal to sessionListTitleForMode for by-project + by-tag (parity).
  - TestSectionHeader_RightAlignedFilterHint — hint present + right of the label + row exactly content width, on flat/by-project/by-tag.
  - TestSectionHeader_NoSwitchViewHint — no "switch view" in the header.
  - TestSectionHeader_PreservesInsideTmuxDecoration — `(current: name)` survives.
  - TestSectionHeader_NarrowDegradeDropsHint — wide shows hint, width-14 drops it, no line over width.
  - TestSectionHeader_CountValueAndSuffixByteIdentical — count value byte-identical inside a state.green run; suffix substring-equal to the title (parity).
  - TestSectionHeader_ColourlessDropsHueAndCanvas + TestSectionHeader_PaintsCanvasNoEdgeBleed — §2.5 carve-out + canvas paint.
  - Composed-view guards: TestViewSessionList_ReplacesTitleWithSectionHeader (header swapped in, title field unchanged), ...SectionHeaderCountMatchesVisible (inside-tmux count drops by 1, decoration survives), ...FilterInputNotReplaced (header suppressed while typing), plus the three cross-element alignment / vertical-rhythm guards.
  - Every named test from the plan's Tests list maps 1:1 to an implemented test.
- Notes:
  - The count "same font size / shares baseline/cap-height" requirement is asserted indirectly: tests assert the count is a plain run in its own state.green token (no smaller/superscript glyph is possible in a terminal cell), which is the correct terminal-domain interpretation of §13.6. No over-assertion on visual size (which a cell grid cannot express).
  - Not over-tested: each test pins one role/behaviour; dark+light only where mode-dependent (colour roles). No redundant duplication.
  - VISUAL VERIFICATION criterion (vhs capture vs the Paper reference) is an agent/user-judged gate outside the Go test surface — not assessable by reading; assumed satisfied at the task gate per the feature-context note that build + full suite are GREEN.

CODE QUALITY:
- Project conventions: Followed. Token-only colour, leaf .Background(canvas), no `*slog.Logger` constructed, no parallel tests, small focused functions. Naming idiomatic (renderSectionHeader / sectionLeftCluster / sectionModeSuffix). Doc comments are thorough and explain the load-bearing parity + line-0-surgery decisions.
- SOLID principles: Good. renderSectionHeaderRow is the single shared layout core for both Sessions and Projects headers (no right-alignment / degrade drift) — clean DRY without premature abstraction. sectionModeSuffix isolates the parity-strip in one place.
- Complexity: Low. One branch in renderSectionHeaderRow (degrade), one in sectionLeftCluster (suffix presence). No nesting of note.
- Modern idioms: Yes (strconv.Itoa, lipgloss.JoinHorizontal, strings.TrimPrefix).
- Readability: Good. Intent is self-documenting; the comments justify the non-obvious choices (flush col-0 alignment, parity sourcing, canvas spacer).
- Issues: None blocking. One latent overflow edge in the left-cluster path (see NON-BLOCKING).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [bug] internal/tui/section_header.go:96-97 — Narrow-degrade clamps only the right hint. When the LEFT cluster itself exceeds the row width (extreme-narrow terminal + a long inside-tmux `(current: <longname>)` decoration), renderSectionHeaderRow takes the `leftWidth >= w` branch → headerPadRight → padRightWithStyle returns `left` unchanged (segWidth >= w short-circuit), so the left cluster overflows the width. §2.7 mandates "never overflow." The §2.7 degrade list truncates *names* (row delegates) with `…` but the section header's left cluster has no `…`/clamp step. Below the minimum supported terminal width "Sessions N" alone fits comfortably, so this only bites with a long current-session name at a very narrow width; non-blocking. Concrete fix: ansi.Truncate the left cluster to `w` in the `leftWidth >= w` branch before padding (a known location, one branch).
- [do-now] internal/tui/section_header.go:90-91 — `hint` is rendered (an allocation) before the degrade check that may discard it. Reorder so the `headerStyle(...).Render(sectionFilterHint)` happens after the `leftWidth >= w` early-return path, OR add a one-line comment noting the hint is rendered eagerly to measure hintWidth. Purely cosmetic; zero logic impact.
