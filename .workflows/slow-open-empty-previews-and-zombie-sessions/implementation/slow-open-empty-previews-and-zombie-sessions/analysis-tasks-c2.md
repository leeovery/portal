---
topic: slow-open-empty-previews-and-zombie-sessions
cycle: 2
total_proposed: 9
---
# Analysis Tasks: slow-open-empty-previews-and-zombie-sessions (Cycle 2)

## Task 1: Replace local fingerprint helpers in composition_e2e_self_eject_integration_test.go with portaltest helpers
status: approved
severity: high
sources: duplication-c2#1

**Problem**: T6-6 (composition_e2e_self_eject_integration_test.go:442-577) authored ~135 LOC of fingerprint diff/format/sort helpers (`assertScrollbackSnapshotsEqualSelfEjectComposite`, `fingerprintFieldDeltasSelfEjectComposite`, `formatFingerprintSelfEjectComposite`, `sortedSnapshotKeysSelfEjectComposite`, `joinDiagLinesSelfEjectComposite`) concurrently with T7-1's consolidation of the same suite into `internal/portaltest`. Format strings are byte-identical to `portaltest.FormatDelta`; field branches replicate the exported `DiffFingerprints` switch.

**Solution**: Replace the assertion body with the `portaltest.DiffFingerprints` + `portaltest.FormatDelta` loop used at `cmd/bootstrap/composition_abc_integration_test.go:272-284`. Delete the four sibling local helpers.

**Outcome**: ~-135 LOC; single source of truth for fingerprint diffs across the entire `cmd/bootstrap` package.

**Do**:
1. Read `composition_e2e_self_eject_integration_test.go` to find the call site of `assertScrollbackSnapshotsEqualSelfEjectComposite`.
2. Replace its body with `deltas := portaltest.DiffFingerprints(snapBefore, snapAfter); if len(deltas) > 0 { for _, d := range deltas { t.Errorf("scrollback delta: %s", portaltest.FormatDelta(d)) }; t.FailNow() }` (or mirror composition_abc's exact pattern).
3. Delete the five local helpers.
4. `go test -tags integration -run TestCompositeBootstrap_ExternalSaverKillTriggersSelfEject ./cmd/bootstrap/...` — must pass.

**Acceptance Criteria**:
- Five local helper definitions removed.
- Call site uses `portaltest.DiffFingerprints` + `portaltest.FormatDelta`.
- The integration test still passes.
- Net LOC reduction ≥ 100.

**Tests**: Existing self-eject integration test must continue to pass.

## Task 2: Collapse SaverPanePID and FirstPanePIDInSession into one helper
status: approved
severity: medium
sources: duplication-c2#3, architecture-c2#1

**Problem**: `tmux.SaverPanePID(c, name)` (saver_pane_pid.go:44, added T5-2) and `tmux.Client.FirstPanePIDInSession(name)` (tmux.go:575, added T4-4) both run `list-panes -t =<name> -F '#{pane_pid}'` and parse the first PID. Divergences: (i) `-s` flag presence, (ii) empty-output shape ((0, nil) vs ErrEmptyPaneList), (iii) "no such session" sentinel wrapping. Both call sites treat any error as "absent"; neither needs the divergent error contracts. Two failure-mode taxonomies for the same tmux call at adjacent component boundaries is an integration hazard.

**Solution**: Collapse to one helper. Promote `SaverPanePID` semantics (typed sentinels, no `-s` flag) to be the lone primitive; have `bootstrapadapter/orphan_sweep.go` call it directly. The `-s` distinction in `FirstPanePIDInSession` is NOT load-bearing for the orphan-sweep use case — verify and remove. If callers truly need `-s` semantics later, add a parameter.

**Outcome**: One helper, one error taxonomy. Adapter at orphan_sweep.go:124 no longer needs to call `HasSession` first (the sentinel handles absent).

**Do**:
1. Verify the `-s` flag is or isn't load-bearing for the bootstrapadapter use case — read the original task and the spec.
2. Delete `Client.FirstPanePIDInSession` from `internal/tmux/tmux.go`.
3. Update `internal/bootstrapadapter/orphan_sweep.go` to call `tmux.SaverPanePID` via `errors.Is` for `ErrNoSuchSession` / `ErrEmptyPaneList` → "legitimate set empty".
4. Remove the now-redundant `HasSession` pre-check.
5. Run tests + integration suite.

**Acceptance Criteria**:
- `FirstPanePIDInSession` removed from `internal/tmux`.
- `bootstrapadapter/orphan_sweep.go` uses `tmux.SaverPanePID` with `errors.Is` classification.
- All existing tests pass.

**Tests**: Existing orphan-sweep and saver-membership tests must continue to pass.

## Task 3: Unify the pgrep-portal-daemon regex pattern and enumeration helper
status: approved
severity: medium
sources: duplication-c2#2

**Problem**: The canonical regex `"^portal state daemon( |$)"` is declared three times (production adapter, test, `internal/state/daemon_identity.go:38`). The pgrep enumeration logic is duplicated between `internal/bootstrapadapter/orphan_sweep.go:60-110` and `cmd/bootstrap/orphan_sweep_integration_test.go:92-96, 515-549`.

**Solution**: Promote a single exported pattern constant — most naturally `state.PortalDaemonArgvPattern` — so `state.IdentifyDaemon`'s `regexp.MustCompile` and the adapter's `pgrepDaemonPattern` const both reference it. For the pgrep enumeration: since test cannot import production internals freely, promote the enumeration helper into `internal/portaltest.PgrepPortalDaemons()`.

**Outcome**: One regex declaration, one pgrep enumeration.

**Do**:
1. Add `state.PortalDaemonArgvPattern = "^portal state daemon( |$)"` exported const.
2. Update `internal/state/daemon_identity.go` and `internal/bootstrapadapter/orphan_sweep.go` to reference the shared const.
3. Add `portaltest.PgrepPortalDaemons() ([]int, error)` mirroring the adapter's exit-1+empty-stdout semantics.
4. Update `cmd/bootstrap/orphan_sweep_integration_test.go` to call the portaltest helper.
5. Delete the test's local `pgrepCandidatePattern` const, `pgrepPortalDaemonCount`, and `pgrepPortalDaemonPIDs`.

**Acceptance Criteria**:
- Regex declared exactly once.
- pgrep enumeration logic declared exactly once.
- All tests pass.

**Tests**: Existing orphan-sweep tests must continue to pass.

## Task 4: Unify recordingLogger and captureLogger into a single Logger fake
status: approved
severity: low
sources: duplication-c2#4

**Problem**: `recordingLogger` (bootstrap_test.go:98-127) and `captureLogger` (orphan_sweep_integration_test.go:359-404) are both `bootstrap.Logger` fakes living in the same `package bootstrap_test`. The captureLogger docstring claims the duplication is for cross-package reasons but both are in `bootstrap_test`.

**Solution**: Delete `captureLogger`; extend `recordingLogger` with `allEntries()` drawing from all four level slices. Migrate captureLogger call sites.

**Outcome**: One fake. Net ~-45 LOC.

**Do**:
1. Add `allEntries() []string` (or whatever shape captureLogger callers consume) to `recordingLogger`.
2. Delete `captureLogger`.
3. Migrate the call sites in `composition_e2e_convergence_integration_test.go:85`, `orphan_sweep_integration_test.go:240`, etc.

**Acceptance Criteria**: One Logger fake remains in `package bootstrap_test`. All existing tests pass.

**Tests**: Existing tests using either fake must continue to pass.

## Task 5: Replace sorted-map-keys helpers with slices.Sorted + maps.Keys
status: approved
severity: low
sources: duplication-c2#5

**Problem**: Three trivial "collect keys, sort.Strings, return" helpers (`sortedSnapKeys`, `sortedKeys`, `sortedSnapshotKeys`) exist across three integration test files. `slices.Sorted(maps.Keys(m))` (Go 1.21+) removes the need for any of them.

**Solution**: Replace each call site inline; delete the helpers.

**Outcome**: -3 helpers; idiomatic stdlib usage.

**Do**:
1. Replace each helper call with `slices.Sorted(maps.Keys(m))` inline.
2. Delete the three helper declarations.

**Acceptance Criteria**: All three helpers removed. All tests pass.

**Tests**: Existing tests must continue to pass.

## Task 6: Consolidate portal_saver.go seams into seam structs with one setter idiom
status: approved
severity: low
sources: architecture-c2#2

**Problem**: `internal/tmux/portal_saver.go` has ~18 package-level mutable seams + ~13 `*Seam()` accessors + two `Set*` setter functions. Three different setter idioms within one file: (a) bare var with no setter, (b) `Set*` function with nil-guard, (c) `*Seam()` returning `*Func` for direct write. Cumulative surface is the largest implicit-dependency cluster in the codebase.

**Solution**: Bundle related seams into one or two seam structs (e.g. `killBarrierSeams{...}`, `saverReadinessSeams{...}`) so tests swap whole clusters atomically. Pick ONE setter idiom uniformly.

**Outcome**: Cosmetic; materially lowers the file's cognitive load. Tests update to swap structs instead of individual vars.

**Do**:
1. Identify related seam clusters (kill-barrier, readiness-barrier, version, etc.).
2. Bundle each cluster into a struct.
3. Replace setter idioms uniformly (recommend the `*Seam()` returning pointer pattern; check existing precedent before deciding).
4. Update tests accordingly.

**Acceptance Criteria**: Seam clusters bundled. One setter idiom across the file.

**Tests**: All existing tests must continue to pass.

## Task 7: Rename killBarrierLogger to saverBarrierLogger
status: approved
severity: low
sources: architecture-c2#3

**Problem**: `waitForSaverDaemonReady` (Component F readiness barrier — distinct from the kill barrier) emits its timeout WARN through `killBarrierLogger.Warn(...)` (portal_saver.go:227, 440). Name lies about scope.

**Solution**: Rename to `saverBarrierLogger` (or `portalSaverLogger`) with `SetBarrierLogger` / `BarrierLogger` renamed in lockstep.

**Outcome**: Maintainer searching for readiness-barrier WARN emission can find it via the broader sink name.

**Do**:
1. Rename `killBarrierLogger` → `saverBarrierLogger` in portal_saver.go.
2. Rename setter and interface as needed.
3. Update production wiring in bootstrapadapter and any other call sites.
4. Update tests.

**Acceptance Criteria**: No reference to `killBarrierLogger` remains. All tests pass.

**Tests**: All existing tests must continue to pass.

## Task 8: Replace fmt.Sprintf("%d", pid) with strconv.Itoa(pid) in identifyPS
status: approved
severity: low
sources: architecture-c2#4

**Problem**: `internal/state/daemon_identity.go:56` uses `fmt.Sprintf("%d", pid)` for a single int, pulling fmt's reflection machinery where `strconv.Itoa(pid)` would format in one allocation. File's docstring emphasises low-overhead identity checks.

**Solution**: One-line swap.

**Outcome**: Marginal performance + idiomatic Go.

**Do**:
1. Edit daemon_identity.go:56 to use `strconv.Itoa(pid)`.
2. Update import if needed.

**Acceptance Criteria**: `fmt` no longer imported in the file (if it was only for this call); `strconv` imported. All tests pass.

**Tests**: Existing daemon_identity tests must continue to pass.

## Task 9: Document or split NewIsolatedStateEnv to reflect parent-env mutation
status: approved
severity: low
sources: architecture-c2#5

**Problem**: `NewIsolatedStateEnv(t)` (internal/portaltest/isolated_env.go:58-59) calls `t.Setenv("HOME", ...)` and `t.Setenv("XDG_CONFIG_HOME", "")` on the caller's process before returning the subprocess env-slice. Correct for the host-noise mitigation use case (fully documented) but the API conflates "build subprocess env" with "scrub parent env". Constructor-shaped name understates the side-effect surface.

**Solution**: Pick one — (a) rename to `SetupIsolatedStateEnv(t)` to signal mutation, OR (b) split into `scrubHostEnv(t)` + `BuildIsolatedEnv(t)` so callers compose explicitly. (a) is lower-effort and preserves current caller call sites.

**Outcome**: API name better reflects side effects.

**Do**:
1. Pick option (a) or (b) — recommend (a) for minimal disruption.
2. Apply rename and update all callers.

**Acceptance Criteria**: New name in place; all callers migrated; tests pass.

**Tests**: Existing tests using `NewIsolatedStateEnv` must continue to pass under the new name.
