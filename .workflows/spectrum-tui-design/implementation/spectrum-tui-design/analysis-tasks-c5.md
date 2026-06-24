---
topic: spectrum-tui-design
cycle: 5
total_proposed: 1
---
# Analysis Tasks: Spectrum TUI Design (Cycle 5)

## Task 1: Extract the shared pad-right geometry behind a fill-style parameter
status: pending
severity: low
sources: duplication

**Problem**: `headerPadRight` (internal/tui/header.go:224-230) and `noticeBandPadRight` (internal/tui/notice_band.go:329-335) are byte-for-byte structurally identical: each takes `(seg, segWidth, w)`, returns `seg` unchanged when `segWidth >= w`, else returns `lipgloss.JoinHorizontal(lipgloss.Top, seg, style.Render(strings.Repeat(" ", w-segWidth)))`. The ONLY difference is which background style paints the trailing pad — `headerCanvasBg(mode, colourless)` (the owned canvas) vs `noticeBandTintStyle(tint, mode, colourless)` (the band's tint). The duplication is already self-documented: notice_band.go's doc comment states "It mirrors headerPadRight but pads with the band's tint instead of the canvas." Two copies of the same right-pad geometry mean a change to the pad rule (e.g. clamping behaviour or a different fill glyph) must be made in two places in lockstep.

**Solution**: Extract a single same-package helper carrying the shared "return-if-full, else join the styled pad" geometry, parameterised by the fill style: `padRightWithStyle(seg string, segWidth, w int, fill lipgloss.Style) string`. Keep `headerPadRight` and `noticeBandPadRight` as thin wrappers that bind their respective fill style and delegate to the new core, so every existing call site keeps its terse mode/colourless(/tint)-bound signature and no call site changes.

**Outcome**: The right-pad geometry (the `segWidth >= w` guard clause and the `JoinHorizontal` of a styled `strings.Repeat(" ", w-segWidth)` pad) lives in exactly one place. `headerPadRight` and `noticeBandPadRight` each shrink to a one-line delegation that supplies only their fill style. Future changes to the pad rule are made once. No behavioural change — output is byte-identical for every input.

**Do**:
1. In internal/tui (place the helper wherever it reads naturally — e.g. beside `headerPadRight` in header.go, or in a shared row/util file if one exists in the package), add `func padRightWithStyle(seg string, segWidth, w int, fill lipgloss.Style) string` containing the shared geometry: `if segWidth >= w { return seg }; pad := fill.Render(strings.Repeat(" ", w-segWidth)); return lipgloss.JoinHorizontal(lipgloss.Top, seg, pad)`.
2. Rewrite `headerPadRight` (internal/tui/header.go:224-230) to delegate: `return padRightWithStyle(seg, segWidth, w, headerCanvasBg(mode, colourless))`. Preserve its existing signature and doc comment.
3. Rewrite `noticeBandPadRight` (internal/tui/notice_band.go:329-335) to delegate: `return padRightWithStyle(seg, segWidth, w, noticeBandTintStyle(tint, mode, colourless))`. Preserve its existing signature and doc comment (the "mirrors headerPadRight" note remains accurate, since both now route through the same core — optionally update it to say both delegate to `padRightWithStyle`).
4. Do NOT touch the unrelated `padTo` (session_item.go:472) or `padLineToCanvasWidth` (model.go:3761) — they are legitimately distinct shapes (unstyled-string pad / trailing-bare-space strip respectively) and are out of scope.
5. Confirm imports are still satisfied (`lipgloss`, `strings` already imported in both files); remove no longer needed imports only if a file genuinely loses its last use (it will not — both files retain other `lipgloss`/`strings` uses).

**Acceptance Criteria**:
- A single `padRightWithStyle(seg string, segWidth, w int, fill lipgloss.Style) string` helper exists in internal/tui and contains the only copy of the return-if-full-else-join-styled-pad geometry.
- `headerPadRight` and `noticeBandPadRight` are thin wrappers that each only supply their fill style and delegate to `padRightWithStyle`; their signatures are unchanged.
- No call site of either wrapper is modified.
- `go build ./...` succeeds and `go test ./internal/tui/...` passes (header rendering and notice-band rendering behaviour unchanged).
- Rendered output of the header band and the notice band is byte-identical to before for all width/mode/colourless/tint combinations.

**Tests**:
- Run the existing internal/tui suite (header and notice_band rendering / consolidation tests) and confirm green — output must be unchanged.
- If the package has a consolidation-style guard test convention (the tree uses *_consolidation_test.go files), add or extend a small table test asserting `padRightWithStyle` returns `seg` unchanged when `segWidth >= w` and joins a styled pad of the correct width (`w - segWidth` cells) otherwise, for at least one canvas-fill and one tint-fill case, so the shared geometry is pinned at the new single owner.
