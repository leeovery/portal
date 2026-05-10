# Analysis Tasks: killed-sessions-resurrect-on-restart (Cycle 1)

Frontmatter:
- topic: killed-sessions-resurrect-on-restart
- cycle: 1
- total_proposed: 7

---

## Task 1: Collapse EagerHydrateSignaler adapter via typed FIFO-signal seam and no-seam production helper
status: pending
severity: medium
sources: architecture, duplication

**Problem**: The eager-signal step carries a redundant production wiring layer. `bootstrap.EagerSignalCore` (`cmd/bootstrap/eager_signal_hydrate.go:30-91`) accepts a `WriteFIFOSignal func(path) error` function-valued field, which forces `bootstrapadapter.EagerHydrateSignaler` (`internal/bootstrapadapter/adapters.go:164-186`) to exist solely to bind one closure: `func(path string) error { return state.WriteFIFOSignal(path, state.OpenFIFOForSignal, time.Sleep) }`. The wrapper allocates a fresh `EagerSignalCore` on every Run (lines 177-185) instead of constructing once at wiring time, requires two parallel `var _ EagerHydrateSignaler = ...` assertions, and forces two doc-comment surfaces to stay in sync. The asymmetry with `MarkerCleanupCore` (constructed inline at `cmd/bootstrap_production.go` because every seam is a typed interface satisfied directly by `*tmux.Client`) is the symptom. Compounding this, `state.WriteFIFOSignal(path, openFIFO, sleep)` (`internal/state/signal_hydrate.go:45-68`) accepts two function-value seams that production callers never vary — both `cmd/state_signal_hydrate.go:97-99` and the bootstrapadapter wrapper always pass `state.OpenFIFOForSignal` + `time.Sleep`. The test seam shape also diverges between callers: `runSignalHydrate` injects `OpenFIFO`+`Sleep` (retry-ladder primitives, real ladder runs in tests) while `EagerSignalCore` injects the entire `WriteFIFOSignal` function (ladder body replaced in tests), so `cmd/bootstrap/eager_signal_hydrate_test.go` cannot exercise ENXIO/EAGAIN retry. As a downstream effect, the test-only `fakeSleep` recorder is byte-identically duplicated in `internal/state/signal_hydrate_test.go:19-26` and `cmd/state_signal_hydrate_test.go:21-28` because the primitive moved cross-package without consolidating its test helper.

**Solution**: Introduce a no-seam production entry point `state.SendHydrateSignal(path string) error` that internally calls `WriteFIFOSignal(path, OpenFIFOForSignal, time.Sleep)`. Replace the function-valued `WriteFIFOSignal` field on `EagerSignalCore` with a typed seam — e.g. `FIFOSignaler interface { SendSignal(path string) error }` — with a default production implementation (e.g. `state.DefaultFIFOSignaler{}` whose `SendSignal` delegates to `state.SendHydrateSignal`). With production now satisfying every seam via typed interfaces, construct `EagerSignalCore` inline at `cmd/bootstrap_production.go` (mirroring `MarkerCleanupCore`) and delete `bootstrapadapter.EagerHydrateSignaler`. Update `runSignalHydrate` to call `state.SendHydrateSignal(fifoPath)` directly. Retain seam-bearing `WriteFIFOSignal(path, openFIFO, sleep)` strictly for retry-ladder unit tests. Consolidate duplicated `fakeSleep` into a shared location — either exported test-only `state.RecordingSleep` or a new `internal/statetest` package alongside `internal/restoretest`/`internal/tmuxtest`.

**Outcome**: One production type for the eager-signal step (no wrapper), one inline construction site, one canonical no-seam production call shape (`state.SendHydrateSignal`), one typed `FIFOSignaler` test seam shared between cmd and cmd/bootstrap, one shared `fakeSleep` helper, retry-ladder coverage stays anchored where the primitive lives.

**Do**:
1. Add `func SendHydrateSignal(path string) error { return WriteFIFOSignal(path, OpenFIFOForSignal, time.Sleep) }` to `internal/state/signal_hydrate.go`.
2. Define `type FIFOSignaler interface { SendSignal(path string) error }` in `cmd/bootstrap/eager_signal_hydrate.go`; replace the `WriteFIFOSignal func(path) error` field on `EagerSignalCore` with `Signaler FIFOSignaler`.
3. Add a default production implementation (e.g. `state.DefaultFIFOSignaler{}` whose `SendSignal(path) error` returns `state.SendHydrateSignal(path)`).
4. In `cmd/bootstrap_production.go:123-133`, construct `&bootstrap.EagerSignalCore{...}` inline with `Signaler: state.DefaultFIFOSignaler{}`.
5. Delete `bootstrapadapter.EagerHydrateSignaler` and its `var _ EagerHydrateSignaler = ...` assertion.
6. Update `runSignalHydrate` in `cmd/state_signal_hydrate.go` to call `state.SendHydrateSignal(fifoPath)` directly (preserve `signalHydrateConfig`'s `OpenFIFO`+`Sleep` injection only if cmd-side retry orchestration tests still need them).
7. Promote `fakeSleep` into a shared test helper.
8. Replace `recordingFIFOWriter` in `cmd/bootstrap/eager_signal_hydrate_test.go` with a `recordingFIFOSignaler` satisfying the new interface.
9. Verify all related tests still pass.

**Acceptance Criteria**:
- `bootstrapadapter.EagerHydrateSignaler` no longer exists.
- `EagerSignalCore` constructed inline at `cmd/bootstrap_production.go` with no glue type.
- `state.SendHydrateSignal(path string) error` exists and is called directly by both production call sites (no closures).
- Only one declaration of `fakeSleep`/`RecordingSleep` survives.
- `state.WriteFIFOSignal` retains seam-bearing form, exercised by `internal/state/signal_hydrate_test.go`.
- `go build ./...` and `go test ./...` succeed.

**Tests**:
- Retry-ladder coverage in `internal/state/signal_hydrate_test.go` continues to pass against the seam-bearing `WriteFIFOSignal`.
- Updated `cmd/bootstrap/eager_signal_hydrate_test.go` uses the typed `FIFOSignaler` fake and asserts `SendSignal` is invoked once per pane with the correct path.
- Existing eager-signal integration tests (`cmd/bootstrap/eager_signal_hydrate_integration_test.go`) pass against inline-constructed production wiring.
- Shared `fakeSleep`/`RecordingSleep` exercised in each consumer package.

**Edge Cases**:
- Avoid nil-receiver panic for zero-value `EagerSignalCore` in tests — provide a concrete default.
- Confirm `cmd/state_signal_hydrate.go` retry orchestration retains whatever cmd-local seam its own tests need.

---

## Task 2: Flip integration-orchestrator builder default for EagerSignaler from NoOp to a real adapter
status: pending
severity: low
sources: architecture

**Problem**: `buildIntegrationOrchestrator` (`cmd/bootstrap/orchestrator_builder_test.go:33-87`, defaulting at lines 63-65) defaults `EagerSignaler` to `bootstrap.NoOpEagerHydrateSignaler{}` unless explicitly overridden. Existing reboot/reattach integration tests — `TestPhase5RebootRoundTripEndToEnd`, `TestPhase5RebootRoundTripBaseIndexDrift`, `TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary` (`cmd/bootstrap/reboot_roundtrip_test.go:332-339`), and every test in `cmd/reattach_integration_test.go:182` — exercise a non-production orchestrator: in production EagerSignalHydrate runs every bootstrap and drives marker-clear before any client attaches; in these tests it is no-op'd and signal-hydrate is driven manually post-Run. The eager-signal step is consequently exercised only in dedicated `eager_signal_hydrate_integration_test.go` / `phase2_hook_fire_integration_test.go`. A future regression that makes the eager step interfere with the manual harness would not surface in the broad reboot suite.

**Solution**: When the caller has provided a real `RestoreAdapter`, default `EagerSignaler` to a real adapter wired against the same `client`/`stateDir`/`logger` triple. Tests that explicitly need the manual-drive harness opt out via `EagerSignaler: bootstrap.NoOpEagerHydrateSignaler{}`.

**Outcome**: Reboot/reattach integration tests exercise the production-shape pipeline including EagerSignalHydrate; manual-harness opt-outs are explicit and visible.

**Do**:
1. Change the EagerSignaler default branch in `buildIntegrationOrchestrator`: if nil and a real `RestoreAdapter` was provided, construct a real adapter (post-Task-1 this is `&bootstrap.EagerSignalCore{...}` with the typed default; pre-Task-1 use the existing wrapper shape).
2. Audit reboot/reattach tests for compatibility with eager-fire-then-manual-fire pipelines. Either explicitly opt out with a comment or update the test to accommodate.
3. Run the full reboot/reattach suite and confirm pass.

**Acceptance Criteria**:
- `buildIntegrationOrchestrator` defaults `EagerSignaler` to a real adapter when `RestoreAdapter` is real.
- Tests needing the manual harness explicitly opt out with comment.
- All existing reboot/reattach tests pass.

**Tests**:
- Reboot roundtrip tests pass under new default.
- All `cmd/reattach_integration_test.go` tests pass.
- Negative-coverage check: stubbing EagerSignalHydrate locally during review should now break a reboot test.

**Edge Cases**:
- Manual signal-hydrate harness in some reboot tests writes to FIFOs from goroutines; ensure eager step does not race destructively (ENXIO/EAGAIN tolerant retry helps but ordering may matter).
- Tests passing only because EagerSignaler was NoOp must surface as explicit opt-outs, not silent absorption.

---

## Task 3: Promote NoOp-defaulted orchestrator builder helper to non-test location to eliminate dual builders
status: pending
severity: low
sources: architecture

**Problem**: Two sibling integration builders construct the same `*bootstrap.Orchestrator` with the same NoOp defaults: `buildIntegrationOrchestrator` in `bootstrap_test` (`cmd/bootstrap/orchestrator_builder_test.go:33-87`) and `buildReattachOrchestrator` in `cmd` (`cmd/reattach_integration_test.go:163-187`). They are split solely because Go test-file symbols are package-private. Both enumerate every step seam (Server, Hooks, Restoring, Saver, Restore, EagerSignaler, StaleMarkers, Sweeper, Clean). The orchestrator-builder file's own comment acknowledges the tax: "Adding a new step interface therefore requires editing two files." This work added the EagerSignaler step and had to edit both.

**Solution**: Promote a NoOp-defaulted builder to a non-test helper in `cmd/bootstrap` — e.g. `bootstrap.NewWithDefaults(server ServerBootstrapper, restoring RestoringMarker, opts ...Option) *Orchestrator`. Production does not use NoOps; the helper's defaulting is reused by both test packages via standard import. New steps require only an `Option` constructor and one default-NoOp wire-up.

**Outcome**: One source of truth for orchestrator default-NoOp wiring. Adding a new step requires editing one file.

**Do**:
1. Decide option-pattern shape (functional options vs `Defaults` struct). Functional options preferred.
2. Add `bootstrap.NewWithDefaults(...)` in a new non-test file (e.g. `cmd/bootstrap/defaults.go`).
3. Add `WithRestore`, `WithEagerSignaler`, `WithSaver`, `WithStaleMarkers`, `WithSweeper`, `WithClean`, `WithHooks`, `WithServer` options.
4. Migrate `buildIntegrationOrchestrator` to delegate.
5. Migrate `buildReattachOrchestrator` similarly.
6. Confirm step ordering in `Run` is unchanged.
7. Update or delete the "two files" comment.

**Acceptance Criteria**:
- Non-test helper exists in `cmd/bootstrap` and is importable from both test packages.
- Both builders delegate to the helper.
- A hypothetical tenth step requires editing only the helper plus one option constructor.
- All tests in `cmd/bootstrap` and `cmd` pass.

**Tests**:
- Existing tests using both builders pass unchanged.
- Smoke test: `bootstrap.NewWithDefaults(server, restoring)` returns an Orchestrator with non-nil seams whose Run() is callable.

**Edge Cases**:
- Helper must not leak NoOp types into production callers — production keeps direct construction at `cmd/bootstrap_production.go`.
- Mandatory seams (no sensible NoOp) should be positional arguments, not options.

---

## Task 4: Promote sessions.json seeding helpers into shared package consumed by both cmd and cmd/bootstrap tests
status: pending
severity: medium
sources: duplication

**Problem**: A ~20-line block that builds a `state.Index`, appends single-window/single-pane `state.Session` entries (`Index: 0, Layout: "tiled", Active: true, ScrollbackFile: "scrollback/<name>-w0-p0.bin"`), encodes via `state.EncodeIndex`, and writes to `state.SessionsJSON(stateDir)` is repeated at six bootstrap-integration test sites: `cmd/bootstrap/eager_signal_hydrate_integration_test.go:172-197`, `:339-364`, `cmd/bootstrap/phase2_hook_fire_integration_test.go:131-156`, `cmd/bootstrap/phase5_integration_test.go:152-174`, `:246-267`, `cmd/bootstrap/phase5_marker_suppression_integration_test.go:89-112`. Identical shape; only session-name list and (in one case) `SavedAt` differ. A near-identical `seedSessionsJSON` / `seedSessionsJSONWithSavedAt` already exists at `cmd/reattach_integration_test.go:199-236` but is in package `cmd`, invisible to `bootstrap_test`.

**Solution**: Promote `seedSessionsJSON` / `seedSessionsJSONWithSavedAt` into a non-test-file location reachable by both packages — preferred home `internal/restoretest` (already imported by every site touched here) or a new `internal/statetest` leaf. Six bootstrap-integration sites collapse to a one-liner; the local `cmd/reattach_integration_test.go` copy deletes.

**Outcome**: One canonical sessions.json seed primitive consumed by all bootstrap- and reattach-integration tests. ~120 lines deleted across six files.

**Do**:
1. Choose a home — `internal/restoretest` recommended (or `internal/statetest` if Task 1 introduces it).
2. Move helpers from `cmd/reattach_integration_test.go:199-236` to the shared package, exporting as `restoretest.SeedSessionsJSON` and `restoretest.SeedSessionsJSONWithSavedAt`.
3. Replace each of the six bootstrap-integration sites and the original `cmd/reattach_integration_test.go` call.
4. Confirm `state.EncodeIndex`, `state.SessionsJSON`, `state.Session`/`state.Index` are visible.
5. Run `go test ./cmd/...` and confirm pass.

**Acceptance Criteria**:
- One declaration of `SeedSessionsJSON` (and `SeedSessionsJSONWithSavedAt`) survives, in a shared package.
- All six bootstrap-integration sites call the shared helper.
- `cmd/reattach_integration_test.go` no longer carries its private copy.
- All affected tests pass.

**Tests**:
- Existing bootstrap-integration tests pass.
- `cmd/reattach_integration_test.go` tests pass.

**Edge Cases**:
- Two seed sites use a distinct `SavedAt` — preserve `SeedSessionsJSONWithSavedAt`.
- Helper uses `*testing.T` — place in a file under the shared package that imports `testing` deliberately (existing `internal/restoretest` already does).

---

## Task 5: Extract signalFIFOAsync goroutine helper in cmd/state_hydrate_test.go
status: pending
severity: low
sources: duplication

**Problem**: A 5-line goroutine pattern — `go func() { f, _ := os.OpenFile(fifo, os.O_WRONLY, 0); _, _ = f.Write([]byte("X")); _ = f.Close() }()` — appears 32 times in `cmd/state_hydrate_test.go` (lines 71, 119, 153, 186, 222, 251, 289, 328, 375, 412, and ~22 more). Each occurrence is structurally identical; only surrounding `cfg` and assertions vary. The work unit added new tests of this shape (timeout-path, hook-firing-on-timeout) extending an already load-bearing duplication. Precedent for extraction in the same file exists: `timeoutCfg`, `instantTimeoutOpenFIFO`, `seedHookStore`, `makeFIFO`.

**Solution**: Extract `signalFIFOAsync(t *testing.T, fifo string)` alongside `makeFIFO` at the top of `cmd/state_hydrate_test.go`. Optionally pair with `makeAndSignalFIFO` for sites that always do both. Out-of-package promotion unnecessary.

**Outcome**: Each of 32 sites collapses 5 lines → 1 line. Future tests compose via the helper.

**Do**:
1. Add `func signalFIFOAsync(t *testing.T, fifo string)` near `makeFIFO`.
2. Optionally add `func makeAndSignalFIFO(t *testing.T, dir string) string`.
3. Replace each of 32 occurrences.
4. Confirm `go test ./cmd -run TestHydrate` passes.

**Acceptance Criteria**:
- `signalFIFOAsync` declared once at the top of the file.
- Zero remaining inline `go func() { ... os.OpenFile(... O_WRONLY ...) ... }()` blocks.
- All hydrate tests pass.

**Tests**:
- Existing tests in `cmd/state_hydrate_test.go` pass.
- No new tests required.

**Edge Cases**:
- Audit each site; sites that diverge (multi-byte writes, delays) keep inline goroutines.
- Preserve any existing `t.Helper()`/`t.Cleanup()` semantics.

---

## Task 6: Promote shared WaitForFileExists sentinel-poll helper into internal/restoretest
status: pending
severity: low
sources: duplication

**Problem**: Two integration tests independently implemented a poll-for-sentinel-file helper: `pollUntilSentinelPresent` (`cmd/bootstrap/phase2_hook_fire_integration_test.go:250-262`) and `awaitSentinelExists` (`internal/restore/exit_closes_pane_integration_test.go:461-476`). Names differ; one parameterises tick, the other hardcodes 50ms; failure messages diverge. Essence identical: poll `os.Stat` for a sentinel file and fail-with-diagnostic if absent within budget. Parallel-implementations-of-the-same-primitive must be kept in sync.

**Solution**: Promote `WaitForFileExists(t *testing.T, path string, budget, tick time.Duration)` into `internal/restoretest` (already imported by both call sites). Both sites dispatch through the shared helper with AC-specific failure-message wrappers (or pass a custom message). ~30 lines deleted across two files.

**Outcome**: One canonical "is the sentinel observable yet" primitive consumed by all hook-firing integration tests.

**Do**:
1. Add `func WaitForFileExists(t *testing.T, path string, budget, tick time.Duration)` to `internal/restoretest`. Body polls `os.Stat(path)` until success or budget elapsed; on timeout `t.Fatalf` with diagnostic including path and elapsed budget.
2. Replace `pollUntilSentinelPresent` at `cmd/bootstrap/phase2_hook_fire_integration_test.go:250-262`.
3. Replace `awaitSentinelExists` at `internal/restore/exit_closes_pane_integration_test.go:461-476`.
4. If AC-specific message phrasings are load-bearing, pass an optional message parameter or wrap at each call site.
5. Run `go test ./cmd/bootstrap/... ./internal/restore/...` and confirm pass.

**Acceptance Criteria**:
- `restoretest.WaitForFileExists` exists.
- `pollUntilSentinelPresent` and `awaitSentinelExists` deleted.
- Both call sites pass through the shared helper.
- Affected integration tests pass.

**Tests**:
- `cmd/bootstrap/phase2_hook_fire_integration_test.go` tests pass.
- `internal/restore/exit_closes_pane_integration_test.go` tests pass.

**Edge Cases**:
- Two original helpers carry different default ticks (parameterised vs hardcoded 50ms); choose 50ms canonical or make tick mandatory.
- Diagnostic must include absolute path + elapsed time for triage.

---

## Task 7: Replace stale `sh -c` wrapper documentation in three integration-test comments
status: pending
severity: low
sources: standards

**Problem**: Three doc comments still describe the helper invocation as `respawn-pane -k 'sh -c portal state hydrate ...; exec $SHELL'` — the shape Fix 3 (Wrapper Drop in `buildHydrateCommand`) deliberately removed. Locations: `cmd/bootstrap/eager_signal_hydrate_integration_test.go:117-118`, `:158-159`, `cmd/reattach_integration_test.go:73-74`. Implementation in `internal/restore/session.go:426-433` correctly emits the bare form, regression guards in `internal/restore/session_build_hydrate_test.go` and `internal/restore/exit_closes_pane_integration_test.go` forbid `sh -c` re-introduction. Test assertions are correct — only comments are stale. Spec calls wrapper-drop load-bearing for AC5, so the divergence creates a misleading paper trail.

**Solution**: Replace the three quoted strings with `respawn-pane -k 'portal state hydrate --fifo X --file Y --hook-key Z'`. Drop the `; exec $SHELL` trailer reference — post-Fix-3 the helper owns the exec via `syscall.Exec`.

**Outcome**: Test-file documentation matches live implementation.

**Do**:
1. Update comment at `cmd/bootstrap/eager_signal_hydrate_integration_test.go:117-118` to bare form.
2. Update comment at `cmd/bootstrap/eager_signal_hydrate_integration_test.go:158-159` to bare form.
3. Update comment at `cmd/reattach_integration_test.go:73-74` to bare form.
4. `Grep` for `sh -c` near `portal state hydrate` to confirm no other stale comments remain.

**Acceptance Criteria**:
- All three comments show the bare form.
- Zero remaining test-file comments referencing `'sh -c portal state hydrate ...; exec $SHELL'` as the live invocation shape.
- No code (assertion strings, regression guards) touched.

**Tests**:
- No test changes — documentation-only.
- `go test ./...` passes (sanity).

**Edge Cases**:
- Some comments may reference `sh -c` historically as "previously this was wrapped..." — only update those describing live behaviour, preserve historical-context notes.
