---
status: in-progress
created: 2026-06-18
cycle: 1
phase: Traceability Review
topic: Spectrum TUI Design
---

# Review Tracking: Spectrum TUI Design - Traceability

## Summary

Both directions checked against the validated specification read in full.

- **Direction 1 (Spec → Plan, completeness):** Two gaps found — §12.3 (the `Ctrl+↑/↓` swallowed-by-terminal validation caveat + fallback contingency) has no plan home anywhere; and the §15.6 light-mode eyeball of rename / the three edit states / `?` help is deferred by task 1-9 to "later phases" but never lands in any later-phase task (3-4, 3-6, 3-9 carry dark-only frame checks).
- **Direction 2 (Plan → Spec, fidelity):** Clean. Every task traces to a spec section. The Bubble Tea v2 / Lipgloss v2 upgrade (task 1-2) is firmly grounded in §2.6 (`tea.RequestBackgroundColor` → `BackgroundColorMsg` "in Bubble Tea v2") and §14.5 ("Lipgloss v2 removed `AdaptiveColor`, so the light/dark choice is wired explicitly") — not hallucinated; the task even carries the spec-faithful ambiguity escape hatch. No deferred items (§16.3 user-overridable theme + transparency opt-out; v2 tag features; §16.4 cut animated cycling-colour border) appear as plan tasks. §15.1 frame map is fully honoured across the plan (Sessions flat/grouped, Filtering ×3, Projects, Loading, Help, Edit ×3, Kill/Rename incl. the Light kill frame, Preview, Edge states ×4). The VERIFICATION MANDATE is embedded per-UI-task with vhs capture + named-Paper-frame compare + behaviour parity, never as a standalone verification task/phase.

## Findings

### 1. §12.3 validation caveat (Ctrl+↑/↓ terminal-swallow check + fallback) has no plan coverage

**Type**: Missing from plan
**Spec Reference**: §12.3 ("Confirm `Ctrl+↑`/`Ctrl+↓` isn't swallowed by the terminal/tmux during in-terminal validation (§15); fall back to another page key if so.")
**Plan Reference**: N/A (absent). Closest adjacent coverage: task 2-1 binds/verifies `Ctrl+↑/↓` as the paging keymap, and task 1-9 is the in-terminal validation/lock-in gate — but neither carries the §12.3 caveat (verifying the chord survives the terminal/tmux during in-terminal validation, with the explicit fallback-to-another-page-key contingency).
**Change Type**: add-to-task

**Details**: §12.3 is a distinct, spec-mandated validation step with a contingency: during the in-terminal validation gate, confirm `Ctrl+↑/↓` is actually delivered (not intercepted by the terminal or tmux passthrough), and if it is swallowed, fall back to another page key. The plan establishes `Ctrl+↑/↓` as the paging binding (2-1) and runs the in-terminal validation gate (1-9), but the swallow-check + fallback contingency is not stated as an obligation in any task. Without it, an implementer could ship a paging binding that is silently dead on some terminals (the exact risk the caveat names) with no documented remedy. The natural home is task 1-9 (the in-terminal validation / lock-in gate that already runs the real-terminal pass) since §12.3 explicitly scopes the check to the §15 in-terminal validation. The fallback choice itself is an implementation decision, but the *obligation to check and have a fallback* is spec content that must appear.

**Current** (task 1-9 acceptance criteria block, verbatim):
```markdown
**Acceptance Criteria**:
- [ ] **(VISUAL — this task's acceptance IS the in-terminal eyeball pass in BOTH modes; carries its own vhs capture + Paper-frame compare in dark AND light)** The four light surface tints (`bg.selection` `#D0C6F0`, `bg.warning`, `bg.track`, light borders `#C9CDDB`) are pinned to concrete hexes and **eyeballed** against `#e1e2e7` in a real terminal — each reads as a distinct surface, not a wash-out (numeric pass alone is insufficient).
- [ ] Every foreground-on-tint pairing (selected-row name/count/attached on `bg.selection`; `text.on-warning` on `bg.warning`) clears the contrast floor in-terminal; any remedy applied is the more-contrast direction, never a lowered floor, with the pair co-tuned to clear simultaneously.
- [ ] Each pinned tint is derived from its dark anchor + the surface it renders on (recorded), not invented.
- [ ] The lock-in (or bail) decision is recorded explicitly with the final pinned hexes (lock-in) or the failing tint/pairing and rationale (bail).
- [ ] vhs captures of the foundation Sessions screen in dark and light each match `Sessions — Modern Vivid v2` / `Sessions — Modern Vivid (Light)` for layout/structure/colour-role.
```

**Proposed** (task 1-9 acceptance criteria block — add the §12.3 criterion as the final bullet):
```markdown
**Acceptance Criteria**:
- [ ] **(VISUAL — this task's acceptance IS the in-terminal eyeball pass in BOTH modes; carries its own vhs capture + Paper-frame compare in dark AND light)** The four light surface tints (`bg.selection` `#D0C6F0`, `bg.warning`, `bg.track`, light borders `#C9CDDB`) are pinned to concrete hexes and **eyeballed** against `#e1e2e7` in a real terminal — each reads as a distinct surface, not a wash-out (numeric pass alone is insufficient).
- [ ] Every foreground-on-tint pairing (selected-row name/count/attached on `bg.selection`; `text.on-warning` on `bg.warning`) clears the contrast floor in-terminal; any remedy applied is the more-contrast direction, never a lowered floor, with the pair co-tuned to clear simultaneously.
- [ ] Each pinned tint is derived from its dark anchor + the surface it renders on (recorded), not invented.
- [ ] The lock-in (or bail) decision is recorded explicitly with the final pinned hexes (lock-in) or the failing tint/pairing and rationale (bail).
- [ ] vhs captures of the foundation Sessions screen in dark and light each match `Sessions — Modern Vivid v2` / `Sessions — Modern Vivid (Light)` for layout/structure/colour-role.
- [ ] **(§12.3 validation caveat)** During the in-terminal validation pass, confirm `Ctrl+↑`/`Ctrl+↓` (the paging chords bound in task 2-1) are actually delivered to Portal and not swallowed by the terminal or tmux (notably tmux passthrough); if either chord is intercepted, record the finding and choose a fallback page key, and flag that fallback for tasks 2-1 / 3-3 / 4-7 (the descriptor + keymap consumers) to adopt.
```

**Resolution**: Pending
**Notes**: The §12.3 check is scoped by the spec to the §15 in-terminal validation, which is task 1-9's domain; placing it there keeps the single in-terminal pass authoritative. The criterion deliberately records *the obligation to check + have a fallback* (spec content), not a pre-chosen fallback key (an implementation detail the spec leaves open). If reviewers prefer the swallow-check to live with the paging binding instead, task 2-1 is the alternative host — but 1-9 is the better fit because the spec ties the check to the in-terminal gate.

---

### 2. §15.6 light-mode eyeball of rename / edit (three states) / `?` help is deferred but never lands

**Type**: Incomplete coverage
**Spec Reference**: §15.6 ("Eyeball the remaining light modal/edit states (rename, the three edit states, `?` help) and the per-screen token wiring in light mode against `#e1e2e7` — each rendered and visually confirmed, not just numerically verified.")
**Plan Reference**: Task 1-9 (which explicitly defers this: *"the full per-modal light eyeball of rename/edit/help lands with those surfaces in later phases, but the foundation Sessions screen's light wiring is confirmed here"*) and its intended landing sites — task 3-4 (`?` help), task 3-6 (rename), task 3-9 (edit, three states). Those three tasks carry only the dark MV frame compare; none carries a light-mode (`#e1e2e7`) render/eyeball obligation. Task 3-5 (kill) is the one modal that does both modes — but only because a `Kill Confirm Modal (Light)` Paper frame exists; §15.6 explicitly says no further Paper mocks are required and the residual light states are an eyeball, not a frame compare.
**Change Type**: add-to-task

**Details**: §15.6 makes the light-mode coverage of rename, the three edit states, and `?` help an *explicit implementation task at the §15 gate* (rendered + visually confirmed against `#e1e2e7`, not just numerically). Task 1-9 correctly judges the foundation gate cannot eyeball modals that do not exist yet and punts the per-modal light eyeball "to later phases." But the later-phase tasks that build those modals (3-4 help, 3-6 rename, 3-9 edit) only verify against the dark frames — so the deferred §15.6 light eyeball has no home and would fall through the cracks. The fix is to attach the light-mode eyeball obligation to each of those three tasks (the surfaces §15.6 names). This is one finding spanning three tasks; the proposed acceptance-criterion add is shown for each.

**Current** (task 3-6 Rename — VISUAL VERIFICATION acceptance criterion, verbatim):
```markdown
- [ ] VISUAL VERIFICATION: a `vhs` tape drives the TUI to the rename modal (Sessions → `r`) and writes a PNG; compared against `Rename Modal (MV)` for layout/structure/colour-role (header, labelled NEW NAME input with violet cursor, `was:` line, footer).
```

**Proposed** (task 3-6 Rename — VISUAL VERIFICATION criterion + a new §15.6 light-eyeball criterion):
```markdown
- [ ] VISUAL VERIFICATION: a `vhs` tape drives the TUI to the rename modal (Sessions → `r`) and writes a PNG; compared against `Rename Modal (MV)` for layout/structure/colour-role (header, labelled NEW NAME input with violet cursor, `was:` line, footer).
- [ ] LIGHT-MODE EYEBALL (§15.6): the rename modal is rendered in light mode against `#e1e2e7` and visually confirmed in a real terminal — each light token reads correctly and the panel/input/`was:` line stay legible (no further Paper mock required per §15.6; this is the deferred light eyeball task 1-9 punted to this surface, not a frame compare).
```

**Current** (task 3-9 Edit render — VISUAL VERIFICATION acceptance criterion, verbatim):
```markdown
- [ ] VISUAL VERIFICATION: `vhs` tapes drive the TUI to each of the three states — (a) navigate focus on Name, (b) a chip focused, (c) editing a chip in place — and write PNGs; compared against `Edit Modal — navigate (name)`, `Edit Modal — chip focused`, and `Edit Modal — edit in place` respectively for layout/structure/colour-role (chip states, violet field label, `+ add` faint slot, `◉ EDIT MODE` only-while-editing, per-mode footer).
```

**Proposed** (task 3-9 Edit render — VISUAL VERIFICATION criterion + a new §15.6 light-eyeball criterion):
```markdown
- [ ] VISUAL VERIFICATION: `vhs` tapes drive the TUI to each of the three states — (a) navigate focus on Name, (b) a chip focused, (c) editing a chip in place — and write PNGs; compared against `Edit Modal — navigate (name)`, `Edit Modal — chip focused`, and `Edit Modal — edit in place` respectively for layout/structure/colour-role (chip states, violet field label, `+ add` faint slot, `◉ EDIT MODE` only-while-editing, per-mode footer).
- [ ] LIGHT-MODE EYEBALL (§15.6): all three edit-modal states are rendered in light mode against `#e1e2e7` and visually confirmed in a real terminal — chip tint/outline/fill, the violet field label, the `+ add` faint slot, and the `◉ EDIT MODE` indicator each read correctly in light (no further Paper mock required per §15.6; this is the deferred light eyeball task 1-9 punted to this surface, not a frame compare).
```

**Current** (task 3-4 `?` help — VISUAL VERIFICATION acceptance criterion, verbatim):
```markdown
- [ ] VISUAL VERIFICATION: a `vhs` tape opens the Sessions help modal (Sessions → `?`) and writes a PNG; compared against `Sessions — Help Modal (?)` for layout/structure/colour-role (two columns, blue glyphs / strong labels, `? Keybindings` header + right `esc close`, complete keymap on a cleared canvas).
```

**Proposed** (task 3-4 `?` help — VISUAL VERIFICATION criterion + a new §15.6 light-eyeball criterion):
```markdown
- [ ] VISUAL VERIFICATION: a `vhs` tape opens the Sessions help modal (Sessions → `?`) and writes a PNG; compared against `Sessions — Help Modal (?)` for layout/structure/colour-role (two columns, blue glyphs / strong labels, `? Keybindings` header + right `esc close`, complete keymap on a cleared canvas).
- [ ] LIGHT-MODE EYEBALL (§15.6): the `?` help modal is rendered in light mode against `#e1e2e7` and visually confirmed in a real terminal — the two-column glyph/label wiring (`accent.blue` glyph / `text.strong` action) and the header (`text.primary` / `text.detail` `esc close`) read correctly in light (no further Paper mock required per §15.6; this is the deferred light eyeball task 1-9 punted to this surface, not a frame compare).
```

**Resolution**: Pending
**Notes**: This is the natural completion of the chain task 1-9 already set up: 1-9 confirms the foundation Sessions light wiring and the four light surface tints, then explicitly delegates the per-modal light eyeball (rename / edit ×3 / `?` help) to "those surfaces in later phases." The three add-to-task edits give that delegation a concrete landing site on each surface as it is built. Kill (3-5) already covers light because a Light Paper frame exists; the §15.6 residual set deliberately excludes kill and is an eyeball, not a frame compare.

---
