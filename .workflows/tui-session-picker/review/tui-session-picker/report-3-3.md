TASK: Unify duplicated modal dispatch into single method (tick-6fac0d)

ACCEPTANCE CRITERIA:
- Only one modal dispatch method exists (`updateModal`)
- `updateProjectModal` is deleted
- The unified method handles all four modal states: `modalKillConfirm`, `modalRename`, `modalDeleteProject`, `modalEditProject`
- All existing tests pass

STATUS: Complete

SPEC CONTEXT: The spec defines four modal types (kill confirmation, rename, project edit, delete confirmation) all using a single reusable modal overlay pattern. The implementation should have one dispatch point for modal input routing, consistent with "All key input routes to the modal while it's active."

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tui/model.go:974-992 (unified `updateModal`)
- Notes:
  - `updateProjectModal` is fully deleted -- grep confirms zero occurrences across the entire `internal/tui` directory
  - Single `updateModal` method at line 974 handles all four modal states via switch: `modalKillConfirm`, `modalRename`, `modalDeleteProject`, `modalEditProject`
  - Both `updateSessionList` (line 915) and `updateProjectsPage` (line 624) delegate to `updateModal` when `m.modal != modalNone`
  - Ctrl+C guard appears exactly once in the unified method (line 976), force-quitting from any modal state
  - Adding a new modal type requires only a single case addition in the switch at line 980

TESTS:
- Status: Adequate
- Coverage:
  - Kill modal tests: open, confirm (y), dismiss (n/Esc), other keys ignored -- all pass through unified dispatch
  - Rename modal tests: open, confirm (Enter), dismiss (Esc), empty name rejection -- all pass through unified dispatch
  - Delete project modal tests: open, confirm (y), dismiss (n/Esc), other keys ignored -- all pass through unified dispatch
  - Edit project modal tests: open, name edit, alias management, confirm, dismiss (Esc) -- all pass through unified dispatch
  - Ctrl+C force-quit tested during kill modal and rename modal (lines 4536-4570)
  - Ctrl+C on projects page (no modal) tested at line 3646
- Notes: No explicit test for Ctrl+C during delete project modal or edit project modal. This is a minor gap -- the unified `updateModal` Ctrl+C guard at line 976 covers all modal states uniformly, so the existing kill/rename modal Ctrl+C tests implicitly validate the pattern. Non-blocking.

CODE QUALITY:
- Project conventions: Followed -- idiomatic Go, method receiver consistency (value receiver `Model`)
- SOLID principles: Good -- Single Responsibility improved by unifying dispatch; Open/Closed improved since new modals need only one case addition
- Complexity: Low -- the unified `updateModal` is a clean switch with 4 cases plus a Ctrl+C guard, cyclomatic complexity is minimal
- Modern idioms: Yes -- standard Go switch dispatch pattern
- Readability: Good -- the method is self-documenting, clear separation between dispatch and individual modal handlers
- Issues: None

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- Consider adding Ctrl+C tests for delete project modal and edit project modal to match the coverage of kill/rename modals, though the unified dispatch makes this low risk
