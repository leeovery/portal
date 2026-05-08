TASK: session-scrollback-preview-2-6 — Viewport scroll keys and resize handling within preview

ACCEPTANCE CRITERIA:
- Down scrolls down by 1; Up scrolls up by 1.
- Up at top is silent no-op; Down at bottom is silent no-op.
- Home jumps to YOffset == 0 via preview-owned m.viewport.GotoTop().
- End jumps to bottom via preview-owned m.viewport.GotoBottom().
- PgUp, PgDn, ctrl-u, ctrl-d, j, k all behave per bubbles/viewport defaults.
- tea.WindowSizeMsg resizes viewport without calling m.reader.Tail.
- 100 successive WindowSizeMsg events trigger zero additional Tail calls beyond initial-open.
- Scroll offset preserved across WindowSizeMsg.

STATUS: Complete

SPEC CONTEXT:
Spec § Within-preview Key Bindings > Keymap policy: Up/Down scroll the focused viewport, not between-session navigation. § History Depth > Scroll within bounds: top boundary is hard edge, scroll-up at top silently no-ops. § Refresh Semantics > Read Trigger Events: resize is not a read trigger. § Interaction Shape > Layout: tea.WindowSizeMsg forwarded for re-flow with scroll offset preserved.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview.go (Update intercepts tea.KeyHome/tea.KeyEnd via GotoTop/GotoBottom; tea.WindowSizeMsg updates Width/Height with chrome subtraction; default scroll keys delegate to viewport.Update)
- Notes: The Home/End interception is necessary because bubbles/viewport@v1.0.0 DefaultKeyMap doesn't bind these. Resize logic does NOT call reader.Tail — observed via mock counter staying at 1 across many resize events.

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_scroll_test.go and internal/tui/pagepreview_layout_test.go
- Coverage:
  - Down/Up/Home/End behaviours.
  - No-ops at top/bottom.
  - Single WindowSizeMsg + 100 successive resizes both keep Tail count at 1.
  - Scroll offset preserved across resize.
  - Empty/single-line edge cases.
  - j/k/PgUp/PgDn/ctrl-u/ctrl-d delegation.
  - Chrome-aware sizing in pagepreview_layout_test.go; height clamp >= 0.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. Doc comment notes bubbles/viewport DefaultKeyMap gap rationale for Home/End.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None of consequence.
