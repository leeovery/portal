---
status: complete
created: 2026-05-06
cycle: 8
phase: Traceability Review
topic: Session Scrollback Preview
---

# Review Tracking: Session Scrollback Preview - Traceability

## Findings

No findings. The plan remains a faithful, complete translation of the specification after cycle 7's two integrity fixes.

### Cycle 7 fix preservation re-verified

Cycle 7's integrity-only fixes touched two task lines:

- **Task 2-3 Tests entry (line 147)** — reverted a cycle-1-introduced regression. The test-synthesis shape now reads `tea.KeyMsg{Type: tea.KeySpace}` with a parenthetical pointing at `internal/ui/browser_test.go` as the in-tree precedent. The pre-cycle-7 wording (`Type: tea.KeyRunes, Runes: []rune{' '}` plus a false claim that `tea.KeySpace` does not exist) was incorrect against the bubbletea API.
- **Task 2-7 Edge Cases (first bullet)** — corrected the env-var resolution chain to match `internal/state/paths.go::Dir`. The bullet now reads "consults `$PORTAL_STATE_DIR` first, then falls back through `$XDG_CONFIG_HOME/portal/state` to `$HOME/.config/portal/state`" instead of the misleading `XDG_STATE_HOME` reference.

Re-verified both fixes against traceability:

- **Did not break any spec → plan link.** The spec at § *Trigger and Entry Point* describes Space binding on the Sessions page conceptually; the spec at § *Cross-cutting Seams > State Package API Reuse > stateDir resolution* mandates "stateDir resolved from the existing `internal/state` paths helper (the same source the daemon and bootstrap orchestrator already use)" without committing to specific env-var names. Both pre-fix and post-fix wordings resolve to the same conceptual obligations. The post-fix wordings are now correct against the codebase, sharpening rather than altering the spec→plan links.
- **Did not break any plan → spec link.** Task 2-3's Solution / Do / Acceptance Criteria still trace to § *Trigger and Entry Point* and § *Refresh Semantics > Initial-open ordering*. Task 2-7's Solution / Do / Acceptance Criteria still trace to § *Cross-cutting Seams > State Package API Reuse* and § *Architecture Summary > Test seams / Wiring shape*. Only implementation-mechanic details (test-synthesis identifier, env-var chain) tightened — no spec-bearing content drifted.
- **Did not introduce inconsistency elsewhere.** The corrected env-var chain in 2-7 stays consistent with how the spec characterises "the same source the daemon and bootstrap orchestrator already use" — and `internal/state/paths.go::Dir` is exactly that source. Task 4-9's "no out-of-scope additions" audit still expects `internal/state/paths.go` to remain unchanged, which is unaffected by 2-7's wording correction.

### Cycle 1 traceability fix preservation re-verified

The cycle 1 finding (surface label honesty constraint missing from Task 3-5's chrome AC) remains correctly applied. Task 3-5 still contains:

- Acceptance criterion: "The chrome wording does not promise liveness — no substrings such as `live`, `now showing`, `current`, `realtime`, or other language implying the rendered content is live tmux output. Preview is a snapshot per spec § *Source of Preview Bytes > Surface label honesty*; chrome wording must reflect that."
- Test: `"chromeLine wording does not promise liveness"` with the negative-substring set pinned.

This pin still satisfies the spec's § *Source of Preview Bytes > Surface label honesty* requirement.

### Cycle 6 fix preservation re-verified

Cycle 6's symbol-alignment fixes (`internal/tmux/client.go` → `internal/tmux/tmux.go`, `c.Commander.Run` → `c.cmd.Run`) remain applied at Task 1-5 and Task 4-9. Both wordings continue to trace to § *Multi-pane Rendering Shape > Concrete enumeration call* and § *Architecture Summary*; the corrected file/field names are codebase-fidelity details that do not affect spec traceability.

### Direction 1 (Spec → Plan): Completeness re-verified

Re-read the specification end-to-end with fresh eyes. Every section, decision, edge case, constraint, data-model element, integration point, validation rule, and acceptance bullet maps to a task with adequate depth. Coverage map (unchanged from cycles 6 and 7):

- **Overview / Use case framing / Side-effect-free contract** → Tasks 4-8, 4-9
- **Trigger and Entry Point** (Space binding on Sessions page only, Enter unchanged, Esc dismiss, self-exclusion inheritance, empty-list / no-highlighted-item no-op) → Tasks 2-3, 2-4, 4-7
- **Interaction Shape** (pagePreview peer of pageFileBrowser, one session per open, full-screen layout, viewport sizing, WindowSizeMsg forwarding, scroll preservation across resize) → Tasks 2-3, 2-6, 3-6
- **Source of Preview Bytes** (always-disk, no live capture, snapshot semantics, surface label honesty, no new capture wrapper) → Tasks 1-1, 1-2, 1-3, 1-6, 3-5 (no-liveness AC), 4-9
- **Multi-pane Rendering Shape** (sequential window-grouped, no layout parser, ~95% single-pane real-world distribution rationale) → Tasks 1-5, 1-6, 3-1, 3-2, 3-3, 3-4
- **Within-preview Key Bindings** (`]`, `[`, `Tab`, `Esc`, viewport scroll passthrough, keymap policy) → Tasks 3-2, 3-3, 3-4, 2-6
- **Pane focus on window cycle** (reset to pane 0, no per-window pane focus retention) → Task 3-3
- **Why bidirectional for windows but not panes** — design rationale; no separate task needed; covered by 3-2 / 3-3 acceptance.
- **Degenerate cases** (single-window single-pane silent no-ops) → Tasks 3-2, 3-3
- **Position on session re-entry** (fresh model, no position memory) → Task 2-4 (re-open constructs fresh model)
- **Indexing convention** (0-based internal, raw enumeration order) → Task 3-1
- **Model lifecycle** (fresh `previewModel` per Space, no caching) → Task 2-2
- **Chrome Floor** (Window M of N, Pane X of Y, window name, keystroke hints) → Tasks 3-5, 3-6, 3-7
- **Chrome data source** (tmux structural enumeration once at open, no live re-enumeration) → Tasks 1-5, 3-7
- **Counter semantics** (1-based ordinals in enumeration order, never raw indices) → Task 3-5
- **Concrete enumeration call** (single new read-only listing method on `tmux.Client`) → Task 1-5
- **Enumeration failure handling** (silent no-open) → Tasks 1-6, 2-2, 2-3
- **Above the floor (rejected for v1)** — covered by absence + Out-of-Scope + 4-9 audit
- **History Depth** (bounded snapshot, scrollable within bounds, N=1000) → Tasks 1-1, 2-6, 4-3
- **Slice size / Memory trade-off** → Tasks 1-1, 2-7 (`previewTailLines = 1000` constant)
- **Scroll within bounds / No deeper-history extend** → Task 2-6
- **Read Pipeline** (tail-N idiom, single-fd invariant, line definition, trailing-newline, output handoff, synchronous, performance budget) → Tasks 1-1, 1-2, 1-4, 2-2
- **Refresh Semantics** (stateless, no timer/polling, fresh disk read on focus events) → Tasks 2-6, 3-2, 3-3, 3-6
- **Read Trigger Events** (initial open, `]`, `[`, `Tab`; initial-open ordering; resize not a trigger; between-session step not a trigger) → Tasks 2-2, 2-3, 2-6, 3-2, 3-3, 3-6
- **Viewport-internal Scroll Does Not Re-read** → Task 2-6
- **Scroll Position Resets on Focus Change** → Tasks 3-2, 3-3
- **Dwell Behaviour** (no in-place refresh) — descriptive consequence of "no timer / no polling" and "reads happen only when user acts"; covered by absence of any refresh task and by Tasks 2-6 / 3-2 / 3-3 only reading on user-driven focus changes.
- **Read-Failure Handling** (atomic-rename, deleted .bin, OS read error) → Tasks 1-2, 1-3, 4-1, 4-2
- **Placeholder** (triggering, non-triggering, error string, retry-on-refocus) → Tasks 4-1, 4-2, 4-3
- **Esc Level Tree** (four-level progressive Esc) → Task 2-4
- **No In-preview Between-Session Stepping** (no between-session keymap, list cursor sync non-question, filter set boundary non-question, refresh trigger list excludes between-session step, reversibility, override traceability) → Tasks 2-4, 2-6 (Up/Down/j/k passthrough)
- **Filter Behaviour with Preview** (two-phase filter, no magic Space, canonical user flow) → Task 2-5
- **Brand-new-session Edge Case** (whole-session, per-pane, chrome integrity, no live capture fallback) → Tasks 4-1, 4-4
- **Privacy / Threat Model** (no design response, no opt-out, no redaction, no documentation gating, no safety fallback) — correctly absent from plan; reinforced by 4-9 audit's "no out-of-scope additions".
- **Cross-cutting Seams**:
  - **Bootstrap Restore-Window Interaction** (no preview-aware bootstrap gating, no marker checks) → Task 4-9 audit confirms `cmd/bootstrap/` unchanged.
  - **Externally-Killed Session During Preview** (content reads degrade to placeholder, chrome captured at open stays stable, re-fetch contract on dismiss) → Tasks 4-5, 4-6
  - **`_portal-saver` Self-Reference** (audit at list-population source, no preview-layer blacklist) → Task 4-7
  - **ANSI Passthrough vs Viewport Width** (raw bytes verbatim, no preprocessing/sanitisation/re-wrap) → Task 2-2 (Acceptance, Tests "passes raw ANSI bytes verbatim"), Task 4-9 audit
  - **State Package API Reuse** (`ScrollbackFile`, `SanitizePaneKey`, resolution chain, `stateDir` resolution, tail-N helper packaging) → Tasks 2-1, 2-2, 2-7, 3-1
- **Architecture Summary** (read pipeline, page state machine, within-preview keymap, chrome rendering, no-changes-to list, test seams, return contract, wiring shape) → Tasks across all phases; 4-9 audit pins the no-changes invariant.
- **Out of Scope (v1)** — Each item is reflected by absence + 4-9 audit covers the audit-only enforcement.
- **Acceptance Criteria** — All bullets pinned:
  - Entry & dismiss → Tasks 2-3, 2-4
  - Within-preview navigation → Tasks 3-2, 3-3, 3-5, 2-6
  - Read pipeline → Tasks 1-1, 2-2, 2-6, 3-2, 3-3, 4-8
  - Chrome → Tasks 3-5, 3-7
  - Edge cases → Tasks 4-1, 4-2, 4-3, 4-4, 4-5, 4-6, 1-6, 2-2
  - Filter integration → Task 2-5
  - Side-effect-free contract → Task 4-8
- **Open Items Handed to Build Phase** — Each pinned where build phase decisions land:
  - N value pinned at 1000 → Tasks 1-1, 2-7
  - Chrome layout details → Tasks 3-5, 3-6
  - Placeholder wording → Task 4-1
  - Error string for OS-level read failures → Task 4-2
  - Tail-N helper name and exact package location → Task 1-1
  - Confirm `_portal-saver` exclusion → Task 4-7
  - Within-preview keymap collisions → Task 3-4

### Direction 2 (Plan → Spec): Fidelity re-verified

Walked every task in all four phases. Every Problem / Solution / Do / Acceptance Criteria / Tests / Edge Cases item traces to a specific spec section:

- **Phase 1 tasks (1-1 through 1-6)** — § *Read Pipeline*, § *Read Pipeline > Single-fd invariant*, § *Read Pipeline > Trailing-newline edge case*, § *Read Pipeline > Performance budget*, § *Multi-pane Rendering Shape > Concrete enumeration call*, § *Architecture Summary > Test seams > ScrollbackReader > Return contract*, § *Source of Preview Bytes > Single read path consequences*, § *Refresh Semantics > Read Trigger Events > Initial-open ordering*.
- **Phase 2 tasks (2-1 through 2-7)** — § *Architecture Summary > Test seams*, § *Cross-cutting Seams > State Package API Reuse > stateDir resolution*, § *Architecture Summary > Wiring shape*, § *Refresh Semantics > Initial-open ordering*, § *Multi-pane Rendering Shape > Model lifecycle*, § *Trigger and Entry Point*, § *Esc Level Tree*, § *No In-preview Between-Session Stepping*, § *Filter Behaviour with Preview*, § *Within-preview Key Bindings > Keymap policy*, § *History Depth > Scroll within bounds*, § *Refresh Semantics > Read Trigger Events* ("Resize is not a read trigger"), § *Interaction Shape > Layout*, § *Cross-cutting Seams > ANSI Passthrough vs Viewport Width*, § *Cross-cutting Seams > State Package API Reuse*.
- **Phase 3 tasks (3-1 through 3-7)** — § *Multi-pane Rendering Shape > Chrome Floor (Counter semantics)*, § *Cross-cutting Seams > State Package API Reuse > Resolution chain*, § *Within-preview Key Bindings*, § *Refresh Semantics > Read Trigger Events*, § *Refresh Semantics > Scroll Position Resets on Focus Change*, § *Multi-pane Rendering Shape > Pane focus on window cycle*, § *Multi-pane Rendering Shape > Degenerate cases*, § *Within-preview Key Bindings > Keymap policy*, § *Open Items Handed to the Build Phase (keymap collisions)*, § *Multi-pane Rendering Shape > Chrome Floor (v1 must-show)*, § *Source of Preview Bytes > Surface label honesty* (no-liveness AC from c1), § *Interaction Shape > Layout*, § *Refresh Semantics > Viewport-internal Scroll Does Not Re-read*, § *Cross-cutting Seams > Externally-Killed Session During Preview*, § *Acceptance Criteria > Side-effect-free contract*.
- **Phase 4 tasks (4-1 through 4-9)** — § *Read-Failure Handling > Placeholder*, § *Architecture Summary > Test seams*, § *Read-Failure Handling > Placeholder > Error string*, § *Read-Failure Handling > Placeholder > Non-triggering condition*, § *History Depth*, § *Brand-new-session Edge Case*, § *Cross-cutting Seams > Externally-Killed Session During Preview > Re-fetch contract*, § *Cross-cutting Seams > `_portal-saver` Self-Reference*, § *Out of Scope (v1)*, § *Overview > Side-effect-free contract*, § *Acceptance Criteria > Side-effect-free contract*, § *Architecture Summary > Wiring shape*, § *Cross-cutting Seams > State Package API Reuse*, § *Architecture Summary > No changes to*, § *Multi-pane Rendering Shape > Concrete enumeration call*.

No hallucinated content was found in this cycle. Build-phase pinning items (e.g. `PORTAL_SKIP_PERF` env opt-out for the perf benchmark, defensive handling of `|` in window names, working error-string label `"(unable to read scrollback)"`, working placeholder label `"(no saved content)"`, the `tea.KeySpace` test-synthesis shape, the `PORTAL_STATE_DIR → XDG_CONFIG_HOME → HOME` chain) are reasonable resolutions of the spec's Open Items or codebase-fidelity tightenings, not invented requirements.

### Conclusion

The plan is a faithful, complete translation of the specification. Cycle 7's two integrity fixes (the `tea.KeySpace` regression revert at Task 2-3, and the env-var chain correction at Task 2-7) did not break any traceability link. The c1 surface-label-honesty AC fix is still preserved at Task 3-5. No new findings.
