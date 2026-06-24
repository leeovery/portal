TASK: spectrum-tui-design-9-2 — Remove the unconsumed bubbles/list help-styling layer (l.Help.Styles.* writes) and correct its stale doc comment

ACCEPTANCE CRITERIA (from tick-4efae6):
- No l.Help.Styles.* assignment exists anywhere in internal/tui (repo-wide grep returns no matches in the render/construction path).
- canvasHelpStyles/colourlessHelpStyles retain only the load-bearing l.Styles.HelpStyle background and pagination/delegate styling.
- brightenHelpStyles is either removed (if fully emptied) or contains only still-needed work; no dead body remains.
- The model.go:1023-1029 comment names renderCondensedFooter and contains no help.Model.FullHelpView claim.
- The dark-only Token.Color() method (theme.go:60-62) is deleted if it has no remaining callers; otherwise retained with documented callers.
- Existing footer/golden tests pass with unchanged canvas and colourless footer output; go build and go test ./... pass.

STATUS: Issues Found (one non-blocking stale-doc finding; all code-level acceptance criteria met)

SPEC CONTEXT:
- §14.1 keeps bubbles/list as the engine (list model, pagination dots, filtering, cursor/nav) — this is why the pagination-dot styling (canvasPaginationDots/colourlessPaginationDots) is correctly RETAINED while the never-rendered help styling is dropped.
- §14.4 establishes the per-page keymap descriptor as the single source of truth driving footer + help, rendered by the descriptor-driven renderCondensedFooter (footer.go), not the list's built-in help.Model. §3.4 covers the condensed keymap footer. The dead l.Help.Styles.* layer was orthogonal to the live footer path; removing it has no rendered-output effect.

IMPLEMENTATION:
- Status: Implemented (correct, matches acceptance) — with one residual stale doc reference (below).
- Location:
  - internal/tui/model.go:884-887 — canvasHelpStyles now writes ONLY l.Styles.HelpStyle.Background(canvas). No l.Help.Styles.* writes.
  - internal/tui/model.go:894-896 — colourlessHelpStyles now writes ONLY l.Styles.HelpStyle.UnsetBackground(). No l.Help.Styles.* writes.
  - internal/tui/model.go:898-937 — canvasPaginationDots/colourlessPaginationDots retained (load-bearing per §14.1), untouched.
  - internal/tui/model.go:980-985 + 1020-1023 — newSessionList / newProjectList doc comments rewritten to name renderCondensedFooter over the §12.1 keymapEntry descriptors; both explicitly state "nothing in the render path consumes the list's own help.Model" / "not via the list's own help.Model". No help.Model.FullHelpView claim survives anywhere.
  - brightenHelpStyles: REMOVED entirely (grep for the symbol returns zero matches in internal/tui) — pagination-dots concern is independently handled by canvas/colourlessPaginationDots, so the function was fully emptied and deleted along with its call sites.
  - internal/tui/theme/theme.go:49-54 — only ColorFor(m Mode) remains on Token; the dark-only Color() convenience method is DELETED (grep "func (t Token)" shows only ColorFor; repo-wide grep for ".Color()" callers in internal/ returns none outside ColorFor/stdlib color.).
- Verification of "no l.Help.Styles.*": repo-wide grep for `l.Help.Styles` in internal/tui returns a SINGLE hit at model.go:985 — and that is the rewritten DOC COMMENT ("so l.Help.Styles.* is never populated"), not an assignment. Zero assignment-path matches. Criterion met.
- Notes: The change is a pure dead-layer removal + comment correction; it does not touch the live render path, so footer output is byte-identical by construction (no rendered-style code was altered).

DANGLING DOC REFERENCE (the 6-4-flagged item — CONFIRMED PRESENT):
- internal/tui/theme/theme.go:10-11 — the PACKAGE doc comment still reads:
  "...renderers resolve each token per mode via ColorFor(mode). The dark-pinned Color() convenience survives only for the handful of not-yet-mode-resolved call sites."
  Token.Color() no longer exists (deleted by this task), so this sentence references a method that is gone — a stale/dangling doc reference. It does not break the build (doc comments are inert) and footer rendering is unaffected, hence non-blocking. The acceptance criterion "retire the method" is met; the criterion did not enumerate the package-doc sweep, so this is a residue, not a regression of an explicit criterion. Should be corrected (sibling task 6-4 surfaced the same line).

TESTS:
- Status: Adequate.
- Coverage: This is a dead-code-removal chore whose acceptance is "live render output unchanged." The existing footer tests directly back that contract by exercising the LIVE descriptor-driven path (the path that survives), not the removed dead layer:
  - internal/tui/footer_test.go — Sessions condensed footer: single-row Core keys + right-aligned ? help, glyph forms (↑↓, ⏎), dark token colours.
  - internal/tui/projects_footer_test.go — Projects condensed copy exact; TokenColours (canvas/dark); ColourlessDropsHueAndCanvas (NO_COLOR carve-out: no canvas bg SGR, no fg role SGR); filter-applied copy; narrow-degrade help anchor.
  - internal/tui/sessions_footer_switch_view_test.go, modal_footer_test.go, pagination_dots_test.go — adjacent footer/pagination surfaces.
  These cover Sessions and Projects × canvas and colourless, which is exactly the "footer byte-identical" acceptance surface. Because the removed l.Help.Styles.* layer was never rendered, no test could have covered it directly, and none should — adding a test asserting "the dead struct is empty" would be testing an implementation detail. Correct to rely on the live-path footer/golden tests as the regression guard.
- Notes: No under-test (the surviving render path is well covered) and no over-test introduced by this task. The Token.Color() removal is guarded by compilation itself (go build would fail on a dangling call) — no dedicated test needed or warranted.

CODE QUALITY:
- Project conventions: Followed. Token resolution goes through ColorFor(mode) (per the theme.go centralisation rule — no raw hex at call sites); the two retained help-style helpers and the two pagination-dot helpers keep the canvas/colourless symmetry the package uses throughout. No t.Parallel() concerns (no test changes).
- SOLID principles: Good. canvasHelpStyles/colourlessHelpStyles are now single-responsibility (background only); pagination-dot concern correctly lives in its own pair of functions. Removing brightenHelpStyles eliminated a function with no live responsibility.
- Complexity: Low. Net reduction in surface area (one function deleted, two trimmed to one statement each, one method deleted).
- Modern idioms: Yes. Lipgloss v2 ColorFor(mode) used throughout; no AdaptiveColor reliance (correctly avoided per §14.5).
- Readability: Good. The rewritten newSessionList/newProjectList comments are accurate and now point a reader to renderCondensedFooter + footer.go. Only blemish is the theme.go package-doc dangling reference (above).
- Issues: Only the stale theme.go:10-11 package-doc reference to the now-deleted Color() method.

BLOCKING ISSUES:
- None. go build and go test ./... reported GREEN; all six acceptance criteria are satisfied at the code level.

NON-BLOCKING NOTES:
- [do-now] internal/tui/theme/theme.go:10-11 — Update the package doc comment that still reads "The dark-pinned Color() convenience survives only for the handful of not-yet-mode-resolved call sites." Token.Color() was deleted by this task; either drop that sentence or rephrase to "All call sites resolve per-mode via ColorFor(mode)." Pure doc edit, zero logic impact. (Same line flagged by sibling verifier 6-4.)
