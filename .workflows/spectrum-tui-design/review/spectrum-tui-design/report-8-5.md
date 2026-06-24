TASK: spectrum-tui-design-8-5 — Point loading_view.go at the shared header.go leaf canvas-style helpers (DRY consolidation chore, tick-469269)

ACCEPTANCE CRITERIA:
- loading_view.go no longer contains an independent re-implementation of the header leaf-paint pair; it routes through headerStyle/headerCanvasBg (directly or via thin delegating aliases).
- The leaf canvas-paint rule exists in exactly one authoritative source (header.go).
- The loading screen renders identically (light / dark / NO_COLOR) to current output.
- Existing loading-view render tests pass with byte-identical output across light, dark, NO_COLOR.
- If aliases are kept, a test (or compile-level check) confirms they delegate to the header helpers rather than re-implementing.

STATUS: Complete

SPEC CONTEXT:
§1 (Canvas ownership) defines the two-layer canvas paint: leaf styles carry .Background(canvas) on every text/accent run, and the NO_COLOR carve-out (§2.5) drops all hue + canvas, rendering on the terminal's native fg/bg. §10.3 specifies the honest loading screen (centred PORTAL wordmark + caret, thick violet/bg.track bar, 5-row tick-list) — every glyph carries the owned canvas, bare under NO_COLOR. The header.go pair (headerStyle = role-token fg over Background(canvas), bare under NO_COLOR; headerCanvasBg = Background(canvas)-only, bare under NO_COLOR) is the canonical leaf-paint rule that all chrome surfaces (header, footer, section header, notice band, modals) route through. loading_view.go was the lone render file that forked its own copy because the §10 loading-phase executor did not see the §3 header helpers.

IMPLEMENTATION:
- Status: Implemented (alias-retention variant — sanctioned by the task as option 2)
- Location: internal/tui/loading_view.go:251-253 (loadingStyle) and :259-261 (loadingFg); canonical source internal/tui/header.go:87-94 (headerStyle), :99-104 (headerCanvasBg).
- Notes: Both forked bodies are now genuine one-line delegating aliases:
    loadingStyle(mode, colourless) -> headerCanvasBg(mode, colourless)
    loadingFg(fg, mode, colourless) -> headerStyle(fg, mode, colourless)
  No residual duplicate logic remains in either body — the colourless carve-out and the Foreground/Background(canvas) composition exist only in header.go. The 14 call sites across loading_view.go (renderSectionGap, renderBlockWordmark, renderSingleRowWordmark, renderLoadingBar, renderErrorFooter, renderTickRow) are unchanged in form, retaining the terse local names, which is the explicitly-permitted alias path ("if the loading code wants its own terse names, make them one-line aliases that delegate"). The delegation mirrors the established SessionDelegate.rowBg/rowToken -> rowBgStyle/rowTokenStyle precedent and the doc comments at :246-250 / :255-258 cite that precedent.
- Single-source verification: A package-wide sweep for the leaf-paint pair shows the role-token-fg-over-canvas rule and the canvas-bg-only rule now live solely in header.go for the header/loading lineage. Other Background(theme.MV.Canvas...) usages are distinct surface concerns and out of scope for this task: session_item.go:299/:317 (rowBgStyle/tokenStyle — the list-row leaf-paint family, its own consolidation lineage with session_style_consolidation_test.go, and explicitly handled by the SEPARATE later task 9-3 per planning.md:212), model.go:1284/:3430 and edit_modal.go:171 (outer-fill / pagination-row / modal canvas — different roles). The 8-5 scope is precisely the loading-vs-header forked pair, and it is collapsed.

TESTS:
- Status: Adequate
- Coverage: internal/tui/loading_style_consolidation_test.go adds three layers of protection:
    1. TestLoadingFg_DelegatesToHeaderStyle — renders a probe across 4 tokens x 2 modes x colourless on/off, asserting loadingFg == headerStyle AND headerStyle == a verbatim pre-refactor inline reproduction (preLoadingFg). The two-way assertion catches both "alias drifted from header" and "header drifted from the original behaviour".
    2. TestLoadingStyle_DelegatesToHeaderCanvasBg — same shape for loadingStyle == headerCanvasBg == preLoadingStyle across 2 modes x colourless on/off.
    3. TestLoadingScreen_ByteIdenticalAcrossConsolidation — full-screen 80x24 goldens for both the mid-restore frame and the §10.5 error frame, across dark/light x colourless on/off (6 goldens per frame name). The goldens were captured from the PRE-refactor forked source, so any drift in canvas composition or the NO_COLOR carve-out is caught byte-for-byte. This directly discharges the "renders byte-identical in light/dark/NO_COLOR" criterion.
  The NO_COLOR criterion is covered by the colourless=true arm in all three tests (the spec's NO_COLOR carve-out maps to the colourless bool). The error-frame golden exercises StateRed/TextFaint tokens and the spacer/message/hint footer path through the aliases.
- Notes: Well-calibrated — not over-tested (the helper-level probes and the full-screen goldens cover different failure classes: composition drift vs. layout drift) and not under-tested (both helpers, both frames, both modes, both colour states). The pre-refactor reproductions (preLoadingFg/preLoadingStyle) are the right golden technique for a pure-delegation refactor: they pin behaviour to the original source rather than trusting the new header helper transitively. The goldens would fail if the delegation were broken or if header.go's leaf rule changed without intent.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (correct — the test file documents the shared-canvas-helper parallelism hazard at the package level). Delegation pattern matches the codebase-established rowBg/rowToken precedent. Standard Go, no new dependencies.
- SOLID principles: Good. The refactor improves SRP/DRY — the leaf canvas-paint rule now has one owner (header.go) and the NO_COLOR carve-out + canvas composition change once.
- Complexity: Low. Two one-line aliases; no added branches.
- Modern idioms: Yes. Idiomatic thin-wrapper delegation.
- Readability: Good. The alias doc comments (:246-250, :255-258) explain WHY the names are retained and cite the mirrored SessionDelegate precedent, so a future reader understands these are intentional terse aliases, not dead forks.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (The retained terse aliases are a sanctioned task option, fully documented and tested; no concrete improving change is warranted. The remaining inline canvas-paint sites in session_item.go are correctly deferred to task 9-3, not a 8-5 finding.)
