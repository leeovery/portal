TASK: session-scrollback-preview-5-4 — Correct View() doc-comment about chrome placement

ACCEPTANCE CRITERIA:
- No doc-comment in pagepreview.go claims spec § Interaction Shape > Layout fixes the header-on-top orientation.
- The build-phase nature of the choice is explicit.

STATUS: Complete

SPEC CONTEXT:
Analysis cycle 1 finding: prior doc-comment claimed header-on-top is "fixed in v1 per § Interaction Shape > Layout". The spec actually defers final placement to a build-phase decision per § Open Items > Chrome Floor.

IMPLEMENTATION:
- Status: Implemented (comment-only edit).
- Location: internal/tui/pagepreview.go:314-319
- Notes:
  - Doc-comment now reads "Header-on-top is the build-phase choice (spec § Open Items > Chrome Floor defers placement)".
  - No reference to "§ Interaction Shape > Layout" fixing orientation.
  - Build-phase nature explicit.

TESTS:
- Status: N/A (comment-only edit; existing tests pass without modification).

CODE QUALITY:
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None of consequence.
