---
status: complete
created: 2026-05-06
cycle: 6
phase: Traceability Review
topic: Session Scrollback Preview
---

# Review Tracking: Session Scrollback Preview - Traceability

## Findings

No findings. The plan remains a faithful, complete translation of the specification after cycle 5's casing fix.

### Cycle 5 fix preservation re-verified

Cycle 5's integrity-only fix changed a single backticked code-bearing identifier in Phase 4 Task 4-6's last Tests bullet (line 297) from `pageSessions` to `PageSessions` to match the live exported page constant in `internal/tui/model.go:26`. Re-verified that fix:

- **Did not break any spec → plan link.** The spec at § *Esc Level Tree* and § *Cross-cutting Seams > Externally-Killed Session During Preview* describes the dismiss transition behaviour without committing to a specific Go identifier casing — both `pageSessions` and `PageSessions` resolve to the same conceptual destination, and the plan's test assertion now compiles against the live constant.
- **Did not break any plan → spec link.** Task 4-6's narrative still describes the spec's "killed-session-mid-preview" scenario verbatim; only the test-assertion identifier was aligned to the codebase symbol.
- **Did not introduce a casing inconsistency elsewhere.** The narrative transition-arrow form `pagePreview → pageSessions` (planning.md lines 118, 134; phase-4-tasks.md lines 203, 207, 208, 209, 218, 230, 249, 257) is correctly preserved — these describe the abstract transition, not a code symbol. The spec uses the same arrow-form at § *Cross-cutting Seams > Externally-Killed Session During Preview > Re-fetch contract* line 313 (`pagePreview → pageSessions transition`), so the plan matches spec wording verbatim.
- **Phase 2 Task 2-4** continues to use `PageSessions` correctly at every code-bearing site (Solution at line 173, Do at line 182, Acceptance at line 189, Tests at line 197) — these were never affected by cycle 5's change because they were already correctly cased.

### Direction 1 (Spec → Plan): Completeness re-verified

Re-read the specification end-to-end. Every section, decision, edge case, constraint, data-model element, integration point, validation rule, and acceptance bullet maps to a task with adequate depth. Confirmed coverage map (unchanged from cycle 5):

- **Overview / Use case framing / Side-effect-free contract** → Tasks 4-8, 4-9
- **Trigger and Entry Point** (Space binding on Sessions page only, Enter unchanged, Esc dismiss, self-exclusion inheritance, empty-list / no-highlighted-item no-op) → Tasks 2-3, 2-4, 4-7
- **Interaction Shape** (pagePreview as peer of pageFileBrowser, one session per open, full-screen layout, viewport sizing, WindowSizeMsg forwarding, scroll preservation across resize) → Tasks 2-3, 2-6, 3-6
- **Source of Preview Bytes** (always-disk, ScrollbackFile reuse, no marker check, no new capture wrapper, snapshot-not-live, single-user-per-machine, surface label honesty) → Tasks 1-1, 1-2, 1-3, 1-6, 3-5, 4-9
- **Multi-pane Rendering Shape** (sequential window-grouped, no layout parser, real-world distribution rationale, key bindings, keymap policy, pane-0 reset on window cycle, bidirectional rationale, degenerate cases, position on re-entry, indexing convention, model lifecycle, chrome floor with counter semantics under base-index drift / non-contiguous gaps, concrete enumeration call options (a)/(b), enumeration failure handling) → Tasks 1-5, 1-6, 2-2, 3-1, 3-2, 3-3, 3-4, 3-5, 3-7
- **History Depth / Read Pipeline** (N=1000 pinned, slice size constant, memory trade-off, scroll within bounds with hard top edge, no deeper-history extend, tail-N reverse-scan idiom, ~30 LOC shape, single-fd invariant, definition of line, trailing-newline edge case, ANSI passthrough output handoff, synchronous read, performance budget p99 < 5ms on 4MB) → Tasks 1-1, 1-2, 1-4, 2-2, 2-7
- **Refresh Semantics** (stateless w.r.t. byte content, no timer/no polling, read trigger events, initial-open ordering 5-step sequence, viewport-internal scroll does not re-read, scroll position resets on focus change, dwell behaviour rationale, between-session step is not a trigger, resize is not a read trigger) → Tasks 2-2, 2-6, 3-2, 3-3, 3-6
- **Read-Failure Handling** (daemon mid-write atomicity, .bin deleted, OS-level read error, placeholder triggering vs non-triggering conditions, error string uniformity, no per-pane error cache, retry-on-refocus) → Tasks 1-2, 1-3, 4-1, 4-2, 4-3
- **Esc Level Tree** (in-preview, committed-filter, mid-typing-filter, no-filter — preview owns level 1; bubbles/list owns levels 2-3; Portal owns level 4) → Task 2-4
- **No In-preview Between-Session Stepping** (cascading consequences, reversibility, override traceability against earlier research) → covered by absence of between-session bindings + Tasks 2-6, 3-2, 3-3, 3-4 confirming only `]` `[` `Tab` `Esc` are owned
- **Filter Behaviour with Preview** (two-phase filter, default bubbles/list semantics, no magic-Space, canonical user flow) → Task 2-5
- **Brand-new-session Edge Case** (whole-session, per-pane, chrome integrity, no live capture fallback) → Task 4-4
- **Privacy / Threat Model** (no opt-out, no redaction, no safety fallback, reversibility, build-phase consequence) → covered by absence of corresponding tasks (spec decision is "no design response") + Task 4-9 surface audit
- **Cross-cutting Seams**:
  - Bootstrap Restore-Window Interaction → Task 4-9 (no bootstrap changes)
  - Externally-Killed Session During Preview → Tasks 4-5, 4-6
  - `_portal-saver` Self-Reference → Task 4-7
  - ANSI Passthrough vs Viewport Width → Task 2-2 (verbatim SetContent), Task 4-9 (no wrapping/sanitisation introduced)
  - State Package API Reuse (ScrollbackFile, SanitizePaneKey verbatim arg shape, resolution chain, stateDir captured once, tail-N helper) → Tasks 1-1, 2-1, 2-2, 2-7, 3-1
- **Architecture Summary** (read pipeline shape, page state machine, within-preview keymap, chrome rendering, no-changes list, test seams with three-shape contract, wiring shape with constructor injection and no tmuxtest import) → Tasks 2-1, 2-7, 4-8, 4-9
- **Out of Scope (v1)** → correctly absent; reversibility-claims preserved by Task 4-9
- **Acceptance Criteria** — every enumerated bullet mapped: Entry & dismiss → 2-2/2-3/2-4/2-5/3-5; Within-preview navigation → 1-5/2-6/3-2/3-3/3-4/3-5; Read pipeline → 1-1/2-2/2-6/3-2/3-3/3-6/4-8; Chrome → 3-5/3-7; Edge cases → 1-6/2-2/4-1/4-2/4-3/4-4/4-5/4-6; Filter integration → 2-5; Side-effect-free contract → 3-7/4-8.
- **Open Items Handed to the Build Phase** (N value pinned at 1000, chrome layout, placeholder wording, error-string wording, tail-N helper name, `_portal-saver` confirmation, keymap collisions) → all surfaced with implementer-discretion latitude: 1-1, 2-7, 3-4, 3-5, 3-6, 4-1, 4-2, 4-7.

Cycle-1's traceability fix (no-liveness AC on Task 3-5) remains in place at phase-3-tasks.md line 246 (acceptance criterion #7) and line 255 (corresponding test name).

### Direction 2 (Plan → Spec): Fidelity re-verified

Walked every task's Problem / Solution / Do / Acceptance Criteria / Tests / Edge Cases back to the specification. Every plan element traces to a named spec section:

- All task **Problem** statements anchor to identified spec requirements or decisions; every task ends with an explicit **Spec Reference** line citing the relevant § headings.
- All **Solution** approaches match spec architectural choices: always-disk, sequential window-grouped, constructor-injected seams, three-shape Tail contract, lazy-per-focus reads, fresh model per open, side-effect-free pathway.
- All **acceptance criteria** verify spec-required behaviours; none invent new requirements.
- All **edge cases** trace to spec-enumerated cases.
- All wording-pinning choices (`(no saved content)`, `(unable to read scrollback)`, `TailScrollback`, chrome layout sketch, header-vs-footer, `previewTailLines = 1000`, `previewChromeHeight = 1`) are explicitly delegated by spec § *Open Items Handed to the Build Phase*.
- The single permitted addition to `tmux.Client` (Task 1-5: `ListWindowsAndPanesInSession`) is explicitly authorised by spec § *Multi-pane Rendering Shape > Concrete enumeration call (option a, preferred)*.
- Implementation-detail polish (`PORTAL_SKIP_PERF` env opt-out, `\x1f` separator option for pipe-in-window-name, `lipgloss.JoinVertical` composition, `errors.Is`/`%w` wrapping pattern) reflects sound build-phase pinning of spec open items, not invented requirements.
- No tasks introduce architectural surfaces forbidden by spec § *Architecture Summary > No changes to* — verified by Task 4-9 audit.
- No tasks add user-facing affordances beyond the spec keymap (`]` `[` `Tab` `Esc` plus inherited viewport scroll keys).
- No tasks introduce a preview-layer `_portal-saver` blacklist (Task 4-7 explicitly forbids it).

### Conclusion

The plan is clean for traceability. Six cycles of review:
- Cycle 1 found and fixed the surface-label-honesty gap on Task 3-5 (traceability).
- Cycles 2-5 confirmed clean for traceability; integrity cycles applied symbol/name/field alignment fixes.
- Cycle 6 (this cycle) re-verified that cycle 5's `PageSessions` casing fix on Task 4-6 preserved every spec→plan and plan→spec link, and confirmed no new traceability gaps surfaced from re-reading the spec end-to-end. Plan is implementation-ready.
