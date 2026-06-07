---
status: complete
created: 2026-06-07
cycle: 2
phase: Gap Analysis
topic: session-tagging-and-grouping
---

# Review Tracking: session-tagging-and-grouping - Gap Analysis

## Findings

### 1. Edit-modal field navigation undefined once a third (Tags) field is added

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Assigning & Managing Tags → Surface — the projects edit modal

**Details**:
The spec says the Tags field is added "alongside Name and Aliases, behaving exactly like the existing alias field." But "exactly like the alias field" defines only the *intra-field* interaction (Enter to add, `x` to remove). It does not define *inter-field navigation* once there are three fields instead of two.

The existing modal cycles focus between exactly two fields with a binary Tab toggle (`model.go:1391-1397`: `if editFocus == editFieldName → editFieldAliases else → editFieldName`). With a third field, the spec leaves unspecified:
- The focus order (Name → Aliases → Tags → Name? or Name → Tags → Aliases?).
- Whether Tab still cycles or whether some other key is introduced.
- Where the Tags field sits visually relative to Aliases.

An implementer must invent the navigation order and Tab semantics. Because the modal's confirm key is also Enter and is currently disambiguated by focus + cursor position, the new field's placement in the focus chain is load-bearing for both navigation and the add-vs-confirm disambiguation. This is a concrete decision an implementer would otherwise guess.

**Proposed Addition**:
Applied. Added three-way field navigation (Name→Aliases→Tags→wrap, Tab cycles, Tags last; Enter=field-scoped add, confirm unchanged).

**Resolution**: Approved
**Notes**: Auto-approved (finding_gate_mode=auto).

---

### 2. `s` toggle behaviour while the `/` filter input is active is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: TUI Rendering & Toggle Behaviour → Toggle key — `s`, single cycle; Filter Composition

**Details**:
The spec carefully establishes that `s` is "verified free on the sessions page" in browse mode (`model.go:1583-1607`) and defines the flatten-on-filter behaviour, but it never states what happens to `s` while the filter input is *actively being typed into*.

In the current handler structure, browse-mode rune keys (`q`, `k`, `n`, etc.) only fire when the filter input is not capturing keystrokes; once filtering is active, characters (including `s`) are consumed by the filter as search text. The spec's flatten-on-filter section says grouping "steps aside" while filtering — which implies cycling modes mid-filter is meaningless — but it does not explicitly say `s` is inert (treated as a literal filter character) while the filter input is focused, nor what happens if the user is in a filtered/flattened state and presses `s` after applying (but not clearing) a filter.

This leaves an implementer to decide whether `s` is swallowed by the filter, ignored, or still cycles the (currently hidden) mode. Worth pinning since the spec is otherwise exhaustive about `s`'s precedence and the filter interaction.

**Proposed Addition**:
Applied. Defined s during active filter input: s is a literal filter char while filter focused; cycle only in browse mode after clearing filter.

**Resolution**: Approved
**Notes**: Auto-approved (finding_gate_mode=auto).

---

### 3. Unknown / Untagged buckets: count display and zero-membership rendering not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Grouping Semantics → Heading label text / Ordering; TUI Rendering → Group headers; Empty States

**Details**:
The spec defines that every group header is "Counted" (e.g. `Portal ··· 2`) and that the pinned **Untagged** (By Tag) and **Unknown** (By Project) buckets behave like other groups. Two small but real rendering questions are left open:

- **Do the Untagged / Unknown headers carry a count too?** The count rule is stated for headers generically, and these are headers, so presumably yes — but the spec never confirms the pinned buckets are counted, and an implementer could reasonably treat the catch-all buckets as special (uncounted) labels.
- **Are the Untagged / Unknown headers rendered when their membership is zero?** In By Tag mode when at least one tag exists but every live session happens to be tagged, is an empty Untagged header shown or suppressed? The "zero tags anywhere" case is fully specified (signpost), but the "some tags exist, no untagged sessions" case is not. The same applies to Unknown when all sessions resolve.

These are minor but would force an implementer to make a visible UX decision (show empty `Untagged ··· 0` header vs omit it) with no guidance.

**Proposed Addition**:
Applied. Stated catch-all buckets carry counts and are suppressed at zero membership; updated Ordering to pin Unknown (By Project) alongside Untagged (By Tag).

**Resolution**: Approved
**Notes**: Auto-approved (finding_gate_mode=auto).

---

### 4. Behaviour of the `s` cycle when there are zero live sessions

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: TUI Rendering & Toggle Behaviour → Toggle key; Mode Persistence & Empty States → By Project with nothing to group

**Details**:
Acceptance criterion 1 is phrased "**Given** any live sessions, **when** the user presses `s` …" and AC for empty states covers "By Project with no live sessions renders an empty list." But the toggle section never states whether `s` still cycles modes (and persists the new mode to `prefs.json`) when the session list is **empty**.

The By-Tag zero-tags signpost and the By-Project empty-list behaviour are each defined as *render* outcomes, but the *cycling/persistence* behaviour with zero sessions is implied rather than stated. Since the cycle is declared "unconditional," the natural reading is that `s` still cycles and persists with zero sessions — but the AC's "Given any live sessions" qualifier introduces just enough doubt that an implementer might gate the toggle on a non-empty list. Worth one sentence confirming the cycle and prefs write are unconditional on session count, matching the unconditional-on-tag-count rule already established.

**Proposed Addition**:
Applied. Stated cycle + prefs write are unconditional on session count (incl. zero); tightened AC1 wording from 'Given any live sessions'.

**Resolution**: Approved
**Notes**: Auto-approved (finding_gate_mode=auto).

---

### 5. Tag removal from a project does not specify live-view refresh timing

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Assigning & Managing Tags; Grouping Semantics; Lifecycle

**Details**:
The spec defines that grouping reads tags live (no per-session tag storage) and that removing the last `work` tag makes the group "cease to exist." It also notes (Lifecycle) that deleted-project tags route a still-live session to Untagged/Unknown. What it does not address is the **in-session refresh contract** between editing tags on the projects page and the grouped sessions view: after a user adds/removes a tag in the edit modal and returns to the sessions page, is the grouped view expected to reflect the change immediately on the next render, and on what trigger?

The TUI already has a sessions-list refresh dispatched on certain page transitions (e.g. the preview-dismiss → sessions transition described in CLAUDE.md), but the spec does not state that a projects-edit → sessions transition must re-read tags / re-group. Because tags are read live from `projects.json` at render time, whether the just-edited tags are visible depends on whether the grouped render re-resolves project records after the edit. This is a concrete behaviour an implementer must decide (re-group on page re-entry vs only on full list reload) and the spec is silent on it.

**Proposed Addition**:
Applied. Added refresh contract: projects-edit → sessions transition dispatches a re-resolve + re-group, mirroring preview-dismiss refresh; no background watch in v1.

**Resolution**: Approved
**Notes**: Auto-approved (finding_gate_mode=auto).

---
