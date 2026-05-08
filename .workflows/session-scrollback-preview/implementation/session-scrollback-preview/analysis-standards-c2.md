AGENT: standards
FINDINGS:

- FINDING: Top-level Model.View() does not route pagePreview to the preview model
  SEVERITY: high
  FILES: internal/tui/model.go:1462-1482
  DESCRIPTION: When `m.activePage == pagePreview`, `Model.View()` falls through to the `default` branch and returns `viewSessionList()`. There is no `case pagePreview: return m.preview.View()` arm. This directly contradicts spec §Interaction Shape ("Preview occupies the full terminal — chrome on a single line plus the embedded `bubbles/viewport` filling the remaining vertical space") and the Acceptance Criteria ("Pressing `Space` on a highlighted session in the Sessions page opens the preview page" / "Chrome shows window M of N, pane X of Y, window name…"). The Update routing at line 924 correctly delegates to `m.preview.Update`, so internal state advances (windowIdx, paneIdx, viewport scroll), but the user sees the unchanged session list view instead of the preview UI. The bug is uncaught by the existing test suite because every preview view-rendering test calls `previewModel.View()` directly (e.g. pagepreview_layout_test.go:28) rather than going through `Model.View()` while `activePage == pagePreview`.
  RECOMMENDATION: Add a `case pagePreview: return m.preview.View()` arm in `Model.View()` immediately above the `default` case at line 1479. Add a top-level integration assertion that drives the model from PageSessions → Space → asserts `Model.View()` contains the chrome line and viewport content, so this regression cannot recur.

STATUS: findings
