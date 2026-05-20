TASK: Update stale portalSaverVersionMismatch references in integration-test doc comments

ACCEPTANCE CRITERIA: All references to portalSaverVersionMismatch in integration-test doc comments updated to reflect the unified predicate (shouldKillSaverOnVersionDecision).

STATUS: Complete

SPEC CONTEXT: Phase 3 task 3-1 collapsed shouldKillSaverOnVersionDecision + portalSaverVersionMismatch into a single unified predicate named shouldKillSaverOnVersionDecision. Phase 4 cycle-2 cleanup task 4-1 catches stale doc-comment references to the now-removed portalSaverVersionMismatch name in integration-test files.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmux/portal_saver_integration_test.go:105 — "the production shouldKillSaverOnVersionDecision comparison"
  - internal/tmux/portal_saver_integration_test.go:170 — "This exercises the real shouldKillSaverOnVersionDecision comparison"
- Notes: Codebase-wide grep for portalSaverVersionMismatch across **/*.go returns zero matches. Only historic references exist in .workflows/.inbox/.archived/ and prior review reports (by-design artefacts). Narrative phrase "version mismatch" remains in comments at lines 21, 104, 169, 172 — these describe behaviour, not function names, so correct as-is. cmd/state_daemon_integration_test.go had no references to update.

TESTS:
- Status: N/A (doc-comment-only task)
- Coverage: No code paths changed; existing portal_saver_test.go table tests (line 1894+) exhaustively cover shouldKillSaverOnVersionDecision.

CODE QUALITY:
- Project conventions: Followed
- SOLID/Complexity/Idioms: N/A (doc-only)
- Readability: Good — comments now name a real symbol the reader can grep for.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
