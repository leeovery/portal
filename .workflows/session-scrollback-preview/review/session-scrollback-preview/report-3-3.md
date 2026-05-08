TASK: session-scrollback-preview-3-3 — Bracket cycles: next/previous window with pane-0 reset and re-read

ACCEPTANCE CRITERIA:
- ] advances windowIdx by 1 mod len(groups) and resets paneIdx to 0; one Tail call issued.
- [ rewinds windowIdx by 1 mod len(groups) (wrapping correctly for windowIdx == 0 → last) and resets paneIdx to 0; one Tail call issued.
- After window cycle, paneIdx is 0 even if non-zero before.
- In single-window session both ] and [ are silent no-ops regardless of pane count.
- After cycling, viewport is at tail (AtBottom() true).
- Reading is synchronous in Update; no tea.Cmd returned for read I/O.

STATUS: Complete

SPEC CONTEXT:
Per § Multi-pane Rendering Shape > Pane focus on window cycle: "After ] or [, the focused pane within the new window resets to pane 0... Per-window pane focus is not preserved across window cycles. Tab then cycles forward from pane 0." Per § Within-preview Key Bindings: ] Next window (wraps from last → first); [ Previous window (wraps from first → last). Per § Multi-pane Rendering Shape > Degenerate cases: single-window single-pane all three cycle keys silently no-op.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview.go:281-306 (] and [ branches)
- Notes: Both branches advance/rewind windowIdx with non-negative wrap, reset paneIdx=0, and call the shared readFocusedPaneIntoViewport dispatcher (which calls reader.Tail synchronously then GotoBottom). Single-window session (len(groups) <= 1) silent no-ops regardless of pane count.

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_bracket_test.go
- Coverage: All 9 acceptance criteria including wrap directions, paneIdx reset, single-window no-op (regardless of pane count), single Tail call with raw-index-derived paneKey, and viewport AtBottom after cycle.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good.
- Complexity: Low. Each branch is ~5 lines.
- Modern idioms: Idiomatic Go modulo wrap with non-negative guard.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None of consequence.
