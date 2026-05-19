---
topic: saver-kill-respawn-loop-leaks-daemons
cycle: 2
total_proposed: 3
---
# Analysis Tasks: saver-kill-respawn-loop-leaks-daemons (Cycle 2)

## Task 1: Update stale `portalSaverVersionMismatch` references in integration-test doc comments
status: approved
severity: low
sources: architecture

**Problem**: Two narrative doc comments in `internal/tmux/portal_saver_integration_test.go` (lines 105 and 170) still cite "the production portalSaverVersionMismatch comparison" as the production code path under test. Cycle 1 collapsed that helper into `shouldKillSaverOnVersionDecision` and removed the dead symbol from `portal_saver.go`. The drift is a future-reader trap.

**Solution**: Replace both occurrences with an accurate reference to the current production predicate.

**Outcome**: Integration-test doc comments name a symbol that actually exists; readers tracing the test to its production counterpart land on the real code path.

**Do**:
1. Open `/Users/leeovery/Code/portal/internal/tmux/portal_saver_integration_test.go`.
2. At line 105, replace "the production portalSaverVersionMismatch comparison" with "the production `shouldKillSaverOnVersionDecision` predicate" (or "the production version-mismatch comparison in `EnsurePortalSaverVersion`").
3. At line 170, apply the same replacement.
4. Run `go build ./...` and `go test ./internal/tmux/...` to confirm no incidental breakage.

**Acceptance Criteria**:
- `rg portalSaverVersionMismatch internal/tmux/` returns zero hits.
- Both doc comments name a symbol that exists in `portal_saver.go`.
- Tests still pass.

**Tests**:
- Re-run the existing integration tests around lines 105 and 170; no behavioural change expected.

## Task 2: Decide and act on `restoretest` package scope drift
status: approved
severity: low
sources: architecture

**Problem**: `internal/restoretest` was introduced as "shared restore drivers, real-tmux socket fixtures". Cycle 1 added `StagePortalBinary`, `BuildPortalBinary`, and `ProjectRoot` to `internal/restoretest/build.go` — generic "compile-portal-and-PATH-prepend" plumbing now consumed by daemon (`cmd/state_daemon_integration_test.go:49`), saver (`internal/tmux/portal_saver_integration_test.go:60`), and TUI integration tests with no semantic tie to restore. The package name misadvertises its contents.

**Solution**: Either rename `restoretest` to a domain-neutral name (e.g. `portaltest`) reflecting its actual scope, or extract the build-and-stage helpers into a sibling test-only package (e.g. `internal/portalbintest`).

**Outcome**: Package name matches contents; future readers can locate the portal-binary build helper by name without prior knowledge.

**Do**:
1. Choose one of the two refactors. Preferred: extract `BuildPortalBinary`, `StagePortalBinary`, and `ProjectRoot` into a new `internal/portalbintest` package (narrower change, leaves restore-specific helpers in `restoretest`).
2. Move `build.go` content into the new package; update package declaration.
3. Update imports at `cmd/state_daemon_integration_test.go:49`, `internal/tmux/portal_saver_integration_test.go:60`, and any other consumers found via `rg "restoretest\.(BuildPortalBinary|StagePortalBinary|ProjectRoot)"`.
4. Update the CLAUDE.md package table entry for `restoretest` (and add `portalbintest`) to reflect new scope.
5. Run `go build ./...` and `go test ./...`.

**Acceptance Criteria**:
- The portal-binary build helpers live in a package whose name reflects their scope.
- All integration tests still compile and pass.
- CLAUDE.md package table is updated to match.
- `restoretest` (if kept) contains only restore-domain helpers.

**Tests**:
- Run the full integration test suite touching daemon, saver, and any restore drivers to confirm no import or symbol breakage.

## Task 3: Collapse eight `install*` seam helpers into a single generic helper
status: approved
severity: low
sources: duplication

**Problem**: `internal/tmux/portal_saver_test.go` contains eight test helpers (`installBarrierReadPID`, `installBarrierIsAlive`, `installBarrierPollInterval`, `installBarrierTimeout`, `installBarrierLogger`, `installKillSaverFn`, `installReadVersionFile`, `installWriteVersionFile` at lines 964, 973, 982, 991, 1000, 1322, 1655, 2083) sharing an identical four-line body: `seam := tmux.<Name>Seam(); prev := *seam; *seam = fn; t.Cleanup(func() { *seam = prev })`. Only the seam getter and the parameter type vary. The repeated pattern is a maintenance burden — a future seam will likely add a ninth copy.

**Solution**: Introduce a Go-1.18+ generic helper `swapSeam[T any](t *testing.T, ptr *T, v T)` that performs the save-install-restore dance once. Either collapse each `install*` wrapper to a one-line call, or remove the wrappers and inline `swapSeam(t, tmux.<X>Seam(), <fn>)` at call sites.

**Outcome**: One canonical save-install-restore implementation; adding a future seam requires no new boilerplate helper.

**Do**:
1. Add to `internal/tmux/portal_saver_test.go` (or a shared test helper file in the same package):
   ```go
   func swapSeam[T any](t *testing.T, ptr *T, v T) {
       t.Helper()
       prev := *ptr
       *ptr = v
       t.Cleanup(func() { *ptr = prev })
   }
   ```
2. Choose one of two refactors:
   - **Option A (minimal)**: rewrite each of the eight `install*` bodies to a single `swapSeam(t, tmux.<X>Seam(), fn)` line.
   - **Option B (deeper)**: delete the eight wrappers and update all call sites to call `swapSeam` directly.
3. Run `go test ./internal/tmux/...` and confirm pass.

**Acceptance Criteria**:
- A single `swapSeam` generic helper exists in the test file.
- Either each `install*` helper is a one-liner delegating to `swapSeam`, or the wrappers are gone and call sites use `swapSeam` directly.
- All existing tests pass with no behavioural change.

**Tests**:
- Re-run the full `internal/tmux` test suite. Cleanup ordering (LIFO via `t.Cleanup`) must remain identical so seam-restore order is unchanged.
