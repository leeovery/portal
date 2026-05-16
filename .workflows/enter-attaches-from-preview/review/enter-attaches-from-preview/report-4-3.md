TASK: enter-attaches-from-preview-4-3 — Extract singlePaneGroups() test helper for repeated stubEnumerator fixture

ACCEPTANCE CRITERIA:
- 21 occurrences across 3 test files replaced
- Literal appears at most once outside helper
- Tests pass unchanged

STATUS: Complete

SPEC CONTEXT: Cycle 2 duplication analysis flagged the literal `&stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}}` appearing 21 times across preview_attach_bail_test.go (8), preview_attach_bail_flash_test.go (10), preview_attach_selected_test.go (3).

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/preview_attach_test.go:16-22
- Notes: Helper named `newSinglePaneEnumerator()` rather than the plan's `singlePaneGroups()`. The constructor-style name is more idiomatic Go for a function returning a fully-built `*stubEnumerator` (vs returning bare `[]tmux.WindowGroup`). Recent commit message "extract newSinglePaneEnumerator test helper" matches the chosen identifier. Acceptance criteria target outcomes (call-site count, single-literal invariant, green tests) rather than the exact helper name, so this naming drift is acceptable.

TESTS:
- Status: Adequate
- Coverage: All 21 call sites in the three target files now invoke `newSinglePaneEnumerator()`; no inline `stubEnumerator{...}` literals remain across the three target files outside their use of the helper return value.
- Notes: The single remaining occurrence of the full literal is line 21 of preview_attach_test.go (the helper body itself), satisfying "at most once outside helper".

CODE QUALITY:
- Project conventions: Followed.
- SOLID principles: N/A.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Helper name diverges from plan task title (`singlePaneGroups()` vs `newSinglePaneEnumerator()`). The chosen name is arguably better, but if the planning system tracks helper names verbatim downstream, a one-line note in the implementation log would tighten plan-to-impl traceability.
