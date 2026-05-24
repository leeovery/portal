---
topic: slow-open-empty-previews-and-zombie-sessions
cycle: 3
total_proposed: 7
---
# Analysis Tasks: slow-open-empty-previews-and-zombie-sessions (Cycle 3)

## Task 1: Promote pgrep enumeration to state.PgrepPortalDaemons
status: approved
severity: high
sources: duplication-c3#1, architecture-c3#4

**Problem**: T8-3 added `portaltest.PgrepPortalDaemons` and unified the regex const via `state.PortalDaemonArgvPattern`, but did NOT collapse the production adapter's `pgrepPortalDaemons` to use the new helper. Both functions are byte-equivalent (`exec.Command("pgrep", "-fx", state.PortalDaemonArgvPattern)`, identical exit-1-empty-stdout handling, identical parse loop). Risk: parsing changes drift silently between production and test enumerators.

**Solution**: Extract `state.PgrepPortalDaemons() ([]int, error)` as the single primitive. Both the bootstrapadapter and portaltest helpers become thin forwarders.

**Outcome**: One pgrep enumeration implementation. Production and test cannot drift.

**Do**:
1. Add `state.PgrepPortalDaemons() ([]int, error)` in `internal/state/daemon_identity.go` (or sibling file in state). Implementation lifts the existing logic from `internal/bootstrapadapter/orphan_sweep.go:75-109`.
2. Refactor `internal/bootstrapadapter/orphan_sweep.go`'s `pgrepPortalDaemons` to forward to `state.PgrepPortalDaemons`.
3. Refactor `internal/portaltest/pgrep.go`'s `PgrepPortalDaemons` to forward to `state.PgrepPortalDaemons`.
4. Run tests.

**Acceptance Criteria**:
- `state.PgrepPortalDaemons` exists with the canonical pgrep logic.
- Both `internal/bootstrapadapter/orphan_sweep.go` and `internal/portaltest/pgrep.go` are forwarders.
- All tests pass.

**Tests**: Existing orphan-sweep and pgrep-using tests must continue to pass.

## Task 2: Add tmux.SaverPanePIDOrAbsent helper centralizing "any error → absent" rule
status: approved
severity: medium
sources: architecture-c3#1

**Problem**: The "what PID owns the `_portal-saver` pane?" question is asked from 3 sites, each decoding `tmux.SaverPanePID`'s rich sentinel contract differently. The "treat any error as absent" rule lives in prose comments rather than a typed helper.

**Solution**: Add `SaverPanePIDOrAbsent(c, name) (pid int, present bool, err error)` in `internal/tmux/` that owns the sentinel collapse. Refactor both Component B's adapter and Component D's probe to consume it.

**Outcome**: One helper owns the absent-collapse policy. Callers express only their own response to `present == false`.

**Do**:
1. Add `tmux.SaverPanePIDOrAbsent(c, name) (pid int, present bool, err error)` in `internal/tmux/saver_pane_pid.go`. Implementation calls `SaverPanePID`, treats `ErrNoSuchSession`/`ErrEmptyPaneList` as `(0, false, nil)`, other errors as `(0, false, err)`, success as `(pid, true, nil)`.
2. Refactor `internal/bootstrapadapter/orphan_sweep.go::saverPanePID` to use the new helper.
3. Refactor `cmd/state_daemon.go::defaultSaverMembershipProbe` to use the new helper.
4. Run tests.

**Acceptance**: Both call sites delegate the absent-collapse to the new helper. All tests pass.

**Tests**: Existing orphan-sweep and self-supervision tests must continue to pass.

## Task 3: Delete waitForAnyDaemonPID (functionally identical to waitForDaemonPID)
status: approved
severity: medium
sources: duplication-c3#2

**Problem**: `waitForDaemonPID` (upgrade_path_integration_test.go) and `waitForAnyDaemonPID` (composition_e2e_harness_integration_test.go) are functionally identical. `waitForAnyDaemonPID`'s docstring claims it is "distinct" but the body enforces `pid == orphanPID` exactly like `waitForDaemonPID`.

**Solution**: Delete `waitForAnyDaemonPID`; have callers use `waitForDaemonPID(t, stateDir, legitimateDaemonPID)` directly.

**Outcome**: One helper. Net ~-20 LOC.

**Do**:
1. Identify callers of `waitForAnyDaemonPID` (likely 1-2 sites).
2. Replace with `waitForDaemonPID(t, stateDir, expectedPID)`.
3. Delete `waitForAnyDaemonPID` and its (incorrect) docstring.
4. Reconcile the constant pairs (`compositeOrphanPIDTimeout`/`Tick` vs `upgradePathPIDFileTimeout`/`Tick`).

**Acceptance**: One helper remains. All tests pass.

**Tests**: Existing tests must continue to pass.

## Task 4: Promote ReadPortalLogSafe to internal/portaltest
status: approved
severity: medium
sources: duplication-c3#3

**Problem**: `readPortalLogSafe` (cmd/state_daemon_self_supervision_integration_test.go) and `readPortalLogSafeBootstrap` (cmd/bootstrap/composition_e2e_self_eject_integration_test.go) are byte-identical wrappers around `os.ReadFile(state.PortalLog(stateDir))`. The duplication is documented but never actioned.

**Solution**: Promote `ReadPortalLogSafe(stateDir string) string` to `internal/portaltest`. Both call sites drop their local defs.

**Outcome**: One helper. Net ~-20 LOC.

**Do**:
1. Add `portaltest.ReadPortalLogSafe(stateDir string) string` (consider whether it should take *testing.T to satisfy the leaf-package convention — likely yes for consistency).
2. Delete both local helpers.
3. Update ~14 call sites to use the portaltest helper.

**Acceptance**: One ReadPortalLogSafe in `internal/portaltest`. All tests pass.

**Tests**: Existing tests must continue to pass.

## Task 5: Rename NewIsolatedStateEnv to communicate parent-env mutation in its name
status: approved
severity: medium
sources: architecture-c3#3

**Problem**: T8-9 documented the parent-env mutation via a "SIDE EFFECT" leading paragraph but kept the `New*` name. Architectural concern: the API SHAPE still misleads at the call site. A contributor reading `env, stateDir := portaltest.NewIsolatedStateEnv(t)` has no syntactic cue that HOME just changed.

**Solution**: Pick one — (a) rename to `IsolateStateForTest(t)` or `MustIsolateStateDir(t)` to signal mutation, OR (b) split into `ScrubHostEnv(t)` + `BuildIsolatedEnv(t)` so callers compose explicitly.

**Outcome**: API name better reflects side effects.

**Do**:
1. Pick option (a) or (b). Recommend (a) — single rename.
2. Apply rename across all ~63 call sites (use grep + bulk replace).
3. Verify tests still pass.

**Acceptance**: New name in place; all callers migrated.

**Tests**: Existing tests must continue to pass.

## Task 6: Embed or collapse the 5 Saver*Seams structs
status: approved
severity: low
sources: architecture-c3#2

**Problem**: T8-6 created 5 separate Saver*Seams structs. `SaverSharedSeams` exists precisely because Barrier and Readiness both need `ReadPID + IdentifyDaemon` — yet the structs remain peers rather than embedded. Tests reach across two structs to drive one logical state machine.

**Solution**: Either (a) embed `SaverSharedSeams` into `SaverBarrierSeams` and `SaverReadinessSeams` so each consumer references one struct, OR (b) collapse all 5 into a single `SaverSeams` with grouped sub-fields.

**Outcome**: Cleaner test ergonomics; consumers reference one struct per code path.

**Do**:
1. Choose (a) or (b).
2. Refactor portal_saver.go.
3. Update export_test.go accessors.
4. Run tests.

**Acceptance**: One struct per consumer path (Barrier, Readiness, etc.) OR one composite struct. Tests pass.

**Tests**: Existing tests must continue to pass.

## Task 7: Fix Component C log-acquire error level (Error → Warn per spec)
status: approved
severity: low
sources: standards-c3#1

**Problem**: Spec § Component C step 4 mandates "log WARN under ComponentDaemon and exit with status 1". Implementation at `cmd/state_daemon.go:209-211` emits `Logger.Error` instead of `Logger.Warn`. Practical impact minor (both surface in portal.log) but the deviation contradicts a verbatim spec mandate.

**Solution**: Change `Logger.Error` to `Logger.Warn`. Retain the wrapped-error return so the daemon still exits status 1.

**Outcome**: Spec verbatim compliance for the lock-acquire WARN path.

**Do**:
1. Edit `cmd/state_daemon.go:210` to use `deps.Logger.Warn` instead of `deps.Logger.Error`.
2. Run tests.

**Acceptance**: Production emits WARN for wrapped lock-acquire errors. Exit status 1 preserved.

**Tests**: If any test asserts on the log level for this path, update accordingly.
