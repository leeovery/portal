---
status: complete
created: 2026-05-07
cycle: 3
phase: Traceability Review
topic: Session Scrollback Preview
---

# Review Tracking: Session Scrollback Preview - Traceability

## Findings

No findings. The plan remains a faithful, complete translation of the specification.

### Direction 1 (Spec → Plan): Completeness re-verified

Walked the specification end-to-end. Every section, decision, edge case, constraint, data-model element, integration point, validation rule, and acceptance bullet maps to a task with adequate depth:

- **Overview / Use case framing / Side-effect-free contract** → Tasks 4-8 (hermetic invariant), 4-9 (no-new-surface audit)
- **Trigger and Entry Point** (Space binding on Sessions page only, Enter unchanged, Esc dismiss, self-exclusion inheritance, empty-list / no-highlighted-item no-op) → Tasks 2-3, 2-4, 4-7
- **Interaction Shape** (pagePreview as peer of pageFileBrowser, one session per open, full-screen layout, viewport sizing, WindowSizeMsg forwarding, scroll preservation across resize) → Tasks 2-3, 2-6, 3-6
- **Source of Preview Bytes** (always-disk, ScrollbackFile reuse, no marker check, no new capture wrapper, snapshot-not-live, single-user-per-machine, surface label honesty) → Tasks 1-1, 1-2, 1-3, 1-6, 3-5, 4-9
- **Multi-pane Rendering Shape** (sequential window-grouped, no layout parser, real-world distribution rationale, key bindings table, keymap policy, pane-0 reset on window cycle, bidirectional rationale, degenerate cases, position on re-entry, indexing convention, model lifecycle, chrome floor with counter semantics under base-index drift / non-contiguous gaps, concrete enumeration call options (a)/(b), enumeration failure) → Tasks 1-5, 1-6, 2-2, 3-1, 3-2, 3-3, 3-4, 3-5, 3-7
- **History Depth / Read Pipeline** (N=1000 pinned, slice size constant, memory trade-off, scroll within bounds with hard top edge, no deeper-history extend, tail-N reverse-scan idiom, ~30 LOC shape, single-fd invariant, definition of line, trailing-newline edge case, ANSI passthrough output handoff, synchronous read, performance budget p99 < 5ms on 4MB) → Tasks 1-1, 1-2, 1-4, 2-2, 2-7
- **Refresh Semantics** (stateless w.r.t. byte content, no timer/no polling, read trigger events, initial-open ordering 5-step sequence, viewport-internal scroll does not re-read, scroll position resets on focus change, dwell behaviour rationale, between-session step is not a trigger, resize is not a read trigger) → Tasks 2-2, 2-6, 3-2, 3-3, 3-6
- **Read-Failure Handling** (daemon mid-write atomicity, .bin deleted, OS-level read error, placeholder triggering vs non-triggering conditions, error string uniformity, no per-pane error cache, retry-on-refocus) → Tasks 1-2, 1-3, 4-1, 4-2, 4-3
- **Esc Level Tree** (in-preview, committed-filter, mid-typing-filter, no-filter — preview owns level 1; bubbles/list owns levels 2-3; Portal owns level 4) → Task 2-4
- **No In-preview Between-Session Stepping** (cascading consequences: no between-session keymap, list cursor sync n/a, filter set boundary n/a, refresh trigger list does not include between-session, reversibility, override traceability against earlier research) → covered by absence + Tasks 2-6, 3-2, 3-3, 3-4 confirming only `]` `[` `Tab` `Esc` are owned
- **Filter Behaviour with Preview** (two-phase filter, default bubbles/list semantics, no magic-Space, canonical user flow) → Task 2-5
- **Brand-new-session Edge Case** (whole-session, per-pane, chrome integrity, no live capture fallback) → Task 4-4
- **Privacy / Threat Model** (no opt-out, no redaction, no safety fallback, reversibility, build-phase consequence) → covered by absence of corresponding tasks (spec decision is "no design response") + Task 4-9 surface audit
- **Cross-cutting Seams**:
  - Bootstrap Restore-Window Interaction → Task 4-9 (no bootstrap changes)
  - Externally-Killed Session During Preview (content reads progressively placeholder, chrome stable, re-fetch on dismiss) → Tasks 4-5, 4-6
  - `_portal-saver` Self-Reference → Task 4-7
  - ANSI Passthrough vs Viewport Width → Task 2-2 (verbatim SetContent), Task 4-9 (no wrapping/sanitisation introduced)
  - State Package API Reuse (ScrollbackFile, SanitizePaneKey verbatim arg shape, resolution chain, stateDir captured once, tail-N helper) → Tasks 1-1, 2-1, 2-2, 2-7, 3-1
- **Architecture Summary** (read pipeline shape, page state machine, within-preview keymap, chrome rendering, no-changes list, test seams with three-shape contract, wiring shape with constructor injection and no tmuxtest import) → Tasks 2-1, 2-7, 4-8, 4-9
- **Out of Scope (v1)** (live capture, literal layout, in-preview between-session stepping, deeper history, auto-refresh, position memory, per-pane current-command, pane position hint, privacy toggle, off-Sessions-page entry, preview-layer `_portal-saver` suppression) → correctly absent; reversibility-claims preserved by Task 4-9
- **Acceptance Criteria** (entry & dismiss, within-preview navigation, read pipeline, chrome, edge cases, filter integration, side-effect-free contract) → every enumerated bullet has a corresponding task acceptance check
- **Open Items Handed to the Build Phase** (N value pinned at 1000, chrome layout, placeholder wording, error-string wording, tail-N helper name, `_portal-saver` confirmation, keymap collisions) → all surfaced in tasks with implementer-discretion latitude

Cycle-1 fix (no-liveness acceptance criterion on Task 3-5) is in place — verified verbatim in `phase-3-tasks.md` lines 246, 255.

### Direction 2 (Plan → Spec): Fidelity re-verified

Walked every task's Problem / Solution / Do / Acceptance Criteria / Tests / Edge Cases back to the specification. Every plan element traces to a named spec section:

- All task **Problem** statements anchor to identified spec requirements or decisions.
- All **Solution** approaches match spec architectural choices (always-disk, sequential window-grouped, constructor-injected seams, three-shape Tail contract, lazy-per-focus reads, fresh model per open, side-effect-free pathway).
- All **acceptance criteria** verify spec-required behaviours; none invent new requirements.
- All **edge cases** trace to spec-enumerated cases (cycle-key wraparound, base-index drift, non-contiguous tmux indices, brand-new pane, externally-killed session, mid-write atomicity via single fd, trailing-partial exclusion, ENOENT vs EACCES branching, resize-not-a-trigger, drag-resize zero reads).
- All wording-pinning choices made under spec **Open Items** (placeholder = `(no saved content)`, error string = `(unable to read scrollback)`, helper name `TailScrollback`, chrome layout sketch, header-vs-footer, `previewTailLines = 1000`, `previewChromeHeight = 1`) are explicitly delegated by spec § *Open Items Handed to the Build Phase*.
- The single permitted addition to `tmux.Client` (Task 1-5: `ListWindowsAndPanesInSession`) is explicitly authorised by spec § *Multi-pane Rendering Shape > Concrete enumeration call (option a, preferred)*.
- Implementation-detail polish (e.g. `PORTAL_SKIP_PERF` env opt-out, `\x1f` separator option for pipe-in-window-name, `lipgloss.JoinVertical` composition) reflects sound build-phase pinning of spec open items, not invented requirements.
- No tasks introduce architectural surfaces forbidden by spec § *Architecture Summary > No changes to* — verified by Task 4-9 audit.
- No tasks add user-facing affordances beyond the spec keymap (`]` `[` `Tab` `Esc` plus inherited viewport scroll keys).
- No tasks introduce a preview-layer `_portal-saver` blacklist (Task 4-7 explicitly forbids it).
- Task 3-5's no-liveness acceptance criterion (added cycle 1) traces faithfully to spec § *Source of Preview Bytes > Surface label honesty*.

### Drift check (cycle 2 integrity-only fix)

Cycle 2's integrity-only fix (planning.md table row method-name alignment for `ListWindowsAndPanesInSession`) was a non-content reformat. Re-verified that:
- `phase-1-tasks.md` Task 1-5 still references `ListWindowsAndPanesInSession(session) ([]WindowGroup, error)` consistently.
- `phase-2-tasks.md` Task 2-1 references the same signature in the `TmuxEnumerator` interface declaration.
- `phase-3-tasks.md` Task 3-7 references `ListWindowsAndPanesInSession` in the regression-pin test.
- `phase-4-tasks.md` Task 4-8 references the same method name in the hermetic test.
- `planning.md` Phase 1 acceptance and task table row references match.

No content drift. Method name is consistent across all four phases.

### Conclusion

The plan is clean for traceability. Three cycles of review (cycle 1 found and fixed the surface-label-honesty gap; cycles 2 and 3 confirm no remaining gaps and no hallucinated content).
