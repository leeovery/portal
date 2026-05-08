TASK: session-scrollback-preview-3-2 — Tab cycle: next pane within current window with wrap and re-read

ACCEPTANCE CRITERIA:
- Tab on N>=2 panes advances paneIdx by 1 mod N and issues exactly one Tail with new paneKey.
- Tab from last pane wraps to paneIdx=0; windowIdx unchanged.
- Tab on single-pane window OR degenerate session is silent no-op (zero Tail, no Cmd, no mutation).
- After Tab triggering a read, viewport.AtBottom() is true.
- windowIdx never modified by Tab.
- Read is synchronous; no tea.Cmd returned.

STATUS: Complete

SPEC CONTEXT:
Per § Within-preview Key Bindings, Tab cycles forward with wraparound. Per § Refresh Semantics > Read Trigger Events, Tab re-reads the newly-focused pane. Per § Scroll Position Resets on Focus Change, scroll returns to tail on every focus change. Per § Read Pipeline, the read happens inline in Update — no tea.Cmd deferral.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview.go:273-280 (Tab branch in previewModel.Update). Shared dispatcher at lines 202-215 (readFocusedPaneIntoViewport).
- Notes: Tab is matched on msg.Type inside the tea.KeyMsg switch, ahead of the viewport passthrough at the function tail (lines 309-311). Logic: paneCount := len(m.currentGroup().PaneIndices); if paneCount <= 1 return m, nil; else m.paneIdx = (m.paneIdx + 1) % paneCount; m.viewport = m.readFocusedPaneIntoViewport(); return m, nil. The dispatcher synchronously calls m.reader.Tail(m.currentPaneKey()), translates the three (bytes, err) shapes to viewport content, and calls vp.GotoBottom(). currentPaneKey uses raw tmux WindowIndex / PaneIndices via state.SanitizePaneKey. windowIdx is never assigned in the Tab branch.

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_tab_test.go
- Coverage:
  - AdvancesPaneIdxByOneWithinMultiPaneWindow.
  - WrapsFromLastPaneBackToZeroWithinSameWindow.
  - SinglePaneWindowIsSilentNoOpZeroTail (multi-window session, single-pane focused window).
  - SingleWindowSinglePaneSessionIsSilentNoOp.
  - TriggersExactlyOneTailCallWithNewlyFocusedPaneKey (uses non-contiguous raw indices WindowIndex=2, PaneIndices=[4,7,9]; asserts paneKey equals state.SanitizePaneKey("work", 2, 7)).
  - ResetsViewportScrollPositionToTail (preloads stale content, GotoTop, asserts AtBottom() after Tab).
  - DoesNotModifyWindowIdx.
  - InterceptedBeforeViewportSeesIt (bonus precedence assertion).
- Notes: Tests bypass NewPreviewModel via unexported newPreviewModelForTab. Reasonable scope.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Single shared (bytes, err) dispatcher across constructor / Tab / ] / [ avoids three-way drift.
- Complexity: Low. Tab branch is 5 lines plus a guard.
- Modern idioms: Uses Go modulo wrap idiom.
- Readability: Doc comments on dispatcher and chrome reference spec sections by name.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The Tab handler uses a local paneCount <= 1 guard rather than calling the degenerate() helper from task 3-1. Functionally equivalent (single-pane current window subsumes the degenerate case here), but the plan rationale referenced degenerate() explicitly. Current choice is defensible — the local guard checks exactly the axis Tab cares about.
