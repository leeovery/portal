TASK: Single-source the command-on-attach usage-error string (cli-verb-surface-redesign-9-1)

ACCEPTANCE CRITERIA:
- Hoist the byte-identical command-on-attach usage-error string "a command (-e/--) can only run in a newly-created session, not an existing one" — previously authored inline at cmd/open.go (openResolved *SessionResult arm) AND cmd/open_burst_run.go (runOpenBurstWithDeps zero-mint guard) — into ONE package-level cmd const (commandAttachOnlyMessage), near singleMissError/aggregatedMissError in cmd/open_burst.go.
- Both guards return NewUsageError(const).
- Literal appears exactly once (the const def); emitted text byte-identical; no behaviour change (command guard still fires before any marker write/spawn).
- go build+vet+golangci-lint clean; existing command-on-attach usage-error tests (single-target + multi-target zero-mint) pass unchanged.

STATUS: Complete

SPEC CONTEXT: spec § "Command passthrough (-e/--) — mint-scoped": a command rides mint (newly-created) surfaces only; an existing/attach target has no safe command-injection channel (send-keys corrupts a busy pane; respawn-pane -k destroys running work), so a command may never run against an attach target. Two arities of the same rule: the single-target attach guard (openResolved's *SessionResult arm) and the multi-target zero-mint guard (an all-attach set carrying a command). Both must emit byte-identical usage-error text and exit 2 (UsageError). This chore consolidates the single authored literal so the two cannot drift — mirroring the doctorRuntimeNotRunning / killSaverInfoMessage / singleMissError single-authoring-site convention.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - const definition: cmd/open_burst.go:100 (commandAttachOnlyMessage), placed between aggregatedMissError (91-93) and singleMissError (108-110) as specified, with a doc comment naming both consumers.
  - single-target guard: cmd/open.go:348-350 — `if len(command) > 0 { return NewUsageError(commandAttachOnlyMessage) }`, strictly BEFORE writeAckMarker(cmd) at line 354 and the openSessionFunc attach at 355.
  - multi-target zero-mint guard: cmd/open_burst_run.go:150-152 — `if len(command) > 0 && !hasMintSurface(surfaces) { return NewUsageError(commandAttachOnlyMessage) }`, positioned BEFORE detect/resolve/spawn (SplitTriggerFirst at 154, Detect at 158, Run at 177).
- Notes:
  - Production literal appears EXACTLY once — verified by grep across all *.go: the sole occurrence of the string in non-test production code is the const def (cmd/open_burst.go:100). No stray inline literal remains at either former authoring site.
  - Both guards use NewUsageError(commandAttachOnlyMessage) — confirmed at cmd/open.go:349 and cmd/open_burst_run.go:151.
  - Byte-identical: the const value character-for-character equals the string the three tests assert; ordering preserved so the guard fires before any marker write (single-target) or any detect/spawn/self-connect (multi-target). No behaviour change — pure string consolidation.

TESTS:
- Status: Adequate
- Coverage:
  - Single-target bare positional: cmd/open_test.go:335-368 (TestOpenCommand..._WithCommand_UsageError) — asserts *UsageError, byte-exact message (line 357), and openSessionFunc NOT called (no attach ⇒ guard fires before the marker/attach handoff).
  - Single-target -s pin: cmd/open_test.go:370-411 (TestOpenCommand_SessionPin_WithCommand_UsageError) — same three assertions via the shared openResolved switch.
  - Multi-target all-attach zero-mint: cmd/open_burst_run_test.go:395-441 (TestRunOpenBurst_AllAttachWithCommand_UsageError) — asserts *UsageError, byte-exact message (line 426), NewBurster NOT built, OpenWindow 0 calls, Connector 0, LocalMint 0 (guard fires before detect/resolve/spawn/self-connect).
- Notes:
  - The three tests hardcode the golden string and assert equality, so the tests act as byte-identical anchors: any drift of the const value fails all three. This is the correct golden-anchor pattern for a "byte-identical output" acceptance criterion.
  - Not over-tested: each test also verifies the load-bearing ordering (no attach / no spawn), which is behaviour, not implementation detail — appropriate and non-redundant.
  - Not under-tested: both arities of the rule are covered; the -s-pin variant additionally proves the single guard covers both bare and pinned session hits through the shared switch.

CODE QUALITY:
- Project conventions: Followed. Matches the established single-authoring-site convention (doctorRuntimeNotRunning / killSaverInfoMessage / singleMissError). Const co-located with sibling error-message helpers; doc comment explains the SOLE-authoring-site intent and names both consumers.
- SOLID principles: Good. Single source of truth for the message; consumers depend on the const, not a duplicated literal.
- Complexity: Low. No control-flow change; a literal-to-const substitution at two call sites.
- Modern idioms: Yes. Idiomatic Go package-level const.
- Readability: Good. The const's doc comment is clear about why it exists and who uses it.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
