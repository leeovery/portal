TASK: CLI bootstrap wait integration

ACCEPTANCE CRITERIA:
- "Starting tmux server..." printed to stderr when serverWasStarted returns true
- No message when serverWasStarted returns false
- WaitForSessions called when serverWasStarted returns true
- WaitForSessions NOT called when serverWasStarted returns false
- list, attach, kill commands call bootstrapWait
- open calls bootstrapWait for non-TUI paths only
- Piping works: status on stderr, output on stdout
- All existing tests continue to pass

STATUS: Complete

SPEC CONTEXT: The spec's "CLI path" under "User Experience" says: "Print a status message to stderr ('Starting tmux server...') and block briefly. Normal command output goes to stdout. Piping works cleanly since the status message is on stderr." The "Two-phase ownership" under "Bootstrap Mechanism" says the TUI path owns its own wait via the Bubble Tea model; the CLI path calls the shared wait function.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/cmd/bootstrap_wait.go:14-22 (bootstrapWait function)
  - /Users/leeovery/Code/portal/cmd/bootstrap_wait.go:27-36 (buildWaiter helper)
  - /Users/leeovery/Code/portal/cmd/list.go:52 (bootstrapWait call)
  - /Users/leeovery/Code/portal/cmd/attach.go:29 (bootstrapWait call)
  - /Users/leeovery/Code/portal/cmd/kill.go:29 (bootstrapWait call)
  - /Users/leeovery/Code/portal/cmd/open.go:89-93 (TUI path skips, non-TUI path calls)
- Notes:
  - The plan specified `bootstrapWait(cmd, waiter)` with a second `waiter func()` parameter and `fmt.Fprintln(os.Stderr, ...)`. The implementation uses `bootstrapWait(cmd)` with waiter injected via `bootstrapDeps.Waiter` and `cmd.ErrOrStderr()` for output. This is a positive deviation: it avoids passing `nil` in every production call site and uses cobra's testable stderr override instead of hardcoded `os.Stderr`.
  - All four commands (list, attach, kill, open) integrate bootstrapWait correctly.
  - The open command correctly routes: TUI path (destination == "") passes `serverWasStarted(cmd)` to `openTUIFunc` for Phase 3 handling; non-TUI path (destination != "") calls `bootstrapWait(cmd)`.
  - The FallbackResult case in open.go line 112 correctly passes `serverStarted=false` to `openTUIFunc`, preventing a double wait. This is also covered by `TestOpenCommand_FallbackToTUI_SkipsSecondWait`.

TESTS:
- Status: Adequate
- Coverage:
  - All 5 specified tests exist in /Users/leeovery/Code/portal/cmd/bootstrap_wait_test.go:
    1. "prints starting message to stderr when server was started" (line 14) -- verifies stderr output
    2. "calls waiter when server was started" (line 37) -- verifies waiter invocation
    3. "does not print message when server was not started" (line 57) -- verifies no stderr output
    4. "does not call waiter when server was not started" (line 79) -- verifies waiter not called
    5. "does not print message when context has no serverStarted" (line 99) -- verifies nil context handling, also checks waiter not called
  - Integration coverage exists in command tests: list_test.go, attach_test.go, kill_test.go all set `bootstrapDeps` with mock bootstrapper. open_test.go has explicit tests for TUI path pass-through (TestOpenCommand_DirectTUI_PassesServerStarted) and fallback double-wait prevention (TestOpenCommand_FallbackToTUI_SkipsSecondWait).
  - The plan mentioned a 6th test "list command outputs to stdout not stderr" but this was not in the task description's test list. The piping behavior is implicitly tested by list_test.go (which captures stdout separately) and the stderr verification in bootstrap_wait_test.go.
- Notes: Tests are focused and not over-tested. Each test verifies exactly one behavior. The injected waiter pattern keeps tests fast and deterministic.

CODE QUALITY:
- Project conventions: Followed. Uses the same dependency injection pattern (package-level *Deps struct, nil for production, non-nil for test) as other commands in the codebase.
- SOLID principles: Good. bootstrapWait has a single responsibility (check flag, print, wait). buildWaiter separates construction from use. Dependency inversion via the BootstrapDeps.Waiter function type.
- Complexity: Low. bootstrapWait is a straight-line function with one early return. No nested conditions.
- Modern idioms: Yes. Uses cobra's cmd.ErrOrStderr() which is idiomatic for testable stderr output. Context-based data propagation follows cobra conventions.
- Readability: Good. Function names are self-documenting. The comment block explains the purpose clearly. The separation of bootstrapWait and buildWaiter is clean.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The plan specified `fmt.Fprintln(os.Stderr, ...)` but the implementation uses `fmt.Fprintln(cmd.ErrOrStderr(), ...)`. This is strictly better for testability and is the idiomatic cobra approach. No action needed.
- The plan specified `bootstrapWait(cmd, waiter)` as the signature but the implementation uses `bootstrapWait(cmd)` with waiter from `bootstrapDeps`. This consolidates injection into one mechanism rather than two (deps struct + function parameter). Cleaner in practice.
