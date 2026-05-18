TASK: Extract tier4Row Helper to Deduplicate Collapsed-Row Reconstruction (preview-visual-distinction-2-4)

ACCEPTANCE CRITERIA:
- Literal expression `border.TopLeft + strings.Repeat(border.Top, max(0, outer-2)) + border.TopRight` appears at most once in production code.
- Helper shared by composeChromeLine and composeChromeLineParts so tier-4 fallback shape cannot drift.

STATUS: Complete

SPEC CONTEXT: Cycle-1 duplication-analysis follow-up (finding #4). After phase 1 introduced both composeChromeLine and composeChromeLineParts with shared tier selection via selectChromeTier, the only remaining drift point was the tier-4 collapsed-row reconstruction. Per § Frame structure / § Top edge composition, the tier-4 shape (corners + ─ filler) is the load-bearing fallback that must be byte-identical across both surfaces.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tui/pagepreview.go:140-147` (helper), `:197` (sole caller after task 2-6 collapsed composeChromeLine)
- Notes: Helper signature `func tier4Row(border lipgloss.Border, outer int) string` matches recommendation. Doc comment states "Shared by composeChromeLine and composeChromeLineParts so the tier-4 fallback shape cannot drift". After task 2-6, the tier-4 literal expression appears only once in production. Grep confirms.

TESTS:
- Status: Adequate
- Coverage: No dedicated test for `tier4Row` — appropriate. Covered transitively by tier-4 tests in pagepreview_compose_chrome_test.go (TestComposeChromeLine_Arg13Tier4FifteenCellTopEdge, degenerate Arg2/Arg1/Arg0 tests at :158-208) and concatenation invariant test (TestComposeChromeLineParts_ConcatenationEqualsComposeChromeLineAtAllThresholds at :212).
- Notes: Test files still spell out `border.TopLeft + strings.Repeat(border.Top, N) + border.TopRight` as expected ground-truth values — correct methodology (independent oracle). Acceptance criterion targets production-code deduplication.

CODE QUALITY:
- Project conventions: Followed. Pure, no I/O, value-style API.
- SOLID: Good. Single-responsibility helper.
- Complexity: Low. One expression, max(0, outer-2) clamp defensive against strings.Repeat panic.
- Modern idioms: Yes — built-in `max` (Go 1.21+).
- Readability: Good. Doc-comment disambiguates `outer` (OUTER terminal width).
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
