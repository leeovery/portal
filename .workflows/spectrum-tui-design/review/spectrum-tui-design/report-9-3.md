TASK: spectrum-tui-design-9-3 — Make SessionDelegate.canvasBg / tokenStyle delegate to the canonical header leaf-style helpers (headerCanvasBg / headerStyle)

ACCEPTANCE CRITERIA:
- SessionDelegate.canvasBg is a single delegation to headerCanvasBg(d.Mode, d.Colourless) with no re-stated colourless/canvas-resolution branch.
- SessionDelegate.tokenStyle delegates to headerStyle(...) with the caller-supplied base composited, no re-stated colourless branch.
- The colourless-fallback and canvas-resolution logic for these two rules exists ONLY in the header leaf helpers.
- Existing session-row render/golden tests pass with byte-identical canvas and colourless output; go build and go test ./... pass.

STATUS: Complete

SPEC CONTEXT:
This is a Phase 9 analysis-cycle DRY-delegation chore (no spec behaviour change). Spec §1 (canvas leaf paint) requires every emitted run to carry the role-token foreground over a Background(canvas) for the resolved Mode, with the §2.5 NO_COLOR carve-out dropping both hue and canvas to render on the terminal's native fg/bg. §4.1 governs the session-row anatomy. The canonical leaf-paint rules already live in header.go (headerStyle, headerCanvasBg); the delegate's canvasBg/tokenStyle were a verbatim fork of those rules, the one pair that re-derived logic the codebase otherwise centralises (loadingStyle/loadingFg, rowBg/rowToken all already delegate). The task collapses that fork.

IMPLEMENTATION:
- Status: Implemented (clean, no-behaviour-change collapse)
- Location:
  - internal/tui/session_item.go:201-203 — canvasBg now returns headerCanvasBg(d.Mode, d.Colourless), single delegation, no re-stated branch.
  - internal/tui/session_item.go:223-225 — tokenStyle now returns headerStyle(fg, d.Mode, d.Colourless).Inherit(base), no re-stated colourless branch.
  - internal/tui/header.go:87-104 — headerCanvasBg / headerStyle are the sole owners of the colourless-fallback + canvas-resolution logic.
- Notes:
  - Both criterion-1 and criterion-2 met exactly: each method is a one-line delegation; the `if colourless { return NewStyle() }` / `Background(theme.MV.Canvas...)` branches exist ONLY in header.go (verified by grep — no residual duplicate of the rule in session_item.go).
  - .Inherit(base) composition is semantically correct. Under NO_COLOR, headerStyle returns NewStyle(); NewStyle().Inherit(base) yields base's fields verbatim, matching the original colourless branch's `return base`. Under canvas, headerStyle sets fg+bg and .Inherit(base) pulls base's unset-on-header fields (Bold), matching the original `base.Foreground(...).Background(...)`. The only bases passed at the two call sites (session_item.go:263-264, lipgloss.Style{}) and via renderSessionRow (nameBase = Bold, empty) never set fg/bg, so there is no Inherit precedence conflict.
  - Doc comments (session_item.go:194-200, 205-222) were rewritten to describe the delegation and explicitly cross-reference the sibling pattern (rowBg/rowBgStyle, rowToken/rowTokenStyle, loadingStyle/loadingFg). The reference patterns named in the task were confirmed present: loading_view.go:251 (loadingStyle→headerCanvasBg), loading_view.go:259 (loadingFg→headerStyle), session_item.go:341 (rowBg→rowBgStyle), session_item.go:348 (rowToken→rowTokenStyle).
  - No drift: the two call sites of canvasBg/tokenStyle (HeaderItem render path) are unchanged; signatures preserved.

TESTS:
- Status: Adequate (well-balanced)
- Coverage: internal/tui/session_style_consolidation_test.go provides three layers:
  1. TestSessionCanvasBg_DelegatesToHeaderCanvasBg — pins canvasBg ≡ headerCanvasBg AND ≡ pre-refactor inline body (preCanvasBg), across both modes × colourless on/off.
  2. TestSessionTokenStyle_DelegatesToHeaderStyle — pins tokenStyle ≡ headerStyle(...).Inherit(base) AND ≡ pre-refactor inline body (preTokenStyle), across 4 representative tokens × both modes × colourless on/off × {empty base, Bold base}. This is the critical test: it directly proves the .Inherit composition reproduces the original base-then-fg/bg layering, including the load-bearing NO_COLOR-returns-base case.
  3. TestSessionDelegateRender_ByteIdenticalAcrossConsolidation — full-delegate-render byte-for-byte goldens for a HeaderItem + grouped session row (selected + unselected) at width 80, captured from the PRE-refactor source, across dark/light/NO_COLOR. This is the acceptance-criterion proof (byte-identical canvas + colourless, grouped rows).
- Notes:
  - The pre-refactor golden anchors (preCanvasBg, preTokenStyle, sessionStyleGoldens map) are exactly the right technique for a no-behaviour-change refactor — they freeze the original output independent of the now-shared source, so the test would fail if the header helpers ever drift the rule. Not over-tested: each test targets a distinct layer (helper equivalence, base composition, full render); the grouped goldens cover both the GroupKey-set indent path and the catch-all/header path. No redundant assertions.
  - Acceptance criterion "grouped + flat" — the render test exercises grouped rows (GroupKey set) explicitly. Flat-row coverage for these two helpers is implicitly equivalent (flat differs only by the indent cell, which routes through the same canvasBg). The helper-level tests (#1, #2) are mode/colourless-exhaustive and base-agnostic, so flat is covered at the source. Acceptable.
  - No t.Parallel() — correct per the project rule (shared canvas/package-level state).

CODE QUALITY:
- Project conventions: Followed. Idiomatic Go, no t.Parallel(), table-driven exhaustive tests, the established delegate-to-free-function/leaf-helper pattern that the package already uses (rowBg/rowToken, loadingStyle/loadingFg). Single source of truth for the §1/§2.5 leaf-paint carve-out now strictly enforced.
- SOLID principles: Good. Strengthens single-responsibility — the colourless/canvas-resolution decision is now owned solely by header.go; the delegate is a thin binding of Mode/Colourless.
- Complexity: Low. Two one-line method bodies; net reduction in branching.
- Modern idioms: Yes. .Inherit composition is the correct lipgloss idiom for layering a caller-supplied base under a computed leaf style.
- Readability: Good. Doc comments are thorough and now correctly describe the delegation + name the mirrored sibling patterns; the Inherit-precedence reasoning is documented inline (session_item.go:216-222).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
