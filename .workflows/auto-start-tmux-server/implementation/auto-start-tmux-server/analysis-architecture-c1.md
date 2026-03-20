AGENT: architecture
FINDINGS:
- FINDING: Package-level mutable state for dependency injection creates test isolation risk
  SEVERITY: medium
  FILES: cmd/attach.go:12, cmd/kill.go:12, cmd/list.go:12, cmd/open.go:21, cmd/root.go:28
  DESCRIPTION: Each command uses a package-level var (attachDeps, killDeps, listDeps, openDeps, bootstrapDeps) that tests mutate and clean up via t.Cleanup. Because cobra commands and rootCmd are also package-level singletons, parallel test execution within the cmd package is unsafe -- any test that sets one of these vars is racing with tests that read it. Today the tests run sequentially (no t.Parallel calls), so this works, but it is a latent fragility: adding t.Parallel to any cmd test will produce data races. The pattern also forces every command to duplicate the "if deps != nil { return injected } else { build real }" fork.
  RECOMMENDATION: Consider a single command-scoped context or a shared deps struct passed via PersistentPreRunE context (similar to how serverStartedKey already works). This would eliminate the package-level mutable vars and make the injection seam consistent. Alternatively, document a "no t.Parallel in cmd tests" rule if the current approach is intentionally chosen for simplicity.

- FINDING: bootstrapWait takes a nilable func() parameter for test injection, inconsistent with the interface-based DI used everywhere else
  SEVERITY: medium
  FILES: cmd/bootstrap_wait.go:14, cmd/attach.go:30, cmd/kill.go:31, cmd/list.go:53, cmd/open.go:89
  DESCRIPTION: bootstrapWait accepts `waiter func()` -- when nil it builds real dependencies internally. Every call site passes nil in production and a closure in tests. This differs from every other command's DI approach (interface + deps struct). More importantly, the nil-or-not fork happens inside the function body, mixing "am I in a test?" branching with business logic. The function's signature is also untyped (bare func()) when the concrete behavior is a well-defined "wait for sessions" operation.
  RECOMMENDATION: Either: (a) make bootstrapWait's waiting behavior injectable through the same bootstrapDeps struct (adding a Waiter field alongside Bootstrapper), or (b) extract a small WaitFunc type and inject it consistently. This eliminates the nil-check branching and aligns with the interface-based DI pattern used by attach, kill, list, and open.

- FINDING: tmux.NewClient(&tmux.RealCommander{}) instantiated 8 times across cmd package with no sharing
  SEVERITY: low
  FILES: cmd/root.go:42, cmd/bootstrap_wait.go:22, cmd/kill.go:52, cmd/open.go:69, cmd/open.go:227, cmd/open.go:334, cmd/attach.go:52, cmd/list.go:103
  DESCRIPTION: The same tmux.Client construction appears 8 times in cmd/. While Client is stateless (just wraps a Commander), the repetition means if construction ever changes (e.g., adding a socket path option), every site must be updated. More architecturally, PersistentPreRunE already creates a Client to run EnsureServer, but that instance is discarded -- each command's RunE then creates its own. A single Client per command execution would be sufficient.
  RECOMMENDATION: Create the Client once in PersistentPreRunE and store it in the command context alongside serverStartedKey. Commands retrieve it via a helper similar to serverWasStarted(cmd). This removes 7 of the 8 construction sites.

SUMMARY: The bootstrap implementation is architecturally sound -- clean two-phase ownership (PersistentPreRunE for server start, command/TUI for session wait), good separation between tmux.Client, wait logic, and TUI loading state. Three medium/low findings: package-level mutable DI vars create latent parallel-test fragility, bootstrapWait's injection mechanism is inconsistent with the rest of the DI pattern, and tmux.Client is constructed redundantly across commands.
