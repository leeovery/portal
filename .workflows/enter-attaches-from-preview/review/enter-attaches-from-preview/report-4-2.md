TASK: enter-attaches-from-preview-4-2 — Delete redundant flashModelOnSessionsPage alias

ACCEPTANCE CRITERIA:
- All callers rewritten to `flashModelWithSessions`
- Function definition removed
- Tests pass unchanged

STATUS: Complete

SPEC CONTEXT: Analysis Cycle 2 duplication finding flagged `flashModelOnSessionsPage` as a one-line wrapper around `flashModelWithSessions` — an indirection hop with no disambiguation benefit. Solution: delete the alias and update callers.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/sessions_flash_render_test.go:33-45 — single canonical definition of `flashModelWithSessions`
  - internal/tui/sessions_flash_clear_test.go — 10 call sites all using `flashModelWithSessions`
  - internal/tui/sessions_flash_render_test.go — 8 call sites all using `flashModelWithSessions`
- Notes: Grep for `flashModelOnSessionsPage` returns zero hits across the codebase. Commit `0322522c` records the deletion.

TESTS:
- Status: Adequate (refactor task — no new tests required)
- Coverage: Pre-existing flash tests continue to exercise the helper.

CODE QUALITY:
- Project conventions: Followed.
- SOLID principles: Good — removes a needless indirection.
- Complexity: Low.
- Modern idioms: N/A.
- Readability: Improved.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
