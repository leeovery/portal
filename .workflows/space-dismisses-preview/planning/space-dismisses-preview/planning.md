# Plan: Space Dismisses Preview

## Phase 1: Apply Change

Add a `tea.KeySpace` arm to `previewModel.Update` that mirrors the existing `tea.KeyEsc` arm, plus a focused unit test asserting the new dismiss path.

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| space-dismisses-preview-1-1 | Add Space dismiss case and test | None — preview page is not in filter input, so no Space-as-text collision. The single Space-open binding invariant in `pagepreview_filter_test.go` is unaffected because its grep scope is `updateSessionList` body, not `previewModel.Update`. |
