---
topic: slow-open-empty-previews-and-zombie-sessions
cycle: 4
total_proposed: 3
---
# Analysis Tasks: slow-open-empty-previews-and-zombie-sessions (Cycle 4)

## Task 1: Promote orphan-daemon spawn + reap-cleanup helpers to internal/portaltest
status: approved
severity: medium
sources: duplication

**Problem**: The "spawn an isolated `portal state daemon` subprocess and arrange a guaranteed reap" pattern is correctly factored inside `bootstrap_test` (via `spawnOrphanDaemonIsolated` at `cmd/bootstrap/composition_e2e_harness_integration_test.go:382-394` and `registerSubprocessCleanup` at `cmd/bootstrap/orphan_sweep_integration_test.go:365-381`), but is reimplemented verbatim in two `internal/tmux/*_integration_test.go` files because the helpers cannot be cross-imported from `bootstrap_test`. The reap-goroutine block in `internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go:191-207` is line-for-line equivalent to `registerSubprocessCleanup`, and `internal/tmux/portal_saver_endstate_integration_test.go:312-334` inlines a simpler Kill+Wait variant of the same envelope. The load-bearing rationale comments (darwin `comm`-match requires unqualified `"portal"` argv[0]; SIGKILL belt-and-braces; errors swallowed because the daemon is typically already exited) are triplicated.

**Solution**: Promote two helpers to a new `internal/portaltest/spawn_daemon.go` file, mirroring the T9-1 / T9-4 / T9-5 promotion pattern that established `internal/portaltest` as the canonical cross-package test-helper home. Then switch the `bootstrap_test` helpers to one-line forwarders (or delete them, migrating call sites), and replace the inline blocks in the two `internal/tmux` integration tests with helper calls.

**Outcome**: ~50 LOC removed; one canonical location for the darwin-`comm` / SIGKILL / reap-goroutine rationale comments; future orphan-daemon-spawning tests in any package have a single seam to reach for.

**Do**:
1. Create `/Users/leeovery/Code/portal/internal/portaltest/spawn_daemon.go` exposing:
   - `SpawnIsolatedDaemon(t *testing.T, envSlice []string) (*exec.Cmd, string)` — spawns `portal state daemon` with `PORTAL_STATE_DIR` appended to `envSlice`, calls `Start`, registers Kill+reap cleanup, and returns the cmd plus the stateDir. Use the same unqualified `"portal"` argv[0] convention (load-bearing for darwin `comm`-match) as the existing helpers.
   - `RegisterSubprocessCleanup(t *testing.T, cmd *exec.Cmd) <-chan struct{}` — the reap-goroutine + `t.Cleanup{Kill; <-reaped}` primitive. Returns the `reaped` channel so callers needing to time process death can `select` on it.
2. Move the load-bearing rationale comments (unqualified `portal` argv[0] reason, SIGKILL belt-and-braces, errors-swallowed-because-typically-already-exited) into the helper godoc — single canonical location.
3. In `cmd/bootstrap/composition_e2e_harness_integration_test.go`, replace `spawnOrphanDaemonIsolated` with a one-line forwarder to `portaltest.SpawnIsolatedDaemon` (or delete and migrate call sites in `composition_abc_integration_test.go` and `upgrade_path_integration_test.go`).
4. In `cmd/bootstrap/orphan_sweep_integration_test.go`, replace `registerSubprocessCleanup` similarly.
5. In `cmd/bootstrap/upgrade_path_integration_test.go:150-158`, switch the inline v(N) daemon spawn to call the new helper.
6. In `internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go:176-207`, replace the ~30-line spawn + reap-goroutine + Cleanup Kill block with a 2-line `portaltest.SpawnIsolatedDaemon` + (if needed) `portaltest.RegisterSubprocessCleanup` call.
7. In `internal/tmux/portal_saver_endstate_integration_test.go:312-334`, replace the inline seeded competing-daemon spawn with the helper. The canonical reap-goroutine shape works for both — the "no separate reaper" variant here is gratuitous.
8. Run `go test ./...` and confirm all integration-tagged tests still pass (`go test -tags integration ./cmd/bootstrap/... ./internal/tmux/...`).

**Acceptance Criteria**:
- `internal/portaltest/spawn_daemon.go` exists with `SpawnIsolatedDaemon` and `RegisterSubprocessCleanup` exported, both taking `*testing.T` as first arg (preserves the structural-prohibition-on-production-import contract per CLAUDE.md "DI / testing pattern").
- The five call sites listed in the duplication finding now go through the helpers — no remaining inline `make(chan struct{}) + go func(){ Wait; close(reaped) }() + t.Cleanup{Kill; <-reaped}` blocks outside the helper.
- Load-bearing rationale comments (unqualified `portal` argv[0]; SIGKILL; reap-goroutine reason) appear exactly once in the codebase, on the helper godoc.
- Net LOC delta is negative (~-50).
- `go test ./...` and `go test -tags integration ./cmd/bootstrap/... ./internal/tmux/...` both pass.
- No production package gains a dependency on `internal/portaltest` (the helper imports `testing`, which is structurally barred from non-`*_test.go` files).

**Tests**:
- Existing integration tests in `cmd/bootstrap/` and `internal/tmux/` continue to pass — no new tests required; the helpers are exercised by the tests being migrated.
- Spot-check at least one test from each of the two packages confirms the migrated test still spawns and reaps cleanly with no zombie processes (the post-test `IsolateStateForTest` fingerprint backstop will catch any state-dir leak).

---

## Task 2: Encode tri-state contract at the OrphanSweepCore.SaverPanePID seam boundary
status: approved
severity: low
sources: architecture

**Problem**: The `OrphanSweepCore.SaverPanePID` seam at `cmd/bootstrap/orphan_sweep.go:60-67` is shaped `func() (int, error)`. The production adapter at `internal/bootstrapadapter/orphan_sweep.go:66-75` wraps the richer `tmux.SaverPanePIDOrAbsent` (which returns `(pid int, present bool, err error)`) by collapsing `!present → (0, nil)`. The consumer at `cmd/bootstrap/orphan_sweep.go:141-148` then must handle three observable shapes — `saverErr != nil` (warn), `saverPID > 0` (legitimate), and the unstated default `(0, nil) = absent` — but the seam's type signature does not encode "0 with nil error means absent". A future implementer returning a real PID of 0 (defensively, on error) would silently flip "absent" into "legitimate empty" with no compile-time signal. The tri-state already exists at the helper layer; flattening it at the seam boundary is information loss for no testability gain.

**Solution**: Widen the seam to `func() (pid int, present bool, err error)` matching the underlying helper, and update the consumer's switch to read `case !present:` explicitly. Alternatively, drop the adapter wrapper and inline `tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName)` at the seam closure site so the convention lives next to its single owner.

**Outcome**: The absent-vs-error distinction is pinned at the type level; a defensive `return 0, ...` from a future implementer cannot silently become "legitimate empty"; one indirection deleted.

**Do**:
1. Choose the approach: widen the seam signature (preserves the adapter as a documentable wrapper) OR inline the helper at the seam closure site (deletes the adapter wrapper entirely). Pick the widen approach — it preserves the adapter contract for testability and matches the helper's existing tri-state.
2. In `cmd/bootstrap/orphan_sweep.go`, change `OrphanSweepCore.SaverPanePID` from `func() (int, error)` to `func() (pid int, present bool, err error)`.
3. In `cmd/bootstrap/orphan_sweep.go:141-148`, update the consumer switch to explicitly handle `case err != nil:` (warn), `case !present:` (skip — no saver pane to identity-check), `default:` / `case pid > 0:` (use pid). Remove any implicit reliance on `pid == 0` meaning absent.
4. In `internal/bootstrapadapter/orphan_sweep.go:66-75`, change the adapter wrapper to pass `tmux.SaverPanePIDOrAbsent`'s three return values through directly instead of collapsing `!present → (0, nil)`.
5. Update any unit tests in `cmd/bootstrap/` that stub the `SaverPanePID` seam to return the three-value shape.
6. Run `go test ./...` and confirm orphan-sweep tests still pass.

**Acceptance Criteria**:
- `OrphanSweepCore.SaverPanePID` seam signature is `func() (pid int, present bool, err error)`.
- The consumer switch in `cmd/bootstrap/orphan_sweep.go` reads the `present` boolean explicitly — no `pid == 0` branch.
- The adapter at `internal/bootstrapadapter/orphan_sweep.go` no longer collapses `!present → (0, nil)`; it forwards the helper's tri-state verbatim.
- All existing orphan-sweep unit and integration tests pass without behavioral change.
- A defensive `return 0, true, nil` from a future implementer would now be a distinct case from `return 0, false, nil` at the type level.

**Tests**:
- Existing orphan-sweep tests in `cmd/bootstrap/orphan_sweep_*_test.go` cover the three branches — they must continue to pass after the seam-signature update (stubs updated to the three-value shape).
- No new tests required; the type-level encoding is the win.

---

## Task 3: Unexport tmux.SaverPanePID since SaverPanePIDOrAbsent is the sole production entry point
status: approved
severity: low
sources: architecture

**Problem**: Since the T9-2 promotion of `SaverPanePIDOrAbsent`, every production consumer (Component D probe at `cmd/state_daemon.go:106`, orphan-sweep adapter at `internal/bootstrapadapter/orphan_sweep.go:67`) routes through `SaverPanePIDOrAbsent` rather than the rich-sentinel `SaverPanePID` at `internal/tmux/saver_pane_pid.go:45-65`. The lower-level function remains exported with a fully documented `(int, error)` contract surfacing `ErrNoSuchSession` / `ErrEmptyPaneList` / `ErrPanePIDParse` — sentinels that no remaining out-of-package caller decodes. Keeping it exported invites future consumers to reach past the centralized "any-error → absent" rule by accident.

**Solution**: Unexport to `saverPanePID`. Callers within the package keep working; `SaverPanePIDOrAbsent` becomes the structurally enforced sole external entry point.

**Outcome**: The "any-error → absent" rule is the only path available to out-of-package callers; future contributors cannot accidentally bypass it.

**Do**:
1. In `/Users/leeovery/Code/portal/internal/tmux/saver_pane_pid.go`, rename the exported function `SaverPanePID` to `saverPanePID` (lowercase first letter).
2. Update all in-package callers (within `internal/tmux/`) to use the new lowercase name. `SaverPanePIDOrAbsent` is the only existing caller of this function based on the finding's description; update accordingly.
3. Update the function's godoc to reflect its now-internal status (drop any "exported entry point" framing if present).
4. Run `go build ./...` and `go test ./...` to confirm no out-of-package consumer remains.
5. If the build fails because a remaining out-of-package consumer exists (the finding implies none, but verify), migrate that consumer to `SaverPanePIDOrAbsent` instead of re-exporting.

**Acceptance Criteria**:
- `internal/tmux/saver_pane_pid.go` no longer exports `SaverPanePID`; the function is `saverPanePID`.
- `SaverPanePIDOrAbsent` remains the only exported entry point to the saver-pane PID lookup.
- `go build ./...` succeeds with no out-of-package consumer breakage.
- `go test ./...` passes — in-package tests of `saverPanePID` continue to work (test files in `internal/tmux/` can call the lowercase name).
- A grep for `tmux\.SaverPanePID\b` (word boundary, excluding `SaverPanePIDOrAbsent`) returns no production matches.

**Tests**:
- Existing in-package unit tests for `saverPanePID` and `SaverPanePIDOrAbsent` continue to pass — no new tests required.
- Confirm via `go build ./...` that the rename does not break any caller anywhere in the tree.
