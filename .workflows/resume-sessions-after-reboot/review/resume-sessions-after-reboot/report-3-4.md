TASK: Extend Clean Command with Hook Cleanup

ACCEPTANCE CRITERIA:
- Prints stale hook removal messages alongside project removals
- No hooks file produces no hook output
- No tmux server skips hook cleanup silently
- All panes live produces no hook output
- Existing project cleanup unchanged
- `cleanDeps` injectable for testing
- All tests pass: `go test ./cmd -run TestClean`

STATUS: Complete

SPEC CONTEXT: The spec's "Stale Registration Cleanup" section says: "Adding hook cleanup to xctl clean is a natural fit -- it already says 'remove stale projects whose directories no longer exist.' Extending to 'remove hook entries for panes that no longer exist' is semantically identical." The clean command is in `skipTmuxCheck` so it does not require a running tmux server. When no server is running, hooks should be preserved (the user may start a server later).

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/clean.go:14-29 (CleanDeps struct and DI helper), cmd/clean.go:53-94 (hook cleanup logic in RunE)
- Notes:
  - The implementation correctly follows the plan's logic: load hook store, check for existing hooks, query live panes, skip if no live panes (server not running), call CleanStale, print removals.
  - Minor deviation: The plan specified defining a local `cleanPaneLister` interface in `cmd/clean.go`. Instead, the implementation uses `hooks.AllPaneLister` (defined in `internal/hooks/executor.go:27`). This is pragmatic since `cmd/clean.go` already imports `hooks` for the store, and avoids interface duplication. Not a concern.
  - Minor deviation: The plan's "Outcome" section says the format should be `Removed stale hook: %3 (on-resume)` (with event type), but the acceptance criteria says `Removed stale hook: %3\n` (without event type). The implementation follows the acceptance criteria format. This is correct since per-pane cleanup removes the entire pane entry, not individual event types.
  - The flow correctly handles: (1) no hooks file -> empty map -> early return, (2) no tmux server -> empty panes -> early return preserving hooks, (3) ListAllPanes error -> early return, (4) CleanStale error -> returned to caller, (5) normal cleanup with removal messages.

TESTS:
- Status: Adequate
- Coverage:
  - "removes stale hooks and prints removal messages" (line 276) -- verifies stale hook removal output and file state
  - "no tmux server running skips hook cleanup preserving existing hooks" (line 319) -- verifies hooks are preserved when no server running
  - "hooks file missing produces no hook removal output" (line 358) -- verifies no output when hooks file absent
  - "all hooks panes still live produces no hook removal output" (line 386) -- verifies no output when all hooks are live
  - "both project and hook removals printed together" (line 418) -- verifies combined output ordering
  - All 7 existing project-only tests preserved unchanged (lines 12-274)
- Missing from plan's test list but implicitly covered:
  - "only hooks removed when no stale projects" -- covered by "removes stale hooks" test (line 276, empty projects file + stale hooks)
  - "only projects removed when no stale hooks" -- covered by existing project tests which don't set PORTAL_HOOKS_FILE (hooks file won't exist at default path in test env)
- Test helpers `writeCleanHooksJSON` and `readCleanHooksJSON` (lines 470-493) are well-structured with `t.Helper()` calls
- Mock `mockCleanPaneLister` (lines 460-467) is minimal and appropriate
- Tests properly use `t.Cleanup` to reset `cleanDeps` after each test

CODE QUALITY:
- Project conventions: Followed. DI pattern with `CleanDeps` struct + package-level var + nil-check helper matches `BootstrapDeps`, `HooksDeps`, `OpenDeps`, etc. Uses `t.Setenv` for config path isolation.
- SOLID principles: Good. Single responsibility maintained -- clean command handles both project and hook cleanup which is its defined scope. Dependency inversion via `hooks.AllPaneLister` interface.
- Complexity: Low. Linear flow with clear early-return guards. No nested loops or complex branching.
- Modern idioms: Yes. Uses standard Go patterns (error handling, interface satisfaction, test helpers).
- Readability: Good. Comments explain the rationale for skipping cleanup when no server is running (lines 77-79). Variable names are clear.
- Issues: None significant.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- Existing project-only clean tests (lines 12-274) do not set `PORTAL_HOOKS_FILE`, so they rely on the default `~/.config/portal/hooks.json` not existing (or no tmux server running). This is a minor isolation gap -- if a developer has a real hooks.json with stale entries and a running tmux server, these tests could produce unexpected hook removal output. Adding `t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(t.TempDir(), "hooks.json"))` to existing tests would make them fully hermetic. Low risk in practice since CI environments won't have this file.
- The plan's "Outcome" section says format `Removed stale hook: %3 (on-resume)` but the acceptance criteria and implementation use `Removed stale hook: %3`. The simpler format is arguably better since CleanStale operates at the pane level, not the event level. No change needed.
