---
status: complete
created: 2026-04-27
cycle: 3
phase: Plan Integrity Review
topic: Built-In Session Resurrection
---

# Review Tracking: Built-In Session Resurrection - Integrity

## Findings

### 1. Task 3-3 carries an unresolved `[needs-info]` (live-index prediction approach)

**Severity**: Important
**Plan Reference**: `phase-3-tasks.md` task `built-in-session-resurrection-3-3` — Solution section and the final Acceptance Criterion bullet
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
Task 3-3's Solution explicitly carries a `[needs-info]` block stating "Planning has chosen Option A (prediction). User should confirm or override before implementation." The final Acceptance Criterion is `[ ] User has confirmed the "predict live indices via server-option read" approach (Option A) over the alternative spec-compatible approaches (Options B, C).` — i.e., the task cannot ship until the user resolves Option A vs B vs C.

This is a third unresolved `[needs-info]` in addition to the two intentional Phase 4 deferrals (4-4 argv source and Phase 4 acceptance #5). It blocks task 3-3 *and* shapes task 3-5 ("Under Option A, the prediction is correct in every realistic scenario; the re-query is belt-and-braces only" — different shape under Option B/C). An implementer following the canonical task template would not be able to start 3-3 without this resolved.

The user has two reasonable options:
- **Resolve it** — pick A, B, or C now and rewrite the Solution / Do / Acceptance to drop the `[needs-info]`. Option A (the planning author's working assumption) is the path of least change.
- **Acknowledge it as intentional** — promote the deferral to the same status as 4-4 / Phase 4 acceptance #5 and leave it as a known blocker. In that case, the acceptance bullet should explicitly say `[ ] [BLOCKED — needs planning decision on live-index source]` (matching task 4-4's shape) so its blocked status is unambiguous, and the rest of the body should not pretend the decision is made.

The proposed change below picks the second option (acknowledge as intentional) because it is the smaller edit; the user can substitute Option A's pinned wording if they prefer to resolve.

**Current**:
```
**[needs-info]**: Task 3-3's "predict live indices via `base-index` / `pane-base-index` server options" is a planning invention — the spec describes a re-query approach (line 324) but does not mandate prediction-before-creation. Prediction works in the common case but adds complexity (option reads, prediction-vs-live divergence handling in task 3-5). Alternative approaches the spec is compatible with:
- **Option B**: pass the saved-paneKey FIFO path to the helper at construction; after pane creation, re-query live paneKey and either (b1) symlink live → saved or (b2) have `signal-hydrate` use the saved paneKey from the index (NOT the live paneKey).
- **Option C**: decouple FIFO naming from paneKey — use a UUID stored in the index.

Planning has chosen Option A (prediction). User should confirm or override before implementation. If Option A stands, task 3-5's drift-detection becomes a defensive log-only branch; if any other Option is chosen, task 3-5 simplifies further.
```

(plus the trailing acceptance bullet:)

```
- [ ] User has confirmed the "predict live indices via server-option read" approach (Option A) over the alternative spec-compatible approaches (Options B, C). See `[needs-info]` in the Solution section.
```

**Proposed**:
```
**[needs-info]**: Task 3-3's "predict live indices via `base-index` / `pane-base-index` server options" is a planning invention — the spec describes a re-query approach but does not mandate prediction-before-creation. The spec is compatible with three approaches:
- **Option A** (planning's working assumption): predict via `base-index` / `pane-base-index` server options before pane creation; FIFO + hydrate command use the predicted live paneKey; task 3-5 is the defensive re-alignment point.
- **Option B**: pass the saved-paneKey FIFO path to the helper; after pane creation, re-query live paneKey and either (b1) symlink live → saved or (b2) have `signal-hydrate` consult the saved paneKey from the index.
- **Option C**: decouple FIFO naming from paneKey — use a UUID stored in the index.

This decision is BLOCKED on user confirmation. Until pinned, task 3-3 (this task) and task 3-5 (drift detection vs. authoritative re-query) cannot be implemented. The Do / Acceptance / Tests sections below describe Option A; if the user picks B or C, those sections are rewritten.
```

(replace the trailing acceptance bullet with:)

```
- [ ] [BLOCKED — needs planning decision on live-index source] User pins Option A, B, or C; subsequent implementation steps are rewritten to match the chosen route.
```

**Resolution**: Fixed
**Notes**:

---

### 2. Phase 6 has no task that deletes the legacy `bootstrap.NewShim`

**Severity**: Minor
**Plan Reference**: `phase-5-tasks.md` task `built-in-session-resurrection-5-3` (introduces the shim) and `phase-6-tasks.md` (no task removes it)
**Category**: Phase Structure / Scope and Granularity
**Change Type**: add-task

**Details**:
Task 5-3 introduces `bootstrap.NewShim(ServerBootstrapper) Runner` as a transitional adapter so existing cmd-package tests that injected a bare `ServerBootstrapper` keep passing during the Phase 5 cutover. The task explicitly states: `Mark the shim as // Deprecated: scheduled for removal in Phase 6 after every cmd-package test migrates to the full Orchestrator seam.` and adds `TODO(phase-6): delete after legacy bootstrappers removed` markers.

Phase 6 currently contains 11 tasks (6-1 through 6-11) covering the structured logger, log retrofit, log rotation, `portal state status`, daemon-kill in cleanup, `--purge`, fatal/soft bootstrap warnings, TUI buffering, and README updates. None of them delete the shim or migrate the remaining `BootstrapDeps.Bootstrapper` callers. As the plan stands, the shim will ship in v1 carrying its `Deprecated` marker forever.

This is a small, mechanical follow-up but it is the kind of TODO that quietly accumulates if not tracked. Add a short Phase 6 task (proposed numbering: `built-in-session-resurrection-6-12`) that performs the migration and deletion. Expect this task to be low-risk and ~30 minutes of mechanical edits, not a full TDD cycle on its own — but explicitly captured in the plan it cannot be forgotten.

**Current**: (no current Phase 6 task addresses shim removal)

**Proposed** (add as new task to `phase-6-tasks.md`; also update the Phase 6 task table in `planning.md` to add a corresponding row, and bump `phase-6-tasks.md` frontmatter `total: 11` → `total: 12`):

```markdown
## built-in-session-resurrection-6-12 | approved

### Task 6-12: Delete legacy `bootstrap.NewShim` and the `BootstrapDeps.Bootstrapper` field

**Problem**: Phase 5 task 5-3 introduced `cmd/bootstrap/shim.go`'s `NewShim(ServerBootstrapper) Runner` and the legacy `BootstrapDeps.Bootstrapper` field as a transitional adapter so existing cmd-package tests that pre-dated the orchestrator continued to compile during the Phase 5 cutover. Both carry `// Deprecated:` markers and `TODO(phase-6): delete after legacy bootstrappers removed` comments. With every Phase 5 / Phase 6 task landed, every cmd-package test now has a path to inject the real `*bootstrap.Orchestrator` (via `BootstrapDeps.Orchestrator` from task 5-3). Leaving the shim shipping in v1 means the deprecated surface lingers indefinitely.

**Solution**: Audit every test that still constructs `BootstrapDeps{Bootstrapper: ...}` (legacy shape), migrate it to `BootstrapDeps{Orchestrator: ...}` with a recording / fake `bootstrap.Runner`, then delete `cmd/bootstrap/shim.go`, `cmd/bootstrap/shim_test.go`, and the `BootstrapDeps.Bootstrapper` field. Run `grep -R 'NewShim\|Bootstrapper' cmd/ internal/` to verify zero residual references after the migration. Expected impact is small — task 5-3 introduced the shim specifically to bound this migration to a Phase 6 follow-up.

**Outcome**: `cmd/bootstrap/shim.go` and its test no longer exist; `BootstrapDeps` carries only `Orchestrator` and `Client`. Every cmd-package test that exercises bootstrap injects a `bootstrap.Runner` (the canonical seam). `go build ./...` and `go test ./...` pass. Repo-wide `grep` for `NewShim` returns zero matches.

**Do**:
- Run `rg -n 'BootstrapDeps\{[^}]*Bootstrapper' cmd/` to enumerate the legacy-shape test fixtures introduced before Phase 5.
- For each legacy fixture: replace `Bootstrapper: <fakeServer>` with `Orchestrator: <fakeRunner>`, where `<fakeRunner>` implements `bootstrap.Runner` with the same effective semantics (typically a recording stub that returns the same `(serverStarted, warnings, err)` triple the test previously asserted).
- Delete `cmd/bootstrap/shim.go` and `cmd/bootstrap/shim_test.go`.
- Delete the `Bootstrapper ServerBootstrapper` field from `cmd/root.go`'s `BootstrapDeps` struct.
- Delete the legacy-shim branch in `buildBootstrapDeps()` (the `if bootstrapDeps.Orchestrator != nil { ... } else { /* shim path */ }` shape collapses to just the orchestrator path).
- Decide whether to also delete `ServerBootstrapper` itself: if no caller outside the deleted shim references it, delete; otherwise keep with its current callers.
- Run `go build ./...` and `go test ./...`; fix any residual failures by migrating the call site to the orchestrator seam.
- Final `grep -R 'NewShim\|legacy' cmd/ internal/` should return zero matches in resurrection code.

**Acceptance Criteria**:
- [ ] `cmd/bootstrap/shim.go` is deleted.
- [ ] `cmd/bootstrap/shim_test.go` is deleted.
- [ ] `BootstrapDeps.Bootstrapper` field is removed from `cmd/root.go`.
- [ ] `buildBootstrapDeps()` no longer has a shim-fallback branch — returns the orchestrator from `BootstrapDeps.Orchestrator` (or a freshly-constructed one) unconditionally.
- [ ] Every cmd-package test that previously injected `BootstrapDeps{Bootstrapper: ...}` now injects `BootstrapDeps{Orchestrator: ...}`.
- [ ] `ServerBootstrapper` interface is either deleted (if no callers remain) or retained with a comment explaining the surviving caller.
- [ ] `go build ./...` and `go test ./...` pass with zero failures.
- [ ] Repo-wide `grep` for `NewShim` returns zero matches.

**Tests**:
- `"go build ./... succeeds with zero references to NewShim"`
- `"go test ./... passes after every legacy fixture is migrated to the Orchestrator seam"`
- `"buildBootstrapDeps returns an Orchestrator (never a shim) for every BootstrapDeps shape"`

**Edge Cases**:
- A test using the shim only because its assertion was scoped to `EnsureServer` (no hook registration / restore expected): the migrated fake `bootstrap.Runner` returns `(serverStarted, nil, nil)` — equivalent semantics, still satisfies the test's pre-shim assertion.
- A test that asserted the old shim's "no hook registration" property: migrate the assertion to "the injected fake `Runner.Run` returns `(true, nil, nil)`" — same effective contract, asserted at the orchestrator boundary instead of at the shim.
- If `ServerBootstrapper` has callers outside the shim path (e.g., another command or test fixture references it independently), retain it with a comment naming those callers; otherwise delete.
- This task is purely mechanical — no new behaviour, no new tests beyond the migration. Treat the build + test pass as the sole signal of completion.

**Context**:
> Task 5-3's shim guidance (Edge Cases section): "`BootstrapDeps.Bootstrapper` + `bootstrap.NewShim` is a bridge, not a permanent API. Phase 6 deletes the shim once every cmd test is migrated to the full `Orchestrator` seam. Marked in the field's godoc comment."
>
> Task 5-3's Acceptance: "`bootstrap.NewShim(ServerBootstrapper) Runner` exists in `cmd/bootstrap/shim.go` and is documented as deprecated."
>
> This task is the deletion-half of the Phase-5 transitional bridge. Expected to be ~30 minutes of mechanical edits; carried as a distinct task so it is not lost as a TODO comment that survives v1.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — no direct reference (this is plan-internal cleanup of a transitional adapter introduced in task 5-3).
```

(also update `planning.md` Phase 6 task table — add row at end:)

```markdown
| built-in-session-resurrection-6-12 | Delete legacy `bootstrap.NewShim` and `BootstrapDeps.Bootstrapper` field | every legacy-shape test migrated, `ServerBootstrapper` deleted iff no remaining callers, `go build` clean after `grep` returns zero `NewShim` matches |
```

**Resolution**: Fixed
**Notes**: Tick task created as tick-9c172c (parent tick-27d572, blocked-by tick-b75737). Manifest task_map updated. phase-6-tasks.md frontmatter total bumped 11 → 12.

---
