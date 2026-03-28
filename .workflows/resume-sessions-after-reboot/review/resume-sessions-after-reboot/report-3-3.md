TASK: Lazy Cleanup in Execution Flow

ACCEPTANCE CRITERIA:
- Cleanup calls ListAllPanes and CleanStale before hook execution
- ListAllPanes error does not block hook execution
- CleanStale error does not block hook execution
- All existing ExecuteHooks tests pass with no-op mocks
- All callers updated
- All tests pass: `go test ./internal/hooks/...` and `go test ./cmd/...`

STATUS: Complete

SPEC CONTEXT: The spec says "When Portal reads hooks (during portal open), cross-reference pane IDs against live tmux panes. Prune entries for panes that don't exist. Invisible to the user." This task adds lazy cleanup at the start of ExecuteHooks, mirroring the TUI's lazy cleanup pattern for project store.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/hooks/executor.go:26-33 (AllPaneLister and HookCleaner interfaces), :64-68 (cleanup logic in ExecuteHooks)
- Notes: Implementation adds `AllPaneLister` and `HookCleaner` interfaces (lines 27-33). These are composed into `TmuxOperator` (line 37-42) and `HookRepository` (line 44-48) respectively, which was done as part of Phase 4 Task 4 refactoring. The cleanup logic is at lines 66-68: calls `ListAllPanes`, on success calls `CleanStale`, both with errors silently ignored. This runs before `store.Load()` at line 70, matching the spec's requirement.
- Caller `buildHookExecutor` in /Users/leeovery/Code/portal/cmd/hook_executor.go:13-21 passes `*tmux.Client` (satisfies TmuxOperator including AllPaneLister) and `*hooks.Store` (satisfies HookRepository including HookCleaner). All three connection paths (attach, TUI, direct path) use `buildHookExecutor`.
- No drift from plan. The composed interface approach is a later refinement (Phase 4 Task 4) but the underlying behavior matches task 3-3 exactly.

TESTS:
- Status: Adequate
- Coverage: All 5 required tests are present in TestExecuteHooks_Cleanup (executor_test.go:407-569):
  1. "cleanup calls ListAllPanes and CleanStale before hook execution" -- verifies both mocks called, pane IDs forwarded, hook execution proceeds
  2. "ListAllPanes error skips cleanup and continues" -- verifies CleanStale NOT called, hook execution proceeds
  3. "CleanStale error skips cleanup and continues" -- verifies CleanStale called, hook execution proceeds despite error
  4. "cleanup runs before loader.Load" -- verifies both cleanup steps called; ordering verified structurally
  5. "no tmux server running skips cleanup gracefully" -- verifies CleanStale called with empty list, hook execution proceeds
- All 10 existing TestExecuteHooks tests use noopTmux()/noopStore() which include no-op AllPaneLister and HookCleaner mocks
- cmd-level tests are unaffected because HookExecutorFunc signature (func(sessionName string)) is unchanged; only internal wiring changed
- Notes: The "cleanup runs before loader.Load" test (lines 501-535) declares `var callOrder []string` but never populates it via callback-based sequencing. The test acknowledges this in comments and relies on structural verification. This is a minor weakness but acceptable given the simple linear code structure.

CODE QUALITY:
- Project conventions: Followed. Uses small interfaces (1 method each for AllPaneLister and HookCleaner), composed into larger interfaces via embedding. DI pattern matches existing codebase conventions.
- SOLID principles: Good. Interface segregation maintained -- AllPaneLister and HookCleaner are separate, composed into TmuxOperator and HookRepository only for convenience. Single responsibility preserved in ExecuteHooks.
- Complexity: Low. Cleanup is 3 lines with clear control flow: call ListAllPanes, on success call CleanStale, then proceed to normal execution.
- Modern idioms: Yes. Uses idiomatic `_, _ =` for intentionally discarded returns. `if err == nil` guard for best-effort pattern.
- Readability: Good. Clear comment "Best-effort cleanup: prune stale hook entries before loading." Intent is immediately obvious.
- Issues: None.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The "cleanup runs before loader.Load" test could be strengthened with callback-based mock sequencing to definitively prove ordering, rather than relying on structural reasoning. The existing approach is sufficient but not rigorous.
- The unused `callOrder` variable in the ordering test (line 503) is dead code that should be removed for cleanliness.
