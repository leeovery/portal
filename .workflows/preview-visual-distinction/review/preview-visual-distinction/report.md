# Implementation Review: Preview Visual Distinction

**Plan**: preview-visual-distinction
**QA Verdict**: Approve

## Summary

The implementation lands the full painted-frame preview surface end-to-end: rounded-blue `lipgloss` border around the viewport, manually-composed top edge carrying a width-cascading chrome line, four-tier cascade with display-cell-aware truncation, SGR-reset injection guarding the right border, per-tick chrome recomputation on resize, and the `previewChromeHeight` → `previewFrameOverhead = 2` rename. All 17 plan tasks (9 phase-1 build + 6 cycle-1 cleanups + 2 cycle-2 cleanups) verified independently against the spec and their acceptance criteria. Zero blocking issues. The implementation made a small number of clean, well-documented departures from the literal task scope where they strengthened the result — splitting `composeChromeLine` into a thin one-liner delegating to `composeChromeLineParts` so View() can style border parts and chrome separately per the spec's stricter Color application contract; extracting `selectChromeTier`, `tier4Row`, and `innerWidth`/`innerHeight` as single sources of truth. Test conventions are clean (no `t.Parallel()`, no `tmuxtest`, constructor-injected mocks, exact-byte assertions against `verboseKeymap`/`compactKeymap`).

## QA Verification

### Specification Compliance

Implementation aligns with the specification. Two minor compliance notes worth flagging in recommendations:

1. The cascade-tier e2e test in 1-9 had to deviate from the spec's literal width set `{200, 60, 40, 25, 15}` because the spec's arithmetic assumed a stale verbose-keymap length (82 cells); real `lipgloss.Width(verboseKeymap) == 57` shifts every tier interval. Implementer documented the corrected widths `{200, 105, 95, 50, 15}` with rationale at the top of the test file. The spec doc itself still carries the misleading widths.
2. The SGR-reset e2e assertion in 1-9 uses `strings.Contains(raw, "\x1b[0m")` at file-global scope rather than the spec's stricter "every non-empty viewport content row" form. Acknowledged in task body; per-row count assertion would tighten the guard.

Neither blocks approval — both are flagged as ideas in Recommendations.

### Plan Completion

- [x] Phase 1 acceptance criteria met (9/9 tasks complete, all 17 phase-acceptance bullets satisfied)
- [x] Phase 2 (Analysis Cycle 1) acceptance criteria met (6/6 tasks complete)
- [x] Phase 3 (Analysis Cycle 2) acceptance criteria met (2/2 tasks complete)
- [x] All tasks completed
- [x] No scope creep — production changes confined to `internal/tui/pagepreview.go` per spec; `internal/tui/model.go:1421` untouched

### Code Quality

No issues. Notable strengths:

- **DRY**: Tier-4 collapsed row construction centralised in `tier4Row`. Inner-dimension arithmetic centralised in `innerWidth`/`innerHeight`. Tier selection lives in one place (`selectChromeTier`).
- **Function purity**: `composeChromeLine`, `composeChromeLineParts`, `selectChromeTier`, `truncateToCells`, `injectSGRResets`, `tier4Row` all pure; styling is applied at the View() seam only.
- **Documentation**: Doc comments cite spec sections, explain *why* (e.g. inline rationale for direct `viewport.Width = ...` assignment since bubbles@v1.0.0 lacks `SetSize`).

### Test Quality

Tests adequately verify requirements. Strong points:

- Constants pinned by literal byte equality (`verboseKeymap`, `compactKeymap`, `previewBorderColor` hex codes).
- Cascade thresholds covered both at the pure-function layer and end-to-end through Update → View.
- Glyph-class coverage for `truncateToCells` (ASCII, CJK, emoji incl. ZWJ, combining marks).
- SGR-injection edge cases (idempotency, trailing-newline trailer, whitespace+SGR, fully empty).
- Chrome-row single-line invariant pinned at exact spec width set.
- Concatenation property test (`TestComposeChromeLineParts_ConcatenationEqualsComposeChromeLineAtAllThresholds`) guards the structural sibling refactor in 2-6.
- TestMain forces TrueColor profile so SGR-byte assertions are not stripped under non-TTY default.

Minor gaps noted in Recommendations:

- The spec's truncated-ZWJ "no mid-sequence cut" probe is not directly exercised (1-3): current ZWJ row uses a budget that fits whole.
- 1-9 SGR-reset assertion is file-global rather than per-row.

### Required Changes

None.

## Recommendations

### Quick-fixes

1. (1-4) `composeChromeLine` doc comment says "below that, returns the empty string" referring to widths `< 2`, but the function returns "" only for `width < 0` and a non-empty tier-4 row for width 0/1. Wording is ambiguous.

### Ideas

2. (1-1) Constant-pinning tests for keymap and border-colour share one file. Splitting into `pagepreview_keymap_constants_test.go` and `pagepreview_border_color_test.go` would clarify scope.
3. (1-3) Spec called for a truncated-ZWJ test case (e.g. `"👨‍👩‍👧hello"` with budget 4) asserting no mid-sequence cut. Current ZWJ row uses budget 3 which fits whole; the truncation arm for ZWJ is not directly exercised. Invariants still hold via ASCII/CJK rows.
4. (1-3) `if s == ""` short-circuit at the top of `truncateToCells` is redundant with `runewidth.StringWidth(s) <= budget` (StringWidth("") == 0). Harmless defensive code.
5. (1-4) `composeChromeLineParts` / `tier4Row` / `selectChromeTier` extend task 1-4's stated scope to support task 1-8's split-styling. Factoring is clean; flagging for traceability.
6. (1-4) `selectChromeTier`'s tier-1 filler computation could be simplified to `filler := nameBudget - lipgloss.Width(truncated)` since truncation only shrinks. Defensive but not worth changing.
7. (1-5) This task's width set `{200, 80, 60, 40, 25, 15, 10, 4, 3, 2, 0}` overlaps but doesn't coincide with the cascade-boundary set used in the sibling test. Both assert `strings.Count(...) == 0`. Add a one-line comment noting which is the spec-mandated invariant, or merge the width sets.
8. (1-6) Per-line allocation in `injectSGRResets` could be reduced via a pre-sized `strings.Builder`. Not worth changing at current cadence.
9. (1-7) `chromeLineForTest` pins width=200 and calls `composeChromeLine` directly. Future cleanup could retire the shim in favour of direct calls.
10. (1-7) If bubbles is upgraded to a version exposing `viewport.SetSize`, the WindowSizeMsg site should switch so YOffset auto-clamping is engaged. A TODO-on-upgrade comment would make the path explicit.
11. (1-8) Tier-4 collapse in `composeChromeLineParts` returns the entire row as `left` with chrome/right empty. A one-line comment in View() noting "at tier 4 chrome is empty and left contains the entire row" would make the surface contract self-evident.
12. (1-9) Specification's tier-mapping widths `{200, 60, 40, 25, 15}` are mathematically wrong against real `verboseKeymap` width. Implementer correctly adjusted, but the spec doc still carries the misleading widths. Follow-up doc-edit pass on the spec to align with real math.
13. (1-9) Test asserts `strings.Contains(raw, "\x1b[0m")` at file-global level; spec's stricter form is "every non-empty viewport content row carries `\x1b[0m`". Per-row count assertion would tighten the guard.
14. (2-2) The wider _test.go corpus has many open-coded `stubEnumerator{}` + `recordingReader{}` + `NewPreviewModel` triples (e.g. `pagepreview_brandnew_test.go`, `pagepreview_error_test.go`). Out of scope for this cycle; future sweep could unify, possibly via a variant taking a groups slice.
15. (2-3) `chromeLineAtModelWidth` could absorb the `stripANSI` wrap that both consumers apply, but would couple it to ANSI-stripping; current symmetric shape with `chromeLineForTest` is preferred.
16. (2-6) Doc comment on `composeChromeLine` duplicates cascade-tier rules documented on `selectChromeTier` and `composeChromeLineParts`. Could trim to "Thin wrapper; see composeChromeLineParts" so tier rules have a single canonical home.
17. (3-1) `NewPreviewModel`'s inline `max(0, width-previewFrameOverhead)` could be replaced with a local var computed before the composite literal. Pure style call.
18. (3-1) `innerHeight()` currently used only in the WindowSizeMsg handler. Cheap optionality if View() ever needs the inner height (e.g. for vertical-degradation handling, which the spec defers).
