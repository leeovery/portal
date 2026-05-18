TASK: Chrome-Row Single-Line Invariant Test (preview-visual-distinction-1-5)

ACCEPTANCE CRITERIA:
- Test function exists and runs as part of `go test ./internal/tui/...`.
- Asserts `strings.Count(composeChromeLine(w, …), "\n") == 0` for every width in `{200, 80, 60, 40, 25, 15, 10, 4, 3, 2, 0}`.
- Fails with a descriptive message if any width produces an embedded newline.

STATUS: Complete

SPEC CONTEXT: Spec § Chrome-row invariant for resize math: resize math `viewport.SetSize(msg.Width − 2, msg.Height − 2)` assumes the top edge is exactly one row at every width. Invariant test guards this assumption across every cascade tier. Negative widths excluded — they return empty string, not load-bearing.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tui/pagepreview_compose_chrome_test.go:65-72` (function `TestComposeChromeLine_NoEmbeddedNewlines`)
- Notes:
  - Iterates exactly the spec-mandated width set `{200, 80, 60, 40, 25, 15, 10, 4, 3, 2, 0}`.
  - Calls `composeChromeLine(w, 0, 1, 0, 1, testWindowName)` where `testWindowName = "nvim-editor"`.
  - Error message includes the offending width, newline count, and raw `%q`-formatted output.
  - Comment block (57-64) cites `specification.md § Chrome-row invariant for resize math` and `§ Tests > Chrome-row invariant test`, explains negative-width exclusion.
  - Test lives in `pagepreview_compose_chrome_test.go` alongside cascade-tier tests (colocation acceptable).

TESTS:
- Status: Adequate
- Coverage: All 11 spec widths exercised; every cascade tier touched.
- Notes: Not over-tested; not under-tested. Sibling `TestComposeChromeLine_NoEmbeddedNewlinesAcrossThresholds` asserts the same predicate over a different (cascade-boundary) width set — independently justified.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`, no `tmuxtest`.
- SOLID principles: Good — pure function under test.
- Complexity: Low — single `for range`.
- Modern idioms: Yes.
- Readability: Good — leading comment ties test to spec.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] This task's width set overlaps but does not coincide with the cascade-boundary set in the sibling test. Both assert `strings.Count(...) == 0`. Independently spec-justified but a future reader may wonder why two newline-count loops coexist. Adding a one-line comment in the sibling test noting that this test is the spec-mandated invariant test would help, or merging the width sets.
