TASK: PersistentPreRunE integration

ACCEPTANCE CRITERIA:
- Commands in skipTmuxCheck (version, init, help, alias, clean) skip both CheckTmuxAvailable and EnsureServer
- When tmux is not installed, tmux-requiring commands fail with existing error before EnsureServer
- When tmux is installed and server is already running, EnsureServer returns (false, nil) and command proceeds
- When tmux is installed and server is not running, EnsureServer calls start-server and command proceeds
- All existing tests in cmd/root_test.go and cmd/root_integration_test.go continue to pass
- The serverStarted bool is discarded with _ (Phase 2 will capture it)

STATUS: Complete

SPEC CONTEXT: The spec states "A shared bootstrap function called early by every Portal command that requires tmux. Commands that skip the existing tmux availability check (version, init, help, alias, clean) also skip bootstrap." The server start phase "runs in PersistentPreRunE (shared by all commands). Calls tmux start-server. Returns immediately." This task wires EnsureServer into the command lifecycle.

IMPLEMENTATION:
- Status: Implemented (with expected Phase 2 evolution applied)
- Location: /Users/leeovery/Code/portal/cmd/root.go:52-72 (PersistentPreRunE), /Users/leeovery/Code/portal/cmd/root.go:22-47 (ServerBootstrapper interface, BootstrapDeps, buildBootstrapDeps), /Users/leeovery/Code/portal/cmd/bootstrap_context.go (context keys and helpers)
- Notes:
  - The plan originally called for `_, err := client.EnsureServer()` (discarding serverStarted). Per task note, Phase 2 changed this to capture serverStarted into context. The implementation correctly stores it via `context.WithValue(cmd.Context(), serverStartedKey, serverStarted)` at root.go:66.
  - The plan called for inline `client := tmux.NewClient(&tmux.RealCommander{})`. The implementation introduces a `ServerBootstrapper` interface and `BootstrapDeps` injection struct, which is a reasonable expansion to enable testability. The `buildBootstrapDeps()` function (root.go:41-47) falls back to `tmux.NewClient(&tmux.RealCommander{})` in production, matching the plan intent.
  - The `tmuxClient` is also stored in context (root.go:67-69) for downstream commands to reuse. This is beyond Phase 1 scope but clearly needed infrastructure.
  - Context key definitions live in a separate `bootstrap_context.go` file with proper unexported key types (preventing collisions) -- good Go practice.

TESTS:
- Status: Adequate
- Coverage:
  - `TestPersistentPreRunE_CallsEnsureServer` (root_test.go:171-291) -- 5 subtests:
    - EnsureServer called for tmux-requiring commands (line 172)
    - EnsureServer error propagates to caller (line 195)
    - EnsureServer not called for skipTmuxCheck commands (line 213)
    - PersistentPreRunE stores serverStarted=true in context (line 230)
    - PersistentPreRunE stores serverStarted=false in context (line 259)
  - `TestTmuxDependentCommandsFailWithoutTmux` (root_test.go:31-61) -- verifies tmux-not-installed path
  - `TestNonTmuxCommandsWorkWithoutTmux` (root_test.go:63-104) -- verifies skip commands bypass
  - `TestTmuxDependentCommandsSucceedWithTmux` (root_test.go:131-157) -- verifies list succeeds with tmux
  - `TestPortalBinaryTmuxMissing` (root_integration_test.go:42-126) -- integration test verifying binary behavior
  - `TestServerWasStarted` (bootstrap_context_test.go:34-72) -- 4 subtests for context retrieval
  - `TestTmuxClient` (bootstrap_context_test.go:10-32) -- panic test for missing client
  - The plan said "No new unit tests needed for PersistentPreRunE itself in Phase 1." The implementation added 5 new subtests. This is slightly beyond plan scope but well-justified -- each test covers a distinct behavioral path. Not over-tested.
- Notes: Tests use proper cleanup (`t.Cleanup`), mock injection via package-level `bootstrapDeps`, and ephemeral cobra subcommands for context verification. All acceptance criteria have corresponding test coverage.

CODE QUALITY:
- Project conventions: Followed. Table-driven tests, proper error wrapping, Commander interface pattern.
- SOLID principles: Good. `ServerBootstrapper` interface enables dependency inversion. `buildBootstrapDeps` separates construction from use. Single responsibility maintained -- bootstrap_context.go handles context plumbing, root.go handles the lifecycle hook.
- Complexity: Low. PersistentPreRunE is linear -- skip check, tmux check, bootstrap, context set. No branching complexity.
- Modern idioms: Yes. Context value propagation via typed keys (unexported `contextKey` type), interface-based DI, cobra lifecycle hooks.
- Readability: Good. Function and variable names are clear. Comments explain the "why" (e.g., "When Client is non-nil it is stored in context; otherwise no client is set").
- Issues: None found.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The `BootstrapDeps` struct has both `Bootstrapper ServerBootstrapper` and `Client *tmux.Client` fields. In production, both are the same `*Client` instance. In tests, `Bootstrapper` is a mock while `Client` is nil. This dual-field design works but adds a small cognitive overhead. Acceptable for testability.
- The `Waiter` field on `BootstrapDeps` is Phase 2 scope that leaked into the struct definition. Not a problem, just noting it was defined ahead of time.
