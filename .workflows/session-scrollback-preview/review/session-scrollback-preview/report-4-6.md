TASK: session-scrollback-preview-4-6 — Externally-killed-session in-preview stability with progressive placeholders

ACCEPTANCE CRITERIA:
- TmuxEnumerator.ListWindowsAndPanesInSession called exactly once across the entire test.
- Chrome Window M of N, Pane X of Y, and window name remain stable across all cycle events.
- As the mock progressively returns (nil, nil), viewport renders placeholder for those panes.
- No panic / no error frame raised when content reads start failing.
- Cycle keys (], [, Tab) continue to traverse the captured shape.
- Esc still works and returns to Sessions list (with refresh per Task 4-5).

STATUS: Complete

SPEC CONTEXT:
Spec § Cross-cutting Seams > Externally-Killed Session During Preview: ".bin files persist briefly, then get cleaned by the daemon... ] / [ / Tab will increasingly land on placeholders as files are cleaned. Window/pane structural counts and names were captured at preview-open. Cycle keys cycle the captured shape; no live re-enumeration is performed mid-preview."

IMPLEMENTATION:
- Status: Implemented (test-only — production code via cached groups + shared dispatcher already satisfies the contract).
- Location: internal/tui/pagepreview_externalkill_test.go
- Notes: Six well-scoped hermetic tests pin enumerator-called-once, chrome stability against a sentinel "REENUMERATED" re-enumeration shape, deterministic bytes→mixed→placeholder progression across a 9-call sequence, full 4-pane traversal, panic-guard via deferred recover exercising View/chromeLine/viewport.View at every step, and clean previewDismissedMsg emission from Esc with no extra Tail or enumerator calls.

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_externalkill_test.go
- Coverage: All six task tests (chrome stable; placeholders progressively appear; no live re-enum; cycle keys traverse; no panic; Esc dismiss).

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] _ = mm.viewport.View() in the panic test is redundant with _ = mm.View() since View() invokes viewport.View() internally; intent ("exercise every panic surface") is reasonable.
- [idea] killedSessionFixture() is local; if a third test file needs the 2x2 shape, consider promoting it into a shared fixtures file.
- [quickfix] Line 219 comment "(windowIdx=1) under preserved 'of 2' total" reads ambiguously next to the "Window 2 of 2" assertion; rephrase.
