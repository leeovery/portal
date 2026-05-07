---
status: in-progress
created: 2026-05-06
cycle: 9
phase: Traceability Review
topic: Session Scrollback Preview
---

# Review Tracking: Session Scrollback Preview - Traceability

## Findings

No findings. Cycle 8's library-API correction (viewport `Home`/`End` wiring at planning.md Phase 2 acceptance line 50, phase-2-tasks.md Task 2-6 Problem/Do/Acceptance/Tests, and phase-3-tasks.md Task 3-4 acceptance line 183) preserved the spec's user-visible keymap intent and did not sever any traceability link.

### Cycle 8 fix preservation re-verified

Re-read every cycle 8 fix location alongside the spec sections it touches:

- **planning.md Phase 2 acceptance line 50**. The acceptance now reads: "Viewport scroll keys (`Up`, `Down`, `PgUp`, `PgDn`, `ctrl-u`, `ctrl-d`, `j`, `k`) scroll within the loaded N-line slice via `bubbles/viewport`'s native keymap; `Home` and `End` are wired in preview's own Update (the viewport library's `DefaultKeyMap` does not bind them) by intercepting `tea.KeyHome` → `m.viewport.GotoTop()` and `tea.KeyEnd` → `m.viewport.GotoBottom()`; scroll-up at the top silently no-ops." Spec § *Within-preview Key Bindings* (line 65) and § *Acceptance Criteria > Within-preview navigation* (line 431) both list `Home`/`End` among viewport defaults that scroll the focused pane's loaded N-line slice. The post-fix plan still delivers exactly that user-visible behaviour — only the wiring mechanism is sharpened, not the contract.
- **phase-2-tasks.md Task 2-6 Problem (line 267)**. The post-fix Problem statement now correctly distinguishes which keys the `bubbles@v1.0.0/viewport/keymap.go::DefaultKeyMap` binds versus which keys preview owns. The user-observable behaviour pinned by the Problem statement ("scroll within the focused pane's loaded N-line slice") matches the spec's § *Within-preview Key Bindings > Keymap policy* and § *History Depth > Scroll within bounds* verbatim.
- **phase-2-tasks.md Task 2-6 Do**. The added `tea.KeyHome` / `tea.KeyEnd` interception (`m.viewport.GotoTop()` / `m.viewport.GotoBottom()`) is permitted by spec § *Open Items Handed to the Build Phase > Within-preview keymap collisions* ("if they do, preview's binding wins inside the preview page"). The clarification that `bubbles@v1.0.0/viewport.Model` exposes `Width`/`Height` as exported fields (no `SetSize` method) is a codebase-fidelity tightening of the original Phase 2 task and does not alter spec-bearing content.
- **phase-2-tasks.md Task 2-6 Acceptance Criteria & Tests**. The `Home`/`End` acceptance bullets and tests are now phrased as "via preview-owned `m.viewport.GotoTop()` / `m.viewport.GotoBottom()`" — the user-observable assertion (`YOffset == 0` for Home, `YOffset == max` for End) is byte-identical to what the spec demands. Spec link to § *Acceptance Criteria > Within-preview navigation* is intact.
- **phase-3-tasks.md Task 3-4 acceptance line 183**. The post-fix wording correctly separates library-default scroll keys (which pass through to `m.viewport.Update(msg)`) from `Home`/`End` (which are preview-owned via task 2-6). This matches spec § *Within-preview Key Bindings > Keymap policy*: "Preview owns `]`, `[`, `Tab`, `Esc`. Everything else either passes through to the embedded `bubbles/viewport` (scroll keys above) or is unbound/no-op." The plan now correctly identifies `Home`/`End` as preview-owned (rather than library-passthrough), satisfying the keymap-collision resolution rule from spec § *Open Items*.

In every location, the post-fix plan content traces to a specific spec sentence or paragraph. No spec-bearing claim was added, removed, or altered — only the implementation mechanism (preview-owned interception versus library passthrough) was sharpened to match library reality. The user-visible spec contract — `Home` jumps to top of loaded slice; `End` jumps to bottom — is fully preserved.

### Cycles 1, 6, and 7 fix preservation re-verified

- **c1 surface-label-honesty AC at Task 3-5** still pinned: AC bullet "The chrome wording does not promise liveness" plus the test `"chromeLine wording does not promise liveness"` with negative-substring set. Traces to § *Source of Preview Bytes > Surface label honesty*.
- **c6 symbol-alignment fixes** (`internal/tmux/tmux.go`, `c.cmd.Run`) at Tasks 1-5 / 4-9 still applied; trace to § *Multi-pane Rendering Shape > Concrete enumeration call* and § *Architecture Summary*.
- **c7 fixes** (`tea.KeySpace` revert at Task 2-3 Tests; env-var chain `PORTAL_STATE_DIR → XDG_CONFIG_HOME → HOME` at Task 2-7 Edge Cases) still applied; trace to § *Trigger and Entry Point* and § *Cross-cutting Seams > State Package API Reuse > stateDir resolution*.

### Direction 1 (Spec → Plan): Completeness re-verified

Re-walked the specification end-to-end. Every section, decision, edge case, constraint, data-model element, integration point, validation rule, and acceptance bullet maps to a task with adequate depth. Coverage map (unchanged from cycles 6, 7, 8):

- **Overview / Use case framing / Side-effect-free contract** → Tasks 4-8, 4-9
- **Trigger and Entry Point** → Tasks 2-3, 2-4, 4-7
- **Interaction Shape** → Tasks 2-3, 2-6, 3-6
- **Source of Preview Bytes** → Tasks 1-1, 1-2, 1-3, 1-6, 3-5 (no-liveness AC), 4-9
- **Multi-pane Rendering Shape** → Tasks 1-5, 1-6, 3-1, 3-2, 3-3, 3-4
- **Within-preview Key Bindings** (`]`, `[`, `Tab`, `Esc`, viewport scroll passthrough including `Home`/`End`) → Tasks 3-2, 3-3, 3-4, 2-6 (post-c8 wiring includes explicit `Home`/`End` preview-owned interception so all scroll keys listed in spec line 65 are user-observable)
- **Pane focus on window cycle** → Task 3-3
- **Degenerate cases** → Tasks 3-2, 3-3
- **Position on session re-entry** → Task 2-4
- **Indexing convention** → Task 3-1
- **Model lifecycle** → Task 2-2
- **Chrome Floor / Counter semantics / Concrete enumeration call / Enumeration failure** → Tasks 3-5, 3-6, 3-7, 1-5, 1-6
- **History Depth** (bounded snapshot, slice size N=1000, scroll within bounds, no deeper-history extend, read pipeline, performance budget) → Tasks 1-1, 1-2, 1-3, 1-4, 2-2, 2-6, 2-7, 4-3
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

The cycle 8 additions specifically:

- **Task 2-6 Do step "intercept `tea.KeyHome` and `tea.KeyEnd` explicitly"** → traces to § *Within-preview Key Bindings* (which lists `Home`/`End` as scroll-the-loaded-slice keys) and § *Open Items > Within-preview keymap collisions* (which permits preview's binding to win on collisions or library gaps).
- **Task 2-6 Acceptance Criteria for `Home`/`End` "via preview-owned `m.viewport.GotoTop()` / `m.viewport.GotoBottom()`"** → traces to § *Acceptance Criteria > Within-preview navigation* line 431 ("`Home` / `End` … scroll within the focused pane's loaded N-line slice").
- **Task 2-6 Tests "preview-owned binding"** → same traces as the AC; the implementation-mechanism note ("Recipe note: the test exercises preview's own Home interception … not viewport's default keymap, which has no Home binding in `bubbles@v1.0.0`") is a codebase-fidelity tightening, not a new requirement.
- **Task 3-4 acceptance line 183 "preview-owned via task 2-6 bindings"** → traces to § *Within-preview Key Bindings > Keymap policy* (preview owns its bindings inside the preview page) and § *Open Items > Within-preview keymap collisions* (preview's binding wins on collisions).

No hallucinated content. Build-phase pinning items (working placeholder/error labels, perf-test env opt-out, defensive `|`-in-window-name handling, `tea.KeySpace` shape, env-var chain, viewport `Home`/`End` preview-owned wiring) are reasonable resolutions of the spec's Open Items or codebase-fidelity tightenings — none invent spec content.

### Conclusion

The plan remains a faithful, complete translation of the specification after cycle 8's library-API correction. The viewport `Home`/`End` wiring change preserved the spec's user-visible keymap intent — `Home` jumps to top of the loaded N-line slice, `End` jumps to bottom — by introducing preview-owned interception in place of a (false) library-default passthrough claim. No traceability link was severed. No new findings.

**Resolution**: Pending
**Notes**: Recommend marking this cycle complete with status `clean`.
