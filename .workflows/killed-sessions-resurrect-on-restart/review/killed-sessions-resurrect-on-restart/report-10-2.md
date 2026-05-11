TASK: killed-sessions-resurrect-on-restart-10-2 — Migrate the last inline OpenLogger preamble in exit_closes_pane_integration_test.go to restoretest.OpenTestLogger

ACCEPTANCE CRITERIA:
- Replace inline state.OpenLogger preamble with restoretest.OpenTestLogger(t, stateDir).
- restoretest import already present, reused without drift.
- Preserve path/filepath and internal/state imports for other call sites.
- Mechanical refactor verified by existing setupExitClosesPane-exercising tests.

STATUS: Complete

SPEC CONTEXT: Phase 10 post-implementation cleanup/dedup. Cycle 7 surfaced that exit_closes_pane_integration_test.go still carried an inline OpenLogger preamble; all twelve cmd/bootstrap callers and the cmd/reattach_integration_test.go caller had already been migrated.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/restore/exit_closes_pane_integration_test.go:375
- Notes:
  - setupExitClosesPane now calls `logger := restoretest.OpenTestLogger(t, stateDir)` and passes the result directly into the Orchestrator literal.
  - Three-line preamble collapsed to a single helper call.
  - restoretest import (line 90) reused.
  - path/filepath remains required (lines 161, 302).
  - internal/state remains required (lines 239, 298, 332, 355, 359 — SanitizePaneKey, EnsureDir, CaptureStructure, EncodeIndex, SessionsJSON).
  - Grep confirms zero remaining state.OpenLogger references in this file.

TESTS:
- Status: Adequate
- Coverage: The three setupExitClosesPane-exercising sub-tests flow through line 375. Helper itself unit-covered in Phase 10 cycle 5 (internal/restoretest/logger_test.go).

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. DRY win — preamble previously duplicated across 14 sites now single-sourced.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Six other internal/restore test files still carry inline state.OpenLogger preambles (integration_test.go, integration_full_test.go, session_test.go, session_markers_test.go, session_geometry_test.go, restore_test.go). Out of scope for task 10-2 but represents next obvious dedup batch.
