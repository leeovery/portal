---
status: in-progress
created: 2026-06-18
cycle: 1
phase: Gap Analysis
topic: spectrum-tui-design
---

# Review Tracking: spectrum-tui-design - Gap Analysis

## Findings

### 1. `?` help modal — close key contradicts the "no-`?`-toggle" navigation model

**Priority:** Important
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §8.5, §12.1, §13.3

**Details**:
§8.5 binds `?` on every page to open the help modal and shows a right-aligned `esc to close` hint, implying the help modal closes on `Esc`. But the spec never states whether pressing `?` again (while the help modal is open) closes it, nor whether any other key dismisses it. More pointedly, §12.1 lists `Esc` on Sessions as `clear-filter / quit` and on Projects as a bare keybind — so when a help modal is open, does `Esc` close the help modal, or fall through to clear-filter/quit? The modal-open key-routing precedence is unspecified. An implementer must guess whether help is a true modal that swallows all keys until dismissed (the natural reading) and whether `?` is a toggle or open-only. This matters because §12.2 explicitly notes `?` was previously *swallowed* to stop `bubbles/list` toggling its own help — the new binding's open/close/toggle semantics should be pinned so the implementer doesn't reintroduce ambiguous routing.

**Proposed Addition**:
§8.1 — add bullet: "Modals are key-exclusive while open … `Esc` resolves against the modal first." §8.5 — append: help closes on `?` (toggle) or `Esc`; key-exclusive, no fall-through to clear-filter/quit.

**Resolution**: Approved
**Notes**: Applied to §8.1 + §8.5. Also resolves finding 7 (modals consume page keys).

---

### 2. Delete-project confirm modal referenced in keymap but never specified

**Priority:** Important
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §8, §12.1, §6.3

**Details**:
§12.1 (Modals) and §12.1 (Projects keymap) reference a **delete-project** confirmation modal: "delete-project `y`/`n`/`Esc`" and Projects `d delete`. §6.3 also defers `d delete` to the help modal. But §8 (Modals) specifies edit, kill, rename, and help modals in detail — and never specifies the delete-project modal's content, copy, colour treatment, or rendering. §8.1 lists "delete-project" nowhere; only kill is given destructive-modal treatment (§8.3). An implementer building the Projects page has a binding (`d`) and a key contract (`y`/`n`/`Esc`) but no spec for what the delete-project modal looks like or says. Since this is a destructive action analogous to kill, the implementer would have to invent its copy/styling (does it reuse `state.red`? what's the consequence line for deleting a *project record* vs killing a session?). [Findings 7, 8 resolved below: 7 covered by finding 1's §8.1 key-exclusive bullet; 8 expanded by user into the `x`-keymap simplification + footer set — applied to §3.4/§6.3/§11.1/§12/§13.2/§8.5 + mocks.] The spec is careful to distinguish "delete a project record" from "kill a session" (§12.2) but then never describes the former modal.

**Proposed Addition**:
New §8.6 Delete project confirm modal (mirrors kill; project-record consequence copy; `y delete · n cancel · esc`); add delete-project to §8.1 reskin-status line.

**Resolution**: Approved
**Notes**: Added §8.6 + updated §8.1.

---

### 3. Inline-flash band placement vs the loading-page / picker boundary and concurrency rules unstated

**Priority:** Minor
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §11, §11.2, §11.4, §11.3

**Details**:
§11's shared convention places left-bar notices "directly under the title separator, above the section header," and §11.2/§11.3/§11.4 each describe a band that occupies that slot (inline flash, no-tags signpost, command-pending banner). What happens when two would occupy the slot at once is unspecified: e.g. a command-pending banner (§11.4, persistent) is showing on Projects and an inline flash (§11.2, transient) fires — do they stack, replace, or queue? Likewise, can an `accent.orange` warning flash and an `accent.violet` info notice coexist? The "auto-clears" flash has no stated duration or clear trigger beyond "auto-clears." For a single-slot chrome region this collision rule is a real layout decision an implementer would otherwise guess at.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 4. Edit-modal: behaviour when committing a duplicate alias/tag chip is undefined

**Priority:** Minor
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §8.2

**Details**:
§8.2 gives precise falling-out rules: empty-on-commit = delete, empty Name reverts, `Esc` backs out one level. It does not state what happens when a committed chip duplicates an existing alias or tag in the same field. The codebase's tag API is documented (CLAUDE.md) as per-project deduped and case-sensitive — so a duplicate tag is silently deduped — but aliases use a different store and the spec doesn't say whether the edit modal rejects, dedupes, or accepts a duplicate, nor whether any visual feedback occurs. Since §8.2 is an explicit *behaviour change* (not a preserve), the duplicate-on-commit path is in-scope new behaviour that an implementer must decide. (If the intent is "defer entirely to the existing store's dedupe semantics," stating that closes the gap.)

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 5. Loading-page step-label mapping when a friendly group spans zero real progress (empty restore)

**Priority:** Minor
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §10.4, §10.3

**Details**:
§10.4 maps 11 real steps to 5 friendly labels, and notes only `Restoring sessions (N/M)` carries a counter from "the restore loop." On a **cold boot with no saved sessions** (M=0), the `Restoring sessions (N/M)` label would render `(0/0)` or have no per-item ticks, and `Resuming Claude sessions` would cover hydrate helpers that never fire. The spec doesn't state how the tick-list (§10.3: `✓`/`◐`/`·`) behaves for a friendly label whose underlying steps did real work but produced no per-item progress, nor whether `(0/0)` is shown or suppressed. The honest-loading-screen goal (§10.2) makes this edge visible: an implementer needs to know whether to show `Restoring sessions (0/0)`, hide the counter, or relabel. Cold-boot-with-nothing-to-restore is the literal first-run case, squarely in scope.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 6. "Minimum supported terminal size" is referenced as a defined value but never defined; degrade-trigger ordering unspecified

**Priority:** Minor
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §2.7, §3.1, §10

**Details**:
§2.7 says "Define a minimum supported terminal size" and the exact thresholds are "pinned as an implementation detail" (per the orchestrator's note, the narrow/short thresholds are intentionally deferred — not flagged). However, the *ordering and independence* of the degrade steps is also unstated and is not a mere threshold value: §2.7 lists "drop the wordmark → compact wordmark, drop the right-side header hint, truncate names" as a flat list, but doesn't say whether these trigger independently (width-driven vs height-driven) or as a single staged sequence. §3.1 ties narrow degrade to "below the minimum width" but the right-side header hint drop and name truncation could each have their own trigger width. This is a design decision (degrade as discrete stages vs all-at-once) distinct from the deferred numeric thresholds; an implementer would have to invent the staging logic. Flagging only the staging-rule gap, not the deferred numbers.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 7. Edit-modal `e`/`d` key collision risk on Projects page vs modal-open routing

**Priority:** Minor
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §8.2, §11.3, §12.1

**Details**:
§11.3's no-tags signpost copy reads "add tags in the project editor (e) …" — referencing `e` to open the edit modal. §8.2 then uses `e` *inside* the edit modal as an alternate "enter edit mode on a chip" key (`Enter`/`e` on a chip). This is internally consistent (different contexts) but the spec never states the key-routing rule that a modal swallows page-level binds while open — the same gap as finding #1 but for the edit modal specifically. Without an explicit "open modals consume all keys until dismissed" statement, an implementer could leave page binds (`s`, `p`, `n`, etc.) live underneath an open modal. Recommend a single cross-cutting statement (in §8.1 or §13.5) that modals are key-exclusive while open.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 8. Footer keymap example for Sessions omits `s switch view` though §3.4/§5 say it lives in the footer

**Priority:** Minor
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §3.4, §5, §4

**Details**:
§3.4 gives the Sessions footer example as `↑↓ navigate · ⏎ attach · / filter · ␣ preview` plus right-aligned `? help` — and §3.2/§5 both state `s switch view` "lives in the footer only." But the §3.4 example footer omits `s switch view`, and §4 (Flat list) never restates the footer. So the canonical Sessions footer string is given two ways: §3.4's example (no `s`) vs §5's rule (`s` in footer). An implementer copying the §3.4 example would ship a footer missing the `s switch view` hint that §5 mandates. The two should be reconciled to one authoritative footer string (does `s switch view` appear on Flat too, or only once grouping is active?).

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 9. Help-modal content for Projects/Preview deferred, but the help modal is itself "new-substantial" with no fallback content contract

**Priority:** Minor
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §8.5, §14.4

**Details**:
§8.5 says "only Sessions help is mocked, the others follow their audited keymaps at implementation" — Projects/Preview help *content* is intentionally deferred (per the orchestrator's note, not flagged). But the help modal is "new-substantial" (§14.4) and the spec gives no contract for how content is *sourced* — is it hand-authored per page, or generated from the same keymap data structure that drives the footer/§12.1? §14.4 estimates "~60–80 lines" implying hand-authored. If hand-authored, the Projects/Preview content being deferred means those help modals can't be built/verified in the same task as Sessions help, which has task-sequencing implications the planner needs. Flagging the missing source-of-truth statement (generated-from-keymap vs hand-authored), not the deferred copy itself.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 10. Selected-row trailing-slot colour for attached marker and window count not given for the selected state in Flat

**Priority:** Minor
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §4.1, §3.3

**Details**:
§4.1 specifies selected-state colours for the name (`text.on-selection`) and window count (`text.strong` when selected). It does **not** state what the `● attached` marker colour becomes on the selected row. The rule "`state.green` is attached-only" (§2.9) implies the bullet stays `state.green` even when selected — but on the `bg.selection` tint with `text.on-selection` name, whether the green bullet keeps `state.green` (and clears the contrast floor against `bg.selection`, which the §2.3 floor only tests against white/black, not the tint) is unstated. The §2.3 note explicitly scopes the hard gate to white/black backgrounds, so green-on-`bg.selection` legibility is neither gated nor specified. An implementer needs to know the selected-row attached-marker colour and whether its contrast against `bg.selection` is checked.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---
