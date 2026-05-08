TASK: session-scrollback-preview-6-1 — Add pagePreview arm to top-level Model.View()

ACCEPTANCE CRITERIA:
- Model.View() returns m.preview.View() when activePage == pagePreview.
- New integration test fails when the arm is removed (mutation-verified).
- All existing tests pass.

STATUS: Complete

SPEC CONTEXT:
Analysis cycle 2 finding: when m.activePage == pagePreview, top-level Model.View() previously fell through to default and returned viewSessionList(). There was no `case pagePreview: return m.preview.View()` arm. Update routed correctly via m.preview.Update, so internal state advanced, but the user saw the unchanged sessions page. Spec drift on § Interaction Shape and Acceptance Criteria.

IMPLEMENTATION:
- Status: Implemented.
- Location: internal/tui/model.go:1479-1480 — pagePreview arm correctly added, parallel to pageFileBrowser.

TESTS:
- Status: Adequate.
- Location: internal/tui/pagepreview_view_routing_test.go
- Coverage: integration test drives Update→View and asserts preview chrome ("Window 1 of 1", "Pane 1 of 1"), window name, and viewport content — would fail if the arm were removed.

CODE QUALITY:
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None of consequence.
