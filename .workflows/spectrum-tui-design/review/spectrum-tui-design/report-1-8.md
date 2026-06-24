TASK: spectrum-tui-design-1-8 — NO_COLOR carve-out: skip detection, suppress canvas, colourless native fg/bg path

ACCEPTANCE CRITERIA:
- (VISUAL) NO_COLOR=1 vhs capture of Sessions shows no painted canvas (terminal native bg), no colour, state carried by glyphs (●/▌/spaced headers) + bold/dim — structurally matching the foundation Sessions layout, legible by construction.
- Under NO_COLOR, OSC 11 detection and its first-paint wait are skipped entirely.
- The outer full-terminal fill suppresses the canvas background and the leaf styles drop .Background(canvas) under NO_COLOR.
- State stays glyph-distinct without colour (works for free off §2.2 glyph-backed state).
- The colourless decision is a single carve-out flag every canvas-dependent surface can inherit (not re-derived per surface).
- Behaviour parity: NO_COLOR changes only rendering; navigation/selection/filter/key behaviour is identical.

STATUS: Complete

SPEC CONTEXT:
§2.5 — Portal honours NO_COLOR: renders colourless on the terminal's NATIVE fg/bg (paints NO canvas at all — the one documented carve-out to the single owned-canvas render path of §1), leaning on glyph-backed state (§2.2) + bold/dim. §2.6 — NO_COLOR skips light/dark detection and its first-paint wait (no canvas to select). §1 — opaque-only v1, NO_COLOR the one carve-out. §2.2 — state never carried by hue alone. The carve-out must apply to EVERY canvas-dependent surface (modal blank-screen, notice bands, preview chrome), proven here only on the foundation Sessions screen.

IMPLEMENTATION:
- Status: Implemented (clean, well-scoped).
- Location:
  - cmd layer single read: cmd/open.go:369 noColorEnabled() (os.LookupEnv present+non-empty, no-color.org convention), threaded at cmd/open.go:553 → tuiConfig.noColor → cmd/open.go:403 Deps.NoColor; capturetool mirrors it at cmd/capturetool/main.go:126.
  - Single inheritable flag: internal/tui/build.go:84 Deps.NoColor → build.go:141 WithColourless → internal/tui/model.go:270 Model.colourless. Set once at construction; every canvas-dependent surface reads m.colourless.
  - Gate skip: internal/tui/appearance_gate.go:106 newColourlessGate() (constructed resolved + unarmable); model.go:1069 selects it FIRST (wins over appearance pin/auto). arm() is a no-op when colourless (appearance_gate.go:117).
  - OSC 11 + first-paint-wait skip: model.go:1875 nils the RequestBackgroundColor cmd under colourless; timeoutCmd() returns nil for the resolved gate; modeResolved() is true at construction so View never holds the blank frame.
  - Canvas suppression (both layers): View() sets no BackgroundColor under colourless (model.go:3271); fillCanvas → fillColourless + insetColourless emit plain spaces with NO background SGR and no mid-line backfill (model.go:3426, 3510, 3538). Leaf layer drops .Background(canvas): rowBgStyle/rowTokenStyle return bare styles (session_item.go:292,309), colourlessHelpStyles/colourlessPaginationDots strip bg (model.go:894,931), the bubbles/list Title box + TitleBar bg unset (model.go:1148,1154), filter input hue/cursor stripped (model.go:1222).
- Notes: Foreground hue is stripped FREE by the Bubble Tea v2 writer layer (colorprofile honours NO_COLOR, verified intact in 1-2); the colourless branches additionally pin hue-free styles so no accent SGR is emitted at all (belt-and-braces, also drops the bg lipgloss would otherwise still emit). The single-flag design held up: later phases (footers, modals, preview, notice bands) all consume m.colourless rather than re-deriving NO_COLOR — the inherit-once criterion is demonstrably satisfied across the full feature, not just Sessions.

TESTS:
- Status: Adequate (well-balanced, one test per acceptance criterion + edges; no redundancy).
- Coverage:
  - internal/tui/colourless_nocolor_test.go: SingleFlagFromDeps (single inheritable flag, on/off), SkipsDetectionAndFirstPaintWait (resolved at construction; Init issues no timeout tick + no BackgroundColorMsg query), ViewSetsNoBackgroundColor, FillEmitsNoCanvasBackground (frameHasAnyBackgroundSGR scanner asserts NO bg SGR anywhere + neither dark nor light canvas sequence present), StateStaysGlyphDistinct (● attached + ▌ selector + names present), StructureMatchesColouredFrame (exact termH rows, every line padded to termW, Sessions header), NavigationParity, FilterParity (applied filter result set identical to the coloured model AND genuinely narrowed), ColouredPathUnaffected (additive carve-out leaves the coloured path painting).
  - cmd/open_nocolor_test.go: TestNoColorEnabled pins the convention truth table (unset / set-empty / "1" / "true" / "0"); TestBuildTUIModel_NoColorSuppressesCanvas proves the flag flows cmd → tui.Deps → model (suppressed vs painted).
  - frameHasAnyBackgroundSGR / sgrBackgroundActive is a robust full-frame SGR scanner that consumes extended-colour runs whole (so a channel value equal to a bg code can't be misread) — strong guard against any leaked canvas bg, including the bubbles/list default Title violet box.
- Notes: The FillEmitsNoCanvasBackground assertion (no bg SGR anywhere in the frame) is the load-bearing test for the "suppress canvas in both layers" criterion and is correctly stringent. Visual criterion is satisfied by the committed testdata/vhs/sessions-flat-nocolor.png (reviewed): native bg, monochrome, ●/▌ glyphs present, structure matches testdata/vhs/sessions-flat.png. Would all fail if the feature broke.

CODE QUALITY:
- Project conventions: Followed. internal/tui stays env-free (NO_COLOR read only at the two cmd entry points); single colour decision centralised; tests do not use t.Parallel (cmd env-mutating test explicitly notes this); colourless leaf-style decisions homed in shared free functions (rowBgStyle/rowTokenStyle) so the role lives in one place for both delegates.
- SOLID principles: Good. The appearanceGate owns the single-resolution race; the colourless carve-out is a property of the gate (constructed resolved + unarmable) rather than a parallel code path — pin and NO_COLOR converge on the same "already resolved, no wait" shape for different reasons, cleanly documented.
- Complexity: Low. The colourless branches are early-return guards (View, fillCanvas, applyCanvasMode, styleFilterInput, rowBgStyle, rowTokenStyle); fillColourless/insetColourless mirror the coloured fillCanvas/insetCanvasCanvas geometry exactly so layout parity is structural, not coincidental.
- Modern idioms: Yes. os.LookupEnv for present-vs-empty; lipgloss.NoColor{} for the hue-free cursor; one reused ansi.Parser per frame.
- Readability: Good. Doc comments are thorough and spec-anchored; the negatively-named gate.pending flag is explained at its declaration.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] cmd/capturetool/main.go:126 — the NO_COLOR present+non-empty convention is duplicated here from cmd/open.go:369 noColorEnabled(). Reuse would require a shared exported helper (e.g. a small internal/... env util) since the two live in different packages and noColorEnabled is unexported in package cmd. Decide whether a shared helper is worth introducing for a two-line convention or whether the duplication is acceptable for the test-only capture path. Low priority.
