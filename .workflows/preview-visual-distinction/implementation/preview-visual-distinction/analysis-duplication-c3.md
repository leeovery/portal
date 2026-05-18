# Duplication Analysis — cycle 3

AGENT: duplication
STATUS: findings
FINDINGS_COUNT: 1

## Findings

### 1. New WindowSizeMsg tests overlap with pre-existing tests
- **SEVERITY**: low
- **FILES**: `internal/tui/pagepreview_resize_test.go:16-33,40-51` (new tests added in task 1-7), `pagepreview_scroll_test.go:146-165` (pre-existing `TestPreviewWindowSizeMsgUpdatesViewportDimensions`), `pagepreview_layout_test.go:51-64,104-117` (pre-existing WindowSizeMsg tests)
- **DESCRIPTION**: The new resize tests re-assert the WindowSizeMsg → viewport-size contract that three pre-existing tests already cover. Both new tests assert the same four-field invariant (`m.width`, `m.height`, `viewport.Width`, `viewport.Height`) using the `X - previewFrameOverhead` idiom. Net result: four files repeat this contract. The new clamp test does add a unique boundary case (`Width=1, Height=0`) not covered by the pre-existing clamp tests.
- **RECOMMENDATION**: Either (a) delete the duplicative new tests, keeping only the unique `Width=1, Height=0` boundary case (merge into pre-existing clamp test), or (b) extract `assertViewportSizedFromResize(t, m, wantWidth, wantHeight)` in `pagepreview_helpers_test.go`. Low-severity — not blocking.
