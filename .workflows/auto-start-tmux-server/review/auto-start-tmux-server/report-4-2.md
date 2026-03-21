TASK: Extract tmux.NewClient construction to a single helper

ACCEPTANCE CRITERIA:
- Only one tmux.NewClient(&tmux.RealCommander{}) call exists in the cmd package (in PersistentPreRunE)
- All commands retrieve the client from context
- All existing tests pass without modification

STATUS: Complete

SPEC CONTEXT: The specification describes a shared bootstrap function called early by every Portal command that requires tmux. PersistentPreRunE is the designated integration point. This task is a pure refactor reducing 8 construction sites to 1, with no behavioral change.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/root.go:45 — sole tmux.NewClient(&tmux.RealCommander{}) call, inside buildBootstrapDeps()
  - cmd/root.go:61-69 — PersistentPreRunE stores client in context via tmuxClientKey
  - cmd/bootstrap_context.go:12,15 — tmuxClientKey context key definition
  - cmd/bootstrap_context.go:34-42 — tmuxClient(cmd) helper retrieves client from context (panics if missing)
  - cmd/kill.go:51 — uses tmuxClient(cmd)
  - cmd/attach.go:51 — uses tmuxClient(cmd)
  - cmd/list.go:103 — uses tmuxClient(cmd)
  - cmd/open.go:231,338 — uses tmuxClient(cmd) in openPath and openTUI
  - cmd/bootstrap_wait.go:32 — uses tmuxClient(cmd)
- Notes: All 6 consumer files retrieve the client via the context helper. No tmux.RealCommander references remain in production cmd code outside root.go. The buildBootstrapDeps() function cleanly separates production (real client) from test (injected BootstrapDeps.Client) paths.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/bootstrap_context_test.go:10-31 — TestTmuxClient verifies panic when no client in context (guards the invariant that PersistentPreRunE must run first)
  - cmd/bootstrap_context_test.go:34-72 — TestServerWasStarted covers 4 subtests for the companion context helper
  - cmd/open_test.go:995 — TestOpenCommand_FallbackToTUI_SkipsSecondWait injects client via BootstrapDeps.Client (tests the DI path)
  - cmd/open_test.go:1058,1069 — TestBuildSessionConnector uses tmux.NewClient directly, but this is a unit test for buildSessionConnector which takes a *tmux.Client parameter; it does not go through context (acceptable — testing the function's own contract, not the context plumbing)
- Notes: The task states "no new tests needed — pure refactor." The TestTmuxClient panic test was added as part of this task and is a sensible addition for the new invariant. Test files appropriately bypass the context client via DI structs. The 3 tmux.NewClient calls in test code are acceptable — they create clients for test parameters, not production code paths.

CODE QUALITY:
- Project conventions: Followed. Uses Go idiomatic context key pattern (unexported type prevents collisions). Helper function follows the existing buildXxxDeps pattern used across all commands.
- SOLID principles: Good. Single responsibility — bootstrap_context.go handles context plumbing only. Dependency inversion — buildBootstrapDeps returns an interface (ServerBootstrapper) alongside the concrete client, keeping testability intact. Open/closed — adding new commands that need tmux requires only calling tmuxClient(cmd), no modification to construction.
- Complexity: Low. buildBootstrapDeps is 6 lines. tmuxClient is 7 lines with a panic guard.
- Modern idioms: Yes. Uses context.Value with type-safe unexported keys. Panic for programming errors (missing PersistentPreRunE) is idiomatic Go for invariant violations.
- Readability: Good. The panic message in tmuxClient is explicit about what went wrong and how to fix it. The separation between bootstrap_context.go (context plumbing) and root.go (lifecycle hook) is clean.
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The TestBuildSessionConnector tests (open_test.go:1054-1076) still construct tmux.NewClient(&tmux.RealCommander{}) directly. Since buildSessionConnector takes a *tmux.Client parameter, this is structurally fine, but these tests could use a stub commander instead of RealCommander to avoid any coupling to the real tmux binary. Very minor — no behavioral impact.
