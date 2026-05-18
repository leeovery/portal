TASK: Rename previewChromeHeight to previewFrameOverhead = 2 (preview-visual-distinction-1-2)

ACCEPTANCE CRITERIA:
- No occurrences of `previewChromeHeight` remain in `.go` files under `internal/tui/`.
- `previewFrameOverhead` declared exactly once with value `2` and documented comment.
- Three sibling test files reference `previewFrameOverhead` in arithmetic.
- `go test ./internal/tui/...` passes.

STATUS: Complete

SPEC CONTEXT: Per spec § Code shape changes, the rename names the magic 2 used in resize math, reflecting the move from a single chrome row above the viewport to a full rounded frame (top + bottom border rows). Constant is package-internal to `internal/tui`. Sibling test files must update both the name and `wantHeight` arithmetic (value 1 → 2).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/pagepreview.go:44-48` — `previewFrameOverhead = 2` with spec doc comment.
  - `internal/tui/pagepreview.go:308` — `NewPreviewModel` uses `viewport.New(max(0, width-previewFrameOverhead), max(0, height-previewFrameOverhead))`.
  - `internal/tui/pagepreview.go:365-374` — `innerWidth()` / `innerHeight()` helpers (added in a later task) consolidate arithmetic.
  - `internal/tui/pagepreview.go:470-473` — `tea.WindowSizeMsg` handler uses helpers.
- Notes: Update handler does direct field assignment (`viewport.Width = m.innerWidth()`) rather than `SetSize`, with inline doc noting bubbles@v1.0.0 lacks `SetSize`. Semantically equivalent. Zero `previewChromeHeight` occurrences in `.go` files (grep verified).

TESTS:
- Status: Adequate
- Coverage:
  - `pagepreview_layout_test.go:56-62, 133-135`
  - `pagepreview_precedence_test.go:171-177`
  - `pagepreview_scroll_test.go:151-157`
  - `pagepreview_resize_test.go:27-31` (added in 1-7)
- Notes: Task specifies no new tests. No hardcoded `Height-1` patterns (grep verified).

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. `innerWidth()`/`innerHeight()` helpers consolidate subtraction to one place.
- Complexity: Low.
- Modern idioms: Yes — idiomatic `max(0, …)` clamp.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
