TASK: session-scrollback-preview-5-2 — Unify previewModel receiver discipline

ACCEPTANCE CRITERIA:
- All previewModel methods use value receivers.
- All cycle branches still re-read after mutation; viewport content updates observable in View().
- NewPreviewModel returns a fully-initialised value with viewport already populated.

STATUS: Complete

SPEC CONTEXT:
Analysis cycle 1 finding: previewModel mixed value and pointer receivers around viewport mutation. Works today because viewport.Model's content survives a value copy, but a future field that does not survive value-copy (mutex, channel, atomic) would silently break the helper's mutations.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview.go
  - All previewModel methods use value receivers:
    - currentGroup (109), currentRawIndices (118), currentPaneKey (128), degenerate (137), chromeLine (163), readFocusedPaneIntoViewport (202, returns viewport.Model), Update (252), View (320).
  - All cycle branches re-read after mutation:
    - Tab (279): m.viewport = m.readFocusedPaneIntoViewport().
    - ] (296): same.
    - [ (304): same.
  - View() at line 321 reads m.viewport.View().
- Notes: readFocusedPaneIntoViewport correctly operates on a local copy vp := m.viewport (line 203), mutates it via SetContent and GotoBottom, and returns the updated value. Doc comment (195-201) explicitly explains the value-receiver-returning-viewport pattern. Construction populates viewport via viewport.New (89), then line 101 calls m.viewport = m.readFocusedPaneIntoViewport() to populate content before returning.

TESTS:
- Status: Adequate (existing pagepreview_test.go tests construction populates viewport and anchors at scroll-tail).
- Note: Cycle re-read coverage isn't directly tested; verified by inspection — non-blocking.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. Doc comment explains the pattern.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Cycle re-read coverage isn't pinned by a test that asserts the post-Update View() output. Pinning could be a one-line addition to existing tests.
