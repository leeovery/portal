---
status: complete
created: 2026-06-09
cycle: 2
phase: Plan Integrity Review
topic: Kill-Rename Prefix Collision
---

# Review Tracking: Kill-Rename Prefix Collision - Integrity

## Outcome

**Clean** — no findings. The plan meets structural quality and implementation-readiness standards.

This cycle re-checked the plan with fresh context after the two cycle-1 findings (both on Task 1's
test-file strategy) were applied. The corrected plan now commits Task 1 to creating
`internal/tmux/exact_target_internal_test.go` (`package tmux`) for the unexported `exactTarget` unit
test, with the regression tests staying in the external `tmux_test.go` (`package tmux_test`) driving
the exported `KillSession`/`RenameSession`. Both prior findings are correctly resolved.

## What Was Reviewed

- Planning file (`planning.md`): phase structure, goal, "why this order", phase acceptance criteria.
- All three task files (`phase-1-tasks.md` + `tick show` for each task): full Problem / Solution /
  Outcome / Do / Acceptance Criteria / Tests / Edge Cases / Context / Spec Reference.
- Specification cross-read for context fidelity (no traceability scope — integrity only).
- Codebase verification of every load-bearing claim the plan makes (see below).

## Criteria Assessment

1. **Task Template Compliance** — PASS. All three tasks carry every required field (Problem,
   Solution, Outcome, Do, Acceptance Criteria, Tests) plus Edge Cases, Context, and Spec Reference.
   Problem statements explain WHY (silent wrong-session kill/rename via prefix-match). Acceptance
   criteria are concrete and verifiable (exact argv strings, godoc presence, grep-zero-inline). Tests
   include edge cases (prefix-collision regression, bare-newName guard), not just happy paths.

2. **Vertical Slicing** — PASS. Each task is an independently verifiable increment: Task 1 (helper +
   KillSession + its tests), Task 2 (RenameSession + its tests), Task 3 (behaviour-neutral migration,
   proven by existing green tests). No horizontal layer-splitting.

3. **Phase Structure** — PASS. Single phase is the right call for a single-root-cause bugfix confined
   to one subsystem (`internal/tmux`) at one argv-construction chokepoint. The "why this order"
   rationale correctly argues that splitting would create thin phases with no real checkpoint. Phase
   acceptance criteria are present and pass/fail.

4. **Dependencies and Ordering** — PASS. Task 2 and Task 3 are both `blocked_by` Task 1 — a genuine
   capability dependency (both consume the `exactTarget` helper Task 1 introduces). This is a real
   convergence/foundation edge, not a redundant sequential-order edge. No circular dependencies. All
   three share priority 2 (uniform within the single phase); Task 1 unblocks the other two and runs
   first by both the explicit edge and natural creation order.

5. **Task Self-Containment** — PASS. Each task pulls forward the relevant spec decisions (chokepoint
   rationale, the RenameSession target-only/bare-newName trap, the migration-is-behaviour-neutral
   proof-by-green-tests), the exact functions/files/line anchors, the regression-test template to
   mirror (`TestHasSessionUsesExactMatchPrefix`), and the project `t.Parallel()` constraint. An
   implementer can execute any single task without reading the others.

6. **Scope and Granularity** — PASS. Each task is one TDD cycle. Task 1's Do has 6 steps but they are
   the cohesive single-increment set (helper + one argv fix + godoc + two test edits/adds); not
   multiple unrelated behaviours. Task 3 is a refactor but warrants its own cycle (5 sites, anti-drift
   end-state, grep-verified) rather than being mechanical boilerplate folded elsewhere.

7. **Acceptance Criteria Quality** — PASS. Criteria are pass/fail (exact argv strings; "begins with
   '='"; argv equals `["rename-session","-t","=foo","bar"]`; zero inline strings remain; build/test
   green). No subjective or interpret-this criteria.

8. **External Dependencies** — N/A (bugfix, not epic).

## Codebase Verification (load-bearing claims confirmed)

- `KillSession` (tmux.go:353), `RenameSession` (361), `PaneTargetExact` (546), `HasSession` (135),
  `HasSessionProbe` (165), `SwitchClient` (377), `SelectWindow` (934) — all "~line" anchors accurate.
- Current bare-`-t` forms confirmed: `kill-session -t name`, `rename-session -t oldName newName`.
- Inline `"="+name` session-target inventory matches Task 3 exactly: `tmux.go:136, 166, 378`;
  `saver_pane_pid.go:49, 84`; window-level `SelectWindow` at `tmux.go:936` (correctly excluded).
- `tmux_test.go` is `package tmux_test`; `export_test.go` and `option_discriminator_internal_test.go`
  are `package tmux` — confirms the internal-test convention Task 1 now follows for `exact_target_internal_test.go`.
- `TestKillSession` (723), `TestRenameSession` (939) pin the buggy bare-`-t` form (the assertions the
  tasks update); `TestHasSessionUsesExactMatchPrefix` (443) is the regression-test template with the
  `RunFunc` / live-`foo-2` / `=foo` simulation the tasks mirror; `MockCommander{RunFunc, Calls}` shape
  confirmed.
- `_portal-saver` `KillSession` callers referenced in the godoc rationale exist:
  `cmd/state_cleanup.go:185`, `internal/tmux/portal_saver.go:366,372,385`.

## Cycle-1 Findings — Resolution Confirmed

- **C1 finding 1 & 2 (Task 1 test-file strategy):** Resolved. Task 1's Do step, Edge Cases, and Tests
  now explicitly create `internal/tmux/exact_target_internal_test.go` (`package tmux`) for the
  unexported `exactTarget` unit assertion, and keep the `MockCommander`-driven regression tests in the
  external `tmux_test.go`. The package-boundary reasoning (external test can't reach the unexported
  helper) is now stated in-task and verified against the actual file package declarations.

## Findings

None.
