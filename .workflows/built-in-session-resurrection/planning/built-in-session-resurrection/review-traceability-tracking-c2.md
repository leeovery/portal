---
status: complete
created: 2026-04-27
cycle: 2
phase: Traceability Review
topic: Built-In Session Resurrection
---

# Review Tracking: Built-In Session Resurrection - Traceability

## Findings

No findings. The plan is a faithful, complete translation of the specification.

## Coverage Summary

### Direction 1: Specification → Plan (completeness)

Every major specification section has explicit task coverage:

| Spec Section | Coverage |
|---|---|
| Scope & Constraints → Minimum Versions (tmux ≥ 3.0) | Phase 1 tasks 1-2, 1-3 |
| Scope (in/out, ephemeral state, deferred items) | Reflected throughout; non-goals respected (no `select-layout -E` fallback, no opportunistic save, no `portal state save`, no `window-renamed`/`pane-exited` registration) |
| Hook System Lifecycle Behavior | Phase 4 (helper-driven firing) |
| Save-Side Architecture → Execution Model (`_portal-saver`, signal handling, lifecycle) | Phase 2 tasks 2-5, 2-6, 2-7, 2-12 |
| Save-Side Architecture → Triggers & Serialization (notify, ticker, dirty flag, max-gap, defensive clear, in-flight atomicity, crash safety) | Phase 2 tasks 2-2, 2-7, 2-12 |
| Save Format & Schema (paths, paneKey, schema, scrollback, dedup, GC, FIFO files) | Phase 2 tasks 2-1, 2-3, 2-9, 2-10 |
| tmux Hook Registration Lifecycle (registration, idempotency, scenarios 1–7, removal) | Phase 1 tasks 1-4, 1-5, 1-6, 1-7, 1-8 |
| Restore-Side Architecture (trigger, skeleton-eager, lazy scrollback, hook-driven hydration, markers, failure modes, user-created sessions, direct attach) | Phase 3 tasks 3-1, 3-3, 3-5, 3-6, 3-7, 3-8 |
| Scrollback Restore Mechanics (helper, FIFO, timeout, file-missing, settle sleep, marker lifecycle, validation reference) | Phase 3 tasks 3-2, 3-8, 3-9, 3-10, 3-11, 3-12 |
| Resume Hook Firing (firing point, deletions, behaviour change, session rename) | Phase 4 tasks 4-1 through 4-7 (4-4 intentionally `[BLOCKED]` per user note) |
| Layout Restoration (per-window order, layout source, fallback, terminal drift, zoom) | Phase 3 task 3-4 |
| Bootstrap Flow (PersistentPreRunE 8-step sequence, ordering rationale, attach flow, return-to-caller, loading page, scope) | Phase 5 tasks 5-1, 5-2, 5-3, 5-6, 5-7 |
| WaitForSessions / bootstrapWait Removal (what is deleted, replacement, what stays) | Phase 5 tasks 5-4, 5-5 |
| CleanStale Behavior (guard removal, where it runs, criteria, refactor scope) | Phase 4 task 4-7 |
| CLI Surface (status, cleanup, daemon, notify, signal-hydrate, hydrate, namespace, no manual save) | Phase 1 task 1-1 (scaffold), Phase 6 tasks 6-4, 6-5, 6-6, 6-7 |
| Observability & Diagnostics (log file, rotation, status, proactive signals, fatal vs soft, TUI interaction, what NOT in scope) | Phase 6 tasks 6-1, 6-2, 6-3, 6-8, 6-9, 6-10 |
| Failure Modes & Recovery (consolidated table, NOT-handled-specially, user feedback channels, self-healing) | Distributed: tasks 2-7, 2-10, 2-12, 3-9, 3-10, 3-11, 3-12, 4-7, 6-8, 6-9 |
| Session & Project Store Interaction (name stability, timestamp handling, no new entries) | Naturally satisfied — Phase 3 restore reads `sess.Name` verbatim and never touches `projects.json`. Spec explicitly states "No architectural changes to `projects.json` or the project store are required by this specification." |
| Documentation Deliverables (Privacy, Uninstall, hooks clarification, tmux ≥ 3.0, storage location) | Phase 6 task 6-11 |

### Direction 2: Plan → Specification (fidelity)

Every plan element traces back to specific spec sentences. Phase ordering rationale, task acceptance criteria, edge cases, and Context blocks all cite verbatim spec excerpts. No invented behavior was found that lacks spec grounding.

The two pre-existing `[needs-info]` markers (task 4-4 argv source for `migrate-rename`, Phase 4 acceptance #5 conditional on the same) are intentional planning-decision deferrals previously surfaced in cycle 1 and accepted by the user — not new findings.

## Notes

- Cycle 1 surfaced 7 traceability findings, all applied. Cycle 1 integrity surfaced 7 findings, all applied.
- Per-phase task detail files (`phase-1-tasks.md` through `phase-6-tasks.md`) were re-read in full alongside `planning.md` and `specification.md`.
- The legacy `bootstrap.NewShim` introduced by task 5-3 is a transitional test-compat scaffold with explicit `TODO(phase-6)` markers; it does not introduce spec-violating behaviour and is justified by the existing cmd-package testing pattern.
