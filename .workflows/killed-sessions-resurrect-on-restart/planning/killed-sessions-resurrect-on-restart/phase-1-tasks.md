---
phase: 1
phase_name: Eager-Signal Hydrate Step (Root-Cause Fix)
total: 8
---

## killed-sessions-resurrect-on-restart-1-1 | approved

### Task killed-sessions-resurrect-on-restart-1-1: Relocate writeFIFOSignal and signalHydrateRetryDelays into internal/state

**Problem**: `writeFIFOSignal` and `signalHydrateRetryDelays` are package-private helpers in `cmd/state_signal_hydrate.go`. The new bootstrap eager-signaling step (task 1-3) needs the same primitives, but `cmd/bootstrap` cannot import `cmd`. The spec mandates a single source of truth so failure semantics stay identical between the new step and the existing `client-attached` / `client-session-changed` paths.

**Solution**: Move both symbols verbatim into `internal/state` (alongside the existing FIFO/marker helpers), exported as `state.WriteFIFOSignal` and `state.SignalHydrateRetryDelays`. The retry ladder, the `O_WRONLY|O_NONBLOCK` open posture, the ENXIO/EAGAIN classification, the immediate ENOENT surface, and the wrapped retries-exhausted error must be preserved byte-for-byte.

**Outcome**: `internal/state` exposes a single shared writer and retry schedule. `cmd/state_signal_hydrate.go` (still in cmd) and `cmd/bootstrap` can both call into the shared package. No public API beyond the two new exports is added.

**Do**:
- Add a new file `internal/state/signal_hydrate.go` (or extend `internal/state/fifo.go`) containing:
  - Exported `var SignalHydrateRetryDelays = []time.Duration{10ms, 20ms, 40ms, 80ms, 160ms, 190ms}` with the same documented 500ms cumulative-budget commentary.
  - Exported `func WriteFIFOSignal(path string, openFIFO func(string) (*os.File, error), sleep func(time.Duration)) error` — the same body as the cmd-level `writeFIFOSignal` but parameterised by the test seams (`OpenFIFO`, `Sleep`) directly rather than via a `signalHydrateConfig` struct, so callers in two packages can pass their own seams without a circular type dependency.
  - A package-private `isRetryableFIFOError(err error) bool` (move the existing helper verbatim).
  - A package-private `OpenFIFOForSignal(path string) (*os.File, error)` exported helper that wraps `os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)` so cmd-side and bootstrap-side callers can share the production FIFO opener.
- Move the `// signalHydrateRetryDelays …` and `// writeFIFOSignal …` doc comments verbatim, updating only the symbol names to their exported forms.
- Add a `internal/state/signal_hydrate_test.go` (or fold tests into `fifo_test.go`) that asserts the retry ladder element-equality, the ENXIO/EAGAIN retry behaviour, the immediate ENOENT surface, and the `retries exhausted opening fifo %s: %w` error wrap on exhaustion. Mirror the existing `cmd/state_signal_hydrate_test.go` table where helpful.
- Do NOT delete `cmd.writeFIFOSignal` / `cmd.signalHydrateRetryDelays` in this task — task 1-2 repoints `cmd/state_signal_hydrate.go` to call the shared package and removes the cmd-side copies.

**Acceptance Criteria**:
- [ ] `state.SignalHydrateRetryDelays` is `{10ms, 20ms, 40ms, 80ms, 160ms, 190ms}` with cumulative sum 500ms.
- [ ] `state.WriteFIFOSignal` returns `nil` on a successful single-byte write to the FIFO.
- [ ] `state.WriteFIFOSignal` retries on `syscall.ENXIO` and `syscall.EAGAIN` per the retry ladder; total retry count equals `len(SignalHydrateRetryDelays)` retries (i.e. `i <= len(delays)` outer loop).
- [ ] `state.WriteFIFOSignal` returns immediately on `syscall.ENOENT` with an `open fifo %s: %w` wrap.
- [ ] On retries-exhaustion, `state.WriteFIFOSignal` returns an error wrapping the last-seen retryable error with the prefix `retries exhausted opening fifo %s:`.
- [ ] `cmd/state_signal_hydrate.go` still compiles unchanged (cmd-side copies remain in place; task 1-2 does the repoint).
- [ ] No `internal/tmux` import is added to `internal/state` (preserving the existing import boundary).

**Tests**:
- `"WriteFIFOSignal writes a single byte and returns nil on first-try success"`
- `"WriteFIFOSignal retries on ENXIO per the retry ladder and eventually succeeds"`
- `"WriteFIFOSignal retries on EAGAIN per the retry ladder and eventually succeeds"`
- `"WriteFIFOSignal returns immediately on ENOENT (no retry) wrapped with open fifo prefix"`
- `"WriteFIFOSignal returns retries-exhausted wrapped error when all delays elapse with retryable errors"`
- `"SignalHydrateRetryDelays table equals the spec-pinned 6-entry ladder"`

**Edge Cases**:
- Retry ladder ENXIO/EAGAIN preserved verbatim (same elements, same order, same cumulative 500ms).
- ENOENT and any non-retryable error surface immediately on first iteration with no `Sleep` call.
- Retries-exhausted wrapping unchanged: `fmt.Errorf("retries exhausted opening fifo %s: %w", path, lastErr)`.
- A `nil` `sleep` seam should not be tolerated (production callers always pass `time.Sleep` or a fake) — keep the existing contract.

**Context**:
> Spec § "Write Primitive" → "Both `writeFIFOSignal` and `signalHydrateRetryDelays` are currently package-private inside `cmd`. The fix moves them into a shared internal package (`internal/state`, alongside the existing FIFO/marker helpers). `cmd/state_signal_hydrate.go` and the new `cmd/bootstrap` step both call into the shared package. No public API is exposed."
>
> Existing helpers to mirror: `internal/state/fifo.go` (CreateFIFO), `internal/state/markers.go` (ServerOptionLister/Writer interfaces). The same nil-safe / explicit-seam style applies.

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Fix 1: Bootstrap Eager-Signaling Step" → "Write Primitive".

## killed-sessions-resurrect-on-restart-1-2 | approved

### Task killed-sessions-resurrect-on-restart-1-2: Repoint cmd/state_signal_hydrate.go at the shared internal/state writer

**Problem**: After task 1-1 lands, `cmd/state_signal_hydrate.go` still owns its private `writeFIFOSignal`, `signalHydrateRetryDelays`, `isRetryableFIFOError`, and `openFIFOForSignal` copies. Two implementations of the same primitive will drift — the spec's "single source of truth" requirement is violated until cmd is repointed at the shared package.

**Solution**: Delete the cmd-side `writeFIFOSignal`, `signalHydrateRetryDelays`, `isRetryableFIFOError`, and `openFIFOForSignal` symbols. Update `runSignalHydrate` to call `state.WriteFIFOSignal(fifoPath, cfg.OpenFIFO, cfg.Sleep)` and the cobra command init to wire `cfg.OpenFIFO = state.OpenFIFOForSignal`. The `signalHydrateConfig` struct's `OpenFIFO`/`Sleep` seams remain — only the body of `writeFIFOSignal` and the production opener move.

**Outcome**: `cmd/state_signal_hydrate.go` is the cobra-command and orchestration layer only; the FIFO write primitive lives once in `internal/state`. All existing `cmd/state_signal_hydrate_test.go` tests still pass without modification (they test through `runSignalHydrate`'s public signature, which is unchanged).

**Do**:
- In `cmd/state_signal_hydrate.go`:
  - Remove the `signalHydrateRetryDelays` `var`.
  - Remove the `writeFIFOSignal` and `isRetryableFIFOError` functions.
  - Remove the `openFIFOForSignal` function.
  - In `runSignalHydrate`, replace the inner `writeFIFOSignal(fifoPath, cfg)` call with `state.WriteFIFOSignal(fifoPath, cfg.OpenFIFO, cfg.Sleep)`.
  - In the `stateSignalHydrateCmd.RunE` body, set `cfg.OpenFIFO = state.OpenFIFOForSignal` (the new exported production opener from task 1-1).
- Verify the existing `cmd/state_signal_hydrate_test.go` test file still compiles and passes — its `OpenFIFO` test seam is provided by callers, so the rename does not affect test surface.
- Verify `runSignalHydrate`'s behaviour preserved end-to-end:
  - Nil `cfg.Logger` (a nil `*state.Logger`) remains a no-op (existing nil-receiver semantics on `*state.Logger.Warn`).
  - `state.ListSkeletonMarkers` failure still soft-warns and returns `nil` from `runSignalHydrate`.
  - A per-pane `state.WriteFIFOSignal` failure still soft-warns and continues to the next pane (the loop's `continue` on error path is unchanged).

**Acceptance Criteria**:
- [ ] `cmd/state_signal_hydrate.go` no longer declares `writeFIFOSignal`, `signalHydrateRetryDelays`, `isRetryableFIFOError`, or `openFIFOForSignal`.
- [ ] `runSignalHydrate` calls `state.WriteFIFOSignal` with `cfg.OpenFIFO` and `cfg.Sleep` as the seams.
- [ ] The cobra command's production `cfg.OpenFIFO` is `state.OpenFIFOForSignal`.
- [ ] Existing `cmd/state_signal_hydrate_test.go` tests pass without modification.
- [ ] Nil-`Logger` no-op behaviour preserved (relies on `*state.Logger`'s nil-receiver semantics).
- [ ] `ListSkeletonMarkers` error path still returns `nil` after a soft warning.
- [ ] Per-pane write failure does not abort sibling panes (loop-continue path preserved).

**Tests**:
- All existing tests in `cmd/state_signal_hydrate_test.go` continue to pass unchanged — this is the regression suite for the repoint.
- `"runSignalHydrate calls state.WriteFIFOSignal for each marker-matched pane"` (verify via the recordingCommander tmux-call assertion + a custom `cfg.OpenFIFO` seam that records paths).
- `"runSignalHydrate soft-warns and returns nil when ListSkeletonMarkers errors"` (already covered; ensure preserved).
- `"runSignalHydrate continues to next pane after WriteFIFOSignal returns error"` (already covered; ensure preserved).

**Edge Cases**:
- nil `cfg.Logger` no-op: ensure no panic, no log.
- `state.ListSkeletonMarkers` failure: soft-warn + return nil (do NOT escalate to error).
- Per-pane write failure: loop-continue; sibling panes still receive their byte.

**Context**:
> The `signalHydrateRunFunc` package-level seam (used by `cmd/state_test.go`'s `TestStateInternalSubcommandsAcceptValidArgv`) must stay in place — production points at `runSignalHydrate`, tests can still override.

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Fix 1: Bootstrap Eager-Signaling Step" → "Write Primitive" → "Sharing mechanism".

## killed-sessions-resurrect-on-restart-1-3 | approved

### Task killed-sessions-resurrect-on-restart-1-3: Define EagerHydrateSignaler seam and EagerSignalHydrate step in cmd/bootstrap

**Problem**: The bootstrap orchestrator has no step that writes the hydrate signal byte to every freshly-armed skeleton pane's FIFO. Without this step, only the user's attached session's helpers ever receive their signal — the N−1 non-attached sessions' helpers wait 3s and time out, leaking `@portal-skeleton-*` markers and silently degrading scrollback save and on-resume hooks.

**Solution**: Add a new seam interface `EagerHydrateSignaler` in `cmd/bootstrap` (alongside the existing `MarkerCleaner`, `FIFOSweeper`, etc.) and a corresponding `EagerSignaler` field on `*Orchestrator`. The step body iterates the freshly-set marker map and writes the signal byte to each pane's FIFO via the seam. Per-FIFO write failures log a soft warning and continue. Zero-marker is a no-op. The step never escalates to a fatal bootstrap error.

**Outcome**: `cmd/bootstrap` exposes a typed seam and a no-op fallback (`NoOpEagerHydrateSignaler`) so the orchestrator's step list is wired-and-tested without requiring task 1-4 (orchestrator integration) or task 1-5 (production adapter).

**Do**:
- In `cmd/bootstrap/bootstrap.go`:
  - Define the seam interface adjacent to the other step interfaces:
    ```go
    // EagerHydrateSignaler signals every freshly-armed skeleton pane via
    // its per-pane FIFO so hydrate helpers proceed without waiting on a
    // client-attached event. Step 6 of the bootstrap sequence (after
    // step 5 Restore, before step 7 Clear @portal-restoring). Best-effort:
    // a non-nil return is logged via Logger.Warn and swallowed.
    type EagerHydrateSignaler interface {
        EagerSignalHydrate() error
    }
    ```
  - Add `EagerSignaler EagerHydrateSignaler` field on `*Orchestrator`.
  - Update the package-level doc comment to reflect the new ten-step list (numbered 1..10), with `EagerSignalHydrate` at position 6 between Restore and Clear.
- In `cmd/bootstrap/noop.go`:
  - Add `NoOpEagerHydrateSignaler` matching the existing NoOp* style — zero-value struct, `EagerSignalHydrate() error { return nil }`. Update the file's policy comment to include the new seam.
- Add a new file `cmd/bootstrap/eager_signal_hydrate.go` containing the production step body and its dependencies:
  - A struct `EagerSignalCore` exposing fields `Markers state.ServerOptionLister`, `StateDir string`, `WriteFIFOSignal func(path string) error`, `Logger Logger`.
  - The struct's method `EagerSignalHydrate() error`:
    1. Substitute `noopLogger{}` for a nil Logger (mirror `MarkerCleanupCore`'s pattern).
    2. Call `state.ListSkeletonMarkers(c.Markers)`. On error return the wrapped error so the orchestrator's Warn-and-swallow path logs it uniformly with siblings (mirror `FIFOSweeper.Sweep`).
    3. If `len(markers) == 0`, return nil immediately — zero-marker no-op, no FIFO writes attempted.
    4. For each `paneKey` in markers: derive `fifoPath := state.FIFOPath(c.StateDir, paneKey)`; call `c.WriteFIFOSignal(fifoPath)`. On error, log via `logger.Warn(state.ComponentHydrate, "eager-signal: write fifo %s: %v", fifoPath, err)` and continue to the next pane. The step never returns an aggregated error from per-FIFO failures — it always returns nil after the loop unless `ListSkeletonMarkers` failed.
  - Asserts `var _ EagerHydrateSignaler = (*EagerSignalCore)(nil)`.
- Add `cmd/bootstrap/eager_signal_hydrate_test.go`:
  - Use the existing `recordingLogger` and a stub `state.ServerOptionLister` (see `listerStub` in `internal/bootstrapadapter/adapters_test.go` for the shape) to drive the marker enumeration.
  - Tests:
    - N markers → N `WriteFIFOSignal` invocations, each path equal to `state.FIFOPath(stateDir, paneKey)`.
    - Zero markers → zero `WriteFIFOSignal` invocations; method returns nil.
    - Per-FIFO `WriteFIFOSignal` error → logger.Warn called with `eager-signal: write fifo` substring; loop continues to siblings; method still returns nil.
    - `ListSkeletonMarkers` error → method returns wrapped error; no `WriteFIFOSignal` calls attempted.
    - Nil-Logger tolerated (no panic).

**Acceptance Criteria**:
- [ ] `bootstrap.EagerHydrateSignaler` interface defined with single method `EagerSignalHydrate() error`.
- [ ] `*bootstrap.Orchestrator` has `EagerSignaler EagerHydrateSignaler` field.
- [ ] `bootstrap.NoOpEagerHydrateSignaler` zero-value struct implements the interface and returns nil.
- [ ] `*EagerSignalCore` (in `cmd/bootstrap/eager_signal_hydrate.go`) implements the interface.
- [ ] N-marker case: writes the signal byte to N FIFOs (one per marker), each at `state.FIFOPath(stateDir, paneKey)`.
- [ ] Zero-marker case: zero `WriteFIFOSignal` invocations; returns nil.
- [ ] Per-FIFO write failure: logger.Warn with `eager-signal: write fifo <path>:` shape; loop continues to sibling panes; method returns nil.
- [ ] `ListSkeletonMarkers` failure: returned wrapped from the step (orchestrator will Warn-and-swallow). No `WriteFIFOSignal` calls attempted.
- [ ] Nil Logger tolerated via local `noopLogger{}` substitution at method entry.
- [ ] Step never escalates to a fatal error — the orchestrator wiring (task 1-4) treats any non-nil return as a soft warning.

**Tests**:
- `"EagerSignalHydrate writes signal byte to every marker's FIFO at FIFOPath(stateDir, paneKey)"`
- `"EagerSignalHydrate is a no-op when zero markers are present"`
- `"EagerSignalHydrate continues past per-FIFO write errors and logs eager-signal: write fifo WARN"`
- `"EagerSignalHydrate returns wrapped error when ListSkeletonMarkers fails (no WriteFIFOSignal attempted)"`
- `"EagerSignalHydrate tolerates nil Logger via local noop substitution"`
- `"NoOpEagerHydrateSignaler.EagerSignalHydrate returns nil"`

**Edge Cases**:
- Zero markers post-Restore is a no-op — return nil with no FIFO writes attempted.
- Per-FIFO write failure logs WARN and continues to sibling panes — never aborts the loop.
- Step never escalates to fatal — orchestrator soft-warns and continues.
- Marker map with paneKeys that contain the canonical sanitiser shape (e.g. `session__w0p0`) — `state.FIFOPath` handles this verbatim.

**Context**:
> Spec § "Pane Enumeration and FIFO Resolution" → "The pane-target list is the marker map itself; the corresponding FIFO path is deterministic via `state.FIFOPath(stateDir, paneKey)`. No `list-panes` enumeration is needed at this layer."
>
> Spec § "Failure Posture" → "Per-FIFO write failures are soft warnings. Log shape: `WARN | hydrate | eager-signal: write fifo <fifoPath>: <error>`."
>
> Existing pattern to mirror: `cmd/bootstrap/stale_marker_cleanup.go` `MarkerCleanupCore` (seam interfaces + Core struct + `var _ MarkerCleaner = (*MarkerCleanupCore)(nil)` assertion + nil-Logger substitution at entry).

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Fix 1: Bootstrap Eager-Signaling Step" → "Behaviour", "Pane Enumeration and FIFO Resolution", "Failure Posture", "Adapter Wiring".

## killed-sessions-resurrect-on-restart-1-4 | approved

### Task killed-sessions-resurrect-on-restart-1-4: Insert EagerSignalHydrate into Orchestrator between Restore and Clear @portal-restoring

**Problem**: The `EagerHydrateSignaler` seam exists after task 1-3 but `Orchestrator.Run` does not call it. Until the step is invoked at the spec-mandated position (after step 5 Restore, before step 7 Clear `@portal-restoring`), eager signaling is dead code and AC1/AC2/AC4/AC8 cannot be satisfied.

**Solution**: Wire `o.EagerSignaler.EagerSignalHydrate()` into `Orchestrator.Run` between the existing step 5 (Restore) and the existing step 6 (Clear `@portal-restoring`). The new call becomes step 6 in the renumbered sequence; the existing Clear shifts to step 7, CleanStaleMarkers to 8, SweepOrphanFIFOs to 9, CleanStale to 10. The step's non-nil return is logged via `Logger.Warn(state.ComponentBootstrap, "step 6 (EagerSignalHydrate) failed: %v", err)` and swallowed — never escalates to a fatal abort. Update the orchestrator's package doc comment, the `Run` method's per-step Debug labels, and the existing ordering test to assert the new step's position.

**Outcome**: `Orchestrator.Run` executes the ten-step sequence in spec order. The ordering test asserts `EagerSignalHydrate` at call-list index 5 (zero-indexed; sixth call). The step runs while `@portal-restoring` is still set (AC8 invariant) and after Restore has populated the marker set.

**Do**:
- In `cmd/bootstrap/bootstrap.go`:
  - Update the package doc comment's numbered step list to ten entries with `EagerSignalHydrate` at position 6 between Restore and Clear; renumber Clear/CleanStaleMarkers/Sweep/CleanStale to 7/8/9/10.
  - Update each later step's Debug label string from `"step N (Name): entering"` to its new number.
  - Insert between the current step 5 (Restore) block and the current step 6 (Clear) block:
    ```go
    // Step 6 — EagerSignalHydrate (best-effort). Runs while
    // @portal-restoring is still set so daemon captureAndCommit
    // suppression remains in force during helper-driven scrollback
    // replay (AC8). Iterates the freshly-set @portal-skeleton-* marker
    // map and writes the hydrate signal byte to each pane's FIFO. A
    // non-nil err is logged and swallowed — eager signaling failures
    // must never block PersistentPreRunE.
    o.Logger.Debug(state.ComponentBootstrap, "step 6 (EagerSignalHydrate): entering")
    if err := o.EagerSignaler.EagerSignalHydrate(); err != nil {
        o.Logger.Warn(state.ComponentBootstrap, "step 6 (EagerSignalHydrate) failed: %v", err)
        // Continue per spec.
    }
    ```
  - Update the soft-warning paths comment block on `Run` to add an `EagerSignalHydrate` entry.
- In `cmd/bootstrap/bootstrap_test.go`:
  - Extend `stepRecorder` with an `EagerSignalHydrate()` method that records `"EagerSignalHydrate"` and an `EagerSignalHydrateErr error` field.
  - Update `newOrchestrator` to wire `r` into the new `EagerSignaler` field.
  - Update `TestOrchestratorRun_executesStepsInSpecOrder`'s `want` slice to: `["EnsureServer", "RegisterPortalHooks", "Set", "EnsureSaver", "Restore", "EagerSignalHydrate", "Clear", "CleanStaleMarkers", "Sweep", "CleanStale"]`.
  - Update any other tests in this file whose `want` slice asserts call ordering (e.g. `TestOrchestratorRun_continuesPastEnsureSaverFailureAndAppendsWarning`) to include `EagerSignalHydrate` at the new position.
  - Add a new test `TestOrchestratorRun_continuesPastEagerSignalHydrateFailure`:
    - `r := &stepRecorder{EagerSignalHydrateErr: sentinel}`.
    - Assert `Run` returns no error.
    - Assert call ordering still completes through `CleanStale`.
    - Assert `logger.warnings` contains a string with substring `step 6 (EagerSignalHydrate) failed`.
- In `cmd/bootstrap/orchestrator_builder_test.go`:
  - Add `EagerSignaler bootstrap.EagerHydrateSignaler` to `orchestratorOpts`.
  - In `buildIntegrationOrchestrator`, default to `bootstrap.NoOpEagerHydrateSignaler{}` when nil.
  - Wire `EagerSignaler: opts.EagerSignaler` into the `*bootstrap.Orchestrator` literal.
- In `cmd/bootstrap_production.go`:
  - Wire a placeholder `EagerSignaler: bootstrap.NoOpEagerHydrateSignaler{}` into `buildProductionOrchestrator`'s `*bootstrap.Orchestrator` literal so production compiles. Task 1-5 replaces this with the real adapter.

**Acceptance Criteria**:
- [ ] `Orchestrator.Run` invokes `o.EagerSignaler.EagerSignalHydrate()` between the existing Restore and Clear blocks.
- [ ] Step labels are renumbered 1..10 in the orchestrator's Debug call sites and package doc comment.
- [ ] A non-nil return from `EagerSignalHydrate` is logged via `Logger.Warn` with substring `step 6 (EagerSignalHydrate) failed` and swallowed — `Run` returns no error from this path.
- [ ] `TestOrchestratorRun_executesStepsInSpecOrder`'s `want` slice has `EagerSignalHydrate` at index 5.
- [ ] All other ordering tests in `bootstrap_test.go` updated to include `EagerSignalHydrate` at the same position.
- [ ] New `TestOrchestratorRun_continuesPastEagerSignalHydrateFailure` test passes.
- [ ] `orchestratorOpts` extended with `EagerSignaler` defaulted to `NoOpEagerHydrateSignaler{}`.
- [ ] `buildProductionOrchestrator` compiles with `EagerSignaler: bootstrap.NoOpEagerHydrateSignaler{}` placeholder (replaced in task 1-5).
- [ ] AC8 invariant: the new step runs strictly before the Clear `@portal-restoring` step (preserved by the insert position; verified by the ordering test).

**Tests**:
- `"Orchestrator.Run executes EagerSignalHydrate at position 6 (between Restore and Clear)"` — the updated ordering test.
- `"Orchestrator.Run continues past EagerSignalHydrate failure with a soft warning"` — new failure-path test.
- `"All existing ordering tests' want slices updated to include EagerSignalHydrate at index 5"` — regression coverage by file-wide update.

**Edge Cases**:
- AC8 invariant: step runs while `@portal-restoring` is still set — guaranteed by inserting before the Clear block (verified structurally by call ordering, not via a separate observation).
- Step runs after Restore populates the marker set — guaranteed by inserting after the Restore block.
- Ordering test asserts `EagerSignalHydrate` at zero-indexed position 5 (sixth call) in the canonical `want` slice.
- Step never escalates to fatal — confirmed by the new failure-path test asserting `Run` returns no error when `EagerSignalHydrateErr` is non-nil.

**Context**:
> Spec § "Bootstrap Step Numbering Update" → ordered list pinning EagerSignalHydrate at position 6 between Restore (5) and Clear (7).
>
> Spec § "Placement and Ordering Invariant" → "The step **must** run while `@portal-restoring` is still set. The daemon's `captureAndCommit` loop is suppressed during the `@portal-restoring` window."
>
> Existing pattern to mirror: the step 7 (CleanStaleMarkers) wiring in `Orchestrator.Run` — Debug entry, method invocation, Warn-and-swallow on non-nil err, comment block above the call. Replicate verbatim for step 6.

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Fix 1: Bootstrap Eager-Signaling Step" → "Placement and Ordering Invariant", "Bootstrap Step Numbering Update".

## killed-sessions-resurrect-on-restart-1-5 | approved

### Task killed-sessions-resurrect-on-restart-1-5: Wire production EagerHydrateSignaler adapter in internal/bootstrapadapter

**Problem**: After task 1-4, `buildProductionOrchestrator` wires a `NoOpEagerHydrateSignaler{}` placeholder for the new step. Without a real adapter, the production cold-start path runs the step as a no-op — markers stay leaked, AC1/AC2/AC4 unsatisfied. The spec requires a production adapter that wires `state.ListSkeletonMarkers` (via `*tmux.Client`) and the shared `state.WriteFIFOSignal` writer.

**Solution**: Add a new adapter `EagerHydrateSignaler` in `internal/bootstrapadapter` that holds `Client state.ServerOptionLister`, `StateDir string`, and `Logger *state.Logger`. The adapter delegates to `(*bootstrap.EagerSignalCore)` with:
- `Markers = a.Client`
- `StateDir = a.StateDir`
- `WriteFIFOSignal = func(path string) error { return state.WriteFIFOSignal(path, state.OpenFIFOForSignal, time.Sleep) }`
- `Logger = a.Logger` (forwarded structurally — `*state.Logger` satisfies `bootstrap.Logger` via Debug/Warn/Error)

Then update `buildProductionOrchestrator` in `cmd/bootstrap_production.go` to replace the `NoOpEagerHydrateSignaler{}` placeholder with the new adapter, reusing the `stateDir` already resolved at orchestrator construction.

**Outcome**: Production cold-start invokes the eager-signal step against the real tmux server, writing the signal byte to every freshly-armed marker's FIFO via the production primitives. `stateDir` resolves once at orchestrator construction (mirroring Restore and FIFOSweeper); FIFOPath derivation is per-paneKey via `state.FIFOPath`; no new public API surface beyond the new adapter struct is exposed.

**Do**:
- In `internal/bootstrapadapter/adapters.go`:
  - Add a new adapter struct adjacent to `FIFOSweeper`:
    ```go
    // EagerHydrateSignaler satisfies bootstrap.EagerHydrateSignaler. Step 6
    // of the bootstrap sequence — runs after step 5 (Restore) populates
    // @portal-skeleton-* markers and before step 7 (Clear @portal-restoring),
    // so the daemon's captureAndCommit suppression remains in force during
    // helper-driven scrollback replay.
    //
    // Client is typed as state.ServerOptionLister (rather than *tmux.Client)
    // so unit tests can inject a stub. Production wiring still passes
    // *tmux.Client, which satisfies the interface via ShowAllServerOptions.
    type EagerHydrateSignaler struct {
        Client   state.ServerOptionLister
        StateDir string
        Logger   *state.Logger // nil tolerated; *state.Logger is nil-safe.
    }

    // EagerSignalHydrate enumerates live skeleton markers and writes the
    // hydrate signal byte to each pane's FIFO via state.WriteFIFOSignal.
    // Per-FIFO failures are logged via Logger and skipped inside the
    // bootstrap.EagerSignalCore body; ListSkeletonMarkers failure is
    // wrapped and returned for the orchestrator's step-6 Warn-and-swallow
    // path.
    func (s *EagerHydrateSignaler) EagerSignalHydrate() error {
        core := &bootstrap.EagerSignalCore{
            Markers:  s.Client,
            StateDir: s.StateDir,
            WriteFIFOSignal: func(path string) error {
                return state.WriteFIFOSignal(path, state.OpenFIFOForSignal, time.Sleep)
            },
            Logger: s.Logger,
        }
        return core.EagerSignalHydrate()
    }
    ```
  - Add the `time` import; add the `cmd/bootstrap` import (allowed because `internal/bootstrapadapter` already lives outside `cmd`).
- In `cmd/bootstrap_production.go`:
  - Replace `EagerSignaler: bootstrap.NoOpEagerHydrateSignaler{}` with:
    ```go
    EagerSignaler: &bootstrapadapter.EagerHydrateSignaler{
        Client:   client,
        StateDir: stateDir,
        Logger:   logger,
    },
    ```
- Add `internal/bootstrapadapter/adapters_test.go` tests:
  - `TestEagerHydrateSignaler_PropagatesListSkeletonMarkersError` — mirror the existing `TestFIFOSweeper_PropagatesListSkeletonMarkersError` pattern: use `listerStub` to inject a `ShowAllServerOptions` error and assert `EagerSignalHydrate` returns a wrapped error containing the cause.
  - `TestEagerHydrateSignaler_ZeroMarkersIsNoOp` — `listerStub` returns empty output; assert `EagerSignalHydrate` returns nil with no FIFO writes attempted (use a sentinel `WriteFIFOSignal` shim if needed, or rely on the underlying `EagerSignalCore` test coverage already established in task 1-3).
  - Optionally a thin smoke test that wires a real `*tmux.Client` via `tmuxtest` — but this overlaps with task 1-6's integration test, so the unit-level lister-stub coverage is sufficient here.

**Acceptance Criteria**:
- [ ] `bootstrapadapter.EagerHydrateSignaler` struct defined with fields `Client state.ServerOptionLister`, `StateDir string`, `Logger *state.Logger`.
- [ ] `EagerSignalHydrate()` delegates to `*bootstrap.EagerSignalCore` with the production `WriteFIFOSignal` closure (`state.WriteFIFOSignal(path, state.OpenFIFOForSignal, time.Sleep)`).
- [ ] `buildProductionOrchestrator` wires the adapter into `EagerSignaler`, reusing the orchestrator-scope `stateDir` (resolved once at orchestrator construction, not per step call).
- [ ] FIFOPath derivation per paneKey happens inside `EagerSignalCore` via `state.FIFOPath(stateDir, paneKey)` — no FIFO path enumeration at the adapter layer.
- [ ] No new public API surface beyond the new adapter struct is exposed (no new exported functions in `internal/state` beyond what task 1-1 already added).
- [ ] `TestEagerHydrateSignaler_PropagatesListSkeletonMarkersError` and `TestEagerHydrateSignaler_ZeroMarkersIsNoOp` pass.
- [ ] Logger forwarding: `*state.Logger` structurally satisfies `bootstrap.Logger` (Debug/Warn/Error) — confirmed by compilation.

**Tests**:
- `"EagerHydrateSignaler propagates ListSkeletonMarkers error wrapped"` — listerStub injection.
- `"EagerHydrateSignaler returns nil with no writes when zero markers present"` — listerStub injection + sentinel writer.
- `"buildProductionOrchestrator wires a non-NoOp EagerSignaler"` — compile-time assertion; can be a simple `if _, ok := o.EagerSignaler.(*bootstrapadapter.EagerHydrateSignaler); !ok { t.Fatalf(...) }` smoke (optional — may be redundant with the production wire-up).

**Edge Cases**:
- `stateDir` resolved once at orchestrator construction in `buildProductionOrchestrator` and shared across Restore, FIFOSweeper, and EagerHydrateSignaler — no per-step re-resolution.
- FIFOPath derivation per paneKey is the responsibility of `EagerSignalCore` (via `state.FIFOPath`), not the adapter — keeps the seam thin.
- No new public API surface in `internal/state` beyond `WriteFIFOSignal`, `OpenFIFOForSignal`, `SignalHydrateRetryDelays` (added in task 1-1).
- `*state.Logger` nil tolerated — both the adapter and `EagerSignalCore` substitute a no-op locally.

**Context**:
> Spec § "Adapter Wiring" → "The production adapter in `internal/bootstrapadapter` wires `state.ListSkeletonMarkers` (with the orchestrator's `*tmux.Client`) for the marker enumeration and the shared `internal/state` package's `WriteFIFOSignal` for the writer. The orchestrator owns `stateDir` and resolves `state.FIFOPath(stateDir, paneKey)` per marker before calling `WriteFIFOSignal`."
>
> Existing pattern to mirror: `internal/bootstrapadapter/adapters.go` `FIFOSweeper` (struct shape, ListSkeletonMarkers wrap-and-return on error, lister-stub test pattern).

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Fix 1: Bootstrap Eager-Signaling Step" → "Adapter Wiring", "Pane Enumeration and FIFO Resolution".

## killed-sessions-resurrect-on-restart-1-6 | approved

### Task killed-sessions-resurrect-on-restart-1-6: Multi-session cold-start integration test asserting empty marker set within 2s (AC1)

**Problem**: AC1 mandates that after a cold-start with N≥2 saved sessions, all `@portal-skeleton-<paneKey>` markers are unset within 2 seconds post-bootstrap, with no client attach required to drive the unset. Existing integration tests in `cmd/bootstrap/` cover only the attached-session signaling path or use `DriveSignalHydrate` to manually drive FIFO writes — they do not exercise the new eager-signal step's bootstrap-time, no-attach-required behaviour. Without this test, AC1 is not gated.

**Solution**: Add a new integration test `TestPhase1Integration_EagerSignalHydrate_MultiSessionMarkersClearedWithin2s` that boots a real tmux server (via `tmuxtest`), seeds an `internal/state` directory with a sessions.json describing N≥2 saved sessions, runs the production orchestrator (or the integration builder with all real adapters wired), and polls `state.ListSkeletonMarkers(client)` with a 2-second timeout. Pass condition: empty marker set within the window. Must NOT issue any tmux `attach-session` or `switch-client` — eager signaling must drive the unset on its own.

**Outcome**: A real-tmux integration test gates AC1. Test fails on regression of: eager step disabled, marker set non-empty after 2s, or step not running at the spec-mandated position.

**Do**:
- Add a new file `cmd/bootstrap/eager_signal_hydrate_integration_test.go` with the `//go:build integration` build tag (mirroring `reboot_roundtrip_test.go`'s build tag pattern) so the heavy fixture gates off the default `go test ./...` lane.
- Use the `tmuxtest.New(t, "ptl-eager-")` socket harness to spin up an isolated tmux server.
- Use `newIntegrationStateDir(t)` and `openTestLogger(t, stateDir)` (already in `orchestrator_builder_test.go`) to set up `PORTAL_STATE_DIR` and a real logger.
- Seed `sessions.json` via the existing `restoretest` package's helpers (see `internal/restoretest` and the way `phase5_integration_test.go` builds sessions). Construct N=2 saved sessions (e.g. `alpha` with 1 window/1 pane and `beta` with 1 window/1 pane). Each session's pane has a structural paneKey that corresponds to the `@portal-skeleton-*` marker the restore step will set.
- Build the orchestrator via `buildIntegrationOrchestrator` with the real `RestoreAdapter` (wrapping `*restore.Orchestrator`), the real `EagerHydrateSignaler` adapter (`bootstrapadapter.EagerHydrateSignaler{Client: client, StateDir: stateDir, Logger: logger}`), and the real `RestoringMarker`. Saver/Hooks/StaleMarkers/Sweeper/Clean default to NoOp.
- The hydrate helper inside each restored pane will exec `portal state hydrate` (via the existing `buildHydrateCommand` path); the test does NOT exec the full helper — it relies on `state.WriteFIFOSignal` reaching the FIFO and the helper, on signal receipt, unsetting its marker.
  - Ensure the test's `portal` binary path is wired so the hydrate helper can actually run. The simplest shape is to reuse the `runRebootRoundTrip`-style `useBinary` fixture if it is straightforward; if not, the test can directly assert that `state.WriteFIFOSignal` was invoked for each marker (use a recording shim around the adapter's WriteFIFOSignal closure) and that **`@portal-skeleton-*` markers transition to empty within 2s** by polling.
  - Acceptable test shape: the integration goal is "eager-signal step writes the byte to every marker's FIFO inside the bootstrap window". Validate via:
    1. `runOrchestrator` returns nil within bootstrap.
    2. Poll `state.ListSkeletonMarkers(client)` every 50ms for up to 2s.
    3. Assert empty marker set within the window.
- Run `o.Run(context.Background())` and immediately begin polling with a 2-second deadline (`time.After(2*time.Second)`).
- Assert no `attach-session` or `switch-client` was invoked (recording-commander style or just don't issue any).
- Sub-tests / variants:
  - `N=2_DefaultIndices`: base-index 0, pane-base-index 0. Tests the default cold-start shape.
  - Optionally `N=3_LargerSet`: three saved sessions to confirm scaling.

**Acceptance Criteria**:
- [ ] Test file `cmd/bootstrap/eager_signal_hydrate_integration_test.go` exists with `//go:build integration` tag.
- [ ] Test gates on `tmuxtest.SkipIfNoTmux(t)` and skips cleanly when tmux is unavailable.
- [ ] Test seeds a sessions.json describing N≥2 saved sessions via `restoretest` helpers.
- [ ] Test runs the orchestrator with real `RestoreAdapter` + real `EagerHydrateSignaler` + real `RestoringMarker`.
- [ ] Test polls `state.ListSkeletonMarkers(client)` with a 50ms tick and 2-second deadline.
- [ ] Pass condition: marker set is empty within the 2-second window.
- [ ] Test does NOT issue any `tmux attach-session` or `tmux switch-client` calls.
- [ ] Test fails clearly when run against a build that omits the eager-signal step or wires `NoOpEagerHydrateSignaler{}`.

**Tests**:
- `"TestPhase1Integration_EagerSignalHydrate_MultiSessionMarkersClearedWithin2s/N=2_DefaultIndices"`
- (Optional) `"TestPhase1Integration_EagerSignalHydrate_MultiSessionMarkersClearedWithin2s/N=3_LargerSet"`

**Edge Cases**:
- N≥2 saved sessions — the deterministic-bug case that the per-session signaling path leaves N−1 sessions stuck.
- Polls `state.ListSkeletonMarkers` directly via the real `*tmux.Client.ShowAllServerOptions` path — no mocking.
- No client attach required to drive the unset — eager signaling alone must clear all markers within the window.
- 2-second bound is generous (the spec's "10× slack at any plausible N" framing); the test should hit empty marker set well under 1s in practice. If the test goes flaky under CI load, the bound is the loosen-to-3s budget per spec — but do not pre-emptively widen it.

**Context**:
> Spec § "Acceptance Criteria" → AC1: "After a tmux server cold-start with N≥2 saved sessions, all `@portal-skeleton-<paneKey>` markers are unset within **2 seconds** post-bootstrap (no client attach required to drive the unset). The integration test polls `state.ListSkeletonMarkers` with a 2-second timeout; pass condition is empty marker set within the window."
>
> Existing patterns to mirror: `cmd/bootstrap/phase5_integration_test.go` (orchestrator + tmuxtest socket harness + RestoreAdapter wiring), `cmd/bootstrap/reboot_roundtrip_test.go` (multi-session sessions.json seeding via restoretest, and the `useBinary` flag if the helper needs a real binary path).

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Acceptance Criteria → Behavioural → AC1", "Test Plan → Integration → Multi-session cold-start".

## killed-sessions-resurrect-on-restart-1-7 | approved

### Task killed-sessions-resurrect-on-restart-1-7: Update CLAUDE.md Server bootstrap section with renumbered 10-step list and EagerSignalHydrate paragraph

**Problem**: `CLAUDE.md`'s "Server bootstrap" section documents the nine-step orchestrator. After Phase 1 lands, the section is out of date — it does not describe `EagerSignalHydrate` and its step numbering does not match the production code. The spec mandates the section be updated **as part of the same PR** so contributor docs stay in sync.

**Solution**: Edit the "Server bootstrap" section in `/Users/leeovery/Code/portal/CLAUDE.md` to:
1. Change the opening sentence from "nine-step" to "ten-step".
2. Insert a new step 6 entry between the existing step 5 (Restore) and the existing step 6 (Clear `@portal-restoring`) — a single-paragraph EagerSignalHydrate description.
3. Renumber the existing Clear/CleanStaleMarkers/Sweep/CleanStale entries from 6/7/8/9 to 7/8/9/10.
4. Update the "After step 9" sentence at the end of the section to "After step 10".
5. Preserve the existing "Return is the post-step boundary, not a numbered step" framing verbatim — do NOT add a numbered "Return" entry.

**Outcome**: CLAUDE.md reflects the production code's ten-step ordering and includes a one-paragraph description of EagerSignalHydrate's role, ordering invariant, and best-effort failure semantics. Step numbering matches the orchestrator's Debug labels and the spec's Bootstrap Step Numbering Update list.

**Do**:
- Open `/Users/leeovery/Code/portal/CLAUDE.md`.
- Locate the "### Server bootstrap" heading (currently around line 69).
- Edit the opening paragraph: change `nine-step` → `ten-step`. Keep `"Return" is the post-step boundary, not a numbered step:` verbatim.
- Insert between existing items 5 and 6 a new numbered entry:
  ```markdown
  6. **EagerSignalHydrate** — best-effort write of the hydrate signal byte to every freshly-armed `@portal-skeleton-*` pane's FIFO via `state.WriteFIFOSignal`. Runs while `@portal-restoring` is still set so daemon `captureAndCommit` suppression remains in force during helper-driven scrollback replay. Eliminates the per-session signaling gap that would otherwise leave N−1 saved sessions' helpers waiting on `client-attached` events that never fire for non-attached sessions. A non-nil return logs at WARN under `ComponentBootstrap` and is swallowed; never escalates to a fatal abort.
  ```
- Renumber the subsequent items:
  - `6. **Clear @portal-restoring**` → `7. **Clear @portal-restoring**`
  - `7. **CleanStaleMarkers**` → `8. **CleanStaleMarkers**`
  - `8. **SweepOrphanFIFOs**` → `9. **SweepOrphanFIFOs**`
  - `9. **CleanStale**` → `10. **CleanStale**`
- Update the next paragraph's `After step 9` → `After step 10`.
- Do NOT modify any other section of CLAUDE.md.

**Acceptance Criteria**:
- [ ] Opening sentence reads "ten-step `bootstrap.Orchestrator`" (was "nine-step").
- [ ] `"Return" is the post-step boundary, not a numbered step` framing preserved verbatim.
- [ ] New step 6 (EagerSignalHydrate) entry exists, single-paragraph, between Restore and Clear.
- [ ] Existing Clear/CleanStaleMarkers/Sweep/CleanStale entries renumbered 7/8/9/10 in that order.
- [ ] `After step 10` (was `After step 9`) in the trailing paragraph.
- [ ] No other CLAUDE.md sections modified.
- [ ] The EagerSignalHydrate paragraph mentions: best-effort, runs while `@portal-restoring` set, daemon `captureAndCommit` suppression, eliminates per-session signaling gap, never fatal.

**Tests**:
- Manual review: open CLAUDE.md and verify the section reads correctly end-to-end.
- `"git diff CLAUDE.md shows only the Server bootstrap section was modified"` — pre-commit visual check.

**Edge Cases**:
- "Return is the post-step boundary, not a numbered step" framing must be preserved exactly — the spec explicitly calls this out.
- Renumbering must update **only** the four subsequent step numbers and the trailing paragraph's `After step N`. Do not introduce any new prose elsewhere.
- One-paragraph EagerSignalHydrate description — not a multi-paragraph expansion; keep parity with the surrounding step entries' density.

**Context**:
> Spec § "Bootstrap Step Numbering Update" → "The `CLAUDE.md` 'Server bootstrap' section is updated **as part of the same PR**. The update only renumbers steps and inserts a one-paragraph EagerSignalHydrate description; the existing 'Return is the post-step boundary, not a numbered step' framing is preserved."
>
> Existing CLAUDE.md "Server bootstrap" section is at `/Users/leeovery/Code/portal/CLAUDE.md` lines 69–85.

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Fix 1: Bootstrap Eager-Signaling Step" → "Bootstrap Step Numbering Update".

## killed-sessions-resurrect-on-restart-1-8 | approved

### Task killed-sessions-resurrect-on-restart-1-8: Integration test asserting daemon captureAndCommit resumes for previously-stuck-marker panes (AC4)

**Problem**: AC4 ("Scrollback save resumes for previously-stuck-marker panes — daemon `captureAndCommit` no longer indefinitely skips any live pane") is the user-visible verification that Symptom C is closed. AC1's marker-cleared assertion (task 1-6) implies AC4 transitively, but does not directly observe the daemon producing a scrollback dump for a previously-stuck pane. Without an explicit AC4 test, a future regression that clears markers but breaks daemon resumption (e.g. daemon caches the suppression flag) would silently re-introduce empty-scrollback-on-next-cold-start.

**Solution**: Extend the Phase 1 multi-session integration test scaffold (task 1-6) with an additional assertion phase that, after markers transition to empty, drives one daemon capture tick (via `state.RunCaptureOnce` or equivalent test seam) and asserts a non-empty scrollback `.bin` file exists for the previously-non-attached session's pane. Use the existing `state.TailScrollback` helper to read the dump and assert at least one record was captured.

**Outcome**: AC4 is verified end-to-end. Test fails on regression of: daemon refuses to capture a pane whose marker was stuck-then-cleared, or scrollback file remains empty post-eager-signal.

**Do**:
- Extend `cmd/bootstrap/eager_signal_hydrate_integration_test.go` (added in task 1-6) with a second sub-test `TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4`.
- Reuse the same N=2 saved-sessions fixture and orchestrator wiring from task 1-6.
- After the marker-cleared poll completes (markers empty within 2 s), drive one daemon capture-tick by directly invoking the daemon's capture-once primitive (e.g. `state.RunCaptureOnce(client, stateDir, logger)`; if no such primitive exists, expose one as a test seam in `internal/state` for this purpose, mirroring the existing capture-loop body).
- Assert via `state.TailScrollback(state.ScrollbackPath(stateDir, betaPaneKey), 10)` that at least one record exists for the non-attached session's pane.
- The test gates on `tmuxtest.SkipIfNoTmux(t)` and the `//go:build integration` tag, consistent with task 1-6.

**Acceptance Criteria**:
- [ ] Sub-test `TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4` exists in `cmd/bootstrap/eager_signal_hydrate_integration_test.go`.
- [ ] Sub-test passes against a build with eager signaling wired; fails if the daemon refuses to capture a previously-stuck pane.
- [ ] No `t.Parallel()` usage.
- [ ] Skips cleanly under `-short` and when tmux is unavailable.

**Tests**:
- `"TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4"` — drives one capture tick post-eager-signal and asserts non-empty scrollback dump for the previously-non-attached session's pane.

**Edge Cases**:
- The capture tick must run after the `@portal-restoring` window has closed (post step 7 Clear). The orchestrator's full Run handles this — the test does not need to manually toggle the marker.
- If the daemon's capture-once primitive does not yet exist, this task adds it as a thin test seam in `internal/state` (no public API beyond the seam needed for production).

**Context**:
> Spec § "Acceptance Criteria → Behavioural" AC4: "Scrollback save resumes for previously-stuck-marker panes — daemon `captureAndCommit` no longer indefinitely skips any live pane."
>
> Spec § "AC ↔ Fix Traceability": AC4 → Fix 1 (eager signaling unsets markers, daemon resumes capturing those panes).

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Acceptance Criteria → Behavioural → AC4" and "AC ↔ Fix Traceability".
