TASK: session-scrollback-preview-4-1 — Placeholder rendering for (nil, nil) Tail outcomes

ACCEPTANCE CRITERIA:
- When Tail returns (nil, nil), viewport content equals "(no saved content)".
- When Tail returns (nil, nil), chrome shows correct Window M of N, Pane X of Y, and window name.
- Placeholder branch fires identically at initial-open and post-cycle reads.
- Placeholder string is a single package-level constant.
- No code path treats (nil, nil) as "error".

STATUS: Complete

SPEC CONTEXT:
Spec § Read-Failure Handling > Placeholder: Working label "(no saved content)". Triggering: ENOENT, zero-byte, zero-line. Spec § Architecture Summary > Test seams: helper unifies the three "no content" cases at the helper layer; the placeholder/error decision lives at the call site in internal/tui.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview.go
  - Placeholder constant at line 24.
  - Dispatcher in readFocusedPaneIntoViewport at lines 202-215.
  - Shared by NewPreviewModel (initial open) and all three cycle handlers.

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_placeholder_test.go
- Coverage: initial open, Tab and ] cycles, chrome integrity under placeholder, canonical wording pin, ENOENT/zero-byte/zero-line collapse.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Single dispatcher.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None of consequence.
