---
status: in-progress
created: 2026-06-01
cycle: 1
phase: Traceability Review
topic: portal-observability-layer
---

# Review Tracking: portal-observability-layer - Traceability

Traceability analysis of the plan (6 phases / 51 tasks across `planning.md` + `phase-1..6-tasks.md`) against the validated specification (14 sections). Both directions checked: (1) Specification → Plan completeness, (2) Plan → Specification fidelity (anti-hallucination).

## Summary

Direction 2 (anti-hallucination) is clean: every task traces to a named spec section, and the plan is notably disciplined about *not* inventing scope — genuine spec/code mismatches are flagged `[needs-info]` (e.g. `write-failed-fsync` having no AtomicWrite step, the alias store's `os.WriteFile`-not-`AtomicWrite` shape, the single-batched-`Save` per-entry-WARN unreachability in hooks/projects `CleanStale`, the daemon `shutdown reason` signal-capture requirement, the hydrate `fifo missing` row collapsing under timeout, the bare-window-index attr having no closed key) rather than resolved by invention. Those `[needs-info]` flags are intentional and are not traceability defects.

Direction 1 (completeness) surfaced ONE gap: the closed-taxonomy component `signal` (one of the 15 components the spec calls "the single source of truth for the component count") has no plan coverage — Phase 1 explicitly defers it to "where Phase 5/6 promote them," but no later phase performs that promotion (whereas the sibling deferred components `capture` and `saver` ARE promoted, in tasks 5-1 and 5-7/5-8). See Finding 1.

## Findings

### 1. `signal` component never homed — `EagerSignalHydrate` + FIFO signal plumbing left mis-attributed under `hydrate`

**Type**: Missing from plan
**Spec Reference**: § Subsystem prefix taxonomy → Closed component value space (15 total) + component-ownership table: "`signal` | FIFO signaling **mechanism** — `EagerSignalHydrate` and the lower-level FIFO signal send/receive plumbing in `internal/state`. (The hydrate helper's own exit-path outcome lines — incl. `signal timeout` — render under `hydrate` per the Hook-firing catalog, which governs the helper's exec-chain.)" Also § Subsystem prefix taxonomy → Rendering mechanism: "`grep "hydrate:" portal.log` produces the per-subsystem audit trail" — the same per-prefix grep guarantee applies to every closed component, including `signal`.
**Plan Reference**: Phase 1 task 1-9 (`phase-1-tasks.md:446` and `:481`) explicitly defers `signal`: "(`capture`/`saver`/`signal`/etc. are introduced where Phase 5/6 promote them … Do NOT pre-introduce `capture`/`saver`/`signal`.)" — but no Phase 5 or Phase 6 task introduces `signal`. (`capture` is introduced by task 5-1; `saver` by tasks 5-7/5-8.) No task re-homes `EagerSignalHydrate`'s WARN to `signal`, and no task instruments the lower-level FIFO signal send/receive plumbing in `internal/state` under `signal`.
**Change Type**: add-task

**Details**:
The spec's component table is the closed, single-source-of-truth taxonomy. It assigns the `signal` component ownership of two concrete code areas: (a) `EagerSignalHydrate` (bootstrap step 7, `cmd/bootstrap/eager_signal_hydrate.go`), and (b) "the lower-level FIFO signal send/receive plumbing in `internal/state`" (`internal/state/signal_hydrate.go` — `WriteFIFOSignal` / `SendHydrateSignal` / `OpenFIFOForSignal` / `DefaultFIFOSignaler`).

Today `EagerSignalHydrate` emits its per-FIFO write-failure WARN under `state.ComponentHydrate` (i.e. the `hydrate` component) — verified in `cmd/bootstrap/eager_signal_hydrate.go:96` (`logger.Warn(state.ComponentHydrate, "eager-signal: write fifo %s: %v", fifoPath, err)`). Phase 1 task 1-9 migrates `ComponentHydrate → hydrate` wholesale, so after Phase 1 this WARN renders under `hydrate:`. Because no later phase re-homes it, the line stays permanently under `hydrate` — contradicting the spec, which carves the signaling *mechanism* out to `signal` precisely so it is distinguishable from the hydrate helper's own exit-path lines (which legitimately stay under `hydrate`).

Consequences of the gap:
- `grep "signal:" portal.log` — a per-prefix reconstruction the closed taxonomy promises for every component — produces nothing.
- The `EagerSignalHydrate` write-failure WARN is mis-attributed under `hydrate`, conflating the bootstrap-side FIFO-signal-send mechanism with the per-pane hydrate-helper exec-chain the spec deliberately keeps separate.
- The 15-component closed taxonomy (the spec's stated component count) is effectively 14 in the delivered plan; the spec's "every contributor consults these lists; no ad-hoc invention" extension policy presumes each listed component is actually wired.

The natural home for the fix is Phase 5 (which already owns the `capture`/`saver` component promotions and the cycle/lifecycle instrumentation of the long-running machinery). The new task re-homes the existing `EagerSignalHydrate` WARN to `signal` and applies the § Call-site logging pattern mechanical rule (DEBUG breadcrumbs at meaningful transitions, INFO terminal point, WARN per recoverable error path) to the `signal`-owned code so the component is genuinely populated. No new attr keys are introduced — only existing closed-vocabulary attrs (`path`, `error`, `error_class`, `took`, plus the cycle-summary attrs already permitted).

Note: the EagerSignalHydrate **cycle summary** is NOT mandated by the § Cycle-level summary "Concrete cycle catalog" (which does not list a signal sweep), so this task does not add a cycle-summary line; it homes the component per the call-site/level-discipline pattern and the taxonomy ownership. The bootstrap step-7 (`EagerSignalHydrate`) `bootstrap: step complete` summary is owned by task 5-2 and is unaffected.

**Proposed**:

Add the following task to Phase 5 (`phase-5-tasks.md`), and add a corresponding row to the Phase 5 task table in `planning.md`. Suggested internal ID `portal-observability-layer-5-11` (appended after the existing 5-10; adjust the phase `total` from 10 to 11).

Planning.md Phase 5 task-table row to add:

```
| portal-observability-layer-5-11 | Home the `signal` component — re-attribute `EagerSignalHydrate`'s write-failure WARN and instrument the `internal/state` FIFO signal send/receive plumbing under `signal` | EagerSignalHydrate WARN moves from `hydrate` to `signal`, FIFO signal-send retry-ladder breadcrumbs DEBUG, write-failure WARN error_class=unexpected, lower-level `internal/state` signal plumbing currently takes no logger (signature/seam change needed — `[needs-info]`), no cycle-summary mandated (not in the Concrete cycle catalog) |
```

Phase 5 acceptance-criteria addition (append to the Phase 5 `**Acceptance**:` block in `planning.md`):

```
- [ ] The `signal` component is populated: `EagerSignalHydrate`'s per-FIFO write-failure WARN renders under `signal:` (not `hydrate:`), and the `internal/state` FIFO signal send/receive plumbing logs under `signal` per the call-site/level-discipline pattern, so `grep "signal:" portal.log` reconstructs the signaling mechanism's behaviour
```

Full task body for `phase-5-tasks.md` (append after task 5-10, and bump the front-matter `total: 10` to `total: 11`):

```markdown
## portal-observability-layer-5-11 | approved

### Task 5-11: Home the `signal` component — re-attribute `EagerSignalHydrate`'s WARN and instrument the `internal/state` FIFO signal plumbing under `signal`

**Problem**: The closed subsystem taxonomy defines `signal` as the component owning "the FIFO signaling **mechanism** — `EagerSignalHydrate` and the lower-level FIFO signal send/receive plumbing in `internal/state`." Phase 1 deliberately did NOT pre-introduce `signal` (it kept `EagerSignalHydrate`'s existing write-failure WARN under `hydrate`/`ComponentHydrate`, promising the component would be homed "where Phase 5/6 promote them"). The sibling deferred components `capture` and `saver` are promoted in tasks 5-1 and 5-7/5-8, but `signal` is never homed — so `EagerSignalHydrate`'s WARN stays mis-attributed under `hydrate` and `grep "signal:" portal.log` produces nothing, leaving one of the 15 closed components un-wired.

**Solution**: Home the `signal` component. (1) Re-attribute `EagerSignalHydrate`'s per-FIFO write-failure WARN from `hydrate` to `signal` by binding `var logger = log.For("signal")` in `cmd/bootstrap/eager_signal_hydrate.go` (replacing the Phase-1-migrated `hydrate` binding for these lines). (2) Apply the § Call-site logging pattern mechanical rule to the lower-level `internal/state` FIFO signal send/receive plumbing (`signal_hydrate.go` — `WriteFIFOSignal` / `SendHydrateSignal`): a DEBUG breadcrumb on the retry-ladder transitions and a WARN on the recoverable write-failure path, all under `signal`. The hydrate helper's own exit-path lines (incl. `signal timeout`) stay under `hydrate` per the Hook-firing catalog (Phase 6) — this task touches only the signaling *mechanism*, not the helper's exec-chain.

**Outcome**: `EagerSignalHydrate`'s write-failure WARN renders under `signal:` (not `hydrate:`); the `internal/state` FIFO signal send/receive plumbing emits its breadcrumbs/WARN under `signal`; `grep "signal:" portal.log` reconstructs the FIFO-signaling mechanism's behaviour; the 15-component closed taxonomy is fully wired.

**Do**:
- In `cmd/bootstrap/eager_signal_hydrate.go`, add `import "github.com/leeovery/portal/internal/log"` and bind a package-level `var logger = log.For("signal")` (component literal `signal` per the closed taxonomy). Re-point the existing per-FIFO write-failure WARN (currently `logger.Warn(state.ComponentHydrate, "eager-signal: write fifo %s: %v", fifoPath, err)`, Phase-1-migrated to a `hydrate` slog WARN) to the `signal`-bound logger: `logger.Warn("eager-signal write fifo failed", "path", fifoPath, "error", err, "error_class", "unexpected")`. Use only closed-vocabulary attrs (`path`, `error`, `error_class`). Per the level-discipline table, a write-failure that leaves a pane's helper un-signalled drops a unit of work → WARN with `error_class="unexpected"`. Pass the wrapped `err` directly (not `.Error()`) per the Phase-4 convention.
- Per the § Call-site logging pattern mechanical rule (DEBUG breadcrumb at each meaningful transition; WARN per recoverable error path), add a DEBUG breadcrumb in `EagerSignalHydrate`'s per-FIFO loop on the successful-signal path (e.g. `logger.Debug("fifo signalled", "path", fifoPath)`) so the per-pane signalling is reconstructible at DEBUG; the WARN above is the anomaly line. Do NOT add a cycle-summary INFO — `EagerSignalHydrate` is bootstrap step 7 and its `bootstrap: step complete` summary is owned by task 5-2; the § Cycle-level summary "Concrete cycle catalog" lists no separate signal-sweep summary, so none is mandated here.
- Instrument the lower-level FIFO signal send/receive plumbing in `internal/state/signal_hydrate.go`. `WriteFIFOSignal` / `SendHydrateSignal` currently take NO logger (they return errors the caller logs). `[needs-info]`: homing these under `signal` requires either (a) binding a package-level `var logger = log.For("signal")` in `internal/state` and emitting the breadcrumb/WARN inside `WriteFIFOSignal`/`SendHydrateSignal` directly (matching the model-observer seam used for the stores), or (b) leaving them error-returning and relying on the caller (`EagerSignalHydrate`) to emit under `signal` — in which case the lower-level plumbing carries only a DEBUG retry-ladder breadcrumb if any. Resolve and document the chosen seam; prefer (a) for the retry-ladder DEBUG breadcrumb (the retryable-FIFO-error retry decision in `isRetryableFIFOError`/`SendHydrateSignal` is a meaningful transition worth a DEBUG line under `signal`), keeping the whole-operation WARN at the `EagerSignalHydrate` call site so it carries the `path`. Confirm the import-cycle guard holds (`internal/state` may import `internal/log`; `internal/log` must not import `internal/state` — Task 1-8's invariant).
- Confirm no behaviour change beyond component re-attribution + additive breadcrumbs: the retry ladder, the FIFO open/write semantics, the orchestrator's Warn-and-swallow of `EagerSignalHydrate`'s return, and the bootstrap step-7 control flow are all unchanged.
- Update any test that asserts the `EagerSignalHydrate` write-failure WARN renders under `hydrate` to expect `signal` instead.

**Acceptance Criteria**:
- [ ] `EagerSignalHydrate`'s per-FIFO write-failure WARN renders under component `signal` (prefix `signal:`), NOT `hydrate`, with `path`/`error`/`error_class=unexpected` attrs and the wrapped error passed directly.
- [ ] A per-FIFO successful signal emits a DEBUG breadcrumb under `signal` (silent at production INFO, present at DEBUG).
- [ ] The lower-level `internal/state` FIFO signal send/receive plumbing logs its retry-ladder breadcrumb / write-failure under `signal` per the resolved seam (the `[needs-info]` choice), with no `internal/log → internal/state` import cycle.
- [ ] No `bootstrap`/`hydrate`-prefixed line is emitted for the signaling mechanism that the spec assigns to `signal`; `grep "signal:" portal.log` reconstructs the FIFO-signaling behaviour.
- [ ] No new attr keys are introduced (only closed-vocabulary attrs); no cycle-summary INFO is added (none is mandated by the Concrete cycle catalog).
- [ ] The retry ladder, FIFO open/write semantics, and bootstrap step-7 control flow are behaviourally unchanged; tests asserting the old `hydrate` attribution are updated to `signal`.

**Tests**:
- `"it emits the eager-signal write-failure WARN under component=signal with path and error_class=unexpected"`
- `"it emits a per-FIFO signalled DEBUG breadcrumb under signal (filtered at INFO, present at DEBUG)"`
- `"the internal/state FIFO signal plumbing logs its retry/write breadcrumb under signal"` (per the resolved seam)
- `"no signaling-mechanism line renders under hydrate or bootstrap after the re-attribution"`
- `"it adds no new attr keys and no cycle-summary line"`

**Edge Cases**:
- EagerSignalHydrate write-failure WARN moves from `hydrate` to `signal` (component re-attribution, not a new line).
- The hydrate helper's own exit-path lines (incl. `signal timeout`, Phase 6) stay under `hydrate` — this task touches only the signaling mechanism, never the helper exec-chain.
- Lower-level `internal/state` signal plumbing currently takes no logger — seam/signature change needed (`[needs-info]`: emit inside the plumbing vs at the EagerSignalHydrate caller); resolve and document, preserve the import-cycle guard.
- No cycle-summary mandated (the signal sweep is not in the Concrete cycle catalog); only the call-site/level-discipline pattern applies.
- `error_class="unexpected"` per the level table (an un-signalled pane drops a unit of work).

**Context**:
> "`signal` | FIFO signaling **mechanism** — `EagerSignalHydrate` and the lower-level FIFO signal send/receive plumbing in `internal/state`. (The hydrate helper's own exit-path outcome lines — incl. `signal timeout` — render under `hydrate` per the Hook-firing catalog, which governs the helper's exec-chain.)" (spec § Subsystem prefix taxonomy → component-ownership table)
>
> "This list is the **single source of truth for the component count.**" — the closed 15-component space; every component is consulted by contributors and presumed wired. (spec § Subsystem prefix taxonomy → Closed component value space)
>
> "Grep idiom preserved: `grep "hydrate:" portal.log` produces the per-subsystem audit trail." — the same per-prefix reconstruction is the point of every closed component, including `signal`. (spec § Subsystem prefix taxonomy → Rendering mechanism)
>
> Mechanical rule (per function authored or amended): DEBUG breadcrumbs at each meaningful state transition; WARN per recoverable error path with `error_class`. (spec § Call-site logging pattern → Mechanical rule)
>
> Current code: `EagerSignalHydrate` (`cmd/bootstrap/eager_signal_hydrate.go:96`) emits `logger.Warn(state.ComponentHydrate, "eager-signal: write fifo %s: %v", fifoPath, err)` — under `hydrate`, not `signal`. The lower-level plumbing (`internal/state/signal_hydrate.go`: `WriteFIFOSignal`, `SendHydrateSignal`, `OpenFIFOForSignal`, `DefaultFIFOSignaler`, `isRetryableFIFOError`) takes no logger and returns errors to the caller. Phase 1 task 1-9 explicitly deferred `signal` to Phase 5/6 promotion.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Subsystem prefix taxonomy (Closed component value space — `signal` ownership, single-source-of-truth, Rendering mechanism), § Call-site logging pattern (Mechanical rule); planning Phase 1 task 1-9 `signal` deferral
```

**Resolution**: Pending
**Notes**: This is the only completeness gap found. It is the direct consequence of Phase 1's explicit "introduced where Phase 5/6 promote them" deferral for `signal` not being honoured by any later phase (unlike the `capture` and `saver` deferrals, which task 5-1 and tasks 5-7/5-8 honour). The proposed task adds no new vocabulary and no new cycle summary — it only homes an existing line to its spec-assigned component and applies the standard call-site pattern to the `signal`-owned plumbing, so it stays within the spec's validated scope. If the reviewer/user judges the `signal` ownership to be satisfied by leaving `EagerSignalHydrate` under `hydrate` (i.e. treats the taxonomy row as advisory rather than binding), this finding can be closed as "won't fix" with that rationale recorded — but the spec's "single source of truth for the component count" framing argues for wiring it.

---
