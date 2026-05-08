TASK: session-scrollback-preview-3-5 — Chrome rendering: counters, window name, and keystroke hints

ACCEPTANCE CRITERIA:
- chromeLine() returns "Window {wOrdinal} of {wTotal}" with wOrdinal = windowIdx + 1, wTotal = len(groups).
- "Pane {pOrdinal} of {pTotal}" with pOrdinal = paneIdx + 1, pTotal = len(currentGroup().PaneIndices).
- Window name (currentGroup().WindowName) verbatim.
- Visible cycle-key hints: ], [, Tab, Esc each appear textually.
- Counters never expose raw tmux WindowIndex or PaneIndices[i] values.
- Pure (no I/O).
- No liveness language.

STATUS: Complete

SPEC CONTEXT:
Per § Multi-pane Rendering Shape > Chrome Floor: must show "Window M of N", "Pane X of Y", window name (#W), and keystroke hints (] [ Tab Esc). Per Counter semantics: "M and X ... are 1-based ordinal positions in enumeration order, not the tmux window_index / pane_index values". Per task 5-3, the #W: prefix has been dropped.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview.go:163-173
- Notes:
  - Format string: "Window %d of %d · Pane %d of %d · %s    ] [ Tab Esc" (no #W: prefix per task 5-3).
  - 1-based ordinals: wOrdinal = m.windowIdx + 1, pOrdinal = m.paneIdx + 1.
  - Window name verbatim from m.currentGroup().WindowName.
  - Visible cycle hints: "] [ Tab Esc".
  - Pure: no I/O, just fmt.Sprintf.
  - No liveness language.

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_chrome_test.go
- Coverage: 1-based ordinals for 0-indexed groups; non-contiguous WindowIndex (0,2,5); pane-base-index 1; window name with whitespace; window name with pipe; visible cycle hints; purity (no I/O); no-liveness substring guard.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Pure function, single responsibility.
- Complexity: Low.
- Modern idioms: Idiomatic Go fmt.Sprintf.
- Readability: Clear comments referencing spec sections.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] chromeLine implementation does not use lipgloss styling. Task said styling is optional; deferred is fine.
