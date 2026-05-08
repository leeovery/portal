AGENT: architecture
FINDINGS:

- FINDING: Model.View() missing pagePreview arm — preview never renders
  SEVERITY: high
  FILES: internal/tui/model.go:1462-1482, internal/tui/pagepreview.go:320-322
  DESCRIPTION: Update routes pagePreview correctly (model.go:924-927: `m.preview, cmd = m.preview.Update(msg)`), but the sibling top-level View() switch at model.go:1462-1482 has no `case pagePreview` arm. When activePage == pagePreview, View falls through to `default:` and returns `m.viewSessionList()` — the sessions list. previewModel.View() (chromeLine + viewport) is never reached, so pressing Space leaves the user looking at the unchanged sessions page. Structural seam asymmetry: every other page (PageLoading, PageProjects, pageFileBrowser) has matched arms in BOTH Update and View; pagePreview has only the Update arm. The page state machine claims four sub-views but the rendering side only knows three.
  RECOMMENDATION: Add `case pagePreview: return m.preview.View()` to the View() switch, parallel to the pageFileBrowser arm. Pin a TUI-level test that asserts View output for activePage == pagePreview contains the preview's chrome line / viewport content (not the sessions list title) so a future regression that drops the arm is caught loudly.

STATUS: findings
