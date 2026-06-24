TASK: spectrum-tui-design-2-10 (tick-4b2180) — Global content gutter (canvas inset) across all pages

ACCEPTANCE CRITERIA:
- A single global content inset (canvas gutter) of 2 cells L/R + 1 row T/B applied to every page; inset cells painted the owned canvas (native bg under NO_COLOR).
- Inset folded into width AND height budgets at every SetSize site; one-row-per-delegate invariant holds (no overflow; grouped re-verified).
- Content no longer flush to the edges; matches the design inset.
- VISUAL: regenerated captures (dark/light/nocolor) show the gutter.
- Behaviour parity: only the composition inset changes; nav/selection/filter/dispatch identical.
- Zero/tiny terminal: inset clamps to 0 rather than negative region or overflow.

STATUS: Complete

SPEC CONTEXT:
§1 (Canvas ownership) — Portal owns a mode-matched canvas painted as an outer-layer full-terminal fill; the fill is the LAST layer wrapping the composed view and must NOT perturb the one-row-per-delegate pagination invariant (§3.5). §3.6 — no full-screen frame; the canvas is a flat fill, not a box. §2.5 — under NO_COLOR Portal paints no canvas (terminal native bg). This task is a UX refinement extending the 1-6 outer fill into a content inset (34px/30px Paper frame padding → 2 cells L/R, 1 row T/B), preserving exact pagination.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:3313-3316 — Hinset=2 / Vinset=1 constants, pinned with a comment tying them to the design 34px/30px padding (AC: pin cell values).
  - internal/tui/model.go:3345-3350 — insetRegion(dim, inset): single content-region derivation, clamps inset to 0 when dim <= 2*inset (AC: clamp, no negative region).
  - internal/tui/model.go:3357-3360 / 3382-3385 — contentWidth()/contentHeight(): 80x24 zero/unset fallback first (via termDims), then insetRegion.
  - internal/tui/model.go:3418-3451 — fillCanvas: single outer wrap; composes view into contentW×contentH, then insetCanvasCanvas places it at (Hinset,Vinset) with canvas-painted gutter; colourless path delegates to fillColourless + insetColourless (no bg SGR).
  - internal/tui/model.go:3458-3556 — gutterPadding (even L/R, T/B split — symmetric, exactly Hinset/Vinset per side when unclamped), insetCanvasCanvas (coloured), insetColourless (NO_COLOR native bg).
  - Budget fold-in at every SetSize site: applySessionListSize/applyProjectListSize are always called with m.contentWidth(), m.contentHeight() — NewModelWithSessions:1264-1265, WindowSizeMsg handler:1941-1942, rebuild paths:1572 / 1780, ProjectsLoaded:2086. All list SetSize routes through the single applyListSize core (model.go:1279). Preview WindowSizeMsg rewritten to inset dims via insetWindowSizeMsg (model.go:2222). Modal-on-cleared-canvas renderers compose into contentWidth/contentHeight (model.go:3833-3844, 4025-4036).
  - View() (model.go:3265) is the single wrap point for all pages; blankFrame pre-detection path (3294) correctly has no inset (no content yet — sound exception).
- Notes: insetRegion is the single source of truth shared by both the budget computation and the fillCanvas placement, so budget and placement can never drift. Symmetric gutter split means leadingGutterCells == Hinset exactly, matching the test contract. No SetSize site bypasses the inset.

TESTS:
- Status: Adequate
- Coverage (internal/tui/content_inset_test.go, 451 lines): every AC has a focused test —
  - inset applied to Sessions / content not flush (TestContentInset_AppliedToSessions).
  - frame dims unchanged / outer fill owns full terminal (TestContentInset_FrameDimensionsUnchanged).
  - gutter painted canvas (TestContentInset_GutterPaintedCanvas) and NO_COLOR native bg / no bg SGR (TestContentInset_NoColorGutterNativeBg).
  - inset folded into width AND height budgets (TestContentInset_FoldedIntoBudgets).
  - pagination invariant under inset — flat (TestContentInset_PaginationInvariantPreserved, w90 h14, 40 sessions) and grouped By-Project (TestContentInset_GroupedPaginationInvariant).
  - composes on Projects + Loading (TestContentInset_AppliesOnProjectsPreviewLoading); Preview inset verified via TestModelViewRoutesPagePreviewToPreviewModel (reads contentRows[Vinset]).
  - clamp to 0 at tiny terminal (TestContentInset_ClampsAtTinyTerminal + pure-function TestInsetRegion_ClampBoundary with boundary cases dim==2*inset).
  - small-but-viable terminals stay clean (TestContentInset_ClampHoldsWhereContentFits).
  - zero-size 80x24-then-inset fallback (TestContentInset_ZeroSizeFallback).
  - behaviour parity / nav unchanged (TestContentInset_NavigationUnchanged).
  - Budget-fold updates to existing tests (model_test.go, pagepreview_view_routing_test.go) tighten assertions to the inset rather than weakening them.
  - Captures regenerated dark/light/nocolor (sessions-flat.png, sessions-flat-light.png, trail/.../2-10.png ×3). Verified by reading: dark + nocolor captures show the gutter and content no longer flush; nocolor shows no canvas paint.
- Notes: Coverage is balanced — not over-tested (each test targets a distinct AC), not under-tested (clamp boundaries, grouped pagination, NO_COLOR, zero-size all covered).

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(); table-driven boundary tests (golang-testing); small single-purpose helpers; exported constants documented; no raw hex (canvas via theme token).
- SOLID principles: Good. insetRegion is a single-responsibility pure function; gutterPadding factors the shared coloured/colourless geometry so the two paths cannot drift; fillCanvas remains the single composition chokepoint.
- Complexity: Low. Linear per-line loops; clear clamp branch; no nesting beyond one level.
- Modern idioms: Yes. Pre-sized slices (make([]string, 0, h)), strings.Builder in SGR strip, single reused ansi.Parser per frame.
- Readability: Good. Comments tie cell values to the design spec and explain the budget-fold rationale at each site.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/content_inset_test.go:266-286 — TestContentInset_AppliesOnProjectsPreviewLoading is named "...PreviewLoading" and its doc-comment claims it covers "Projects, Preview, and Loading", but it has only "projects" and "loading" subtests — Preview is not exercised here (it is covered separately by TestModelViewRoutesPagePreviewToPreviewModel). Rename the test to ...ProjectsLoading (or add an explicit "preview" subtest via assertFramedAndInset) and align the doc-comment so the name does not overstate coverage.
