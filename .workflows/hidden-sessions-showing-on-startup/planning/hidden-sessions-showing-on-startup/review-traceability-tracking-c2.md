---
status: complete
created: 2026-04-30
cycle: 2
phase: Traceability Review
topic: Hidden Sessions Showing On Startup
---

# Review Tracking: Hidden Sessions Showing On Startup - Traceability

## Findings

No findings. Cycle 2 re-review confirms the plan is a faithful, complete translation of the specification in both directions.

### Direction 1 (Spec → Plan) — Coverage Verified

Every specification section has plan coverage:

- **Root Cause 1** → Task 1-1 (chokepoint filter in `Client.ListSessions`).
- **Root Cause 2** → Task 2-1 (rename via `PortalBootstrapName` constant).
- **Fix A — Behaviour Contract, Placement Rationale, Filter Definition, Filter Application Order, Return-Value Contract** → Task 1-1 acceptance criteria, edge cases, and tests.
- **Fix A — Interaction With The Capture Path** (`ListSessionNames` delegation invariant, capture-path double-filter no-op) → Task 1-1 acceptance and edge cases.
- **Fix A — Empty-List Behaviour** (`cmd/list.go` MUST-verify, TUI out-of-scope) → Task 1-2.
- **Fix A — Diagnostic Escape Hatch** → spec marks deferred; no task required.
- **Fix B — Behaviour Contract, Naming Constraint, Lifecycle, Why Rename Instead Of Kill, Sole Production Caller Verified, Precondition** → Task 2-1 acceptance, edge cases, and context.
- **Doc-Comment Cleanup — `tmux.PortalSaverName`** → Task 1-2.
- **Doc-Comment Cleanup — `tmux.StartServer`** (five-point checklist) → Task 2-1 acceptance and Do-block.
- **Doc-Comment Cleanup — Convention Precedent** → Task 2-1 context.
- **Test Requirements — Unit `Client.ListSessions` Excludes `_*`** → Task 1-1 unit test.
- **Test Requirements — Unit `StartServer` Uses Reserved Bootstrap Name** → Task 2-1 `TestStartServer` update.
- **Test Requirements — End-To-End Post-Bootstrap Session State** (Assertions 1 and 2) → Task 2-2.
- **Test Requirements — Capture-Path Regression Guard** → Task 1-1 acceptance.
- **Out Of Scope / Deferred — `portal list --all`, `ListAllSessions`, bootstrap orchestrator step, generalised naming policy, audit of other `List*` methods** → spec marks deferred; no tasks required.
- **Out Of Scope / Deferred — Cleanup Of Pre-Existing `0` Sessions On Upgrade** (release-notes MUST clause) → Task 2-3 (added in cycle 1, verified present in cycle 2).
- **Rollout — two-commit shape, end-to-end test ships with commit 2, review as a pair** → Phase 1 and Phase 2 "Why this order" framing and acceptance criteria.

### Direction 2 (Plan → Spec) — Fidelity Verified

Every plan element traces back to the specification:

- **Task 1-1** content (filter mechanics, non-nil empty slice, `ListSessionNames` delegation preservation, unit test mix-of-names + fully-filtered + leading-space-no-trim cases, capture-path regression gate) — all map to Fix A's Behaviour Contract, Filter Definition ("no trimming, no case-folding"), Return-Value Contract, Interaction With The Capture Path, and Test Requirements.
- **Task 1-2** content (`cmd/list.go:66-68` verification, `internal/tui` untouched, `tmux.PortalSaverName` doc-comment refresh-or-note, `tmux.StartServer` doc-comment NOT touched in this phase) — all map to Fix A Empty-List Behaviour and Doc-Comment Cleanup → `tmux.PortalSaverName`, with the TUI-out-of-scope and StartServer-Phase-2-ownership boundaries directly from spec.
- **Task 2-1** content (`PortalBootstrapName` constant, `new-session -d -s` invocation, `TestStartServer` args assertion, doc-comment five-point checklist, no literal string in production or tests, precondition preservation) — all map to Fix B Behaviour Contract, Doc-Comment Cleanup → `tmux.StartServer`, and Test Requirements → Unit `StartServer`.
- **Task 2-2** content (both assertions, raw-tmux read MUST bypass `ListSessions`, Phase 2 ship-shape, `_seed` allowed-reserved-set fixture-specific allowance, `NoOpSaver` context note, integration build tag) — all map to Test Requirements → End-To-End and Rollout. The `_seed` and `NoOpSaver` notes are fixture-implementation details derived from the existing test's structure, not invented requirements; they are correctly framed as "test-only" and not propagated to production.
- **Task 2-3** content (release-notes one-liner referencing literal `0`, no auto-cleanup code, ships with Phase 2 not Phase 1) — all map to Out Of Scope / Deferred → Cleanup Of Pre-Existing `0` Sessions On Upgrade (the MUST clause).

### Cycle 1 Fix Verified

The cycle 1 finding (release-notes MUST clause missing from plan) was applied:

- Task 2-3 row present in Phase 2 task table in `planning.md`.
- Phase 2 Acceptance section contains the release-notes bullet.
- `phase-2-tasks.md` front-matter shows `total: 3`.
- `phase-2-tasks.md` contains the full Task 2-3 detail block with Problem / Solution / Outcome / Do / Acceptance / Tests / Edge Cases / Context / Spec Reference, all traced to the spec's "Cleanup Of Pre-Existing `0` Sessions On Upgrade" section.
- Tick task `tick-1687f9` referenced in cycle 1 closure notes.

### No Hallucinated Content

No plan content was identified that lacks a specification basis. Implementation-detail extrapolations (e.g. `cmd/list.go:66-68` line range, `internal/tmux/tmux.go:175-181` line range, `_seed` fixture allowance, leading-space "no-trim" test case) are file-locator and test-design specifics that the spec implicitly authorises by mandating filter behaviour and test coverage; none introduce new requirements or invented edge cases.

---

**Outcome**: Plan is a faithful, complete translation of the specification. No findings to action.
