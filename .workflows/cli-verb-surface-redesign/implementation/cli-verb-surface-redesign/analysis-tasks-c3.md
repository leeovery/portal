---
topic: cli-verb-surface-redesign
cycle: 3
total_proposed: 1
---
# Analysis Tasks: CLI Verb Surface Redesign (Cycle 3)

## Task 1: Single-source the command-on-attach usage-error string
status: approved
severity: medium
sources: duplication

**Problem**: The exact user-facing string `"a command (-e/--) can only run in a newly-created session, not an existing one"` is hardcoded at two separate `NewUsageError` call sites — `openResolved`'s `*SessionResult` arm (cmd/open.go:326, the single-target attach-command guard) and `runOpenBurstWithDeps`'s zero-mint-surface guard (cmd/open_burst_run.go:151, the multi-target arity of the same rule). open_burst_run.go's own comment identifies itself as "the multi-target arity of the single-target attach-command guard (openResolved's *SessionResult arm)", confirming the two express one rule with one message. This is the lone shared open/burst message string NOT single-sourced: every sibling shared message in the feature is already centralised (singleMissError/aggregatedMissError in cmd/open_burst.go; GoneMessage/PartialFailureMessage/UnsupportedNoopMessage in internal/spawn/message.go; the doctorRuntimeNotRunning, killSaverInfoMessage consts). A reword at one site would silently diverge from the other — the exact copy-paste-drift class the codebase deliberately closes everywhere else.

**Solution**: Hoist the literal to a single package-level const in cmd and have both guards reference it, mirroring the established const-for-shared-message convention (doctorRuntimeNotRunning / killSaverInfoMessage) and the singleMissError/aggregatedMissError single-authoring-site pattern. Logic stays per-site; only the string is consolidated.

**Outcome**: The command-on-attach usage message has exactly one authoring site in the cmd package; both the single-target and multi-target guards return `NewUsageError` with that shared const, so a future reword cannot drift the two guards apart. No behaviour change — both call sites emit byte-identical output to today.

**Do**:
1. Add a package-level const in the cmd package (alongside the existing shared-message consts, e.g. near singleMissError/aggregatedMissError in cmd/open_burst.go, or a sensible cmd-level location), for example:
   `const commandAttachOnlyMessage = "a command (-e/--) can only run in a newly-created session, not an existing one"`
   with a short doc comment noting it is the single authoring site consumed by both the single-target (openResolved *SessionResult arm) and multi-target (runOpenBurstWithDeps zero-mint) attach-command guards, so the two cannot drift.
2. In cmd/open.go, replace the inline literal at the `*SessionResult` arm (currently line 326) with `return NewUsageError(commandAttachOnlyMessage)`.
3. In cmd/open_burst_run.go, replace the inline literal at the zero-mint-surface guard (currently line 151) with `return NewUsageError(commandAttachOnlyMessage)`.
4. Confirm no third site references the literal (`grep` for the string) so the const becomes the sole source.

**Acceptance Criteria**:
- The literal string appears exactly once in cmd (the new const definition); neither cmd/open.go:326 nor cmd/open_burst_run.go:151 carries the inline literal.
- Both guards return `NewUsageError(commandAttachOnlyMessage)`.
- The emitted usage-error text is byte-identical to the pre-change output at both the single-target and multi-target guards.
- No behaviour change: the single-target attach-command path and the multi-target zero-mint path still fail with a UsageError (and the ack-marker / spawn ordering is untouched — the command guard still fires before any marker write or spawn).
- `go build ./...` and `go vet ./...` pass; `golangci-lint run` is clean.

**Tests**:
- Run the existing cmd-package tests that assert the command-on-attach usage error at both the single-target guard (openResolved *SessionResult arm) and the multi-target zero-mint guard (runOpenBurstWithDeps) — they must continue to pass unchanged, proving the emitted string is byte-identical after consolidation. If either guard's message is asserted against a hardcoded string literal in the test, leave the test assertion as-is (it validates the const's value) — the point is the production message did not change.
- No new test is required for a pure single-sourcing refactor with identical output; if the existing tests do not already assert both guards' message text, add a minimal assertion at each guard confirming `NewUsageError(commandAttachOnlyMessage)` is returned (message text equals the expected literal).
