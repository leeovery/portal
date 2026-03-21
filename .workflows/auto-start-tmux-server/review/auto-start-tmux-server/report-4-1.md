TASK: Fix double session wait on open command fallback-to-TUI path

ACCEPTANCE CRITERIA:
- When open falls back to TUI after CLI bootstrapWait already ran, the TUI loading interstitial is skipped
- When open goes directly to TUI (no destination), the TUI loading interstitial still appears if the server was just started

STATUS: Complete

SPEC CONTEXT: The spec defines a two-phase ownership model: (1) server start runs in PersistentPreRunE, (2) session wait belongs to either the CLI path or the TUI path, not both. The CLI path owns the wait via bootstrapWait (stderr message + poll). The TUI path owns the wait via its loading interstitial. When the CLI path runs bootstrapWait and then falls back to TUI, the TUI must not run its own wait a second time.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/open.go:89-91 (direct TUI path passes serverWasStarted(cmd)), cmd/open.go:93 (CLI path runs bootstrapWait), cmd/open.go:111 (fallback-to-TUI passes false for serverStarted)
- Notes: The fix is clean and minimal. Line 90 passes `serverWasStarted(cmd)` for the direct-to-TUI path (no destination), preserving the real bootstrap state. Line 93 runs `bootstrapWait(cmd)` for the CLI resolution path. Line 111 hardcodes `false` for serverStarted when falling back to TUI via FallbackResult, correctly preventing the double wait. The two-phase ownership model from the spec is respected: once bootstrapWait has consumed the wait on the CLI side, the TUI is told the server was not started so it skips its interstitial.

TESTS:
- Status: Adequate
- Coverage:
  - TestOpenCommand_FallbackToTUI_SkipsSecondWait (cmd/open_test.go:990): Sets up a scenario where server was just started (started: true), destination resolves to FallbackResult (no alias, no zoxide, no dir match), captures the serverStarted argument passed to openTUIFunc, and asserts it is false. This directly verifies the first acceptance criterion.
  - TestOpenCommand_DirectTUI_PassesServerStarted (cmd/open_test.go:1027): Sets up a scenario where server was just started (started: true), no destination is provided, captures the serverStarted argument, and asserts it is true. This directly verifies the second acceptance criterion.
  - TestBuildTUIModel_ServerStarted (cmd/open_test.go:891): Verifies that the TUI model correctly enters PageLoading when serverStarted=true and PageSessions when serverStarted=false, confirming the downstream effect of the boolean.
- Notes: Tests are focused and behavior-driven. Each test verifies exactly one acceptance criterion without redundancy. The use of openTUIFunc override is appropriate since the real TUI requires terminal interaction. Not over-tested -- there are no redundant assertions.

CODE QUALITY:
- Project conventions: Followed. Uses package-level injectable function var (openTUIFunc) pattern consistent with the codebase. Tests use t.Cleanup for teardown. Test naming follows Go conventions.
- SOLID principles: Good. The openCmd RunE has clear branching: direct TUI vs CLI resolution with fallback. Each branch correctly determines the serverStarted value. Single responsibility maintained.
- Complexity: Low. The fix is a single boolean literal (false) at the fallback call site. The branching logic in RunE is straightforward: empty destination -> direct TUI, non-empty -> resolve -> PathResult or FallbackResult.
- Modern idioms: Yes. Uses cobra command context for state propagation, type switches for resolution results.
- Readability: Good. The intent is clear from reading the code. The comment in the test (line 991-993) explicitly explains why serverStarted=false is expected.
- Issues: None.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- (none)
