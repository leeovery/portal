---
topic: session-scrollback-preview
cycle: 2
total_proposed: 1
---
# Analysis Tasks: session-scrollback-preview (Cycle 2)

## Task 1: Add pagePreview arm to top-level Model.View() — preview never renders
status: approved
severity: high
sources: standards, architecture

**Problem**: When `m.activePage == pagePreview`, top-level `Model.View()` (`internal/tui/model.go:1462-1482`) falls through to the `default:` branch and returns `viewSessionList()` — the unchanged sessions list. There is no `case pagePreview: return m.preview.View()` arm. `Model.Update` (`model.go:924-927`) DOES route correctly via `m.preview, cmd = m.preview.Update(msg)`, so internal state advances (windowIdx, paneIdx, viewport scroll, dispatched Tail reads), but the user sees the unchanged sessions page. This contradicts spec § Interaction Shape ("Preview occupies the full terminal — chrome on a single line plus the embedded `bubbles/viewport`") and the Acceptance Criteria.

The bug is uncaught by the existing test suite because every preview view-rendering test calls `previewModel.View()` directly (e.g. pagepreview_layout_test.go:28) rather than driving through `Model.View()` while `activePage == pagePreview`. Structural seam asymmetry: every other page (PageLoading, PageProjects, pageFileBrowser) has matched arms in BOTH Update and View; pagePreview has only the Update arm.

**Solution**: Add a `case pagePreview: return m.preview.View()` arm to `Model.View()` immediately above the `default:` case. Add a TUI-level integration test that drives Model from PageSessions → Space → asserts `Model.View()` contains the preview's chrome line and viewport content so this regression cannot recur.

**Outcome**: Pressing Space genuinely renders the preview chrome + viewport in production. The page state machine's four sub-views are matched on both the Update and View sides.

**Do**:
1. In `internal/tui/model.go` `View()` (around line 1462-1482), add a `case pagePreview: return m.preview.View()` arm immediately above the `default:` case (parallel to the existing `pageFileBrowser` arm at line 1477-1478).
2. Add a TUI-level integration test (in a new or existing test file in `internal/tui`) that:
   - Constructs Model with seam mocks (TmuxEnumerator, ScrollbackReader).
   - Drives the model into pagePreview via Space (or by setting `activePage = pagePreview` and `m.preview = NewPreviewModel(...)` directly).
   - Calls `Model.View()` and asserts the rendered output contains the chrome's "Window N of M" / "Pane N of M" / window name AND does NOT contain the sessions-list title.

**Acceptance Criteria**:
- `Model.View()` returns `m.preview.View()` when `m.activePage == pagePreview`.
- The new integration test fails when the arm is removed (verifiable via mutation: comment out the arm; the test fails).
- Existing tests continue to pass.

**Tests**:
- New: `TestModelViewRoutesPagePreviewToPreviewModel` (or similar) — drives Model into pagePreview state and asserts View output.
- Existing: all preview tests + model_test.go suite continue to pass.
