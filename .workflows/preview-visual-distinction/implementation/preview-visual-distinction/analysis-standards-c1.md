# Standards Analysis — cycle 1

AGENT: standards
STATUS: findings
FINDINGS_COUNT: 1

## Findings

### 1. Stale `chromeLine` reference in helpers-test docstring
- **SEVERITY**: low
- **FILES**: `internal/tui/pagepreview_helpers_test.go:17`
- **DESCRIPTION**: The docstring on `newPreviewModelForHelpers` lists `chromeLine` among the "helpers under test" that must remain pure. Per spec § Code shape changes > Replace `chromeLine()` with `composeChromeLine`, the `chromeLine()` method was deleted; the test-only shim `chromeLineForTest` invokes the pure `composeChromeLine` instead. The doc reference is stale.
- **RECOMMENDATION**: Replace `chromeLine` in the parenthesised list with `composeChromeLine` (or remove the entry).

## Summary

Implementation conforms tightly to the specification. All required constants (`verboseKeymap`, `compactKeymap`, `previewBorderColor`, `minWindowNameCells = 8`, `previewFrameOverhead = 2`), the four-tier cascade, the pure `composeChromeLine` boundary, display-cell-aware truncation with ellipsis-cell reservation, SGR-reset injection with the spec's exact algorithm, manual top-edge composition with the border-parts-styled / chrome-unstyled split per § Top edge composition > Color application, viewport sizing with `max(0, …)` clamps on both constructor and resize paths (semantically equivalent to spec's `viewport.SetSize` per the note about bubbles@v1.0.0), recompute-every-tick chrome, and Surface 1–5 test coverage (including the cascade-tier E2E with the documented threshold adjustments) are all present and match the spec's contracts. Corner glyphs are sourced from `lipgloss.RoundedBorder()` rather than hardcoded per § Style sourcing.
