TASK: Fix Stale chromeLine Reference in Helpers-Test Docstring (preview-visual-distinction-2-5)

ACCEPTANCE CRITERIA:
- Standalone `chromeLine` token not referenced in the `newPreviewModelForHelpers` docstring.
- `go test ./internal/tui/...` passes.

STATUS: Complete

SPEC CONTEXT: Per spec § Code shape changes, `chromeLine()` method on `previewModel` was deleted; the test-only shim `chromeLineForTest` now invokes pure `composeChromeLine`. The docstring on `newPreviewModelForHelpers` listed `chromeLine` among "helpers under test that must remain pure" — stale after method deletion. Analysis-standards-c1 flagged this; this task addresses it.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tui/pagepreview_helpers_test.go:18-21`
- Notes: Parenthesised helper list now reads `(currentGroup, currentRawIndices, currentPaneKey, degenerate, composeChromeLine)`. Stale `chromeLine` token replaced with `composeChromeLine`. Remaining `chromeLine()` mention at line 32 is inside `chromeLineForTest`'s own docstring explaining "the deleted `chromeLine()` method on previewModel" — intentional historical context, marked non-actionable by analysis-standards-c2.

TESTS:
- Status: Adequate (N/A — docstring-only change)
- Coverage: No behaviour change.
- Notes: Plan explicitly states no new tests for docstring-only change.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: N/A.
- Complexity: N/A.
- Modern idioms: N/A.
- Readability: Good — docstring accurately enumerates pure helpers, matches current code shape.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
