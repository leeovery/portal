---
phase: 1
phase_name: Version-stamped latch + full-bootstrap set-point
total: 4
---

## skip-bootstrap-when-warm-1-1 | approved

### Task skip-bootstrap-when-warm-1-1: Latch read/verdict helper with version-aware three-way semantics

**Problem**: Portal has no signal for "has this tmux server already been bootstrapped by *this* binary?" The later entry-path branch (Phase 2) needs to reduce a single `TryGetServerOption("@portal-bootstrapped")` read — whose result can be {absent, present-and-version-matches, present-and-version-mismatches, read-error/down-server} — to one boolean verdict, satisfied *only* when the latch is present AND its stored value equals the running binary version. No such helper or option-name constant exists yet.

**Solution**: Add a `@portal-bootstrapped` server-option name constant and a version-aware verdict helper to `internal/state/markers.go`, alongside the existing `RestoringMarkerName` / `IsRestoringSet` precedent. The helper reuses the existing `RestoringChecker` seam (`TryGetServerOption(name) (val, found, err)`, satisfied by `*tmux.Client`) and takes the running version as a plain function parameter — keeping `internal/state` a leaf (no `cmd.version` import) and making the version-mismatch branch unit-testable without rebuilding the binary. Satisfaction is a parse-free plain string equality (`stored == runningVersion`).

**Outcome**: `state.BootstrappedLatchSatisfied(checker, runningVersion) bool` returns `true` iff the latch is present and its stored value exactly equals `runningVersion`; every other outcome (absent, empty value, version-mismatch, read-error/down-server) returns `false`. A `state.BootstrappedMarkerName` constant (`"@portal-bootstrapped"`) is the single source of truth for the option name.

**Do**:
- In `internal/state/markers.go`, add a `BootstrappedMarkerName = "@portal-bootstrapped"` const next to `RestoringMarkerName` (line ~19), with a doc comment noting: same server-option mechanism as `@portal-restoring` (dies with the tmux server); differs in that its **value is load-bearing** — the binary version — not presence-only.
- Add `func BootstrappedLatchSatisfied(c RestoringChecker, runningVersion string) bool` alongside `IsRestoringSet`. Body: `val, found, err := c.TryGetServerOption(BootstrappedMarkerName)`; if `err != nil` return `false`; if `!found` return `false`; return `val == runningVersion`. Reuse the existing `RestoringChecker` interface verbatim — do NOT add a new seam.
- Document in the helper's godoc the exact three-way (four-outcome) semantics from spec §"Semantics — satisfied": absent → false; present+match → true; present+mismatch → false; read-error/down-server → false. Note that both "value mismatch" and "unreadable/error" fold into *not satisfied* deliberately (a separate `ServerRunning()` probe is not required — the read fails gracefully on a down server), and that equality is a naive parse-free compare because the stored value format is exactly `cmd.version` in v1 (no delimiter/forensic extras).
- Note explicitly in the godoc that an empty stored value (`found=true, val==""`) is not satisfied *unless* `runningVersion` is itself empty; production always passes a non-empty version so empty-value latches never satisfy in practice — this is the plain equality falling out, not a special case.

**Acceptance Criteria**:
- [ ] `state.BootstrappedMarkerName` equals `"@portal-bootstrapped"`.
- [ ] `BootstrappedLatchSatisfied` returns `true` only when `found==true` AND `val==runningVersion`.
- [ ] Absent (`found==false`) → `false`.
- [ ] Present but version-mismatch (`found==true`, `val != runningVersion`) → `false`.
- [ ] Read-error / down-server (`err != nil`) → `false` (error swallowed, folded into not-satisfied — the helper returns a bare bool, not `(bool, error)`).
- [ ] Empty stored value with a non-empty running version → `false`.
- [ ] The running version is a plain string parameter — no `cmd.version` import in `internal/state`, no global to swap; the mismatch branch is exercised by passing a different string.
- [ ] `go build -o portal .` and `go test ./internal/state/...` pass; `golangci-lint run` clean.

**Tests** (add to `internal/state/markers_test.go`, reuse the existing `checkerMock` at line ~23 which already returns `(val, found, err)` verbatim; mirror the `TestIsRestoringSet` table style; NO `t.Parallel()`):
- `"it returns true when latch present and version matches"` — `checkerMock{val: "1.2.3", found: true}`, `runningVersion="1.2.3"` → `true`.
- `"it returns false when latch absent"` — `checkerMock{found: false}`, any version → `false`.
- `"it returns false when stored version mismatches running version"` — `checkerMock{val: "1.2.2", found: true}`, `runningVersion="1.2.3"` → `false`.
- `"it returns false on read error / down server"` — `checkerMock{err: errors.New("tmux exploded")}` → `false` (error swallowed).
- `"it returns false when stored value is empty and running version is non-empty"` — `checkerMock{val: "", found: true}`, `runningVersion="1.2.3"` → `false`.
- `"it reads exactly the @portal-bootstrapped option name"` — assert the name passed to `TryGetServerOption` is `state.BootstrappedMarkerName` (extend `checkerMock` with a captured-name field if the existing mock does not already record it, or assert via a wrapper).

**Edge Cases**:
- Absent → not satisfied. Version-mismatch → not satisfied. Read-error/down-server → not satisfied (swallowed to `false`). Empty stored value (non-empty running version) → not satisfied. Present + exact version match → satisfied.
- Do NOT return `(bool, error)`: the spec deliberately folds "unreadable/error" into *not satisfied → full bootstrap*, and the Phase 2 entry-path consumer wants one boolean with no error to branch on. This differs from `IsRestoringSet`, which propagates its error — call that difference out in the godoc so a future maintainer does not "fix" it to match.

**Context**:
> Spec §"The Version-Stamped Latch → Semantics — satisfied": "The latch is **satisfied** only when it is **present *and* its stored version equals the running binary's `cmd.version`**." Four-outcome table: Absent → full bootstrap; Present+match → abridged; Present+mismatch → full bootstrap; Read error/down-server → full bootstrap. "Both 'value mismatch' and 'unreadable/error' fold into *not satisfied → full bootstrap*. A separate `ServerRunning()` probe is not required — the read fails gracefully on a down server."
> Spec §"Value format (v1)": "the stored value is **exactly `cmd.version`** — a bare version string, nothing else. The satisfied test is a plain string equality (`stored == cmd.version`), so the format must stay parse-free."
> Spec §"Test Strategy → Design-for-test": "Make the 'current version' **injectable** (it is `cmd.version`) so a version-mismatch branch is unit-testable without rebuilding the binary." Here that means a plain function parameter — the helper needs no global swap.
> Spec §"Latch mechanism (reuse existing) → internal/state/markers.go": reuse seam interface `RestoringChecker` (TryGet); `@portal-restoring` is the direct precedent. Keeps `internal/state` free of an `internal/tmux` import (avoids a cycle — `internal/tmux` imports `internal/state`).
> **Ambiguity note**: The spec never states the helper's exact name or return signature (only its verdict semantics). This task fixes it as `BootstrappedLatchSatisfied(c RestoringChecker, runningVersion string) bool` — a bare-bool return that swallows the read error into `false` per the four-outcome fold. Phase 2's entry-path branch consumes exactly this shape; if that phase needs the error preserved for logging, it should read the latch itself and only borrow this helper's compare logic — but the spec's "single read, computed once" directive is satisfied by this bool.

**Spec Reference**: `.workflows/skip-bootstrap-when-warm/specification/skip-bootstrap-when-warm/specification.md` — §"The Version-Stamped Latch" (Storage, Semantics — satisfied, Value format, Why version-stamped), §"Latch mechanism (reuse existing)", §"Test Strategy → Design-for-test".

## skip-bootstrap-when-warm-1-2 | approved

### Task skip-bootstrap-when-warm-1-2: Set the latch as the final action of a successful Orchestrator.Run

**Problem**: A full bootstrap must record that it ran to completion so later warm commands can take the cheap abridged path. Nothing currently stamps `@portal-bootstrapped`. The write must be atomic-with-success (inside `Run`, not the two callers, so the synchronous and concurrent-goroutine invocation modes get it identically), set at the *end* (early-setting would let a concurrent command take the abridged path before Restore recreated the sessions), gated on no *fatal* error (soft warnings still latch), best-effort (a write failure is a pure WARN log line — never fatal, never in `warnings`, never on the progress channel), and — on the concurrent path — written before the terminal completion event fires.

**Solution**: Add a best-effort server-option writer seam to the `Orchestrator` plus a stamped version value, and insert a single `SetServerOption("@portal-bootstrapped", <version>)` call in `Run` after the last soft step (SweepOrphanFIFOs) and after the fatal-error gate (all fatal steps return early), immediately before the `bootstrap: orchestration complete` summary and the `return`. Wire the seam and version at production construction in `cmd/bootstrap_production.go` (`*tmux.Client` satisfies the writer via `SetServerOption`; the version is the ldflags-injected `cmd.version` the `saverAdapter` already reads). On the concurrent path no change is needed to `cmd/bootstrap_progress.go`: its goroutine sends the terminal `Done` event *after* `runner.Run` returns, so a latch written as `Run`'s final pre-return action is already guaranteed to precede `BootstrapCompleteMsg` / the progress pipe's `Done`.

**Outcome**: A `Run` that reaches the Return boundary with no fatal error stamps `@portal-bootstrapped = <version>` exactly once via the new seam, then emits the orchestration-complete summary and returns; a run that aborts at a fatal step returns before the stamp so the latch stays unset; a stamp-write error is logged WARN under the bootstrap component and swallowed (not appended to `warnings`, not on the progress channel, never fatal); identical behaviour on both invocation modes.

**Do**:
- In `cmd/bootstrap/bootstrap.go`, add a `LatchWriter` seam interface (a `SetServerOption(name, value string) error`-shaped interface, satisfied implicitly by `*tmux.Client`) and add fields to `Orchestrator`: `Latch LatchWriter` and `Version string` (the value to stamp). Document `Latch` as best-effort/nil-tolerant and `Version` as the ldflags-injected binary version.
- In `Run`, after the `emitStep(10, stepSweepOrphanFIFOs)` call (the last soft step) and before the `o.Logger.Info("orchestration complete", ...)` summary line, add the latch write. Because all fatal steps (`EnsureServer`, `RegisterPortalHooks`, `SetRestoring`, `ClearRestoring`) `return` early via `o.fatalf`, execution only reaches this point on a non-fatal run — so no extra error gate is needed; add a one-line comment stating this. Guard the call `if o.Latch != nil { ... }` so tests / fallbacks may leave it nil.
- Write body: `if err := o.Latch.SetServerOption(state.BootstrappedMarkerName, o.Version); err != nil { o.Logger.Warn("latch write failed", "marker", state.BootstrappedMarkerName, "error", err) }`. Do NOT append to `warnings`; do NOT emit a progress `StepEvent`; do NOT return the error. (Note: `cmd/bootstrap` already imports `internal/log`; import `internal/state` for the const — verify no cycle: `internal/state` does not import `cmd/bootstrap`, so this is safe.)
- In `cmd/bootstrap_production.go` `buildProductionOrchestrator`, set `Latch: client` and `Version: version` on the `&bootstrap.Orchestrator{...}` literal (the `client` is the already-built `*tmux.Client`; `version` is the same package-level `cmd.version` `saverAdapter` reads on line ~62).
- Update the `Run` godoc and the package/`Orchestrator` doc block to mention the terminal latch stamp: "after the last soft step and the fatal-error gate, before the orchestration-complete summary, a best-effort `SetServerOption(@portal-bootstrapped, version)` records that a full bootstrap ran to completion; a write failure is logged WARN and swallowed (never fatal, never in warnings, never on the progress channel)."
- No change to `cmd/bootstrap_progress.go`: confirm (and note in the task's implementation) that its goroutine emits `Done` only after `runner.Run(emitCtx)` returns (bootstrap_progress.go `start`, ~line 180-198), so the in-`Run` stamp already precedes the terminal event on the concurrent path — this ordering falls out, no code change.

**Acceptance Criteria**:
- [ ] `Orchestrator` has a `Latch LatchWriter` (best-effort, nil-tolerant) field and a `Version string` field; `*tmux.Client` satisfies `LatchWriter` via its existing `SetServerOption`.
- [ ] A `Run` with only soft warnings (SaverDownWarning / CorruptSessionsJSONWarning / a soft-failing step) reaches the stamp and calls `SetServerOption("@portal-bootstrapped", version)` exactly once, before the orchestration-complete summary line.
- [ ] A `Run` that aborts at a fatal step (EnsureServer / RegisterPortalHooks / SetRestoring / ClearRestoring) returns before the stamp — the latch writer is never called.
- [ ] A stamp-write error is logged WARN under the bootstrap component and swallowed: `Run` still returns `(serverStarted, warnings, nil)`, the returned `warnings` slice is unchanged (no latch-write warning appended), and no `StepEvent` is emitted for the write.
- [ ] The stamp uses `state.BootstrappedMarkerName` and `o.Version` verbatim (parse-free value equal to the injected version).
- [ ] Production wiring (`buildProductionOrchestrator`) sets `Latch: client` and `Version: version`.
- [ ] On the concurrent path the latch is written before the terminal `Done` / `BootstrapCompleteMsg` (verified by the in-`Run`-before-return ordering; no `bootstrap_progress.go` change required).
- [ ] `go build -o portal .` and `go test ./cmd/... ./cmd/bootstrap/...` pass; `golangci-lint run` clean.

**Tests** (in `cmd/bootstrap` package; NO `t.Parallel()`; use a recording latch-writer stub that captures `(name, value)` calls and can be primed to return an error; version injected via the orchestrator's `Version` field — no global swap needed at this layer):
- `"it stamps the latch with the version after a soft-warning-only run"` — build an orchestrator via the shared `buildIntegrationOrchestrator` / `NewWithDefaults` (all NoOp steps, plus a `WithSaver` that returns an error to produce a `SaverDownWarning`), wire a recording `Latch` + `Version="v1.2.3"`; run; assert exactly one `SetServerOption("@portal-bootstrapped", "v1.2.3")` call AND that the returned `warnings` still contains the SaverDownWarning (soft warnings still latch).
- `"it does not stamp the latch when a fatal step aborts the run"` — wire a `RestoringMarker` (or a fatal step stub) whose `Set()`/`Clear()` returns an error so `Run` aborts at a fatal step; assert the recording `Latch` recorded zero `SetServerOption` calls and `Run` returned a non-nil `*FatalError`.
- `"it swallows a latch-write failure as a pure WARN"` — prime the recording `Latch` to return an error; run a clean (all-NoOp) orchestrator with a `logtest.Sink`-backed logger; assert `Run` returns `(_, warnings, nil)` with `warnings` NOT containing any latch-write entry, that no progress `StepEvent` was emitted for the write (feed a recording `ProgressEmitter` via `WithProgressEmitter` and assert its indices are 1..10 only, no extra), and that the sink captured a WARN line under the bootstrap component mentioning the latch marker.
- `"it stamps exactly once, before the orchestration-complete summary"` — with a `logtest.Sink` logger, assert the latch `SetServerOption` call is recorded and, if ordering is observable via interleaved recording, that it precedes the "orchestration complete" INFO record (or at minimum that the stamp happened and the summary still emitted with `steps=10` from task 1-3).
- Add a compile-time assertion (in the production package test or `bootstrap_production_test.go`) that `var _ bootstrap.LatchWriter = (*tmux.Client)(nil)` holds.

**Edge Cases**:
- Fatal-step abort leaves the latch unset (writer never called) so the next command retries the full bootstrap.
- Soft-warning run still latches (spec: requiring a totally-clean run would let one transient `SaverDownWarning` force every command back to full bootstrap for the whole server lifetime).
- Write failure is a pure log line: NOT appended to the returned `warnings` slice, NOT routed through the progress channel / `bootstrapWarnings` sink (unlike SaverDownWarning), NOT fatal — it self-heals because the next command re-runs the near-no-op full bootstrap and retries the write.
- Concurrent path: latch written before the terminal completion event because it is `Run`'s final pre-return action and the goroutine sends `Done` only after `Run` returns — so "latch present ⟺ a full bootstrap ran to completion" holds by the time the picker transitions and any reopen burst could fire.

**Context**:
> Spec §"Latch Set-Point & Timing → Decision": "**Set the latch as the final action of a *successful* `Orchestrator.Run` — after the last step, gated on no fatal error.** … The latch is set *inside* `Run`, not by the two callers, so the synchronous path and the concurrent cold+TUI goroutine both get it identically — no second set-point to keep in sync. 'Latch present' ⟺ 'a full bootstrap ran to completion.'"
> Spec §"…Set at the *end*, not early": early-setting is unsafe (a concurrent command would take the abridged path *before Restore recreated the sessions*). End-setting is sufficient: the reopen burst can't fire until the user multi-selects in the picker, which appears only *after* bootstrap completes.
> Spec §"…'Successful' = no *fatal* error; soft warnings still latch": "Only a **fatal** step (EnsureServer / RegisterPortalHooks / SetRestoring / ClearRestoring — the steps that already abort with a non-zero exit / red TUI frame) leaves the latch **unset**."
> Spec §"Write posture" + §"Insertion point in `Run`": "The terminal `SetServerOption('@portal-bootstrapped', version)` is **best-effort**: on failure, log WARN and swallow — never fatal. … The latch write goes **after the last soft step and after the fatal-error gate, but before the orchestration-complete summary + return**. … A latch-write failure is a **pure log line** (WARN under the bootstrap component) on both paths — it is **not** appended to the returned `warnings` slice and **not** routed through the progress channel / `bootstrapWarnings` sink … the same treatment applies inside the concurrent goroutine."
> Spec §"Ordering bonus": because the latch is set only *after* `EagerSignalHydrate` and `Clear @portal-restoring`, "latch present" guarantees hydrate signalling finished and `@portal-restoring` was cleared — the latch and `@portal-restoring` can never both be set on an abridged command.
> Verified grounding: `cmd/bootstrap_progress.go` `start` runs `started, warnings, err := runner.Run(emitCtx)` then sends the `Done`/terminal event — so the in-`Run` stamp precedes the terminal event with no progress-file change. Depends on task 1-1's `state.BootstrappedMarkerName` const. The `steps=10` in the orchestration-complete summary is task 1-3's change; this task's ordering assertion should tolerate whichever `totalSteps` value is live when it runs.

**Spec Reference**: `.workflows/skip-bootstrap-when-warm/specification/skip-bootstrap-when-warm/specification.md` — §"Latch Set-Point & Timing" (Decision, Write posture, Insertion point in Run, Ordering bonus), §"Edge Cases & Latch Invalidation → Latch-set write failure", §"Affected Code Surface → Orchestrator".

## skip-bootstrap-when-warm-1-3 | approved

### Task skip-bootstrap-when-warm-1-3: Remove the CleanStale step from the orchestrator (11 → 10 steps)

**Problem**: The single-abridged-path contract forces hooks stale-cleanup out of the orchestrator entirely — a command-classified cleanup ("clean on `open`, not `attach`") is the rejected multi-variant design, and keeping it in the one abridged path would run it under the 20× `attach` reopen burst. `CleanStale` (former step 11) must be removed from the orchestrator — the step *and* its seam/adapter — dropping the orchestrator from 11 → 10 steps. It is re-homed on the daemon in Phase 3; this task only removes it from the orchestrator. `runHookStaleCleanup` (`cmd/run_hook_stale_cleanup.go`) is RETAINED (Phase 3 consumes it).

**Solution**: Delete the step-11 body, the `stepCleanStale` const, the `emitStep(11, …)` call, the `StaleCleaner` seam interface, the `Clean` orchestrator field, the `NoOpStaleCleaner` noop, the `cleanStaleAdapter` (and its `_ AllPaneLister` assertion + the production fallback branch), and the `orchestratorOpts.Clean` / `WithClean` / defaults plumbing. Change `totalSteps` from 11 to 10 and update all "eleven"→"ten" doc comments. The surviving marker sweep (step 9) and FIFO sweep (step 10) KEEP their indices — this is a drop-step-11, NOT a renumber. Retune the affected tests to a ten-step orchestrator that no longer runs / expects `CleanStale`.

**Outcome**: `Orchestrator.Run` executes ten steps, ending at `SweepOrphanFIFOs` (step 10) then the latch stamp (task 1-2) then the Return boundary; `totalSteps == 10`; the `orchestration complete` summary reports `steps=10`; no `StaleCleaner` seam, `Clean` field, `NoOpStaleCleaner`, `cleanStaleAdapter`, or `WithClean` option remains; `runHookStaleCleanup` and its test still exist untouched; the full suite is green.

**Do**:
- In `cmd/bootstrap/bootstrap.go`: delete the entire "Step 11 — CleanStale" block (the `Debug`/`stepStart`/`o.Clean.CleanStale()`/`Warn`/`Info`/`emitStep(11, …)` lines ~458-466); delete the `stepCleanStale = "CleanStale"` const (line ~78); delete the `StaleCleaner` interface (lines ~208-211); delete the `Clean StaleCleaner` field from the `Orchestrator` struct (line ~244); change `const totalSteps = 11` to `10` and update its doc comment ("eleven-step sequence" → "ten-step sequence"); update the package doc block (remove the "11. CleanStale" line and the "eleven-step" wording → "ten-step"); update the `Orchestrator`/`Run` godoc (the `steps=11` reference → `steps=10`, "eleven bootstrap steps" → "ten bootstrap steps", and the soft-warning bullet list — remove the "Step 11 (CleanStale)" bullet).
- In `cmd/bootstrap/noop.go`: delete `NoOpStaleCleaner` and its `CleanStale` method (lines ~84-89), and remove `StaleCleaner` from the package doc comment's enumerated degradable-step list.
- In `cmd/bootstrap/defaults.go`: delete the `WithClean` option (lines ~140-143), the `clean StaleCleaner` field from `defaultsConfig` (line ~76), the `if cfg.clean == nil { cfg.clean = NoOpStaleCleaner{} }` default (lines ~214-216), and the `Clean: cfg.clean` field from the returned `&Orchestrator{}` literal (line ~228); update the doc comment's degradable-step list (drop `Clean`).
- In `cmd/bootstrap/orchestrator_builder_test.go`: delete the `Clean bootstrap.StaleCleaner` field from `orchestratorOpts` (line ~44) and the `if opts.Clean != nil { withOpts = append(..., bootstrap.WithClean(opts.Clean)) }` block (lines ~93-95); update the doc comments that enumerate degradable steps (drop `Clean`).
- In `cmd/bootstrap_production.go`: delete the `cleanStaleAdapter` type + its `CleanStale` method (lines ~65-89), the `var _ AllPaneLister = (*tmux.Client)(nil)` assertion IF nothing else in the file needs it (grep first — it exists specifically for `cleanStaleAdapter.lister`; the daemon in Phase 3 will re-add its own; remove it here), the `var cleaner bootstrap.StaleCleaner … loadHookStore … NoOpStaleCleaner{}` fallback block in `buildProductionOrchestrator` (lines ~150-156), and the `Clean: cleaner` field from the `&bootstrap.Orchestrator{}` literal (line ~204); remove the now-unused `internal/hooks` import if `hooks.Store` is no longer referenced in this file (grep), and update the file-level doc block that mentions `cleanStaleAdapter` / the hook-store path chain.
- Retune `cmd/bootstrap/bootstrap_test.go`: this file has extensive `CleanStale` assertions — the `stepRecorder.CleanStale` method (~line 86), the expected-call ordering slices that include `"CleanStale"` (~lines 293, 441, 505, 608), the `o.Clean = r2` re-wire (~line 624), and the ordering tests `TestOrchestratorRun_runsSweepBetweenClearAndCleanStale` and siblings (~lines 842-1017) which assert `Clear < CleanStaleMarkers < Sweep < CleanStale`. Rewrite these so the terminal expected step is `SweepOrphanFIFOs` ("Sweep"), the `stepRecorder` no longer records/needs `CleanStale`, and the ordering tests assert `Clear < CleanStaleMarkers < Sweep` (dropping the `< CleanStale` tail). Rename the ordering tests if their names embed "CleanStale" as the terminal reference (e.g. `…runsSweepBetweenClearAnd…` → keep intent but retarget to Sweep-as-terminal). Any "CleanStale must still run after X fails" resilience tests are removed (the step no longer exists).
- Retune `cmd/bootstrap/defaults_test.go`: delete the `Clean` type assertion (lines ~87-88), the `stubStaleCleaner` type (~line 268-270), and the `WithClean(clean)` wiring + `o.Clean` assertions (lines ~127-162).
- Retune `cmd/bootstrap_production_test.go`: delete `TestCleanStaleAdapter_CleanStale` and any `cleanStaleAdapter` / `cleanStaleAdapterT` mirror references (the file header at lines ~4-9 documents these) — the adapter is gone. Preserve any test coverage of `runHookStaleCleanup` itself (that helper is retained); only the orchestrator-adapter test is removed.
- Grep the whole repo once more for `StaleCleaner`, `NoOpStaleCleaner`, `WithClean`, `cleanStaleAdapter`, `.Clean` (orchestrator field), and `stepCleanStale` to catch any remaining reference (e.g. `cmd/reattach_integration_test.go`, `cmd/state_commit_now_symptom_integration_test.go`, or other integration sites that construct orchestrators) and retune each.
- Do NOT touch `cmd/run_hook_stale_cleanup.go` or its test — retained for Phase 3. Do NOT touch `internal/hooks/store.go` `CleanStale` (the store method survives; it is `portal clean`'s and Phase 3's callee). Do NOT touch `internal/tui/loading_progress.go` — that is task 1-4.

**Acceptance Criteria**:
- [ ] `cmd/bootstrap/bootstrap.go` no longer contains a `StaleCleaner` interface, a `Clean` orchestrator field, a `stepCleanStale` const, an `o.Clean.CleanStale()` call, or an `emitStep(11, …)` call.
- [ ] `const totalSteps == 10`; the `orchestration complete` summary emits `steps=10`.
- [ ] The package doc block and all `Run`/`Orchestrator` godoc enumerate ten steps ("ten"), ending at `SweepOrphanFIFOs`; no "eleven" / "step 11" residue.
- [ ] `NoOpStaleCleaner` is gone from `cmd/bootstrap/noop.go`; `WithClean` / `clean` / `Clean:` are gone from `cmd/bootstrap/defaults.go`; `orchestratorOpts.Clean` / `WithClean` wiring is gone from `orchestrator_builder_test.go`.
- [ ] `cleanStaleAdapter`, its `CleanStale` method, its `_ AllPaneLister` assertion, and the `NoOpStaleCleaner` fallback branch are gone from `cmd/bootstrap_production.go`; the `Clean:` field is gone from the production orchestrator literal; any now-unused imports removed.
- [ ] `cmd/run_hook_stale_cleanup.go` and its test are unchanged and still compile; `internal/hooks` `CleanStale` store method is untouched.
- [ ] Surviving marker sweep and FIFO sweep keep step indices 9 and 10 (no renumber).
- [ ] `go build -o portal .` passes; `go test ./...` is fully green; `golangci-lint run` clean (no unused symbols, no dead imports).

**Tests**:
- Retuned `cmd/bootstrap/bootstrap_test.go` ordering/happy-path tests assert the terminal step is `SweepOrphanFIFOs` and that `CleanStale` is never invoked (the recorder has no such call).
- `"it runs exactly ten steps ending at SweepOrphanFIFOs"` — a full-sequence recording run asserts the ordered call list ends at the FIFO sweep with no `CleanStale`.
- `"it reports steps=10 in the orchestration-complete summary"` — with a `logtest.Sink` logger, assert the `orchestration complete` INFO record carries `steps=10`.
- Retuned `defaults_test.go` no longer references `Clean`/`stubStaleCleaner` and still passes its default-wiring assertions for the remaining nine degradable seams.
- A repo-wide `go test ./...` green run is the drift guard that no orphaned `StaleCleaner` reference survives.

**Edge Cases**: none (mechanical removal; the risk is missed references, mitigated by the whole-repo grep + `go build` + `go test ./...` + lint).

**Context**:
> Spec §"Scope": "**Hooks stale-cleanup (former step 11) is removed from the orchestrator entirely** and re-homed on the `_portal-saver` daemon (orchestrator drops from 11 → 10 steps)."
> Spec §"Daemon-Owned Hooks Cleanup → Decision": "**Former step 11 (`CleanStale` hooks):** **removed from the orchestrator entirely** — the step *and* its seam/adapter — taking the orchestrator from 11 → 10 steps. The `_portal-saver` daemon (`portal state daemon`) becomes its **sole automatic home**." Rationale for full removal (not just skipping): a bootstrap-time cleanup would only *uniquely* help when a full bootstrap runs AND EnsureSaver fails to start the daemon — already catastrophic — where an inert stale-hook entry is noise. Bonus: removes exposure to the `bootstrap-cleanstale-wipes-hooks-on-tmux-transient` bug.
> Spec §"Affected Code Surface → Orchestrator": "Remove the `CleanStale` step + its seam/adapter (11 → 10 steps). … also touches the **`totalSteps = 11` constant** (a documented 'load-bearing contract' feeding the `orchestration complete` summary's `steps` attr → set to `10`), the package doc comment enumerating the 'eleven-step' sequence, and the removed step's `emitStep(11, …)` call."
> Grounding: `cmd/run_hook_stale_cleanup.go` `runHookStaleCleanup` is RETAINED — Phase 3 re-homes it on the daemon; this task removes only the orchestrator's seam/adapter/step/plumbing. The `_ AllPaneLister` assertion in `bootstrap_production.go` exists solely for `cleanStaleAdapter.lister`; Phase 3 re-adds its own daemon-side wiring, so removing it here is correct.

**Spec Reference**: `.workflows/skip-bootstrap-when-warm/specification/skip-bootstrap-when-warm/specification.md` — §"Scope", §"The Two Bootstrap Paths → Step classification / The two paths", §"Daemon-Owned Hooks Cleanup → Decision", §"Affected Code Surface → Orchestrator".

## skip-bootstrap-when-warm-1-4 | approved

### Task skip-bootstrap-when-warm-1-4: Retune loading_progress.go to ten bootstrap steps

**Problem**: `internal/tui/loading_progress.go` independently encodes the bootstrap step count for the loading-screen progress bar and label mapping. With `CleanStale` (step 11) removed from the orchestrator (task 1-3), the loading bar's denominator (`totalBootstrapSteps = 11`) would top out at 10/11 ≈ 91% and never reach 100% on a successful full bootstrap, and the `stepLabelTable` would carry a phantom step-11 entry. This is a *separate* task from 1-3 because `internal/tui` must NOT import `cmd/bootstrap` — the two step-count constants drift independently, each with its own drift-guard test.

**Solution**: Drop the `11:` entry from `stepLabelTable` (keys stay contiguous 1..10 — a drop-key-11, NOT a renumber; step 9 and 10 keep their labels), change `totalBootstrapSteps` from 11 to 10 (the bar-fraction denominator, so the bar reaches exactly 100% after the tenth step), and update the doc comments referencing "eleven"/`1..11`/"11 real steps". Retune the drift-guard test file `internal/tui/loading_progress_test.go` — the `1..11` loop bounds → `1..10`, the `1/11`/`6/11` fractions → `/10`, the step-11 mapping case removed, and the `TestMappingCoversAllElevenStepsNoGaps` renamed/retuned to cover exactly 1..10.

**Outcome**: `totalBootstrapSteps == 10`; `stepLabelTable` has exactly keys 1..10 (no gaps, no key 11); the loading bar reaches exactly 1.0 after the tenth completed step; every retuned drift-guard test passes; no "eleven"/"11 real steps" residue in the file's comments.

**Do**:
- In `internal/tui/loading_progress.go`: remove the `11: LabelRunningResumeCommands, // CleanStale` entry from `stepLabelTable` (line ~95). Keep entries 1..10 exactly as they are (step 9 `CleanStaleMarkers` and step 10 `SweepOrphanFIFOs` retain their `LabelRunningResumeCommands` mapping) — this is a drop-key-11, not a renumber.
- Change `const totalBootstrapSteps = 11` to `10` (line ~41) and update its doc comment: "eleven increments" → "ten increments", and the drift-guard reference ("asserts the table covers exactly 1..11" → "1..10").
- Update the file-header doc comment (lines ~5-24): "11 real steps" / "11 internal steps" / "collapses the 11 real steps" / "BootstrapProgressMsg.Index (1..11)" → the ten-step wording (`1..10`). Update the `Apply`/`View`/`labelState` inline comments that say "eleven steps"/"11" (e.g. `View`'s "distinct completed steps)/11", `labelState`'s "once all eleven steps complete", `labelDone`'s loop bound already reads `idx <= totalBootstrapSteps` so it auto-follows the const — no numeric literal there to change, but verify).
- In `internal/tui/loading_progress_test.go`: retune every `11`/`/11` reference. Specifically: `TestStepMapsToFriendlyLabel` — delete the `"step 11 CleanStale"` case (line ~63); `TestBarAdvancesEveryStep` — change the `for step := 1; step <= 11` loop and the `float64(step) / 11.0` / `step < 11` / "after step 11" assertions to `10`; `TestLabelStateTransitions` — change the `for step := 6; step <= 11` loops (~lines 139, and the "After step 11 (the last real step)" comment) to `10`; `TestRestoringSessionsCounter` and `TestEmptyRestoreSuppressesCounterAndTicksDone` — change `5.0/11.0` and `6.0/11.0` fractions to `/10.0`; `TestRunningResumeCommandsTicksDoneWithNoItems` — change the step-11 done-tick to step-10 (the last real step is now 10; "Running resume commands" is done once steps 8-10 complete), retune the `for step := 6; step <= 10` frontier assertion accordingly (at step 9 it is active with step 10 pending; step 10 → done); `TestIdempotentPerStepIndex` — change `1.0/11.0` and `2.0/11.0` to `/10.0`; `TestMappingCoversAllElevenStepsNoGaps` — rename to `TestMappingCoversAllTenStepsNoGaps`, change the `for step := 1; step <= 11` loop to `<= 10`, keep the out-of-range guard for `{0, 12, 99}` but ensure `11` is now ALSO treated as out-of-range (add `11` to the out-of-range `[]int` or assert `LabelForStep(Index:11) == ""`).
- Update the test file's header comment (lines ~3-9) enumerating "11 real steps"/"11 increments, reaching 100% only after step 11" → ten-step wording.
- Confirm the build constraint holds: `internal/tui/loading_progress.go` still does NOT import `cmd/bootstrap` (it keys off the raw `BootstrapProgressMsg.Index`); no new import.

**Acceptance Criteria**:
- [ ] `totalBootstrapSteps == 10`.
- [ ] `stepLabelTable` has exactly keys `1..10` with no gaps and no key `11`; step 9 and step 10 retain their `LabelRunningResumeCommands` mapping (no renumber).
- [ ] `LabelForStep(BootstrapProgressMsg{Index: 11})` returns `""` (step 11 is now out of range / unmapped).
- [ ] After ten distinct completed step indices the bar fraction is exactly `1.0`; after nine it is `< 1.0`.
- [ ] The renamed drift-guard test asserts the table covers exactly `1..10` and passes; the out-of-range guard now also treats `11` as unmapped.
- [ ] No "eleven" / "11 real steps" / "1..11" residue in the file or its test comments.
- [ ] `go build -o portal .` passes; `go test ./internal/tui/...` is green; `golangci-lint run` clean.

**Tests** (retuned in `internal/tui/loading_progress_test.go`; NO `t.Parallel()`):
- `"it advances the bar to exactly 1.0 after the tenth step"` (retuned `TestBarAdvancesEveryStep`) — loop 1..10, each `float64(step)/10.0`, reaches 1.0 only after step 10.
- `"it maps every step 1..10 to a valid friendly label with no gaps"` (renamed `TestMappingCoversAllTenStepsNoGaps`) — loop 1..10 all map to a valid label; `{0, 11, 12, 99}` all map to `""` and advance no bar.
- `"it does not map removed step 11"` — `LabelForStep(BootstrapProgressMsg{Index: 11}) == ""` and feeding `Index: 11` leaves the bar at 0 (out-of-range, no phantom step).
- Retuned `TestRestoringSessionsCounter` / `TestEmptyRestoreSuppressesCounterAndTicksDone` assert the step-6 fractions against `/10` and the counter behaviour is otherwise unchanged.
- Retuned `TestRunningResumeCommandsTicksDoneWithNoItems` asserts "Running resume commands" (steps 8-10) ticks done once step 10 completes, with no per-item counter.

**Edge Cases**:
- Bar reaches exactly 100% after the tenth step (denominator = 10) — the load-bearing reason `totalBootstrapSteps` must move in lockstep with the removed step.
- Drift-guard table covers exactly 1..10 with no gaps and no phantom key 11 — a future step change cannot silently leave a step unmapped or leave a dangling key.
- `Index: 11` is now out-of-range: `LabelForStep`/`Apply` must return ""/no-op for it (defensive against a stale producer emitting the old index).

**Context**:
> Spec §"Affected Code Surface → Orchestrator → internal/tui/loading_progress.go": "two independent constants must move 11 → 10: **`stepLabelTable`** … removing `CleanStale` drops its `11:` table entry. Steps 9 and 10 keep their indices (only step 11 is removed), so the surviving keys stay contiguous at 1..10 — a drop-key-11, **not** a renumber; the drift-guard test's `1..N` assertion … retunes to 1..10. **`totalBootstrapSteps = 11`** is the **denominator of the loading-bar fraction** (`BarFraction = len(completedSteps) / totalBootstrapSteps`). It must become `10` or the bar tops out at 10/11 ≈ 91% and never reaches 100% on a successful full bootstrap. Verify the real-step→label mapping and the `N/M` counter (only on `Restoring sessions`) still hold at 10 steps."
> Build constraint (from `loading_progress.go` header): "It deliberately does NOT import cmd/bootstrap (wrong import direction / cycle risk — internal/tui must not depend on cmd). The mapping keys off the BootstrapProgressMsg.Index (1..11)." → update to `1..10`; the two step-count constants (this file's `totalBootstrapSteps` and `cmd/bootstrap`'s `totalSteps`) drift independently, each with its own guard — the reason task 1-4 is separate from task 1-3.
> Grounding: `labelDone` loops `for idx := 1; idx <= totalBootstrapSteps` so it auto-follows the const change; `restoreStep = 6` is unaffected; the step-6 RestoreM dual-mapping is unchanged. The only cleanup step that now folds under "Running resume commands" is `SweepOrphanFIFOs` (step 10) plus `ClearRestoring`/`CleanStaleMarkers` (steps 8/9) — the label grouping is otherwise intact.

**Spec Reference**: `.workflows/skip-bootstrap-when-warm/specification/skip-bootstrap-when-warm/specification.md` — §"Affected Code Surface → Orchestrator → internal/tui/loading_progress.go".
