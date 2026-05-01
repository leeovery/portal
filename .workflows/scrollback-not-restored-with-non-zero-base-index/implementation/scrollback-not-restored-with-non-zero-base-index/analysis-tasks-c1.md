# Analysis Tasks: scrollback-not-restored-with-non-zero-base-index (Cycle 1)

## Task 1: Export hydrationTriggerEvents and eliminate two test-side shadows
status: approved
severity: medium
sources: duplication, architecture

**Problem**: The canonical `hydrationTriggerEvents` slice (`["client-attached", "client-session-changed"]`) is hand-rolled in three places: production source `internal/tmux/hooks_register.go:26-29`, in-package external-test mirror `expectedHydrationTriggerEvents` at `internal/tmux/hooks_register_test.go:28-31`, and cross-package integration mirror `leadingDashHydrationTriggerEvents` at `cmd/bootstrap/reboot_roundtrip_test.go:1144-1147`. Spec § Fix Scope explicitly anticipates extension ("If the slice is later extended, the migration scan must follow it"). On extension, production updates while both test mirrors silently under-cover.

**Solution**: Export the canonical slice from `internal/tmux`; have both test files consume the exported symbol directly.

**Outcome**: One canonical list. Adding a new event in production automatically widens coverage in both migration tests and the cross-package round-trip integration test.

**Do**:
1. In `internal/tmux/hooks_register.go`, expose the canonical slice — rename `hydrationTriggerEvents` to exported `HydrationTriggerEvents` (`var []string`), or expose `func HydrationTriggerEvents() []string` returning a defensive copy. Prefer the var form unless mutation drift is a concern.
2. Update all in-package iteration sites (`hooks_register.go`, migration code) to the exported name.
3. Delete `expectedHydrationTriggerEvents` at `internal/tmux/hooks_register_test.go:28-31`; replace its 8 iteration sites in `hooks_migration_test.go` with the exported symbol.
4. Delete `leadingDashHydrationTriggerEvents` at `cmd/bootstrap/reboot_roundtrip_test.go:1144-1147`; update `verifyHydrationHookEntries` (line 1347) and any other call sites in that file. `bootstrap_test` already imports `tmux`.
5. Remove the obsolete justification docstring at `cmd/bootstrap/reboot_roundtrip_test.go:1137-1143`.

**Acceptance Criteria**:
- The literal `["client-attached", "client-session-changed"]` appears exactly once in the repo.
- `grep -rn 'expectedHydrationTriggerEvents\|leadingDashHydrationTriggerEvents' .` returns no matches.
- Both `hooks_migration_test.go` and `reboot_roundtrip_test.go` reference the exported symbol from `internal/tmux`.
- All existing tests pass.

**Tests**:
- `go test ./internal/tmux/...`
- `go test -tags=integration ./cmd/bootstrap/...`

---

## Task 2: Extract shared portal.log scan helper for assertion reuse
status: approved
severity: low
sources: duplication

**Problem**: `cmd/bootstrap/reboot_roundtrip_test.go` contains `verifyNoPredictedVsLiveWarns` (lines 443-458) and `verifyNoHydrateTimeoutWarns` (lines 1375-1397) sharing ~12 lines of byte-identical file-IO scaffolding: open `portal.log`, return on `os.IsNotExist`, split on `"\n"`, iterate lines, fail on match. Only the per-line predicate differs (regex vs. dual substring).

**Solution**: Extract a single helper taking a predicate and per-AC failure message; rewrite both existing helpers as one-line wrappers.

**Outcome**: One implementation of the portal.log scan plumbing; future AC assertions add a one-line wrapper instead of duplicating IO scaffolding.

**Do**:
1. Add `func assertNoLogLineMatches(t *testing.T, logPath string, pred func(string) bool, failFmt string, args ...any)` near the existing helpers.
2. Rewrite `verifyNoPredictedVsLiveWarns` (lines 443-458) as a one-line wrapper.
3. Rewrite `verifyNoHydrateTimeoutWarns` (lines 1375-1397) as a one-line wrapper.
4. Preserve each helper's distinct diagnostic message exactly.

**Acceptance Criteria**:
- Both pre-existing helpers retain public names and call signatures.
- File-IO + ENOENT-tolerance + line-iteration plumbing appears exactly once.
- Distinct failure-message strings remain byte-identical to today.

**Tests**:
- `go test ./cmd/bootstrap/...`

---

## Task 3: Promote applyBaseIndices to a shared test helper
status: approved
severity: low
sources: duplication

**Problem**: The new `applyBaseIndices` helper at `cmd/bootstrap/reboot_roundtrip_test.go:513-519` issues the canonical four-call `set-option -g/-s × base-index/pane-base-index` sequence. The pre-existing `TestPhase3Integration_RestoreUsesLiveIndicesUnderBaseIndexDrift` performs the same four calls inline at `internal/restore/integration_test.go:325-328` with hard-coded `"1"`.

**Solution**: Promote `applyBaseIndices` to a shared test-only package (`internal/tmuxtest` preferred — both call sites already use `tmuxtest.Socket`).

**Outcome**: One implementation of the base-index configuration sequence shared across all integration-tag tests.

**Do**:
1. Move `applyBaseIndices` from `cmd/bootstrap/reboot_roundtrip_test.go:513-519` to `internal/tmuxtest`. Suggested signature: `func ApplyBaseIndices(t *testing.T, ts *Socket, base, paneBase int)`.
2. Update the round-trip caller in `reboot_roundtrip_test.go` to `tmuxtest.ApplyBaseIndices(t, ts, base, paneBase)`.
3. Replace the inline block at `internal/restore/integration_test.go:325-328` with `tmuxtest.ApplyBaseIndices(t, ts, 1, 1)`.

**Acceptance Criteria**:
- `applyBaseIndices` defined exactly once.
- The four-call sequence appears verbatim in exactly one helper body.
- Both caller files consume the helper.

**Tests**:
- `go test -tags=integration ./internal/restore/...`
- `go test -tags=integration ./cmd/bootstrap/...`

---

## Task 4: Tighten AC #2 unit test to cover FIFO byte-write clause
status: approved
severity: low
sources: standards

**Problem**: Spec § AC #2 requires the cobra-level argv parse test to "drive the cobra command tree via Execute() against a leading-dash positional argument and assert exit 0 + FIFO byte written". `TestStateSignalHydrate_AcceptsLeadingDashSessionViaCobraExecute` at `cmd/state_signal_hydrate_test.go:622-647` installs a no-op replacement for `signalHydrateRunFunc` that captures `cfg.Session` and asserts exit 0 + captured session, but the seam short-circuits `runSignalHydrate` before any FIFO is opened.

**Solution**: Tighten the unit test to install a stub FIFO seam and assert a byte was written, preserving the existing seam pattern. Fallback: explicit cross-reference comment if seam stubbing requires production-code changes outside scope.

**Outcome**: AC #2 satisfied verbatim by a single unit test.

**Do**:
1. In `cmd/state_signal_hydrate_test.go:622-647`, identify the FIFO open/write seam used by `runSignalHydrate`.
2. Replace the no-op `signalHydrateRunFunc` swap with the real `runSignalHydrate` plus a stub FIFO recording bytes written.
3. Drive cobra via `Execute()` with the leading-dash positional argv; assert exit 0, captured session equals the leading-dash value, and the stub FIFO received the expected payload.
4. Restore the seam via `t.Cleanup()` per project DI pattern.
5. Fallback only if seam stubbing requires production-code seam additions outside this fix's scope: add an explicit cross-reference comment pointing to `TestRebootRoundTrip_LeadingDashSessionName`.

**Acceptance Criteria**:
- Unit test drives `Execute()` with leading-dash session argv.
- Asserts exit 0 AND that a byte was written through the FIFO seam (or fallback comment).
- No production-code seam additions outside the existing `*Deps` / package-level-var pattern.
- Test does not use `t.Parallel()`.

**Tests**:
- `go test ./cmd -run TestStateSignalHydrate_AcceptsLeadingDashSessionViaCobraExecute`
- `TestRebootRoundTrip_LeadingDashSessionName` continues to pass unchanged.

---

## Task 5: Collapse RegisterPortalHooks/RegisterPortalHooksWithLogger into a single entry point
status: approved
severity: low
sources: architecture

**Problem**: `internal/tmux/hooks_register.go:233-282` exposes `RegisterPortalHooks(c)` and `RegisterPortalHooksWithLogger(c, log)`. The only production caller (`bootstrapadapter.HookRegistrar` at `internal/bootstrapadapter/adapters.go:68-70`) already has a logger. The no-logger form is exercised only by `hooks_register_test.go`. Future production code could route through the no-logger form and silently lose eviction visibility.

**Solution**: Collapse into a single `RegisterPortalHooks(c *Client, log MigrationLogger) error` tolerating `nil`.

**Outcome**: One name, one shape, no risk of future production code routing through a logger-less path.

**Do**:
1. In `internal/tmux/hooks_register.go`, delete `RegisterPortalHooksWithLogger`; rename its body to `RegisterPortalHooks(c *Client, log MigrationLogger) error`. Keep the `nil` guard.
2. Update `internal/bootstrapadapter/adapters.go:68-70` (`HookRegistrar`) to call the unified `RegisterPortalHooks(c, log)`.
3. Update test call sites in `internal/tmux/hooks_register_test.go` to pass `nil` or a recording logger.
4. Update doc comments — drop "stable external-caller surface" language.

**Acceptance Criteria**:
- `RegisterPortalHooksWithLogger` no longer exists.
- `RegisterPortalHooks` takes `*Client` and `MigrationLogger`.
- All production and test call sites compile and pass.
- Passing `nil` for `log` falls through to the noop logger.

**Tests**:
- `go test ./internal/tmux/...`, `go test ./internal/bootstrapadapter/...`, `go test ./cmd/bootstrap/...` all pass.

---

## Task 6: Unexport MigrateHydrationHooks to seal it inside RegisterPortalHooks
status: approved
severity: low
sources: architecture

**Problem**: `MigrateHydrationHooks` at `internal/tmux/hooks_register.go:189-227` is exported; its doc pins it as bootstrap-internal. Non-production callers are three tests at `internal/tmux/hooks_migration_test.go:346,395,441` exercising the migration in isolation. The export creates a second valid entry point for "hook installation" — a future contributor could invoke it standalone and end up with no hydration hooks registered.

**Solution**: Unexport to `migrateHydrationHooks`; refactor the three tests to drive `RegisterPortalHooks` with a recording logger and `MockCommander`.

**Outcome**: Migration is structurally sealed inside `RegisterPortalHooks`.

**Do**:
1. Rename `MigrateHydrationHooks` to `migrateHydrationHooks`. Update its single in-package call site.
2. Refactor the three tests to drive `RegisterPortalHooks` (or `RegisterPortalHooksWithLogger` if Task 5 not landed) with a recording logger and `MockCommander`. Arrange `MockCommander` so the migration phase exhibits each test's condition; assert the same observable outcome.
3. Confirm via repo-wide grep that no caller outside `internal/tmux` references `MigrateHydrationHooks`.

**Acceptance Criteria**:
- `MigrateHydrationHooks` (capital M) no longer exists.
- `migrateHydrationHooks` is called from exactly one site.
- The three migration tests still cover their original conditions through `RegisterPortalHooks`.
- All tests pass.

**Tests**:
- `go test ./internal/tmux/...`
- `grep -rn 'MigrateHydrationHooks' .` returns no matches.

---

## Discarded Findings
- A1 (MigrationLogger interface duplicates bootstrap.Logger / *state.Logger shape) — analyst explicitly stated "No action required for this fix"; flagged as an optional cleanup rather than an actionable finding.
