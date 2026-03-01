TASK: Extract rune-key matching helper (tick-d6ffad)

ACCEPTANCE CRITERIA:
- No remaining instances of the verbose rune-key pattern in model.go
- `isRuneKey` helper exists as a package-private function in model.go
- All existing tests pass without modification

STATUS: Complete

SPEC CONTEXT: The TUI uses rune-key matching extensively across four update functions (updateProjectsPage, updateSessionList, updateKillConfirmModal, updateDeleteProjectModal) to dispatch on single-character key presses like "q", "k", "r", "n", "p", "x", "s", "e", "d", "b", "y". This is a pure DRY refactor -- no behavioral change.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tui/model.go:414-417
- Notes: Helper function `isRuneKey(msg tea.KeyMsg, ch string) bool` is defined as package-private (lowercase). It encapsulates the 3-part expression `msg.Type == tea.KeyRunes && string(msg.Runes) == ch`. All 17 call sites across 4 functions (updateProjectsPage: 7, updateDeleteProjectModal: 2, updateSessionList: 6, updateKillConfirmModal: 2) now use the helper. The only remaining instance of the verbose pattern is inside the helper itself, which is correct. One inline `text == "x"` in updateEditProjectModal (line 824) is correctly left as-is because it operates inside a `case tea.KeyRunes:` switch arm where the type is already asserted -- a different pattern than what this task targets.

TESTS:
- Status: Adequate
- Coverage: This is a pure refactor with no behavioral change. The task explicitly states no new tests are needed. The existing test suite contains 36 test functions covering all key-handling paths (q, k, r, n, p, x, s, e, d, b, y across sessions, projects, modals, and command-pending mode). Tests construct real `tea.KeyMsg` values directly, which is correct -- they test behavior, not implementation details of the helper.
- Notes: No issues. Tests exercise the same code paths as before; the refactor is transparent to them.

CODE QUALITY:
- Project conventions: Followed. Package-private function, Go doc comment using "reports whether" convention per Go style.
- SOLID principles: Good. Single responsibility -- one helper doing one thing.
- Complexity: Low. Single-expression function body.
- Modern idioms: Yes. Idiomatic Go helper extraction.
- Readability: Good. `isRuneKey(msg, "q")` is significantly more scannable than the verbose 3-part expression. Function name clearly conveys intent.
- Issues: None.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- (none)
