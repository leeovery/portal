TASK: session-scrollback-preview-5-3 — Drop #W: prefix from preview chrome

ACCEPTANCE CRITERIA:
- Rendered chrome no longer contains substring "#W:".
- Window name still appears, separated from pane counter by " · ".
- All preview tests pass after assertion updates.

STATUS: Complete

SPEC CONTEXT:
Analysis cycle 1 finding: chrome format string embedded "#W:" as a user-facing label. In tmux, #W is a format-code reference, not user-facing text. Users unfamiliar with tmux read "#W:" as opaque.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview.go:163-173
- Notes: Format string is now "Window %d of %d · Pane %d of %d · %s    ] [ Tab Esc". Helper stays pure (no I/O), value receiver, doc comment still accurate.

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_chrome_test.go
- Coverage: 1-based ordinals, non-contiguous window indices, pane-base-index 1, verbatim window name (incl. space and pipe), hint tokens, no raw-index leakage, no liveness wording, no I/O, and 1x1 edge case. No assertion referenced "#W:" so no churn was needed beyond the format change.

CODE QUALITY:
- SOLID/DRY/readability all clean; complexity trivial.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Could add an explicit negative assertion !strings.Contains(got, "#W:") in pagepreview_chrome_test.go to pin the cleanup against future regressions.
