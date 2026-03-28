TASK: Consolidate Hooks Set/Rm Shared Boilerplate in cmd/hooks.go

ACCEPTANCE CRITERIA:
- requireTmuxPane is the single place that validates TMUX_PANE presence
- Only one code path constructs the fallback tmux.Client for hooks commands
- All existing hooks tests pass without modification (or with minimal mock-setup adjustment)
- No behavioral change: identical CLI behavior, error messages, and exit codes

STATUS: Complete

SPEC CONTEXT: The spec requires `hooks set` and `hooks rm` to validate `$TMUX_PANE` presence, returning error "must be run from inside a tmux pane" when absent. Both commands need a tmux client for volatile marker operations. This task consolidates the duplicated validation and client construction that existed across the two commands.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/hooks.go:32-46 (requireTmuxPane and buildHooksTmuxClient helpers)
- Notes:
  - `requireTmuxPane()` (line 34-40) is the single TMUX_PANE validation point. Only one `os.Getenv("TMUX_PANE")` call exists in production code (line 35).
  - `buildHooksTmuxClient()` (line 44-46) is the single tmux.Client construction site. Only one `tmux.NewClient(&tmux.RealCommander{})` call in the file (line 45).
  - Both `hooksSetCmd.RunE` (line 83) and `hooksRmCmd.RunE` (line 138) call `requireTmuxPane()`.
  - Both commands use the `hooksDeps != nil` pattern to select between injected mock and `buildHooksTmuxClient()` (lines 102-107 and 152-157).
  - The old `buildHooksDeps` and `buildHooksDeleteDeps` functions are completely removed -- no references remain in production code.
  - `HooksDeps` struct (line 27-30) consolidates both interfaces in one struct, with each command using the appropriate field.

TESTS:
- Status: Adequate
- Coverage:
  - TestHooksSetCommand (7 subtests): validates TMUX_PANE reading, missing TMUX_PANE error, flag requirement, idempotent overwrite, JSON structure, volatile marker setting
  - TestHooksRmCommand (7 subtests): validates TMUX_PANE reading, missing TMUX_PANE error, flag requirement, silent no-op, selective removal, volatile marker deletion, pane key cleanup
  - TestHooksListCommand (5 subtests): tab-separated output, empty store, missing file, sorted output, tmux bootstrap bypass, no-args enforcement
  - All tests exercise the consolidated requireTmuxPane helper (both set and rm test the TMUX_PANE-absent case)
  - Tests use the consolidated HooksDeps struct with appropriate interface fields
- Notes: Tests are well-balanced. Each subtest verifies a distinct behavior. No over-testing detected.

CODE QUALITY:
- Project conventions: Followed. Uses the established DI pattern (package-level *Deps struct, nil check for production path, t.Cleanup for restoration). Follows the existing interface segregation pattern (small 1-method interfaces).
- SOLID principles: Good. Single responsibility (requireTmuxPane does one thing, buildHooksTmuxClient does one thing). Interface segregation maintained (ServerOptionSetter and ServerOptionDeleter are separate 1-method interfaces).
- Complexity: Low. Both helpers are trivial. Command functions follow a linear flow with early returns on error.
- Modern idioms: Yes. Standard Go error handling, fmt.Errorf for error construction, clean function signatures.
- Readability: Good. Helper functions are well-named and documented with comments explaining their purpose. The code is self-documenting.
- Issues: None identified.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The `if hooksDeps != nil { use field } else { buildHooksTmuxClient() }` pattern still appears twice (lines 102-107 and 152-157). This is acceptable because each site uses a different interface (setter vs deleter), and consolidating further would add indirection without clear benefit. The analysis task explicitly acknowledged this pattern as the expected outcome.
