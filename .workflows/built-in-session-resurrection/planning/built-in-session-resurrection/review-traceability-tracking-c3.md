---
status: complete
created: 2026-04-27
cycle: 3
phase: Traceability Review
topic: Built-In Session Resurrection
---

# Review Tracking: Built-In Session Resurrection - Traceability

## Findings

No findings. The plan remains a faithful, complete translation of the specification.

## Coverage Summary

### Direction 1: Specification → Plan (completeness)

Re-verified every major spec section has explicit task coverage:

| Spec Section | Coverage |
|---|---|
| Overview / Product Goal / Organizing Principles | Reflected in plan goals; non-goals respected throughout |
| Scope & Constraints → Minimum Versions (tmux ≥ 3.0) | Phase 1 tasks 1-2, 1-3 |
| Scope (in/out, ephemeral state, deferred items) | Reflected throughout; non-goals respected (no `select-layout -E` fallback, no opportunistic save, no `portal state save`, no `window-renamed`/`pane-exited` registration, no schema migration) |
| Hook System Lifecycle Behavior (single-mode, caller-pattern, no `&&` self-removal, CLI surface unchanged) | Phase 4 (helper-driven firing) + task 4-6 (marker removal) |
| Save-Side Architecture → Execution Model (`_portal-saver`, signal handling, lifecycle, version-marker restart) | Phase 2 tasks 2-5, 2-6, 2-7, 2-12 |
| Save-Side Architecture → Triggers & Serialization (notify, ticker, dirty flag, max-gap, defensive clear, in-flight atomicity, crash safety) | Phase 2 tasks 2-2, 2-7, 2-12 |
| Save Format & Schema (paths, paneKey sanitizer, schema, scrollback, dedup, GC, FIFO files, hook-key drift) | Phase 2 tasks 2-1, 2-3, 2-9, 2-10 |
| tmux Hook Registration Lifecycle (registration, idempotency, scenarios 1–7, removal) | Phase 1 tasks 1-4, 1-5, 1-6, 1-7, 1-8 |
| Restore-Side Architecture (trigger, skeleton-eager, lazy scrollback, hook-driven hydration, markers, failure modes, user-created sessions, direct attach) | Phase 3 tasks 3-1, 3-3, 3-5, 3-6, 3-7, 3-8 |
| Scrollback Restore Mechanics (helper, FIFO, timeout, file-missing, settle sleep, marker lifecycle, validation reference) | Phase 3 tasks 3-2, 3-8, 3-9, 3-10, 3-11, 3-12 |
| Resume Hook Firing (firing point, deletions, behaviour change, session rename) | Phase 4 tasks 4-1 through 4-7 (4-4 intentionally `[BLOCKED]` per user-acknowledged needs-info) |
| Layout Restoration (per-window order, layout source, fallback, terminal drift, zoom) | Phase 3 task 3-4 |
| Bootstrap Flow (PersistentPreRunE 8-step sequence, ordering rationale, attach flow, return-to-caller, loading page, scope) | Phase 5 tasks 5-1, 5-2, 5-3, 5-6, 5-7 |
| WaitForSessions / bootstrapWait Removal (what is deleted, replacement, what stays) | Phase 5 tasks 5-4, 5-5 |
| CleanStale Behavior (guard removal, where it runs, criteria, refactor scope) | Phase 4 task 4-7 |
| CLI Surface (status, cleanup, daemon, notify, signal-hydrate, hydrate, migrate-rename, namespace, no manual save, unchanged user-facing surface) | Phase 1 task 1-1 (scaffold all 7 subcommands), Phase 6 tasks 6-4, 6-5, 6-6, 6-7 |
| Observability & Diagnostics (log file, rotation, status, proactive signals, fatal vs soft, TUI interaction, what NOT in scope) | Phase 6 tasks 6-1, 6-2, 6-3, 6-8, 6-9, 6-10 |
| Failure Modes & Recovery (consolidated table, NOT-handled-specially, user feedback channels, self-healing) | Distributed: tasks 2-7, 2-10, 2-12, 3-9, 3-10, 3-11, 3-12, 4-7, 6-8, 6-9 |
| Session & Project Store Interaction (name stability, timestamp handling, no new entries, orphan saved session) | Naturally satisfied — Phase 3 restore reads `sess.Name` verbatim and never touches `projects.json`. Spec explicitly states "No architectural changes to `projects.json` or the project store are required by this specification." |
| Documentation Deliverables (Privacy, Uninstall, hooks-fire-on-reboot-only clarification, tmux ≥ 3.0, storage location) | Phase 6 task 6-11 |

### Direction 2: Plan → Specification (fidelity)

Re-verified every plan element traces back to specific spec sentences:

- Each phase's Goal and Why-this-order paragraphs cite verbatim spec ordering rationales.
- Every task's Problem / Solution / Outcome ties to a spec section.
- Every task's Acceptance Criteria, Tests, and Edge Cases are derivable from spec requirements or are documented planning decisions inside spec-permitted scope.
- Context blocks throughout cite spec excerpts verbatim.

No new hallucinated content was found.

### Intentional planning decisions (spec-permitted, not hallucinations)

- **Task 3-3 Option A (`base-index` / `pane-base-index` prediction-before-creation)** — flagged inline as `[needs-info]`, accepted by the user across prior cycles; spec explicitly allows planning to choose mechanism.
- **Task 4-4 argv source for `migrate-rename`** — flagged `[needs-info]` (Route A: tmux format variable vs. Route B: daemon-side rename-delta side-band); spec explicitly delegates to planning ("Planning-phase decides the exact wiring"). Phase 4 acceptance #5 conditional on this.
- **Task 5-3 `bootstrap.NewShim`** — transitional test-compat scaffold with explicit `TODO(phase-6)` markers; no spec-violating behaviour; justified by the existing cmd-package testing pattern.
- **Task 6-7 `purgeStateDir` symlink-leaf refusal** — defensive housekeeping for a destructive command; spec is silent but the safety check protects users from `os.RemoveAll(<symlinked-state-dir>)` blowing through to unintended targets. Implementation detail within spec scope ("`--purge` removes `~/.config/portal/state/` only when explicitly requested").
- **Task 5-2 retry budget (3 attempts × 100ms for `_portal-saver` creation)** — concrete interpretation of spec's "retries a small number of times" (Failure Modes); spec is intentionally non-prescriptive on the exact count.
- **Task 6-1 `parseLevel` default-to-WARN on invalid `PORTAL_LOG_LEVEL`** — implementation choice within spec's stated default ("warnings and errors by default").

## Notes

- Cycle 1 surfaced 7 traceability findings — all applied. Cycle 1 integrity surfaced 7 findings — all applied.
- Cycle 2 traceability was clean (5 integrity findings applied prior; traceability had no findings).
- Cycle 3 confirms the plan continues to be a faithful translation; no regressions introduced by prior cycle fixes.
- Per-phase task detail files (`phase-1-tasks.md` through `phase-6-tasks.md`) were re-read in full alongside `planning.md` and `specification.md`.
- The two pre-existing `[needs-info]` markers (task 4-4 argv source for `migrate-rename`, task 3-3 Option A; Phase 4 acceptance #5 conditional on the same) are intentional planning-decision deferrals previously surfaced and accepted by the user — not new findings, per the orchestrator's instruction.
