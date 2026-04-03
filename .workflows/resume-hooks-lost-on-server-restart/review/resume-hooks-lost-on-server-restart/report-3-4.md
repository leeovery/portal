TASK: Migrate clean Command Tests to Structural Keys

ACCEPTANCE CRITERIA:
- All TestCleanCommand tests use structural key values in mock data
- All marker-related assertions use @portal-active-session:window.pane format
- No production code changes to cmd/clean.go
- go test ./cmd/ -run TestClean passes

STATUS: Complete

SPEC CONTEXT: The structural key migration (session_name:window_index.pane_index) replaces pane IDs (%N) throughout the codebase. The clean command calls `hookStore.CleanStale(livePanes)` where livePanes now contains structural keys from `ListAllPanes()`. Test mocks must supply structural keys to match the new data model. The clean command itself has no marker-related code; it only performs hook store cleanup via CleanStale.

IMPLEMENTATION:
- Status: Implemented (test-only changes as specified)
- Location: /Users/leeovery/Code/portal/cmd/clean_test.go
- Notes: No production code changes to cmd/clean.go confirmed. The production code is key-format-agnostic (CleanStale takes []string and does set membership), so no changes were needed. All 10 test subtests in TestCleanCommand use structural key values in mock data (e.g., "my-session:0.0", "other-session:1.0", "my-session:0.1", "other-session:1.1"). No pane ID values (%N format) remain anywhere in the file.

TESTS:
- Status: Adequate
- Coverage:
  - "removes stale hooks and prints removal messages" (line 282): uses structural keys in hooks JSON and mock pane list, asserts structural key in removal output
  - "no tmux server running skips hook cleanup preserving existing hooks" (line 325): uses structural key "my-session:0.1" with empty pane list, verifies hooks preserved
  - "hooks file missing produces no hook removal output" (line 364): uses structural key in mock pane list
  - "all hooks panes still live produces no hook removal output" (line 392): uses structural keys for both hooks and mock panes
  - "both project and hook removals printed together" (line 424): uses structural key "other-session:1.1" for stale hook
  - Earlier subtests (lines 11-280) test project cleanup only, unaffected by this migration
- Notes: The plan expected a test named "removes orphaned marker options" but the clean command has no marker/server-option functionality. Volatile markers (@portal-active-*) are tmux server options that naturally disappear on server restart. The acceptance criterion about marker-related assertions is vacuously satisfied since clean_test.go has no marker assertions to update. The test names differ slightly from the plan's expected names but the semantic coverage matches.

CODE QUALITY:
- Project conventions: Followed. Uses the standard DI pattern (package-level cleanDeps struct with t.Cleanup restoration). No t.Parallel() as required by CLAUDE.md.
- SOLID principles: Good. mockCleanPaneLister implements AllPaneLister interface cleanly.
- Complexity: Low. Test cases are straightforward with clear setup/execute/assert structure.
- Modern idioms: Yes. Uses t.TempDir(), t.Setenv(), t.Helper() in helpers.
- Readability: Good. Test names clearly describe scenarios. Structural key values are realistic (session-like names with window.pane indices).
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The plan's expected test "removes orphaned marker options" does not exist because the clean command has no marker cleanup logic. This is a plan inaccuracy, not a code deficiency. Marker cleanup is not the clean command's responsibility.
- Variable name `paneID` on clean.go:88 is a residual from the old naming but was intentionally left for task 4-3 (rename residual parameter names), which has already been completed in commit b233d20.
