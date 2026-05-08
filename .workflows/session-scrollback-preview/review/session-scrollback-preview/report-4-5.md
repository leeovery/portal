TASK: session-scrollback-preview-4-5 — Sessions-list re-fetch on pagePreview → pageSessions transition

ACCEPTANCE CRITERIA:
- Audit recorded.
- After Esc from preview, Sessions list reflects current live tmux state via fresh enumeration.
- Externally-killed session does not appear in post-dismiss list.
- When previously-selected session still exists, cursor remains on it.
- When previously-selected session is gone, cursor lands on a valid neighbouring entry.
- Refresh is observably a no-op when nothing changed.
- Filter state preserved across the dismiss-with-refresh transition.

STATUS: Complete

SPEC CONTEXT:
Spec § Cross-cutting Seams > Externally-Killed Session During Preview: "Esc back to list. The Sessions list re-fetches the live session list on return — the killed session simply isn't there anymore. Cursor lands on a neighbouring session via bubbles/list's default behaviour." Re-fetch contract: "Preview owns the re-fetch on the pagePreview → pageSessions transition."

IMPLEMENTATION:
- Status: Implemented correctly with audit-documented gap fix.
- Location:
  - internal/tui/model.go:618-626 — applySessions shared helper.
  - internal/tui/model.go:668-689 — refreshSessionsAfterPreviewCmd.
  - internal/tui/model.go:691-719 — reanchorSessionCursor.
  - internal/tui/model.go:876-910 — previewDismissedMsg + previewSessionsRefreshedMsg handlers.
  - internal/tui/pagepreview.go:217-239 — message types.
  - internal/tui/pagepreview.go:262-263 — Esc → previewDismissedMsg.
- Notes:
  - Audit recorded inline in test file documents "GAP" (no pre-existing on-entry refresh on Sessions page); resolution is a tea.Cmd dispatched from previewDismissedMsg handler.
  - preserveName := m.preview.session captured BEFORE m.preview = previewModel{} (model.go:894-896), with explicit comment warning against re-ordering.
  - Lister errors in refresh handler (model.go:905-907) silently swallowed; user lands on PageSessions with pre-refresh list intact.

TESTS:
- Status: Adequate, focused, not over-tested.
- Location: internal/tui/pagepreview_refetch_test.go
- Coverage: 7 test cases covering refetch, killed-session, cursor-preserve, cursor-fallback, no-op, filter-preserve, lister-error.
- pressSpaceThenEscWithRefresh helper drives the full Update→Cmd→msg→Update round-trip.
- Stateful stepListerStub models externally-killed-session-during-preview by emitting different lists per call.
- Filter preservation test sets FilterApplied state pre-Space and asserts identical state post-dismiss (works because applySessions only calls SetItems).

CODE QUALITY:
- SOLID — refreshSessionsAfterPreviewCmd and reanchorSessionCursor are single-purpose; applySessions removes duplication.
- Complexity: Low.
- Modern Bubble Tea idiom (msg → cmd → msg).
- Comments explain non-obvious ordering and policy decisions.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] refreshSessionsAfterPreviewCmd returns nil when sessionLister is unset (test-tolerance only). Production always wires it; consider construction-time non-nil assertion or explicit "test-only path" doc.
- [idea] reanchorSessionCursor clamps to len(visible) - 1 when previous name is missing. Spec says "neighbour"; last-index clamp is a valid interpretation but could land far from the original cursor. Refinement is additive.
