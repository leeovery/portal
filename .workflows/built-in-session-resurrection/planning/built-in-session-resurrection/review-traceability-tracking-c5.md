---
status: complete
created: 2026-04-27
cycle: 5
phase: Traceability Review
topic: Built-In Session Resurrection
---

# Review Tracking: Built-In Session Resurrection - Traceability

## Findings

No findings. The plan remains a faithful, complete translation of the specification.

## Coverage Summary

**Direction 1: Specification → Plan (completeness)**

Every spec section retains plan coverage; cycle 4 enumerated the full mapping and no spec content has shifted since. Re-verified for this cycle:

- Overview / Product Goal / Organizing Principles → encoded in phase goals and rationales.
- Scope & Constraints (tmux ≥ 3.0, captured/excluded state, non-goals, deferred items) → tasks 1-2 / 1-3 (version guard), 2-3 (schema honours scope), 2-8 (filters `_*`, removed-form env), 6-11 (README documents non-goals & deferred opt-out workarounds).
- Hook System Lifecycle Behavior (single persistent behavior, caller wrapper pattern, unchanged CLI surface, rationale) → tasks 4-5 / 4-6 preserve `hooks set/list/rm` shape; behaviour change documented in 6-11; rejected `&&` shell-chaining is design-level rationale (no implementation needed).
- Save-Side Architecture: Execution Model (host process `_portal-saver`, session visibility/filtering, defensive `destroy-unattached off -t`, signal handling, lifecycle summary) → tasks 2-5, 2-6, 2-7, 2-8 (filtering), 2-12 (signal handlers).
- Save-Side Architecture: Triggers & Serialization (event-driven hooks, periodic ticker, no opportunistic trigger, single-writer dirty-flag mechanism, daemon tick pseudocode, defensive startup clear, tick cadence rationale, in-flight capture atomicity, crash safety) → tasks 1-7 (hook registration), 2-2 (notify), 2-7 (startup clear), 2-12 (full tick + atomicity).
- Save Format & Schema (storage location/permissions, directory layout, scrollback files, base-index handling, canonical paneKey, `sessions.json` v1 schema, atomic commit discipline, content-hash dedup with seed, GC, retention, FIFO files) → tasks 2-1, 2-3, 2-8, 2-9, 2-10, 3-2, 3-12.
- tmux Hook Registration Lifecycle (events, registration shape, content-based idempotency, scenarios 1–7, removal in reverse index order, false paths) → tasks 1-4, 1-5, 1-6, 1-7, 1-8, 2-6 (version-marker restart), 4-4 (BLOCKED — migrate-rename argv source).
- Restore-Side Architecture (restoration trigger, skeleton-eager + scrollback-lazy, hook-driven hydration, marker coordination `@portal-skeleton-*` and `@portal-restoring`, failure-mode behavior, user-created sessions mid-restore, direct `tmux attach` path) → tasks 3-3 (BLOCKED — live-index source), 3-5, 3-6, 3-7, 2-11 (marker enumeration).
- Scrollback Restore Mechanics (helper pre-shell via FIFO, signal mechanism, helper behavior on startup, 3s timeout, 100ms settle sleep, marker lifecycle summary, mechanism-level failure modes, validation reference) → tasks 3-2, 3-8, 3-9, 3-10, 3-11.
- Resume Hook Firing (firing point inside helper exec chain, what is deleted, what stays unchanged, behavior change "no live attach firing", `run-shell` blocking note, net simplification, session rename hook key migration) → tasks 4-1, 4-2, 4-3, 4-4 (BLOCKED), 4-5, 4-6.
- Layout Restoration (per-window restoration order, layout string source, pane-count mismatch / fallback, terminal size drift, zoom state, summary order) → task 3-4.
- Bootstrap Flow (Integrated) (PersistentPreRunE 8-step sequence, ordering rationale, attach flow after bootstrap, return-to-caller timing, loading-page minimum display, scope of bootstrap decisions vs implementation) → tasks 5-1, 5-2, 5-3, 5-6, 5-7.
- WaitForSessions / bootstrapWait Removal (what's deleted, why, replacement, what stays, behavioral improvement) → tasks 5-4, 5-5.
- CleanStale Behavior (change, why guard existed, why removed, where it runs, stale-detection criteria unchanged, refactor scope) → task 4-7.
- CLI Surface (`portal state status`, `portal state cleanup` with `--purge`, internal subcommands `daemon`/`notify`/`signal-hydrate`/`hydrate`, namespace rationale, no manual save command, unchanged user-facing surface) → tasks 1-1 (scaffolding), 6-4 / 6-5 (status), 1-9 / 6-6 / 6-7 (cleanup), 2-7 (daemon), 2-2 (notify), 3-11 (signal-hydrate), 3-8 / 3-9 / 3-10 / 4-2 (hydrate).
- Observability & Diagnostics (motivation, log file format, log rotation with concurrent-writer discipline, `portal state status`, proactive health signals, fatal bootstrap errors, what's NOT in scope) → tasks 6-1, 6-2, 6-3, 6-4, 6-5, 6-8, 6-9, 6-10.
- Failure Modes & Recovery (guiding principle, consolidated failure-handling table, what's NOT handled specially, user feedback on partial restoration, recovery self-healing properties) → distributed across phases 2 / 3 / 4 / 5 task acceptance criteria + 6-8 / 6-9.
- Session & Project Store Interaction (restored session names, projects.json timestamp handling, restoration never creates new entries, edge case orphan saved session, consistency with existing semantics) → no plan tasks needed (these are preserved semantics — name verbatim is task 3-3, no projects.json mutation is implicit via not touching the file in restore).
- Documentation Deliverables (README Privacy, Uninstall, hooks behaviour, tmux ≥ 3.0, storage location, non-scope) → task 6-11.

Coverage depth re-spot-checked on:

- Task 3-1 (`ReadIndex`) — now includes the `ErrCorruptIndex` sentinel applied via integrity cycle 4; spec sections "Bootstrap Flow → step 5", "Failure Modes → corrupt sessions.json", and "Restore-Side Architecture → Restoration Trigger" remain fully reflected.
- Task 3-3 (`RestoreSession`) — `[BLOCKED — live-index source]` block correctly enumerates the spec-compatible Options A / B / C; no invented constraint beyond what the spec leaves to planning.
- Task 4-2 (hook firing in helper) — every helper sub-step (signal-arrived, file-missing, timeout) is traced verbatim to the spec's "Helper Behavior on Startup" and "Resume Hook Firing → Firing Point" sections.
- Task 4-3 (`migrate-rename` body) — atomic-write + best-effort + collision-warn semantics trace to "Resume Hook Firing → Session Rename → Argument source / Failure mode".
- Task 5-1 (`skipTmuxCheck`) — exempt list matches the spec's "exempt commands" enumeration verbatim including all five `state` internal subcommands plus `migrate-rename`.
- Task 6-1 (logger format / level filtering) — `timestamp | level | component | message` format and the six component identifiers (`daemon`, `restore`, `hydrate`, `notify`, `hooks`, `bootstrap`) match the spec's "Observability & Diagnostics → Log File" section.
- Task 6-11 (README) — every required section (Privacy, Uninstall, hooks behaviour, tmux ≥ 3.0 requirement, storage location) traces to "Documentation Deliverables"; spec non-goals (exhaustive tmux API refs, internal architecture diagrams, changelog entries) are explicitly excluded in the task body.

**Direction 2: Plan → Specification (fidelity)**

Every plan element traces to a spec section. No hallucinated content found in this cycle.

- All 9 phase 1 tasks → "Bootstrap Flow", "tmux Hook Registration Lifecycle", "CLI Surface → Internal Subcommands", "Scope & Constraints → Minimum Versions".
- All 12 phase 2 tasks → "Save Format & Schema", "Save-Side Architecture", "Observability & Diagnostics → Log File / Rotation".
- All 13 phase 3 tasks → "Restore-Side Architecture", "Scrollback Restore Mechanics", "Layout Restoration", "Bootstrap Flow → step 5".
- All 7 phase 4 tasks → "Resume Hook Firing", "CleanStale Behavior".
- All 10 phase 5 tasks → "Bootstrap Flow", "WaitForSessions / bootstrapWait Removal", "Loading-Page Minimum Display".
- 11 of 12 phase 6 tasks → "Observability & Diagnostics", "CLI Surface", "Failure Modes & Recovery", "Documentation Deliverables".
- Task 6-12 (delete legacy `bootstrap.NewShim`) is plan-internal cleanup of a transitional adapter introduced in task 5-3. The task body explicitly notes "no direct reference (this is plan-internal cleanup of a transitional adapter introduced in task 5-3)" — accepted as legitimate plan hygiene, not hallucinated scope.

**Intentional BLOCKED items (per orchestrator context, NOT raised as findings):**

- Task 3-3 — `[BLOCKED — needs planning decision on live-index source]`: spec is compatible with prediction (Option A), re-query+symlink (Option B), or UUID FIFO names (Option C); planning has not pinned the route.
- Task 4-4 — `[BLOCKED — needs planning decision on prior-name argv source]`: spec defers wiring decision to planning between Route A (tmux format variable) and Route B (daemon-side rename-delta side-band).
- Phase 4 acceptance bullet #5 — explicitly conditional on `[needs-info]` resolution in task 4-4.

These three deferrals are spec-aligned (the spec itself defers planning decisions); the plan correctly surfaces them as BLOCKED rather than inventing answers.

**Cross-cycle context:**

- Cycle 1 raised one finding (task 4-4 argv wiring contradicted spec contract); applied.
- Cycle 2 traceability: clean.
- Cycle 3 traceability: clean.
- Cycle 4 traceability: clean.
- Cycle 5 (this review): clean. Integrity cycles 1–4 applied 18 findings total across plan and task bodies; spot-checks confirm all newly-introduced content (e.g., `ErrCorruptIndex` sentinel in task 3-1, orchestrator seam in tasks 5-2/5-3, observability rename detection deferral in task 4-4) traces faithfully to the specification.

The plan continues to be a faithful, complete translation of the specification with no hallucinated content and no missing coverage.
