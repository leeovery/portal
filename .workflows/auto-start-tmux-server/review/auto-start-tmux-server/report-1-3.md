TASK: EnsureServer bootstrap function

ACCEPTANCE CRITERIA:
- When server is already running: returns (false, nil) without calling start-server
- When server is not running and start-server succeeds: returns (true, nil)
- When server is not running and start-server fails: returns (true, err)
- ServerRunning() is always called first
- StartServer() is only called when ServerRunning() returns false
- Tests use MockCommander with RunFunc to differentiate behavior by subcommand

STATUS: Complete

SPEC CONTEXT: The spec defines a "Two-phase ownership" model where Phase 1 (server start) runs in PersistentPreRunE shared by all commands. EnsureServer is this shared server-start phase. The return bool indicates "a start was attempted" (not "start succeeded") so downstream code (CLI wait, TUI interstitial) can decide whether to enter the session-wait flow. Bootstrap is one-shot with no retry.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tmux/tmux.go:129-141
- Notes: Implementation matches the plan exactly. The method calls ServerRunning() first (fast path), then conditionally calls StartServer(). Return values for all three cases (already running, started successfully, start failed) are correct. Doc comment on lines 129-132 accurately describes the semantics.

TESTS:
- Status: Adequate
- Coverage:
  - "returns false when server is already running" (line 446): RunFunc returns success for "info"; verifies started=false, err=nil; t.Fatalf guards against unexpected start-server call
  - "starts server and returns true when server is not running" (line 468): RunFunc returns error for "info", success for "start-server"; verifies started=true, err=nil
  - "returns true and error when start-server fails" (line 493): RunFunc returns error for both; verifies started=true, err!=nil
  - "does not call start-server when server is running" (line 518): Explicitly checks mock.Calls has exactly 1 entry containing only "info"
- Notes: All four planned tests are present. Tests use MockCommander with RunFunc as required. Tests verify behavior (return values) and interaction (which commands were called). No over-testing -- each test covers a distinct scenario with focused assertions. The combination of tests 1 and 4 double-covers the "no start-server when running" criterion from two angles (behavioral guard via t.Fatalf and explicit call-count assertion), which is slightly redundant but acceptable since each test has a distinct primary purpose.

CODE QUALITY:
- Project conventions: Followed. Uses existing MockCommander with RunFunc pattern, subtests with t.Run, error wrapping with fmt.Errorf and %w matching the pattern in NewSession/KillSession/etc.
- SOLID principles: Good. EnsureServer composes ServerRunning and StartServer (SRP for each, OCP via Commander interface). No violations.
- Complexity: Low. Three code paths, no loops, no nesting beyond a single if-check.
- Modern idioms: Yes. Idiomatic Go error handling, method on pointer receiver consistent with other Client methods.
- Readability: Good. The method is 8 lines, self-documenting, with a clear doc comment explaining the three return cases.
- Issues: None.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- (none)
