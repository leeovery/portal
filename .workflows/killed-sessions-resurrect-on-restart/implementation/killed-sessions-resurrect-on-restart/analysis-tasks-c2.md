# Analysis Tasks: killed-sessions-resurrect-on-restart (Cycle 2)

frontmatter:
- topic: killed-sessions-resurrect-on-restart
- cycle: 2
- total_proposed: 4

---

## Task 1: Promote duplicated state.FIFOSignaler recording fakes into internal/statetest
status: approved
severity: medium
sources: duplication, architecture

**Problem**: Two consumer test packages each declare a private struct that records every `state.FIFOSignaler.SendSignal(path)` call — `recordingSignaler` in `cmd/state_signal_hydrate_test.go:20-52` and `recordingFIFOSignaler` in `cmd/bootstrap/eager_signal_hydrate_test.go:12-43`. Bodies are byte-equivalent: identical `calls []string` + `errOn map[string]error` + `err error` triple, identical `SendSignal(path) error` body with the same priority order (global err -> per-path err -> nil), identical `var _ state.FIFOSignaler = (*T)(nil)` compile-time assertion. Same DRY-drift-after-cross-package-promotion pattern cycle 1 fixed for `fakeSleep` (-> `internal/statetest.RecordingSleep`). Cycle 1 finding 2 explicitly anticipated this consolidation once the typed seam landed in T4-1; the follow-through was skipped.

**Solution**: Promote a single canonical recording fake — `statetest.RecordingFIFOSignaler` — into `internal/statetest/` alongside `RecordingSleep`. Both existing sites import the shared type and delete their private copies.

**Outcome**: One canonical recording `state.FIFOSignaler` fake in `internal/statetest`; both consumer test packages share it; ~22 lines deleted per site; package precedent (`statetest.RecordingSleep`) extended consistently for `state.*` test seams.

**Do**:
1. Create `internal/statetest/fifo_signaler_recorder.go` mirroring `internal/statetest/sleep_recorder.go`. Define `RecordingFIFOSignaler` with exported fields `Calls []string`, `ErrOn map[string]error`, `Err error`, and method `SendSignal(path string) error` whose body matches the existing private fakes' priority order (global `Err` -> per-path `ErrOn[path]` -> append path to `Calls`, return nil). Include `var _ state.FIFOSignaler = (*RecordingFIFOSignaler)(nil)`.
2. Add a package doc comment on the new file explaining the helper exists because two consumer test packages each duplicated this fake before promotion (mirror `sleep_recorder.go`'s package-doc rationale).
3. In `cmd/state_signal_hydrate_test.go`: delete the private `recordingSignaler` struct, methods, compile-time assertion (lines 20-52). Replace usages with `statetest.RecordingFIFOSignaler`. Adjust field-access casing (`calls` -> `Calls`, `errOn` -> `ErrOn`, `err` -> `Err`).
4. In `cmd/bootstrap/eager_signal_hydrate_test.go`: delete the private `recordingFIFOSignaler` struct, methods, compile-time assertion (lines 12-43). Replace usages with `statetest.RecordingFIFOSignaler`. Adjust field-access casing.
5. Add necessary imports of `github.com/leeovery/portal/internal/statetest` in both test files.

**Acceptance Criteria**:
- `internal/statetest/fifo_signaler_recorder.go` exists with exported `RecordingFIFOSignaler` matching `RecordingSleep`'s structural conventions.
- No private `recordingSignaler` or `recordingFIFOSignaler` definition remains in `cmd/` or `cmd/bootstrap/` test files.
- `go build ./...` and `go test ./...` both pass.
- Net deletion (~22 lines x 2 sites minus the new helper file) observed.

**Tests**:
- Existing tests in `cmd/state_signal_hydrate_test.go` and `cmd/bootstrap/eager_signal_hydrate_test.go` continue to pass with the shared fake.
- Add `internal/statetest/fifo_signaler_recorder_test.go` covering the three-branch priority order (global `Err` set, per-path `ErrOn` set, default record-and-return-nil), mirroring whatever coverage exists for `RecordingSleep`.

---

## Task 2: Drop redundant explicit EagerSignaler wiring in three integration tests
status: approved
severity: low
sources: duplication

**Problem**: Three integration tests redundantly hand-wire `EagerSignaler: &bootstrap.EagerSignalCore{Markers: client, StateDir: stateDir, Signaler: state.DefaultFIFOSignaler{}, Logger: logger}` through `buildIntegrationOrchestrator(...)` at `cmd/bootstrap/eager_signal_hydrate_integration_test.go:209-214`, `cmd/bootstrap/eager_signal_hydrate_integration_test.go:344-349`, and `cmd/bootstrap/phase2_hook_fire_integration_test.go:181-186`. Cycle 1's `NewWithDefaults` helper auto-defaults `EagerSignaler` to a real `*EagerSignalCore` with byte-identical field values when `WithRestore` is supplied with a non-nil `Restorer` (`defaults.go:182-195`). All three sites supply `WithRestore` with a real `RestoreAdapter`, so removing the explicit field would produce a byte-identical orchestrator. The auto-default branch is already pinned by dedicated unit tests (`TestBuildIntegrationOrchestrator_EagerSignalerDefaultsToRealWhenRestoreReal`, `TestNewWithDefaults_EagerSignalerDefaultsToRealWhenRestoreReal`). The explicit wiring violates the cycle 1 promotion's docstring intent ("the EagerSignaler conditional default-to-real semantics ... is computed inside the helper").

**Solution**: Delete the explicit `EagerSignaler:` field from all three integration test sites; rely on `NewWithDefaults`' auto-default to produce identical wiring.

**Outcome**: ~18 lines deleted (6 x 3 sites). Test sites pin only what they actually need to control; the auto-default contract is exercised in production-fidelity form rather than re-stated; existing dedicated unit tests continue to guard the default behaviour.

**Do**:
1. In `cmd/bootstrap/eager_signal_hydrate_integration_test.go` lines 209-214: delete the `EagerSignaler: &bootstrap.EagerSignalCore{...}` field from the `buildIntegrationOrchestrator` call.
2. In `cmd/bootstrap/eager_signal_hydrate_integration_test.go` lines 344-349: delete the same `EagerSignaler:` field.
3. In `cmd/bootstrap/phase2_hook_fire_integration_test.go` lines 181-186: delete the same `EagerSignaler:` field.
4. Verify each remaining call still passes `WithRestore` with a non-nil `RestoreAdapter` (precondition for the auto-default branch). Remove any newly unused imports (e.g. `state.DefaultFIFOSignaler`, `bootstrap.EagerSignalCore`).
5. Run `go test -tags integration ./cmd/bootstrap/...` to confirm the orchestrator wiring is byte-identical.

**Acceptance Criteria**:
- No occurrence of `EagerSignaler: &bootstrap.EagerSignalCore{` remains in any integration test file.
- `go test -tags integration ./cmd/bootstrap/...` passes.
- `TestBuildIntegrationOrchestrator_EagerSignalerDefaultsToRealWhenRestoreReal` and `TestNewWithDefaults_EagerSignalerDefaultsToRealWhenRestoreReal` still pass.

**Tests**:
- The two existing dedicated unit tests already pin the auto-default contract — no new tests required.
- Run all three modified integration tests to confirm behaviour unchanged.

---

## Task 3: Update CLAUDE.md step-6 to reference post-Task-1 production primitive
status: approved
severity: low
sources: standards

**Problem**: The "Server bootstrap" section in `CLAUDE.md:78` describes EagerSignalHydrate as writing the signal byte "via `state.WriteFIFOSignal`". After cycle-1 Task 1 landed, production wiring at `cmd/bootstrap_production.go:131-135` injects `state.DefaultFIFOSignaler{}`, whose `SendSignal` delegates to `state.SendHydrateSignal` — the no-seam production entry point that pins `OpenFIFOForSignal` + `time.Sleep`. `state.WriteFIFOSignal` is now the seam-bearing variant retained strictly for retry-ladder unit tests. Spec § Definition of Done item 4 mandates `CLAUDE.md` is updated as part of the same PR; the prior update correctly inserted the new step but the post-Task-1 primitive rename was not propagated into this paragraph. Other surfaces (`cmd/bootstrap/eager_signal_hydrate.go:25-28`, `cmd/state_signal_hydrate.go:13-15`, `cmd/bootstrap_production.go:126-127`) describe the post-Task-1 split correctly — only this single CLAUDE.md line is stale.

**Solution**: Edit `CLAUDE.md:78` to replace the stale `state.WriteFIFOSignal` reference with the post-Task-1 primitive name(s).

**Outcome**: CLAUDE.md's "Server bootstrap" description is in lock-step with the production wiring at `cmd/bootstrap_production.go:134` and the doc-comments in `eager_signal_hydrate.go:24-28`; no remaining surface in the repo describes `state.WriteFIFOSignal` as the production write path.

**Do**:
1. Open `CLAUDE.md` and locate line 78 (the "Server bootstrap" paragraph mentioning `state.WriteFIFOSignal`).
2. Replace `via state.WriteFIFOSignal` with wording naming the post-Task-1 split — e.g. `via state.DefaultFIFOSignaler / state.SendHydrateSignal`.
3. Re-read the surrounding paragraph to confirm wording flows naturally and matches the level of detail used elsewhere.
4. Cross-check with `cmd/bootstrap_production.go:131-135` and `cmd/bootstrap/eager_signal_hydrate.go:24-28` for terminology consistency.

**Acceptance Criteria**:
- `CLAUDE.md` no longer contains `state.WriteFIFOSignal` in any context describing production wiring (any retained reference must explicitly note it is the seam-bearing variant used only for retry-ladder unit tests).
- The replacement wording at `CLAUDE.md:78` names `state.DefaultFIFOSignaler` and/or `state.SendHydrateSignal`.
- `grep -n "state.WriteFIFOSignal" CLAUDE.md` returns no production-context hits.

**Tests**:
- Documentation-only change; no code tests apply. Manual review of the rendered CLAUDE.md paragraph for accuracy and flow is sufficient.

---

## Task 4: Reconcile internal/restoretest package doc with current build-tag reality
status: approved
severity: low
sources: architecture

**Problem**: The package doc-comment in `internal/restoretest/restoretest.go:1-29` opens with `//go:build integration` and states the package "is gated `//go:build integration` because every consumer is also gated — keeping the gate here means the package contributes zero compile cost and zero surface to default `go test ./...` runs". Cycle 2 added two new files (`internal/restoretest/sessions_json.go`, `internal/restoretest/waitfor_file_exists.go` via T4-4 / T4-6) that intentionally omit the `//go:build integration` tag, with corresponding test files (`sessions_json_test.go`, `waitfor_file_exists_test.go`) that run under default `go test ./...`. The package is now a hybrid of integration-gated build helpers (`BuildPortalBinaryDir`, `DriveSignalHydrate`, `WaitForSkeletonMarkersCleared`) and always-built test fixtures (`SeedSessionsJSON`, `WaitForFileExists`). The documented package-level integration gating no longer matches reality; a future maintainer reading the package doc will be misled.

**Solution**: Reconcile doc with reality. Option A (preferred): update the package doc in `restoretest.go` to describe the current mixed-tag reality, naming which helpers are integration-only and which are general-purpose. Option B: add `//go:build integration` to the new files to restore the original "every consumer gated" invariant. Use option A unless future general-purpose consumers are unwanted — option A documents truth without restricting reach.

**Outcome**: The `internal/restoretest` package doc accurately describes the current build-tag layout; readers can determine at a glance which helpers are gated and which run under default `go test ./...`; doc-vs-mechanism drift closed.

**Do**:
1. Open `internal/restoretest/restoretest.go` and read the package doc-comment (lines 1-29).
2. Apply option A: rewrite the package doc-comment so it:
   - Removes the unconditional claim that the package "is gated `//go:build integration`".
   - Lists, by file or helper name, which symbols are integration-only (those that need tmux fixtures or the `go build` subprocess: `BuildPortalBinaryDir`, `DriveSignalHydrate`, `WaitForSkeletonMarkersCleared`).
   - Lists which symbols are always-built general-purpose seed primitives (`SeedSessionsJSON`, `WaitForFileExists`).
   - Notes the convention: integration-only helpers live in `//go:build integration`-tagged files; general-purpose helpers omit the tag.
3. Confirm the package doc-comment is on a file whose own build tag does not contradict the new wording (if `restoretest.go` itself carries `//go:build integration`, either move the package doc onto an untagged file or note that the file-level tag is independent of the package-level claim).
4. Run `go build ./...`, `go test ./...` (default), and `go test -tags integration ./...` to confirm nothing regressed.

**Acceptance Criteria**:
- The package doc-comment in `internal/restoretest` no longer claims package-level `//go:build integration` gating.
- The doc explicitly enumerates which helpers are integration-only and which are always-built.
- `go test ./internal/restoretest/...` (default) and `go test -tags integration ./internal/restoretest/...` both pass.
- A reader of the package doc can correctly predict the build tag of each helper without inspecting individual source files.

**Tests**:
- Documentation change with no behavioural impact. Run `go vet ./internal/restoretest/...` to ensure the doc-comment remains well-formed; run both default and integration test invocations to confirm no compile regression.
