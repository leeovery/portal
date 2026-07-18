---
status: in-progress
created: 2026-07-18
cycle: 1
phase: Traceability Review
topic: CLI Verb Surface Redesign
---

# Review Tracking: CLI Verb Surface Redesign - Traceability

Traceability analysis performed in both directions against the validated
specification (`.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md`),
the planning file, and all six phase task-detail files.

**Direction 1 (Spec → Plan, completeness):** Every governing decision, axiom,
resolution rule, flag, burst mechanic, maintenance verb, retirement, and
surface-presentation item in the spec was traced to at least one task with
matching acceptance criteria and depth. One gap was found (below).

**Direction 2 (Plan → Spec, fidelity / anti-hallucination):** Every task's
Problem / Solution / implementation detail / acceptance criteria / tests traces
back to a specific spec section. The several planner decisions where the spec
was silent (miss-message wording, absent-vs-corrupt `sessions.json` via
`ReadIndex`, empty `-f` → usage error, aggregated multi-target miss wording,
pin-fault aggregation) are each flagged inline in the task detail and are
intentional per the planning context — not re-flagged here. The two
user-resolved burst decisions (multi-target unsupported-terminal block; permission
wall still connects the trigger) and the deliberate scope retentions
(`internal/spawn`, `spawn` log component, `@portal-spawn-*` markers) are correctly
recorded and are not fidelity violations. No hallucinated content found.

## Findings

### 1. Missing daemon automatic stale-project pruning — `clean`'s promised replacement is only half-built

**Type**: Missing from plan
**Spec Reference**: § `doctor` — `clean` deleted ("Stale-project pruning folds into the daemon's automation on a slow cadence (hourly-ish; hooks already prune on the idle tick). … Net effect: `doctor` reads *healthy* almost always because the automation keeps it that way; `--fix` is the manual trigger of the same repairs."); § Command Surface Summary — Removed public commands (`portal clean [--logs]` → `portal doctor --fix` (repairs) **+ automatic daemon pruning**).
**Plan Reference**: Phase 4 (Tasks 4-5, 4-7) — no task covers the automatic-pruning half.
**Change Type**: add-task

**Details**:
The spec deletes `clean` on a two-part promise: (a) the manual half — `doctor --fix` performs the repairs (Task 4-5, covered); and (b) the automatic half — stale-**project** pruning folds into the `_portal-saver` daemon's throttled idle-tick automation, so gone-dir projects are cleaned continuously and `doctor` reads healthy without a manual `--fix`. The removed-commands table names this second half explicitly: "`portal clean [--logs]` → `portal doctor --fix` (repairs) **+ automatic daemon pruning**".

Stale-project pruning was `clean`'s *one genuinely-unique* job (per Task 4-5's own Problem statement and the spec's Rationale — the hook prune and log sweep were already redundant). Today the daemon prunes only stale **hooks** (`cmd/state_daemon.go`'s `maybeRunHookCleanup` → `runHookStaleCleanup`); it has never pruned projects. Task 4-7 deletes `clean` and confirms the daemon's *sole* remaining throttled cleanup is the hook path ("the sole remaining `runHookStaleCleanup` caller is the daemon's `maybeRunHookCleanup`"). Task 4-7 even cites "automatic daemon pruning" in its Context as part of the clean→replacement mapping, but no task builds it.

Consequence of the gap: after Phase 4, deleting `clean` removes the only mechanism that ever pruned stale projects outside a manual invocation. Gone-dir projects would accumulate and `doctor`'s stale-project check (Task 4-3) would trend toward failing until a user manually runs `doctor --fix` — directly contradicting the spec's design intent that "`doctor` reads *healthy* almost always because the automation keeps it that way." The plan implements the manual half (`--fix`) and the diagnosis (`doctor`), but not the automation the whole reshuffle rests on.

This is not one of the pre-cleared deferrals (which are `internal/spawn` / the `spawn` log component / `@portal-spawn-*` markers, plus the two inbox detection/TUI bugs). It is a stated behaviour with no task and no deferral note. Remedy: add a task to Phase 4 that extends the daemon's throttled automation to prune stale projects, reusing the same filesystem-only `project.Store.CleanStale()` classification `doctor --fix` and the doctor stale-project check already use. Cadence/mechanism is explicitly an implementation detail per the spec.

**Proposed**:

*Add the following row to `planning.md`'s Phase 4 `#### Tasks` table (after the `cli-verb-surface-redesign-4-7` row):*

```
| cli-verb-surface-redesign-4-8 | Fold stale-project pruning into the daemon's throttled automation | filesystem-only classification (gone-dir stale, permission-denied retained) mirroring `project.Store.CleanStale`; slow (hourly-ish) throttled cadence like the existing hook cleanup, not per-tick; best-effort / non-fatal to the capture loop; runs only inside the live `_portal-saver` pane so the down-server false-orphan hazard does not apply; no new log component (closed taxonomy) |
```

*Bump `phase-4-tasks.md` front-matter `total: 7` → `total: 8`, and append the following task to `phase-4-tasks.md`:*

```markdown
## cli-verb-surface-redesign-4-8 | approved

### Task 4.8: Fold stale-project pruning into the daemon's throttled automation

**Problem**: `clean`'s one genuinely-unique job was pruning stale (gone-dir) projects. The redesign deletes `clean` (Task 4-7) on the promise that stale-project pruning "folds into the daemon's automation on a slow cadence" so `doctor` reads healthy almost always and `doctor --fix` is only the manual trigger of the same repair. Today the `_portal-saver` daemon prunes stale hooks on its throttled idle tick (`maybeRunHookCleanup` → `runHookStaleCleanup`) but has never pruned stale projects. Deleting `clean` (Task 4-7) and delivering only the manual `doctor --fix` (Task 4-5) leaves the automatic half of `clean`'s replacement unbuilt — gone-dir projects would accumulate until a manual `--fix`, contradicting the spec's "the automation keeps it healthy" intent.

**Solution**: Extend the daemon's throttled idle-tick cleanup in `cmd/state_daemon.go`, alongside the existing `maybeRunHookCleanup`, to also prune stale projects via `project.Store.CleanStale()` — the same filesystem-only classification `doctor --fix` (Task 4-5) and the `doctor` stale-project check (Task 4-3) use — on a slow cadence (hourly-ish, mirroring the hook-cleanup throttle). This is the automatic-daemon-pruning half of `clean`'s replacement; the mechanism/cadence is an implementation detail per the spec.

**Outcome**: The `_portal-saver` daemon periodically prunes gone-dir projects (filesystem-only), so `doctor`'s stale-project check reads clean almost always without a manual `doctor --fix`; `doctor --fix` remains the manual trigger of the identical repair.

**Do**:
- In `cmd/state_daemon.go`, add a throttled stale-project prune next to the existing `maybeRunHookCleanup` idle-tick path. Reuse (or mirror) the existing hook-cleanup throttle interval so the project prune runs on a slow cadence rather than every 1s tick (cadence is an implementation detail — hourly-ish is fine).
- Load the project store the same way the daemon resolves its other config-backed stores and call `project.Store.CleanStale()` (filesystem-only: `os.Stat` → `ErrNotExist` is stale; permission-denied and other errors are retained — identical to Task 4-3/4-5).
- Keep the prune best-effort and non-fatal to the capture loop: a `CleanStale` error is logged and swallowed, exactly like the hook cleanup, and never aborts `captureAndCommit` / the daemon.
- Emit one INFO cycle breadcrumb per prune batch under the existing clean-stale log vocabulary (reuse `storelog.EmitCleanStaleSummary` / the project store's own mutation breadcrumb where applicable) — introduce NO new log component (the taxonomy is closed).
- The prune runs only inside the live `_portal-saver` pane (the daemon only exists on a running server), so the down-server false-orphan hazard that guards the *hook* prune does not apply to the filesystem-only project prune — no hazard guard is needed here.

**Acceptance Criteria**:
- [ ] The daemon prunes gone-dir projects (`os.Stat` → `ErrNotExist`) on its throttled idle-tick cadence via `project.Store.CleanStale()`; permission-denied paths are retained (not pruned).
- [ ] The prune runs on a slow cadence (not every tick), reusing/mirroring the existing hook-cleanup throttle.
- [ ] The prune is best-effort and non-fatal — a `CleanStale` error is logged and swallowed and never disrupts the capture/commit loop.
- [ ] After the daemon has run its cadence, `doctor`'s stale-project check (Task 4-3) reads clean without a manual `doctor --fix`.
- [ ] No new log component is introduced; the prune emits its breadcrumb under the existing clean-stale vocabulary.

**Tests**:
- `"the daemon prunes a gone-dir project on its throttled tick"` — seed a project at a deleted path; advance the daemon's throttle; assert the project is removed from `projects.json`.
- `"the daemon retains a permission-denied project path"` — a path whose stat errors non-`ErrNotExist` is not pruned.
- `"the project prune is throttled, not per-tick"` — assert the prune does not fire on every 1s tick (only on the slow cadence).
- `"a project-prune error is swallowed and the capture loop continues"` — force a `CleanStale`/store error; assert the daemon keeps ticking and `captureAndCommit` still runs.
- Integration-tagged (`//go:build integration`, `IsolateStateForTest`, `SpawnIsolatedDaemon`, `tmuxtest`, `portalbintest`): `"a real daemon prunes a genuinely stale project over its cadence and doctor then reads clean"`.

**Edge Cases**:
- Filesystem-only classification (gone-dir → stale; permission-denied → retained), matching `project.Store.CleanStale` and the Task 4-3/4-5 doctor semantics.
- Slow cadence (hourly-ish), throttled like the existing hook cleanup — cadence is an implementation detail.
- Best-effort / non-fatal — never disrupts capture.
- Runs only on a live server (inside the `_portal-saver` pane), so the hook prune's down-server hazard guard is not needed for the filesystem-only project prune.

**Context**:
> Spec § `clean` deleted: "**Stale-project pruning folds into the daemon's automation** on a slow cadence (hourly-ish; hooks already prune on the idle tick). Mechanism/cadence is an implementation detail. Net effect: `doctor` reads *healthy* almost always because the automation keeps it that way; `--fix` is the manual trigger of the same repairs." Spec § Command Surface Summary — Removed public commands: "`portal clean [--logs]` → `portal doctor --fix` (repairs) **+ automatic daemon pruning**."
>
> This task delivers the *automatic-daemon-pruning* half of `clean`'s replacement; Task 4-5 delivers the *manual* `doctor --fix` half and Task 4-7 deletes `clean`. It reuses `project.Store.CleanStale` (`internal/project/store.go`) — the exact filesystem-only classification the old `clean` and the new `doctor --fix` use — and hangs it on the daemon's existing throttled idle-tick path (`cmd/state_daemon.go`'s `maybeRunHookCleanup`), so no new algorithm is introduced.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` §§ `doctor` — Diagnostics & Repair (`clean` deleted); Command Surface Summary — Removed public commands.
```

**Resolution**: Pending
**Notes**: Companion edits are bundled in the Proposed content: (1) the new task row in `planning.md`'s Phase 4 task table, and (2) the `total: 7 → 8` front-matter bump plus the full task block appended to `phase-4-tasks.md`. Placed at the end of Phase 4 because it operates on the maintenance-surface reshuffle (reuses `project.Store.CleanStale` from Task 4-5 and pairs with the `clean` deletion in Task 4-7); it has no ordering dependency on Tasks 4-1..4-6 beyond sharing the `CleanStale` reuse.

---
