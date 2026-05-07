---
status: complete
created: 2026-05-06
cycle: 4
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

Cycle-1 fix (no-liveness acceptance criterion on Task 3-5) remains in place — verified in Task 3-5 acceptance criteria and tests; structural-content guard ("no substrings such as 'live', 'now showing', 'current', 'realtime'") still pinned.

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

### Drift check (cycle 3 integrity field-name fix)

Cycle 3's integrity-only fix renamed Phase-2 task references from `m.page` / `m.list` to `m.activePage` / `m.sessionList` so the plan matches the actual `internal/tui` Model field names. The user requested this cycle re-verifies that fix did not break any traceability links.

Re-verified directly:

- These field names are internal Go-level implementation references — they exist in the codebase, not the specification. The specification is silent on Go struct field naming, so a field-name change cannot, by construction, affect spec→plan traceability.
- The behavioural semantics of the renamed references (`m.activePage = pagePreview` for page transition; `m.sessionList.Index()` / `m.sessionList.SettingFilter()` / `m.sessionList.FilterValue()` / `m.sessionList.IsFiltered()` for cursor and filter state preservation) are identical to what was there before — same calls, same purpose, same spec mapping.
- Spec → task wiring still resolves correctly:
  - § *Trigger and Entry Point* → Task 2-3 still binds `Space` on the Sessions page and routes through the `m.activePage` page-state machine.
  - § *Esc Level Tree* → Task 2-4 still preserves `m.sessionList.Index()` and `m.sessionList.IsFiltered()` / `FilterValue()` across open/dismiss, satisfying the level-1 Esc behaviour.
  - § *Filter Behaviour with Preview* → Task 2-5 still gates Space passthrough on `m.sessionList.SettingFilter()`, satisfying the default-`bubbles/list`-semantics decision.
- Cross-phase consistency:
  - Task 2-3 and Task 2-4 both reference `m.activePage = pagePreview` / `m.activePage = PageSessions` in the same direction.
  - Task 2-3 (`m.sessionList.SettingFilter()`, `m.sessionList.SelectedItem() == nil`) and Task 2-5 (`m.sessionList.SettingFilter()` / `m.sessionList.FilterValue()`) and Task 2-4 (`m.sessionList.Index()` / `IsFiltered()` / `FilterValue()`) all use the same field name verbatim.
  - Task 4-5 (Sessions-list re-fetch on dismiss) and Task 4-7 (`_portal-saver` exclusion at source) inspect the same Sessions-list source and remain compatible.

No traceability link is severed, weakened, or muddied by the cycle 3 rename.

### Conclusion

The plan is clean for traceability. Four cycles of review:
- Cycle 1 found and fixed the surface-label-honesty gap on Task 3-5.
- Cycle 2 confirmed clean.
- Cycle 3 confirmed clean (traceability) and applied a field-name integrity fix to Phase 2 tasks.
- Cycle 4 re-verified that the field-name fix did not break any spec→plan or plan→spec links and confirmed clean.
