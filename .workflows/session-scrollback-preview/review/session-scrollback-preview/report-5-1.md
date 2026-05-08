TASK: session-scrollback-preview-5-1 — Extract applySessions helper to deduplicate session-list refresh

ACCEPTANCE CRITERIA:
- Both handlers compile and produce identical observable behaviour.
- The four-step sequence appears only once in the file.
- The inside-tmux title rewrite remains at the SessionsMsg call site.

STATUS: Complete

SPEC CONTEXT:
Analysis cycle 1 finding: SessionsMsg handler and previewSessionsRefreshedMsg handler both ran the same four-step sequence (assign sessions, compute filteredSessions, call sessionList.SetItems, conditional SetSize). Solution: extract (*Model).applySessions(sessions) colocated with filteredSessions.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:618-626 — applySessions defined immediately after filteredSessions.
  - internal/tui/model.go:819 — called from SessionsMsg.
  - internal/tui/model.go:908 — called from previewSessionsRefreshedMsg.
- Notes:
  - The four-step sequence appears exactly once.
  - The inside-tmux title rewrite remains at the SessionsMsg call site (model.go:821-823).

TESTS:
- Status: Adequate (existing tests pass without modification).

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good — DRY without over-abstraction.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None of consequence.
