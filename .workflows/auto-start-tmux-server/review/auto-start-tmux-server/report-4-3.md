TASK: Make bootstrapWait injection consistent with interface-based DI pattern

ACCEPTANCE CRITERIA:
- bootstrapWait no longer accepts a bare func() parameter
- Wait behavior is injected via the bootstrapDeps struct
- No nil-check branching remains in bootstrapWait for DI purposes

STATUS: Complete

SPEC CONTEXT: The spec defines a two-phase bootstrap: (1) server start in PersistentPreRunE, (2) session wait owned by the calling context (CLI or TUI). The session wait for CLI commands is handled by bootstrapWait. The task is a pure refactoring chore to align bootstrapWait's DI mechanism with the interface-based deps-struct pattern used by attach, kill, list, and open commands.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/bootstrap_wait.go:14-36, cmd/root.go:32-36 (BootstrapDeps struct with Waiter field)
- Notes: bootstrapWait(cmd *cobra.Command) takes only the cobra command -- no bare func() parameter. The Waiter field on BootstrapDeps (type func()) is used for injection. A separate buildWaiter helper (lines 27-36) follows the same pattern as buildAttachDeps, buildKillDeps, buildListDeps: check if deps struct is set, return injected dep or build real one. Call sites in cmd/attach.go:29, cmd/kill.go:29, cmd/list.go:52, cmd/open.go:93 all pass only cmd. No drift from plan.

TESTS:
- Status: Adequate
- Coverage: bootstrap_wait_test.go covers 5 scenarios: (1) prints stderr message when server started, (2) calls waiter when server started, (3) no message when server not started, (4) no waiter call when server not started, (5) no message/call when context lacks serverStarted key. All inject via bootstrapDeps.Waiter. Command tests (attach_test.go, kill_test.go, list_test.go, open_test.go) all set bootstrapDeps with a mock Bootstrapper and no Waiter (which is fine -- they test command logic, not wait behavior). All call sites use the updated bootstrapWait(cmd) signature.
- Notes: Tests are well-scoped. Each test verifies one behavior. No over-testing -- the separation of "prints message" and "calls waiter" into distinct tests is reasonable since they are independent concerns.

CODE QUALITY:
- Project conventions: Followed. The buildWaiter pattern matches buildAttachDeps/buildKillDeps/buildListDeps exactly.
- SOLID principles: Good. Single responsibility -- bootstrapWait handles the CLI wait path only. Dependency inversion -- wait behavior is injected through the deps struct.
- Complexity: Low. bootstrapWait is 7 lines, buildWaiter is 9 lines. Clear control flow.
- Modern idioms: Yes. Uses cobra's ErrOrStderr for testability. Context-based state propagation.
- Readability: Good. Comments accurately describe behavior. Function names are self-documenting.
- Issues: The nil-check on bootstrapDeps in buildWaiter (line 28) is the standard DI pattern used across the entire cmd package. The analysis task's third AC says "No nil-check branching remains in bootstrapWait for DI purposes" -- this is satisfied because bootstrapWait itself contains no nil-check; the check lives in the extracted buildWaiter helper, consistent with how all other commands handle DI. Task 4-4 separately addresses the broader concern of package-level mutable DI vars (and chose Option B: documented constraint).

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The Waiter field is typed as func() rather than an interface. This is pragmatic for a single-method operation and avoids ceremony, but differs slightly from the "interface-based DI" title. The analysis task itself suggested either an interface or a type alias, and func() was chosen. This is acceptable -- it matches the DI injection point (the deps struct) pattern even if the injected value is a function rather than an interface implementor.
