TASK: session-scrollback-preview-2-3 — Add pagePreview arm to page state machine and bind Space on Sessions page

ACCEPTANCE CRITERIA:
- pagePreview declared in page const block in internal/tui/model.go.
- Top-level Update routes pagePreview to m.preview.Update.
- Space on Sessions page constructs previewModel and transitions when ok==true.
- Space is no-op on empty list.
- Space is no-op when no item is highlighted (SelectedItem() == nil).
- Space is no-op when NewPreviewModel returns ok==false.
- Loading, Projects, FileBrowser handlers do not invoke NewPreviewModel.
- When Space is pressed during SettingFilter(), this branch does not call NewPreviewModel.

STATUS: Complete

SPEC CONTEXT:
Spec § Trigger and Entry Point binds Space to Sessions page only with empty-list / no-highlighted-item silent no-ops. § Refresh Semantics > Initial-open ordering: enumeration failure or empty result yields silent no-open. § Architecture Summary > Page state machine: pagePreview is a peer of pageFileBrowser. § Filter Behaviour with Preview: Space does not intercept while filtering.

IMPLEMENTATION:
- Status: Implemented
- Locations:
  - internal/tui/model.go:32 — pagePreview declared as peer of pageFileBrowser.
  - internal/tui/model.go:183-185 — enumerator, reader, preview fields on root Model.
  - internal/tui/model.go:924-927 — top-level Update routes pagePreview to m.preview.Update.
  - internal/tui/model.go:1273-1288 — Space branch in updateSessionList: empty-list no-op, SelectedItem()==nil no-op, NewPreviewModel ok==false no-op, ok==true transitions to pagePreview.
  - internal/tui/model.go:1264-1266 — SettingFilter() short-circuit precedes the Space branch (task 2-5 integration).
  - internal/tui/model.go:1479-1480 — View() routes pagePreview to m.preview.View().
  - Loading handler (914-919), Projects handler (953-1013), and FileBrowser handler (1230-1236) do NOT match Space.
- Notes: Space matched via msg.Type == tea.KeySpace (canonical bubbletea v1 shape). Guard ordering correct: SettingFilter → empty list → SelectedItem nil → NewPreviewModel; defensive guards precede any seam invocation, honouring the "seam fields may be nil during early construction" edge case.

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_entry_test.go (10 tests, all 8 acceptance criteria covered):
  - TestSpaceOnSessionsPageTransitionsToPagePreviewWhenHighlighted
  - TestSpaceOnSessionsPageNoOpWhenListEmpty
  - TestSpaceOnSessionsPageNoOpWhenSelectedItemNil — uses committed-filter-narrowed-to-zero (realistic production trigger).
  - TestSpaceOnSessionsPageRemainsOnSessionsWhenEnumerationFails
  - TestSpaceOnSessionsPageRemainsOnSessionsWhenEnumerationEmpty
  - TestSpaceDuringSettingFilterDoesNotCallNewPreviewModel — drives `/` to enter filter mode.
  - TestSpaceOnLoadingPageDoesNotCallNewPreviewModel
  - TestSpaceOnProjectsPageDoesNotCallNewPreviewModel
  - TestSpaceOnFileBrowserPageDoesNotCallNewPreviewModel
  - TestPagePreviewRoutesUpdateToPreviewModel
- stubEnumerator and recordingReader reused from pagepreview_test.go (no duplication). Behavioural assertions, not implementation details. No t.Parallel(). No tmuxtest import.

CODE QUALITY:
- Project conventions: Followed. Functional options for Model wiring (WithEnumerator, WithScrollbackReader at model.go:455-473), package-private previewModel, exported NewPreviewModel.
- SOLID: Constructor-injected seams; no package-level mutable seam state. Single-responsibility upheld.
- Complexity: Low. Flat guard-and-transition sequence.
- Modern idioms: tea.KeyMsg type-switch matches surrounding code.
- Readability: Strong inline comment at model.go:1267-1272 anchors the no-op conditions to spec.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The Space branch lives inside updateSessionList's tea.KeyMsg switch but precedes the inner switch { case ... }. Extracting handleSpacePreview would make precedence structurally explicit and shorten updateSessionList.
- [idea] TestPagePreviewRoutesUpdateToPreviewModel asserts only that activePage stays on pagePreview after KeyDown — could additionally pin that viewport YOffset moved.
