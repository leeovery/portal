---
status: in-progress
created: 2026-06-01
cycle: 1
phase: Plan Integrity Review
topic: portal-observability-layer
---

# Review Tracking: portal-observability-layer - Integrity

Standalone-document integrity review of the plan (6 phases / 52 tasks across `planning.md` + `phase-1..6-tasks.md`, mirrored in tick topic `tick-7ac1a9`). Checked: task-template compliance, vertical slicing, phase structure, dependencies/ordering, self-containment, scope/granularity, acceptance-criteria quality.

## Summary

The plan is implementation-ready and of high structural quality. Every task carries the full canonical template (Problem / Solution / Outcome / Do / Acceptance Criteria / Tests / Edge Cases / Context / Spec Reference). Acceptance criteria are concrete and pass/fail; Tests enumerate edge cases, not just happy paths; tasks are vertical slices each verifiable as a single TDD cycle; phase progression is logical (foundation → rotation/lifecycle → config audit trail → boundary preservation → cycle/lifecycle catalogs → hydrate trail). The deliberate `[needs-info]` flags are intentional implementer-decision markers grounded in the live code (per the planning brief) and are NOT integrity defects.

Two findings, both below Critical:

- **Finding 1 (Important)**: Task 5-8 hedges shared ownership of the package-level `saverLogger` var with Task 5-7 ("or add it if 5-7 is not yet applied"), creating a duplicate-declaration hazard at an intra-phase convergence point that lacks an explicit dependency edge.
- **Finding 2 (Minor)**: A handful of Do-sections exceed the "≤5 concrete steps" scope signal (1-8, 3-4, 3-5, 2-15); each remains a single coherent TDD cycle, so this is a readability note, not a split mandate.

No circular dependencies, orphaned tasks, duplicate tasks, or incoherent phase boundaries were found. Cross-phase predecessor relationships (e.g. Phase-2 markers filling Phase-1 seams, 5-8 sitting above 4-4's breadcrumb, 5-10 not emitting the 5-9 shutdown line) are each self-documented in the dependent task body, so natural creation-date ordering produces correct execution order without explicit edges — these are correctly NOT flagged.

## Findings

### 1. Task 5-8 hedges `saverLogger` ownership with Task 5-7 — duplicate-declaration hazard at an intra-phase convergence point

**Severity**: Important
**Plan Reference**: `phase-5-tasks.md` Task 5-8 (`portal-observability-layer-5-8`), Do-section first bullet (line ~422)
**Category**: Dependencies and Ordering (intra-phase convergence point lacking an explicit edge)
**Change Type**: update-task

**Details**:
Task 5-7 unconditionally creates the shared package-level logger: "Add a package-level `var saverLogger = log.For("saver")` in `internal/tmux/portal_saver.go`". Task 5-8 then says: "Use the `saverLogger = log.For("saver")` from task 5-7 (**or add it if 5-7 is not yet applied**; either way component `saver`)."

Both tasks touch the same file (`internal/tmux/portal_saver.go`) and both 5-7's `placeholder created`/`respawn-daemon`/`daemon ready` events and 5-8's `kill-barrier started`/`escalated`/`placeholder died` events live there. The hedge "or add it if 5-7 is not yet applied" instructs the implementer to add the `var saverLogger` declaration inside 5-8 when 5-7 is not yet done. But in the normal execution order (natural creation-date order runs 5-7 before 5-8), 5-7 has already declared the var — so following 5-8's "add it" branch produces a duplicate `var saverLogger` declaration and a compile error.

The correct structural relationship is: **5-8 depends on 5-7** for the shared `saverLogger` var. The plan should either (a) make 5-7 the unconditional owner of the var and have 5-8 simply use it (declaring the dependency on 5-7), or (b) move the var declaration into a foundation step both consume. Option (a) is the minimal, lowest-churn fix and matches the natural execution order. This is a genuine convergence point (5-8 needs a capability 5-7 produces) that the criterion calls out as a flaggable dependency issue rather than ordinary sequential intra-phase flow.

**Current**:
> - Use the `saverLogger = log.For("saver")` from task 5-7 (or add it if 5-7 is not yet applied; either way component `saver`).

**Proposed**:
> - Use the package-level `var saverLogger = log.For("saver")` introduced by task 5-7 in `internal/tmux/portal_saver.go` (component `saver` per the closed taxonomy). This task depends on 5-7 for that shared logger var — do NOT re-declare it here (a second `var saverLogger` in the same file is a duplicate-declaration compile error). Tasks 5-7 and 5-8 land in dependency order (5-7 first); 5-7 owns the var declaration, 5-8 only references it. If for any reason 5-8 is implemented in isolation ahead of 5-7, add the var to 5-7's scope first rather than duplicating it here.

**Resolution**: Pending
**Notes**: Self-contained, low-risk fix — clarifies an ownership/ordering relationship that is otherwise sound. The `kill-barrier escalated` INFO correctly sits above the Phase-4 (task 4-4) DEBUG breadcrumb, and that cross-phase relationship is well-documented; only the intra-phase shared-var ownership needs the dependency made explicit. If the orchestrator/tick supports `blocked_by`, optionally set 5-8 blocked_by 5-7 to encode the edge structurally.

---

### 2. Several Do-sections exceed the "≤5 concrete steps" scope signal

**Severity**: Minor
**Plan Reference**: `phase-1-tasks.md` Task 1-8 (10 Do-bullets); `phase-3-tasks.md` Tasks 3-4 and 3-5 (9 Do-bullets each); `phase-2-tasks.md` Task 2-15 (7 bullets, several multi-part); plus 3-6 (8), 1-9 (8)
**Category**: Scope and Granularity
**Change Type**: update-task

**Details**:
Task Design's "Too big" scope signal lists "the Do section exceeds 5 concrete steps" as a split indicator. Several tasks here exceed that — most prominently 1-8 (10 bullets, retyping logging seams across `cmd/bootstrap`, `internal/tmux`, `internal/restore`, `internal/bootstrapadapter`, `internal/tui`, `internal/state`), 3-4 and 3-5 (9 bullets each).

On inspection each of these is still a single coherent TDD cycle: 1-8 is the "retype every intermediate seam to `*slog.Logger`" mechanical sweep that *must* land atomically with 1-9 (the plan explicitly says they co-land as one big-bang PR, so splitting 1-8 further would not produce independently-buildable increments); 3-4/3-5 instrument one store each (`project.Store` / `alias.Store`) where the bullet count is driven by enumerating the store's several mutation methods plus the `via`-threading and `[needs-info]` resolutions, not by multiple unrelated behaviours. They do not cross multiple architectural boundaries in a way that would force a split, and each has a single describable test surface.

This is therefore a readability/granularity note rather than a split mandate: the bullet counts reflect faithful enumeration of many call sites of one mechanical change, which is the legitimate exception to the heuristic. No restructuring is required; flagging for the user's awareness only. If the user prefers tighter cycles, 1-8 is the only candidate where a per-package split could be considered — but doing so would break the "1-8 and 1-9 land together, build green only when both land" contract the plan deliberately establishes, so a split is NOT recommended.

**Current**:
> (No single contiguous snippet — this is a cross-task granularity observation on Tasks 1-8, 3-4, 3-5, 2-15, 3-6, 1-9. Their Do-sections are correct as written; see Details.)

**Proposed**:
> No content change recommended. The over-length Do-sections are the legitimate "enumerate many call sites of one mechanical change" exception to the ≤5-step heuristic, and each task remains a single coherent, independently-testable TDD cycle. Splitting 1-8 in particular would violate the deliberate "1-8 + 1-9 land together as one big-bang PR" contract. Recorded for user awareness; close as "won't fix / acknowledged" unless the user wants tighter cycles.

**Resolution**: Pending
**Notes**: Raised for transparency against the Task Design scope signal. The plan author appears to have made a conscious, defensible trade-off (atomic mechanical sweeps as single tasks) that is consistent with the big-bang-migration design. No implementation ambiguity results from the length — the bullets are concrete and ordered.

---
