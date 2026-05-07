---
status: in-progress
created: 2026-05-06
cycle: 10
phase: Traceability Review
topic: Session Scrollback Preview
---

# Review Tracking: Session Scrollback Preview - Traceability

## Findings

No findings. Cycle 9's integrity fixes (Task 2-2 `GotoBottom()` insertion and Task 2-3 `termWidth`/`termHeight` field-name correction) preserve every spec→plan and plan→spec link.

### Cycle 9 fix preservation re-verified

Re-read every cycle 9 fix location alongside the spec sections it touches:

- **phase-2-tasks.md Task 2-2 Do step 6**. The post-fix Do section now contains: "Call `m.viewport.GotoBottom()` so the first frame renders at scroll-tail. `bubbles@v1.0.0/viewport.SetContent` only auto-jumps to bottom when `YOffset > len(lines)-1`; a fresh viewport has `YOffset == 0`, so without an explicit `GotoBottom()` the user sees the OLDEST lines, contradicting spec § *History Depth > Scroll within bounds* ('the viewport renders the tail by default') and the Phase 4 task 4-3 test that asserts `AtBottom()` immediately post-construction." This step traces directly to spec § *History Depth > Scroll within bounds* (line 122) — "The viewport (`bubbles/viewport`) renders the tail by default; the user can scroll up within the loaded N lines using the viewport's native scroll keymap" — and to spec § *Read-Failure Handling > Placeholder > Non-triggering condition* (line 205) — "Preview simply renders whatever lines are present, with the viewport at scroll-tail." The mechanism note (`bubbles@v1.0.0/viewport.SetContent` behaviour) is codebase fidelity, not new spec content.

- **phase-2-tasks.md Task 2-2 Acceptance Criteria new bullet (line 89)**. "After `SetContent`, the viewport is at scroll-tail (`m.viewport.AtBottom()` returns true) — `GotoBottom()` is called explicitly because `bubbles@v1.0.0/viewport.SetContent` does not jump to bottom when `YOffset == 0`." This AC traces to the same spec sections as the Do step. It also aligns with Phase 4 task 4-3's existing AC "The viewport is at scroll-tail (bottom) on initial open / focus change" and tasks 3-2 / 3-3 (which already invoked `GotoBottom()` after focus-change reads). The cycle 9 fix only restored the same invariant for the initial-open path.

- **phase-2-tasks.md Task 2-2 Tests new entry (line 99)**. "`it positions the viewport at scroll-tail on initial open`" — fixture bytes longer than viewport height; asserts `m.viewport.AtBottom() == true` immediately after construction. This test pins the user-observable behaviour mandated by spec § *History Depth > Scroll within bounds* and § *Read-Failure Handling > Placeholder > Non-triggering condition*. It is a regression-pin against the very class of issue cycle 9 caught — no new requirement, only an operationalisation of the spec contract.

- **phase-2-tasks.md Task 2-3 Do step 4 (line 134)**. The post-fix line now reads: "Construct `previewModel := NewPreviewModel(sessionName, m.enumerator, m.reader, m.termWidth, m.termHeight)`. The seams `m.enumerator` and `m.reader` arrive from task 2-7's TUI construction; in this task the fields are added with placeholder zero values acceptable for compilation. The root `Model` already caches terminal dimensions in `termWidth` / `termHeight` — see `internal/tui/model.go` line 174 and the `tea.WindowSizeMsg` branch at line 700." This is a codebase-identifier correction (the actual field names on the existing `Model` struct) — it has no spec-bearing semantics. Spec § *Interaction Shape > Layout* mandates "viewport width = terminal width; viewport height = terminal height minus chrome lines" and "`tea.WindowSizeMsg` is forwarded to the embedded viewport so the slice re-flows on resize"; passing the cached terminal dimensions through the `previewModel` constructor is the mechanism by which preview honours that contract on initial open. The fix preserves the same behaviour with the correct identifiers.

In every location, the post-fix plan content traces to a specific spec sentence or paragraph (or is a codebase-fidelity tightening with no spec-bearing semantics). No spec-bearing claim was added, removed, or altered. The user-visible spec contract — the viewport renders the tail by default; preview opens at scroll-tail — is fully preserved.

### Cycles 1, 6, 7, and 8 fix preservation re-verified

- **c1 surface-label-honesty AC at Task 3-5** still pinned: AC bullet "The chrome wording does not promise liveness" plus the test `"chromeLine wording does not promise liveness"` with negative-substring set. Traces to § *Source of Preview Bytes > Surface label honesty*.
- **c6 symbol-alignment fixes** (`internal/tmux/tmux.go`, `c.cmd.Run`) at Tasks 1-5 / 4-9 still applied; trace to § *Multi-pane Rendering Shape > Concrete enumeration call* and § *Architecture Summary*.
- **c7 fixes** (`tea.KeySpace` revert at Task 2-3 Tests; env-var chain `PORTAL_STATE_DIR → XDG_CONFIG_HOME → HOME` at Task 2-7 Edge Cases) still applied; trace to § *Trigger and Entry Point* and § *Cross-cutting Seams > State Package API Reuse > stateDir resolution*.
- **c8 viewport `Home`/`End` preview-owned wiring** at planning.md Phase 2 acceptance line 50, phase-2-tasks.md Task 2-6 (Problem / Do / Acceptance / Tests), and phase-3-tasks.md Task 3-4 acceptance line 183 — still applied; trace to § *Within-preview Key Bindings* (line 65), § *Acceptance Criteria > Within-preview navigation* (line 431), and § *Open Items > Within-preview keymap collisions*.

### Direction 1 (Spec → Plan): Completeness re-verified

Re-walked the specification end-to-end. Every section, decision, edge case, constraint, data-model element, integration point, validation rule, and acceptance bullet maps to a task with adequate depth. Coverage map (consistent with cycles 6, 7, 8, 9):

- **Overview / Use case framing / Side-effect-free contract** → Tasks 4-8, 4-9
- **Trigger and Entry Point** → Tasks 2-3, 2-4, 4-7
- **Interaction Shape** → Tasks 2-3, 2-6, 3-6 (post-c9 termWidth/termHeight wiring is the call-site mechanism for the "viewport width = terminal width" mandate)
- **Source of Preview Bytes** → Tasks 1-1, 1-2, 1-3, 1-6, 3-5 (no-liveness AC), 4-9
- **Multi-pane Rendering Shape** → Tasks 1-5, 1-6, 3-1, 3-2, 3-3, 3-4
- **Within-preview Key Bindings** (`]`, `[`, `Tab`, `Esc`, viewport scroll passthrough including `Home`/`End`) → Tasks 3-2, 3-3, 3-4, 2-6
- **Pane focus on window cycle** → Task 3-3
- **Degenerate cases** → Tasks 3-2, 3-3
- **Position on session re-entry** → Task 2-4
- **Indexing convention** → Task 3-1
- **Model lifecycle** → Task 2-2
- **Chrome Floor / Counter semantics / Concrete enumeration call / Enumeration failure** → Tasks 3-5, 3-6, 3-7, 1-5, 1-6
- **History Depth** (bounded snapshot, slice size N=1000, scroll within bounds, no deeper-history extend, read pipeline, performance budget) → Tasks 1-1, 1-2, 1-3, 1-4, 2-2 (post-c9 includes explicit scroll-tail invariant for initial open), 2-6, 2-7, 4-3
- **Refresh Semantics** (stateless, no timer/polling, fresh disk read on focus events, initial-open ordering, viewport-internal scroll does not re-read, scroll position resets on focus change, dwell behaviour) → Tasks 2-2, 2-3, 2-6, 3-2, 3-3, 3-6
- **Read-Failure Handling / Placeholder / Error string / Non-triggering condition** → Tasks 1-2, 1-3, 4-1, 4-2, 4-3
- **Esc Level Tree** → Task 2-4
- **No In-preview Between-Session Stepping** → Tasks 2-4, 2-6
- **Filter Behaviour with Preview** → Task 2-5
- **Brand-new-session Edge Case** → Tasks 4-1, 4-4
- **Privacy / Threat Model** — correctly absent from plan; reinforced by 4-9 audit's "no out-of-scope additions"
- **Cross-cutting Seams** (Bootstrap, Externally-Killed Session, `_portal-saver`, ANSI Passthrough, State Package API Reuse) → Tasks 2-1, 2-2, 2-7, 3-1, 4-5, 4-6, 4-7, 4-9
- **Architecture Summary** → Tasks across all phases; 4-9 audit pins the no-changes invariant
- **Out of Scope (v1)** → reflected by absence + 4-9 audit
- **Acceptance Criteria** — all bullets pinned to tasks across phases 1–4
- **Open Items Handed to Build Phase** — N=1000, chrome layout, placeholder wording, error-string wording, helper name/location, `_portal-saver` confirmation, keymap collisions — all pinned where build-phase decisions land

### Direction 2 (Plan → Spec): Fidelity re-verified

Walked every task in all four phases. Every Problem / Solution / Do / Acceptance Criteria / Tests / Edge Cases item traces to a specific spec section.

The cycle 9 additions specifically:

- **Task 2-2 Do step 6 "Call `m.viewport.GotoBottom()` so the first frame renders at scroll-tail"** → traces to § *History Depth > Scroll within bounds* (line 122: "the viewport renders the tail by default") and § *Read-Failure Handling > Placeholder > Non-triggering condition* (line 205: "with the viewport at scroll-tail"). Also internally consistent with tasks 3-2 / 3-3 which already call `GotoBottom()` after focus-change reads.
- **Task 2-2 AC "After `SetContent`, the viewport is at scroll-tail (`m.viewport.AtBottom()` returns true)"** → traces to the same spec sections as the Do step, plus Phase 4 task 4-3's existing AC "The viewport is at scroll-tail (bottom) on initial open / focus change." The mechanism note about `bubbles@v1.0.0/viewport.SetContent` not auto-jumping when `YOffset == 0` is codebase fidelity, not a new spec requirement.
- **Task 2-2 Test "it positions the viewport at scroll-tail on initial open"** → traces to § *History Depth > Scroll within bounds* and § *Acceptance Criteria > Edge cases* (line 448: "A pane with fewer than 1000 lines renders all available lines (no placeholder)" — Phase 4 task 4-3 already operationalises this with `AtBottom()` checks).
- **Task 2-3 Do step 4 "use `m.termWidth, m.termHeight`"** → traces to § *Interaction Shape > Layout* by being the mechanism that passes the cached terminal dimensions into the `previewModel` constructor so the viewport can be sized to "terminal width" and "terminal height minus chrome lines" per spec. The codebase identifier correction has no spec-bearing semantic content.

No hallucinated content. Build-phase pinning items (working placeholder/error labels, perf-test env opt-out, defensive `|`-in-window-name handling, `tea.KeySpace` shape, env-var chain, viewport `Home`/`End` preview-owned wiring, scroll-tail invariant on initial open) are reasonable resolutions of the spec's Open Items or codebase-fidelity tightenings — none invent spec content.

### Conclusion

The plan remains a faithful, complete translation of the specification after cycle 9's `GotoBottom()` and `termWidth`/`termHeight` corrections. The scroll-tail invariant on initial open is now explicit in Task 2-2's Do, AC, and Tests, fully aligned with the spec's "viewport renders the tail by default" mandate and with Tasks 3-2 / 3-3 / 4-3's existing scroll-tail invariants. The codebase-identifier correction at Task 2-3 step 4 preserves the same `tea.WindowSizeMsg`-and-cached-dimensions wiring contract from spec § *Interaction Shape > Layout*. No traceability link was severed. No new findings.

**Resolution**: Pending
**Notes**: Recommend marking this cycle complete with status `clean`.
