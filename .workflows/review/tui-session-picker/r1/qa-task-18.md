TASK: Command-Pending Mode Core (tick-310db8)

ACCEPTANCE CRITERIA:
- WithCommand sets commandPending = true and starts on Projects page
- Pressing s in command-pending mode does nothing
- Pressing x in command-pending mode does nothing
- Pressing e in command-pending mode does nothing
- Pressing d in command-pending mode does nothing
- Help bar does not show s, x, e, or d keybindings in command-pending mode
- Normal mode still has s, x, e, and d keybindings working
- enter label shows "run here" in command-pending mode

STATUS: Complete

SPEC CONTEXT: When `portal open -e cmd` is used, the TUI enters command-pending mode. It is locked to the Projects page -- `s` and `x` keybindings are not registered (pressing them does nothing, they don't appear in help bar). Help bar shows: `[enter] run here  [n] new in cwd  [b] browse  [/] filter  [q] quit`. The spec also mentions `e` and `d` should be disabled (they are project edit/delete keys that don't apply in command-pending mode).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `/Users/leeovery/Code/portal/internal/tui/model.go:129-130` -- `command []string` and `commandPending bool` fields
  - `/Users/leeovery/Code/portal/internal/tui/model.go:267-276` -- `WithCommand()` sets `commandPending=true`, `activePage=PageProjects`, swaps help keys
  - `/Users/leeovery/Code/portal/internal/tui/model.go:390-399` -- `commandPendingHelpKeys()` returns only enter/b/n/q bindings
  - `/Users/leeovery/Code/portal/internal/tui/model.go:644-666` -- Guards in `updateProjectsPage` for s, x, e, d keys when `commandPending` is true
- Notes: Implementation matches all acceptance criteria. The `WithCommand` function uses a value receiver and returns a copy (consistent with `WithInitialFilter` and `WithInsideTmux`). The guard pattern (`if m.commandPending { return m, nil }`) is applied consistently to all four restricted keys. The help key swap in `WithCommand` replaces the project list's `AdditionalShortHelpKeys` and `AdditionalFullHelpKeys` with command-pending-specific bindings that omit s, x, e, d and change "new session" to "run here".

TESTS:
- Status: Adequate
- Coverage:
  - "command-pending mode starts in project picker view" (line 2140) -- verifies WithCommand + Init loads projects, lands on Projects page
  - "pressing s in command-pending mode does nothing" (line 2495) -- verifies ActivePage stays PageProjects after pressing s
  - "pressing x in command-pending mode does nothing" (line 2531) -- verifies ActivePage stays PageProjects after pressing x
  - "pressing e in command-pending mode does nothing" (line 2561) -- verifies no edit modal appears after pressing e
  - "pressing d in command-pending mode does nothing" (line 2593) -- verifies no delete modal appears after pressing d
  - "help bar omits s, x, e, and d in command-pending mode" (line 2621) -- verifies "sessions", "edit", "delete" absent; "browse", "new in cwd" present
  - "help bar shows run here for enter in command-pending mode" (line 2657) -- verifies "run here" present and "new session" absent
  - "normal mode retains s, x, e, and d keybindings" (line 2687) -- verifies help bar contains "sessions", "edit", "delete", "new session" in normal mode
- Notes: All 8 tests from the task plan exist. Tests verify behavior (view output and page state) not implementation details. The "normal mode retains" test only checks help bar text, not actual key behavior, but key behavior in normal mode is thoroughly tested by other tasks (Phase 2 tasks for s, e, d, x). No over-testing detected -- each test covers a distinct acceptance criterion.

CODE QUALITY:
- Project conventions: Followed. Uses functional options for configuration (`WithCommand`), value-receiver copy pattern consistent with other `With*` methods, table-less subtests appropriate here since each test has unique setup.
- SOLID principles: Good. `commandPendingHelpKeys` is a separate function from `projectHelpKeys`, keeping help key configuration clean. The guard checks in `updateProjectsPage` are simple conditional returns -- single responsibility maintained.
- Complexity: Low. Each guard is a simple `if m.commandPending { return m, nil }` check. No nested conditions or complex branching added.
- Modern idioms: Yes. Uses `key.Binding` and `bubbles/list` help key system properly. The `AdditionalShortHelpKeys`/`AdditionalFullHelpKeys` function swapping is the idiomatic way to change help bar content in bubbles/list.
- Readability: Good. The `commandPendingHelpKeys` function is clearly documented with a comment explaining which keys are shown vs omitted. Guard checks are inline and obvious.
- Issues: None.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The four guard clauses for s, x, e, d in `updateProjectsPage` (lines 644-666) follow a repetitive pattern. A minor DRY opportunity exists to extract a helper like `if m.commandPending && isRuneKey(msg, "s", "x", "e", "d")`, but the current approach is clear and each key has distinct non-command-pending behavior, making the individual cases appropriate.
- The "normal mode retains" test only verifies help bar presence, not that the keys actually function. This is acceptable since functional key tests exist in Phase 2 task tests, but a comment in the test noting this would improve clarity.
