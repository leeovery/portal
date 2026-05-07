---
status: in-progress
created: 2026-05-07
cycle: 2
phase: Traceability Review
topic: Session Scrollback Preview
---

# Review Tracking: Session Scrollback Preview - Traceability

## Findings

No findings. The plan is a faithful, complete translation of the specification.

### Direction 1 (Spec → Plan): Completeness verified

Every spec section maps to one or more tasks with adequate depth:

- **Overview / Side-effect-free contract** → Tasks 4-8, 4-9
- **Trigger and Entry Point** (Space binding, Esc dismiss, empty list no-op, self-exclusion) → Tasks 2-3, 2-4, 4-7
- **Interaction Shape** (pagePreview arm, layout, viewport sizing, WindowSizeMsg forwarding) → Tasks 2-3, 2-6, 3-6
- **Source of Preview Bytes** (disk-only reads, no marker check, no new capture wrapper, snapshot-not-live, surface label honesty) → Tasks 1-2, 1-3, 1-6, 3-5, 4-9
- **Multi-pane Rendering Shape** (sequential window-grouped, key bindings, pane-0 reset on cycle, degenerate, position on re-entry, indexing convention, model lifecycle, chrome floor with counter semantics, concrete enumeration call, enumeration failure) → Tasks 1-5, 1-6, 2-2, 3-1, 3-2, 3-3, 3-5
- **History Depth / Read Pipeline** (N=1000, single-fd, definition of line, trailing-newline edge case, output handoff, synchronous read, performance budget) → Tasks 1-1, 1-2, 1-4, 2-2, 2-7
- **Refresh Semantics** (read trigger events, initial-open ordering, viewport-internal scroll, scroll resets on focus change, dwell, resize-not-trigger) → Tasks 2-2, 2-6, 3-2, 3-3, 3-6
- **Read-Failure Handling / Placeholder / Error string** → Tasks 1-2, 1-3, 4-1, 4-2
- **Esc Level Tree** → Task 2-4
- **No In-preview Between-Session Stepping** → covered by absence of binding; Tasks 2-6, 3-2, 3-3, 3-4 confirm only `]` `[` `Tab` `Esc` are owned
- **Filter Behaviour with Preview** → Task 2-5
- **Brand-new-session Edge Case** → Task 4-4
- **Privacy / Threat Model** (no opt-out, no redaction, no safety fallback) → covered by Task 4-9 surface audit (no preview-layer suppression layer); spec decision is "no design response", so absence of tasks is faithful
- **Cross-cutting Seams**:
  - Bootstrap Restore-Window Interaction → Task 4-9 (no bootstrap changes)
  - Externally-Killed Session During Preview → Tasks 4-5, 4-6
  - `_portal-saver` Self-Reference → Task 4-7
  - ANSI Passthrough vs Viewport Width → Task 2-2 (verbatim SetContent)
  - State Package API Reuse (ScrollbackFile, SanitizePaneKey, resolution chain, stateDir resolution, tail-N helper) → Tasks 1-1, 2-1, 2-2, 2-7, 3-1
- **Architecture Summary / Test seams / Wiring shape** (TmuxEnumerator, ScrollbackReader three-shape contract, constructor injection, no `tmuxtest` import) → Tasks 2-1, 2-7, 4-8
- **Out of Scope (v1)** → covered by absence; cross-cutting items asserted by 4-9
- **Acceptance Criteria**: every enumerated criterion has a corresponding task acceptance check

### Direction 2 (Plan → Spec): Fidelity verified

Every task ties back to identified spec section(s) via Spec Reference, with no hallucinated content found:

- All task Problems trace to spec requirements/decisions.
- Task wording choices made under spec open items (placeholder = `(no saved content)`, error string = `(unable to read scrollback)`, helper name `TailScrollback`, chrome layout, header-vs-footer) are explicitly delegated to the build phase by spec § *Open Items Handed to the Build Phase*.
- The single permitted addition to `tmux.Client` (Task 1-5) is explicitly authorised by spec § *Multi-pane Rendering Shape > Concrete enumeration call*.
- Task 3-5's no-liveness acceptance criterion (added cycle 1) traces to spec § *Source of Preview Bytes > Surface label honesty*.
- No tasks introduce architectural surfaces forbidden by spec § *Architecture Summary > No changes to* (verified by Task 4-9 audit).
- No tasks invent edge cases beyond those enumerated in the spec; cycle-key wraparound, base-index drift, non-contiguous indices, brand-new pane, externally-killed session, mid-write atomicity, and trailing-partial behaviour all originate in named spec sections.

Cycle-1 fix (no-liveness AC on Task 3-5) is correctly applied and has not introduced gaps elsewhere — the chrome rendering chain (3-5 → 3-6 → 3-7) remains coherent.
