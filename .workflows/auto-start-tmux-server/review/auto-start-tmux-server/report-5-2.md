TASK: Thread Cobra.Command Through OpenTUI to Eliminate Implicit OpenCmd Coupling

ACCEPTANCE CRITERIA:
- openTUI no longer references the package-level openCmd variable
- openTUIFunc signature includes *cobra.Command as its first parameter
- openCmd.RunE passes cmd explicitly when calling openTUIFunc
- All tests pass: go test ./cmd/...

STATUS: Complete

SPEC CONTEXT: The spec defines bootstrap as a two-phase process where the server start runs in PersistentPreRunE and session wait is context-specific. The TUI path and CLI path branch inside openCmd.RunE, making it important that openTUI receives the correct cobra.Command for accessing context values (e.g., serverWasStarted). Threading cmd explicitly eliminates implicit coupling to the package-level openCmd variable, improving testability and correctness.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/open.go:22 -- openTUIFunc declared with *cobra.Command as first parameter
  - cmd/open.go:337 -- openTUI function signature: func openTUI(cmd *cobra.Command, ...)
  - cmd/open.go:338 -- tmuxClient(cmd) uses the explicit cmd parameter
  - cmd/open.go:90 -- openCmd.RunE passes cmd: openTUIFunc(cmd, "", command, serverWasStarted(cmd))
  - cmd/open.go:111 -- fallback path passes cmd: openTUIFunc(cmd, r.Query, command, false)
  - cmd/open.go:413 -- init() assigns openTUIFunc = openTUI
- Notes: Zero references to openCmd inside openTUI body. All four occurrences of openCmd in the file are the declaration (line 79), a comment (line 21), and init() registration (lines 414-415). The refactor is clean and complete.

TESTS:
- Status: Adequate
- Coverage: The plan explicitly states "no new tests needed -- signature refactor." Existing tests at cmd/open_test.go verify the wiring:
  - TestOpenCommand_FallbackToTUI_SkipsSecondWait (line 990): stubs openTUIFunc with *cobra.Command first param, verifies serverStarted propagation
  - TestOpenCommand_DirectTUI_PassesServerStarted (line 1027): same pattern, verifies direct TUI path
  - Both test stubs correctly use the updated signature (lines 1009, 1036)
- Notes: Tests verify behavior through the real openCmd.RunE, confirming cmd is threaded correctly. No over-testing concern -- these are the same tests that existed pre-refactor, updated for the new signature.

CODE QUALITY:
- Project conventions: Followed -- idiomatic Go, proper error handling, exported/unexported naming
- SOLID principles: Good -- this refactor directly improves dependency inversion by replacing an implicit global reference with explicit parameter passing
- Complexity: Low -- straightforward parameter threading, no new branches
- Modern idioms: Yes -- function type variable with explicit signature, init() for cycle-breaking
- Readability: Good -- the comment on line 21 accurately explains why init() is used to break the initialization cycle
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The comment on line 21 ("Initialized in init() to break the openTUIFunc -> openTUI -> openCmd -> openTUIFunc cycle") still references openCmd, but this accurately describes the variable initialization cycle that init() breaks. Not a coupling concern.
