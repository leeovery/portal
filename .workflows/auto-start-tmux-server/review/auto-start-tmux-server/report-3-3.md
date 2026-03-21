TASK: Wire serverStarted into TUI launch path

ACCEPTANCE CRITERIA:
- tuiConfig has serverStarted field
- buildTUIModel passes WithServerStarted(true) when cfg.serverStarted is true
- openTUI accepts serverStarted parameter
- TUI path passes serverWasStarted(cmd) to openTUI
- Fallback path passes serverWasStarted(cmd) to openTUI
- Non-TUI path uses bootstrapWait, not openTUI
- When server already running, TUI opens to normal view
- All existing tests pass

STATUS: Complete

SPEC CONTEXT: The spec defines two-phase ownership: server start in PersistentPreRunE, session wait owned by the TUI (loading interstitial) for the TUI path and by the command (bootstrapWait) for the CLI path. This task bridges the bootstrap context (Phase 2) with the TUI model (Phase 3 tasks 3-1/3-2) by wiring serverStarted through the openTUI call chain.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - tuiConfig.serverStarted field: cmd/open.go:290
  - buildTUIModel conditional WithServerStarted: cmd/open.go:303-305
  - openTUI signature with serverStarted param: cmd/open.go:337
  - openTUI setting serverStarted on config: cmd/open.go:367
  - TUI path (destination empty) passing serverWasStarted(cmd): cmd/open.go:90
  - Fallback path passing false: cmd/open.go:111
  - Non-TUI path calling bootstrapWait: cmd/open.go:93
  - openTUIFunc var with bool param: cmd/open.go:22
- Notes: The fallback path (FallbackResult) passes `false` instead of `serverWasStarted(cmd)`. The plan's acceptance criterion says "Fallback path passes serverWasStarted(cmd) to openTUI" but the implementation hardcodes `false`. This is a deliberate and correct deviation: when the fallback path is reached, `bootstrapWait(cmd)` on line 93 has already executed the CLI wait. Passing `serverStarted=true` to the TUI would cause a double wait (CLI wait then TUI loading interstitial). The test `TestOpenCommand_FallbackToTUI_SkipsSecondWait` explicitly validates this design decision. The plan's own context section partially acknowledges this ("If the fallback routes to openTUI, the serverStarted flag is still passed through"), but the implementation's approach is superior.

TESTS:
- Status: Adequate
- Coverage:
  - TestBuildTUIModel_ServerStarted (cmd/open_test.go:891-948): 4 subtests covering serverStarted true/false/default and preserving other options
  - TestOpenCommand_DirectTUI_PassesServerStarted (cmd/open_test.go:1027-1052): Integration test verifying the TUI path passes serverWasStarted(cmd) = true when server was started
  - TestOpenCommand_FallbackToTUI_SkipsSecondWait (cmd/open_test.go:990-1025): Integration test verifying the fallback path passes serverStarted=false to avoid double wait
  - Existing TestBuildTUIModel tests (cmd/open_test.go:760-889): Verify serverStarted=false (default) still works correctly
- Notes: Tests cover all key behaviors including the deviation from plan (fallback passes false). Test names are descriptive and assertions are focused. No over-testing detected -- each test verifies a distinct behavior.

CODE QUALITY:
- Project conventions: Followed. Uses the established tuiConfig pattern, functional options via tui.WithServerStarted, and openTUIFunc indirection for testability. Follows existing code patterns in the file.
- SOLID principles: Good. tuiConfig as a parameter object keeps the interface clean. The openTUIFunc indirection follows DIP for testability. Single responsibility maintained -- openTUI wires dependencies, buildTUIModel constructs the model.
- Complexity: Low. The conditional at line 303-305 is simple and clear. The RunE branching logic (TUI path vs CLI path vs fallback) is straightforward.
- Modern idioms: Yes. Uses functional options pattern (WithServerStarted), context-based value passing (serverWasStarted), and cobra command patterns idiomatically.
- Readability: Good. The code is self-documenting. The serverStarted field name clearly communicates intent. The comment on openTUIFunc (line 19-21) explains the indirection.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The plan acceptance criterion "Fallback path passes serverWasStarted(cmd) to openTUI" does not match the implementation (passes `false`). The implementation is correct; the plan criterion was imprecise. The test explicitly documents the rationale. No action needed -- this is a plan documentation issue, not a code issue.
