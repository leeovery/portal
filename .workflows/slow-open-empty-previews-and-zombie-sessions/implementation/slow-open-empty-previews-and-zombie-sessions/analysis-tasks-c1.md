---
topic: slow-open-empty-previews-and-zombie-sessions
cycle: 1
total_proposed: 6
---
# Analysis Tasks: slow-open-empty-previews-and-zombie-sessions (Cycle 1)

## Task 1: Consolidate fingerprint diff/format/sort helpers into internal/portaltest
status: pending
severity: high
sources: duplication-c1#finding-1

**Problem**: Three independent integration-test files re-implement the same five-helper diff suite over `portaltest.Fingerprint` maps (~400 LOC combined), and a fourth near-twin lives as `emitFieldDeltas` in `internal/portaltest/fingerprint.go:203-226`. Any change to `Fingerprint`'s shape requires four coordinated edits with no compile-time guard.

**Solution**: Promote a single consolidated helper into `internal/portaltest`. New API: `DiffFingerprints(pre, post map[string]Fingerprint) []FingerprintDelta` returning structured deltas, plus `FormatFingerprint(fp) string` and `FormatDelta(d FingerprintDelta) string`. Refactor the three test files and the package-internal `emitFieldDeltas`/`reportStateDirDelta` to consume the new helpers.

**Outcome**: Single source of truth for fingerprint diffing/formatting. Shape changes to `Fingerprint` become single-site edits with compile-time fan-out. ~300 LOC removed across test files.

**Do**:
1. Add `FingerprintDelta` struct + `DiffFingerprints`/`FormatFingerprint`/`FormatDelta` in `internal/portaltest/fingerprint.go`.
2. Refactor `internal/portaltest`'s own `emitFieldDeltas` / `reportStateDirDelta` to delegate to the new helpers.
3. Delete `assertSnapshotsEqual` + four siblings from `internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go:359-475`; replace call sites with `t.Fatalf` wrapping `DiffFingerprints` + `FormatDelta`.
4. Repeat for `cmd/state_daemon_self_supervision_integration_test.go:888-1026`.
5. Repeat for `cmd/bootstrap/composition_abc_integration_test.go:300-446`.
6. Run `go test ./...` to confirm parity.

**Acceptance Criteria**:
- `DiffFingerprints`, `FormatFingerprint`, `FormatDelta` exist as exported symbols in `internal/portaltest`.
- Zero local re-implementations of fingerprint diff/format/sort logic remain across the three integration-test files.
- `internal/portaltest`'s own `emitFieldDeltas` either calls `DiffFingerprints` or is removed in favor of it.
- `go build ./...` and `go test -tags integration ./...` pass.
- Net LOC reduction across the four files is at least 250.

**Tests**:
- Unit test `DiffFingerprints` in `internal/portaltest/fingerprint_test.go` covering: identical maps, additions-only, removals-only, field-mutation, mixed.
- Unit test `FormatDelta` for stable output ordering (sorted keys).
- Existing integration tests must continue to pass unchanged in semantics.

## Task 2: Collapse spawnOrphanDaemonIsolated and spawnOrphanDaemonIsolatedNamed
status: pending
severity: medium
sources: duplication-c1#finding-2

**Problem**: `spawnOrphanDaemonIsolated` (orphan_sweep_integration_test.go:455-467) and `spawnOrphanDaemonIsolatedNamed` (composition_e2e_harness_integration_test.go:383-395) are line-for-line identical in the same `package bootstrap_test`, differing only in return signature (`*exec.Cmd` vs `(*exec.Cmd, string)`).

**Solution**: Keep the Named variant (strict superset), drop the suffix, delete the un-Named twin, update call sites to use `_` for the second return when unused.

**Outcome**: One spawn helper in `bootstrap_test`. Net -13 LOC.

**Do**:
1. Rename `spawnOrphanDaemonIsolatedNamed` to `spawnOrphanDaemonIsolated` in `cmd/bootstrap/composition_e2e_harness_integration_test.go`.
2. Delete the original `spawnOrphanDaemonIsolated` body from `cmd/bootstrap/orphan_sweep_integration_test.go:455-467`.
3. Audit all callers; change `cmd := spawnOrphanDaemonIsolated(...)` to `cmd, _ := spawnOrphanDaemonIsolated(...)` where unused.
4. Run `go test -tags integration ./cmd/bootstrap/...`.

**Acceptance Criteria**:
- Exactly one definition of `spawnOrphanDaemonIsolated` exists in `package bootstrap_test`.
- All call sites compile and pass under `-tags integration`.
- Net LOC across the two files reduced by at least 10.

**Tests**:
- Existing `cmd/bootstrap` integration tests must continue to pass.

## Task 3: Promote applyHostNoiseMitigation into internal/portaltest
status: pending
severity: medium
sources: duplication-c1#finding-3, architecture-c1#finding-1

**Problem**: The two-line helper `t.Setenv("HOME", t.TempDir()); t.Setenv("XDG_CONFIG_HOME", "")` is inlined in three test packages (cmd/bootstrap, cmd, internal/tmux), each with a ~12-line apologetic rationale comment, because the canonical copy in a sibling `_test` file cannot be cross-imported.

**Solution**: Promote into `internal/portaltest` as `NeutralizeHostEnv(t *testing.T)` (or fold into `NewIsolatedStateEnv` so callers cannot forget the ordering invariant — preferred). Delete all three private copies.

**Outcome**: Single canonical host-noise mitigation primitive. Rationale comment lives in one place. ~36 lines of duplicated comment + body removed.

**Do**:
1. Add `NeutralizeHostEnv(t *testing.T)` to `internal/portaltest` with the consolidated rationale comment.
2. Decide whether to fold into `NewIsolatedStateEnv` (preferred) or expose separately; if folded, document the ordering invariant in `NewIsolatedStateEnv`'s godoc.
3. Delete `applyHostNoiseMitigation` from `cmd/bootstrap/orphan_sweep_integration_test.go:412-427`.
4. Delete the local twin from `cmd/state_daemon_self_supervision_integration_test.go`.
5. Delete the local twin from `internal/tmux/portal_saver_endstate_integration_test.go`.
6. Replace call sites accordingly.

**Acceptance Criteria**:
- `internal/portaltest` exports the host-env neutralization primitive (or it is folded into `NewIsolatedStateEnv` and documented).
- Zero inlined copies of the two-line body remain outside `internal/portaltest`.
- Rationale comment exists exactly once.
- `go test -tags integration ./...` passes.

**Tests**:
- All three integration tests previously calling the inlined helper continue to pass.
- If exposed as a separate function, add a basic unit test confirming env vars are cleared/reset under `t.Cleanup`.

## Task 4: Collapse duplicated identify/read-PID seam pairs in portal_saver.go
status: pending
severity: medium
sources: architecture-c1#finding-2

**Problem**: `internal/tmux/portal_saver.go:178-279` exposes two pairs that are the same primitive under different names: (a) `killBarrierIdentifyDaemon` and `saverReadinessIdentify` both wrap `state.IdentifyDaemon`; (b) `killBarrierReadPID` and `saverReadinessReadPID` both wrap `state.ReadPIDFile`. No test composes both paths through a single stub. Package surface (12 package-level vars) is over-segmented.

**Solution**: Collapse each duplicated pair to a single seam (`saverIdentifyDaemon`, `saverReadPID`) shared between kill-barrier escalation and readiness barrier.

**Outcome**: 12 → 10 package-level seams in `internal/tmux`. Cleaner surface; one seam per primitive.

**Do**:
1. Introduce `saverIdentifyDaemon = state.IdentifyDaemon` and `saverReadPID = state.ReadPIDFile`.
2. Replace all internal references to `killBarrierIdentifyDaemon` / `saverReadinessIdentify` with `saverIdentifyDaemon`.
3. Replace all internal references to `killBarrierReadPID` / `saverReadinessReadPID` with `saverReadPID`.
4. Delete the four obsolete vars.
5. Update tests in `internal/tmux/*_test.go` that stage these seams; verify kill-barrier and readiness-barrier scenarios stage distinct outcomes through the same seam by ordering / setup-teardown.

**Acceptance Criteria**:
- Exactly two seams exist in `portal_saver.go` for identify/read-PID primitives (down from four).
- Package-level seam count reduced from 12 to 10.
- All existing kill-barrier and readiness-barrier tests pass without behavioral regression.
- `go test -tags integration ./internal/tmux/...` passes.

**Tests**:
- Existing kill-barrier escalation and readiness-barrier tests continue to pass.
- Add or adjust at least one test demonstrating the unified seam can stage distinct outcomes across the two code paths via sequential staging.

## Task 5: Colocate WriteVersionFile with WritePIDFile in defaultDaemonRun
status: pending
severity: low
sources: architecture-c1#finding-3

**Problem**: T4-8 moved `acquireDaemonLock` + `WritePIDFile` into `defaultDaemonRun` (cmd/state_daemon.go:189-209), but `WriteVersionFile` was left at RunE (cmd/state_daemon.go:446). Startup writes split across two functions.

**Solution**: Move `WriteVersionFile` into `defaultDaemonRun` immediately after `WritePIDFile`. The AST adjacency invariant asserts the acquire→pidfile relationship only, so colocation does not break it.

**Outcome**: All startup writes live in one function.

**Do**:
1. Move the `WriteVersionFile` call from `RunE` (cmd/state_daemon.go:446) into `defaultDaemonRun` immediately after `WritePIDFile`.
2. Run `TestDaemonAcquireLockOrdering_WritePIDFollowsAcquire` to confirm the AST invariant still holds.
3. Verify no change in error propagation / cleanup semantics.
4. Add a comment above the block listing the sequence: lock → pidfile → versionfile.

**Acceptance Criteria**:
- `WriteVersionFile` is invoked from `defaultDaemonRun`, not `RunE`.
- `TestDaemonAcquireLockOrdering_WritePIDFollowsAcquire` passes unchanged.
- All `cmd` daemon tests pass.
- `WriteVersionFile` error handling parity verified.

**Tests**:
- Existing daemon-startup tests continue to pass.
- Add a regression test asserting `WriteVersionFile` is called during `defaultDaemonRun` (check the version file exists after a successful run in an isolated state dir).

## Task 6: Document the bootstrap.Logger four-method contract
status: pending
severity: low
sources: architecture-c1#finding-4

**Problem**: `bootstrap.Logger` (cmd/bootstrap/bootstrap.go:171-195) gained `Info` in T4-3, bringing mandatory surface to Debug/Info/Warn/Error. Future contributors might bolt on Trace/Fatal without thinking about emission-site coupling.

**Solution**: Add a godoc comment to `bootstrap.Logger` noting that the four methods correspond to Run's emission levels and a fifth must not be added without a corresponding emission site.

**Outcome**: Interface contract is self-documenting.

**Do**:
1. Open `cmd/bootstrap/bootstrap.go:171-195`.
2. Add a godoc comment above the `Logger` interface explaining the four-method contract and warning against adding a fifth without a corresponding emission site.
3. Verify via `go doc cmd/bootstrap.Logger`.

**Acceptance Criteria**:
- `bootstrap.Logger` has a godoc comment explaining the four-method contract.
- Comment explicitly warns against adding a fifth method without a corresponding emission site.
- `go vet ./...` passes.

**Tests**:
- No new tests required (documentation-only). Confirm `go doc` displays the comment.
