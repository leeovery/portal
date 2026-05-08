TASK: session-scrollback-preview-5-5 — Add invariant comments around preview lifecycle fragilities

ACCEPTANCE CRITERIA:
- All three comments present at the indicated locations.
- No code-path behaviour change; tests pass without modification.

STATUS: Complete

SPEC CONTEXT:
Analysis cycle 1 finding: three small lifecycle fragilities in the preview code had no inline guard:
1. Home/End interception is necessary because bubbles/viewport@v1.0.0 DefaultKeyMap does not bind them.
2. Dismiss handler reads m.preview.session then zeroes m.preview then dispatches refreshSessionsAfterPreviewCmd(preserveName); flipping read-then-zero would silently send empty PreserveName.
3. previewModel.session is identity-bearing but type carries no comment forbidding method calls on the zero value.

IMPLEMENTATION:
- Status: Implemented (comment-only).
- Locations:
  - internal/tui/pagepreview.go:264-266 — Home/End rationale comment present.
  - internal/tui/model.go:892-893 — preserveName ordering comment present immediately above the capture line.
  - internal/tui/pagepreview.go:45-47 — Zero-value reserved comment present on previewModel type doc block.

TESTS:
- Status: N/A (comment-only; existing tests pass without modification).

CODE QUALITY:
- All three comments accurate, non-behavioural wording.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None of consequence.
