---
phase: 2
phase_name: Delete PredictLiveIndices and the Misleading Drift Diagnostic
total: 2
---

## scrollback-not-restored-with-non-zero-base-index-2-1 | approved

### Task 1: Excise Diagnostic Prediction Path and Audit Test Scaffolding

**Problem**: `PredictLiveIndices` and its drift-warning consumer (`Orchestrator.warnOnPaneKeyDrift`) read `base-index` and `pane-base-index` via `GetServerOption`, which queries server-scope options. Neither index is a server option (they are session- and window-scope respectively), so `readIndexOption` always falls back to `0`, the predictor always returns `(0, 0)`, and the WARN fires whenever the user has non-zero indices — falsely implicating "drift" while the real hydration failure (Phase 1) sits elsewhere. The function has no functional consumer beyond this misleading diagnostic, and the spec's "Index Semantics" section mandates "re-query live indices, never predict." The dead path must be removed in full so the WARN can never fire again under any tmux config.

**Solution**: Delete the prediction symbols (`PredictLiveIndices`, `flattenSavedPanePositions`, the `savedPanePos` struct, and `readIndexOption` if it has no remaining callers) from `internal/restore/session.go`; delete `Orchestrator.warnOnPaneKeyDrift` and its call site from `internal/restore/restore.go`; sweep all `_test.go` files across the repo (especially `internal/restore/session_test.go`, `internal/restore/restore_test.go`, and anything under `cmd/bootstrap/`) for references to the deleted symbols and remove or refactor the affected scaffolding. Surface any unexpected production reference outside the deletion list for review rather than silently deleting it.

**Outcome**: `go build ./...` and `go test ./...` both pass; repo-wide `grep` for `PredictLiveIndices`, `warnOnPaneKeyDrift`, `flattenSavedPanePositions`, and `savedPanePos` returns zero hits; `readIndexOption` returns zero hits (or, if some unrelated caller is found, is retained with a justification noted in the commit message); the live-index path through `armPanes`, `ApplyWindowGeometry`, and `ApplySkeletonMarkers` is untouched; pane-count mismatch logging at `armPanes:202` is preserved verbatim.

**Do**:
- Run repo-wide grep for each of `PredictLiveIndices`, `warnOnPaneKeyDrift`, `flattenSavedPanePositions`, `savedPanePos`, and `readIndexOption` (e.g. `rg -n 'PredictLiveIndices|warnOnPaneKeyDrift|flattenSavedPanePositions|savedPanePos|readIndexOption' .`). Record every hit.
- For each hit not already in the deletion list (production or test): if it is a transitive consumer of a deletion target, add it to the deletion list; if it is unrelated, stop and surface it for review before proceeding.
- Edit `internal/restore/restore.go`: remove the call to `o.warnOnPaneKeyDrift(sr, sess, livePanes)` (currently around line 144) and the surrounding "Drift diagnostic" comment block; remove the entire `warnOnPaneKeyDrift` method (currently lines ~147-170).
- Edit `internal/restore/session.go`: remove `PredictLiveIndices` (currently ~lines 412-426) and its docstring; remove `flattenSavedPanePositions` and the `savedPanePos` struct (currently ~lines 172-191); remove `readIndexOption` (currently ~lines 428-442) only if no callers remain after the prior deletions — if a caller remains, leave `readIndexOption` in place and document the surviving caller in the commit message.
- Verify imports in both files are still used; remove `strconv` from `session.go` if `readIndexOption` was its last consumer.
- Audit `_test.go` files: in `internal/restore/session_test.go` and `internal/restore/restore_test.go`, delete tests targeting the removed symbols (e.g. `TestPredictLiveIndices*`, `TestWarnOnPaneKeyDrift*`, `TestFlattenSavedPanePositions*`); search `cmd/bootstrap/` for any fixtures, mocks, or helper functions referring to these names and remove or refactor them.
- Run `go build ./...` and `go test ./...`; fix any residual compilation or test failures before completing the task.
- Confirm `armPanes` in `internal/restore/session.go` still contains the `r.Logger.Warn(state.ComponentRestore, "session %q: live pane count %d != saved count %d ...", ...)` call at the original location (~line 202); do not touch it.

**Acceptance Criteria**:
- [ ] Repo-wide `grep` for `PredictLiveIndices`, `warnOnPaneKeyDrift`, `flattenSavedPanePositions`, and `savedPanePos` returns zero hits.
- [ ] `readIndexOption` returns zero hits, OR is retained with a documented surviving caller noted in the commit message.
- [ ] Any reference found outside the deletion list during pre-deletion grep was surfaced for review (and either added to the deletion list with rationale or left untouched), not silently deleted.
- [ ] `internal/restore/session.go::PredictLiveIndices` is removed.
- [ ] `internal/restore/session.go::flattenSavedPanePositions` and `savedPanePos` struct are removed.
- [ ] `internal/restore/restore.go::Orchestrator.warnOnPaneKeyDrift` is removed.
- [ ] The call site `o.warnOnPaneKeyDrift(...)` in the orchestrator's restore loop is removed.
- [ ] Test files across the repo no longer reference any deleted symbol; no dead mocks/fixtures remain in `cmd/bootstrap/`.
- [ ] `armPanes` pane-count mismatch warning at `internal/restore/session.go:202` is byte-identical to before.
- [ ] `go build ./...` succeeds with no errors.
- [ ] `go test ./...` succeeds with no failing tests.

**Tests**:
- `"go build ./... succeeds after deletion"` — compile gate proving no production caller of the deleted symbols was missed.
- `"go test ./... succeeds after deletion"` — proves no test reference was missed; covers the `_test.go` audit.
- Existing `armPanes` pane-count mismatch behavioural tests (whichever file exercises `ListPanesInSession` returning a count mismatch) continue to pass — proves the preserved warning at line 202 is intact.
- Existing live-index restore tests (the ones that exercise `Restore` → `armPanes` → `ApplyWindowGeometry` → `ApplySkeletonMarkers`) continue to pass — proves the live-index path is untouched.

**Edge Cases**:
- Unexpected reference outside deletion list: stop and surface for review (e.g. a comment in spec/docs referencing the symbol name, or a downstream consumer not anticipated). Do not silently delete.
- `readIndexOption` retains a caller after other deletions: keep `readIndexOption` and document the caller; do not force-remove a function that has live consumers.
- Test helpers/fixtures in `cmd/bootstrap/` (e.g. mock predictors, fake orchestrator wirings) still referencing the deleted symbols: remove them outright if they exist solely to mock the deleted surface; refactor them if they have other useful consumers.
- `armPanes:202` pane-count mismatch logging must remain intact — it is the spec's coarser but consistent replacement signal for the deleted predicted-vs-live diagnostic.
- Live-index path (`armPanes`, `ApplyWindowGeometry`, `ApplySkeletonMarkers`, `SanitizePaneKey` calls in the restore flow) must be byte-untouched.

**Context**:
> From specification § "Part 2 — Delete `PredictLiveIndices` and Its Consumers":
> "Delete the dead diagnostic prediction path entirely... Pre-deletion verification: For each symbol in the list above, confirm zero remaining references across the entire repository... If any references are found that are not also in the deletion list, surface them for review rather than silently deleting."
>
> From specification § "Rationale for deletion over repair":
> "The function exists only to power a diagnostic WARN with no functional consumer... Pane-count mismatch is already logged at `armPanes:202`, providing a coarser but consistent signal."
>
> The orchestrator call site is at `internal/restore/restore.go:144`. The diagnostic helper is at `restore.go:153`. The predictor is at `session.go:424`. The structural flattener and `savedPanePos` are at `session.go:175-191`. `readIndexOption` is at `session.go:432`. `armPanes:202` is the preserved pane-count mismatch log line.

**Spec Reference**: `.workflows/scrollback-not-restored-with-non-zero-base-index/specification/scrollback-not-restored-with-non-zero-base-index/specification.md` § "Part 2 — Delete `PredictLiveIndices` and Its Consumers"

## scrollback-not-restored-with-non-zero-base-index-2-2 | approved

### Task 2: Regression Assertion: No predicted-vs-live WARN Under Non-Zero base-index

**Problem**: Task 2-1 deletes the source of the misleading `predicted=...__0.0 live=...__X.Y` WARN, but the spec's AC #4 demands a runtime guarantee that the diagnostic is gone, not silenced. Without an automated regression assertion, a future re-introduction of any predicted-vs-live diagnostic (or an accidental restoration of `warnOnPaneKeyDrift`) could resurface the WARN under non-zero base-index configs and go unnoticed until a user's `portal.log` is inspected by hand. We need a test that runs the full bootstrap orchestrator against a real tmux server configured with non-zero `base-index` / `pane-base-index` and asserts no log line matches the offending regex.

**Solution**: Add a regression sub-test (or extend the existing base-index drift sub-test) in `cmd/bootstrap/reboot_roundtrip_test.go` that runs the bootstrap-and-restore round-trip with non-zero `base-index` and `pane-base-index` on the test tmux server, then reads `~/.config/portal/state/portal.log` (resolved via `PORTAL_STATE_DIR`) and asserts zero lines match the regex `predicted=.*__\d+\.\d+ live=.*__\d+\.\d+`. The test must use the isolated `internal/tmuxtest` socket fixture so the developer's primary tmux server is untouched, and must use a session name without a leading dash (Phase 1's task 1-3 owns the leading-dash leg of the integration matrix; this test's distinct contribution is the base-index regression assertion, not the leading-dash one).

**Outcome**: A new (or extended) `_test.go` test under `cmd/bootstrap/` runs under the `integration` build tag, drives the bootstrap orchestrator against an isolated tmuxtest socket with non-zero base indices, completes a full restore + hydrate cycle, and fails if `portal.log` contains any line matching the predicted-vs-live regex. The test passes after Task 2-1 lands and would have failed before it.

**Do**:
- Open `cmd/bootstrap/reboot_roundtrip_test.go`. Decide whether to extend `TestPhase5RebootRoundTripBaseIndexDrift` (which already runs with `restoreBase: 1, restorePaneBase: 1`) with the regex assertion, or to add a sibling test (e.g. `TestPhase5RebootRoundTripNoPredictedVsLiveWarn`) that wraps `runRebootRoundTrip` and then performs the assertion. Prefer extending the existing drift sub-test if the assertion can be threaded in without disturbing its current shape.
- After bootstrap completes inside the test (i.e. after the orchestrator has run and hydrate has been driven), resolve the path to `portal.log` via the `PORTAL_STATE_DIR` env var the test already sets (`stateDir := t.TempDir(); t.Setenv("PORTAL_STATE_DIR", stateDir)`). The log file lives at `<stateDir>/portal.log` — confirm by reading `internal/state/paths.go` if uncertain.
- Read the entire `portal.log` file into memory. Compile the regex `regexp.MustCompile(\`predicted=.*__\d+\.\d+ live=.*__\d+\.\d+\`)`. Iterate lines; fail the test with `t.Fatalf("portal.log contains predicted-vs-live WARN line that should be impossible after Phase 2: %q", line)` if any line matches.
- Use a session name without a leading dash (the existing fixture's `"alpha"` / `"beta"` are fine). Phase 1 task 1-3 already exercises the leading-dash session name; this test's contribution is orthogonal — the base-index regression — so reusing the standard fixture names keeps the two integration legs cleanly separated.
- Confirm the `tmuxtest.New` fixture is the one in use (it spins up an isolated socket via `tmux -L <test-socket>`); do **not** add code that touches the developer's primary tmux server, and do **not** invoke `tmux kill-server` against anything other than the fixture socket.
- Run `go test -tags=integration ./cmd/bootstrap/...` and confirm the new assertion passes.
- Verify the regex is meaningful and false-positive-safe with a unit test (in the same `_test.go` file or a sibling `cmd/bootstrap/predicted_vs_live_regex_test.go`) named `TestPredictedVsLiveRegex_MatchesOffendingShapeAndIgnoresArmPanesWarning`. The test compiles the same `predicted=.*__\d+\.\d+ live=.*__\d+\.\d+` regex used by the integration assertion and asserts:
  - It matches a representative offending line (`WARN | restore | session "alpha": pane 0 predicted=alpha__0.0 live=alpha__1.1`).
  - It does NOT match the preserved `armPanes:202` shape (e.g. `WARN | restore | session "alpha": live pane count 2 != saved count 3`).
  This unit test is plain `go test ./cmd/bootstrap/...` (no `integration` build tag) so it runs on every CI invocation without requiring tmux.

**Acceptance Criteria**:
- [ ] A test in `cmd/bootstrap/reboot_roundtrip_test.go` runs under the `integration` build tag with non-zero `base-index` and `pane-base-index` on the test tmux server.
- [ ] After bootstrap and hydrate complete, the test reads `<PORTAL_STATE_DIR>/portal.log` and asserts zero lines match `predicted=.*__\d+\.\d+ live=.*__\d+\.\d+`.
- [ ] The test uses only the isolated `internal/tmuxtest` socket fixture; the developer's primary tmux server is never touched.
- [ ] The test uses a session name without a leading dash (orthogonal to Phase 1's leading-dash test).
- [ ] The regex is anchored enough to avoid false positives on unrelated log lines (e.g. it must not match `armPanes:202`'s pane-count warning, which uses a different shape).
- [ ] The assertion runs **after** the bootstrap orchestrator has completed and hydrate has been driven — not before — so any drift WARN that would have been emitted has had a chance to be written to disk.
- [ ] `go test -tags=integration ./cmd/bootstrap/...` passes.
- [ ] A unit test (`TestPredictedVsLiveRegex_MatchesOffendingShapeAndIgnoresArmPanesWarning`) compiles the same regex used by the integration assertion and proves it (a) matches a representative `predicted=...__0.0 live=...__1.1` line and (b) does not match the preserved `armPanes:202` "live pane count != saved count" shape.
- [ ] No `t.Parallel()` call is added (per project convention in `CLAUDE.md`).

**Tests**:
- `"portal.log contains zero predicted-vs-live WARN lines under non-zero base-index"` — the headline assertion this task adds.
- `"regex does not false-positive on the preserved armPanes:202 pane-count mismatch warning"` — verify by inspection that the existing warning shape (`live pane count %d != saved count %d`) cannot match `predicted=.*__\d+\.\d+ live=.*__\d+\.\d+`.
- `"existing base-index drift round-trip behaviour is unchanged"` — the rest of `runRebootRoundTrip`'s assertions (structural-key drift, hook firing, scrollback replay) continue to pass.
- `"no developer-primary-server interaction"` — verify by inspection that the test only references `tmuxtest.New(...)` / its returned `Client`; no bare `exec.Command("tmux", ...)` against the default socket.

**Edge Cases**:
- Isolated tmux socket only: the test must use `tmuxtest.New(t, "ptl-rt-")` (or equivalent fixture) and must not invoke `tmux kill-server` against any socket other than the fixture's. The spec's Testing Constraint § "Do Not Restart The Active Tmux Server" is binding.
- Session name without leading dash: keeps this test orthogonal to Phase 1 task 1-3, which owns the leading-dash regression. Both can coexist; neither subsumes the other.
- Regex false-positive avoidance: `predicted=.*__\d+\.\d+ live=.*__\d+\.\d+` requires both `predicted=...__N.N` and `live=...__N.N` segments. The preserved `armPanes:202` warning ("`live pane count %d != saved count %d`") cannot match — confirm by inspection rather than by post-hoc grepping at runtime.
- Assertion timing: must run after bootstrap and hydrate complete, otherwise a WARN emitted late could be missed. Place the assertion at the tail of `runRebootRoundTrip` (or in a wrapping test that calls `runRebootRoundTrip` synchronously and then asserts).

**Context**:
> From specification § "Acceptance Criteria" item 4:
> "No misleading `predicted=...__0.0 live=...__X.Y` WARN appears in `portal.log` under any tmux config. The diagnostic is gone, not silenced. Verified by the reboot round-trip integration test (Testing Requirements item 2) running with non-zero `base-index` / `pane-base-index`: after bootstrap completes, `portal.log` must contain zero lines matching the regex `predicted=.*__\d+\.\d+ live=.*__\d+\.\d+`."
>
> From specification § "Testing Constraint — Do Not Restart The Active Tmux Server":
> "Reboot round-trip tests and any manual reproduction must use a separate, isolated tmux server — typically by pointing tmux at a dedicated socket via `tmux -L <test-socket>` (or equivalent fixture, e.g. `internal/tmuxtest`'s real-tmux socket helper)."
>
> The existing `TestPhase5RebootRoundTripBaseIndexDrift` (lines ~154-165 of `cmd/bootstrap/reboot_roundtrip_test.go`) already runs with `restoreBase: 1, restorePaneBase: 1`, making it the natural host for the regex assertion. The test infrastructure resolves `portal.log` via `PORTAL_STATE_DIR` (set at `runRebootRoundTrip` line ~182).

**Spec Reference**: `.workflows/scrollback-not-restored-with-non-zero-base-index/specification/scrollback-not-restored-with-non-zero-base-index/specification.md` § "Acceptance Criteria" item 4 and § "Testing Requirements" item 2
