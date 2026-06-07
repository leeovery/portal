---
status: in-progress
created: 2026-06-07
cycle: 1
phase: Input Review
topic: session-tagging-and-grouping
---

# Review Tracking: session-tagging-and-grouping - Input Review

## Findings

### 1. `/` filter as interim tag-exclusion narrowing tool

**Source**: discussion.md "Grouped TUI rendering + toggle behaviour → Decided (cont.)" (lines 403–405): "Tag exclusion — deferred from v1 (user agreed). A filter layer on top of grouping; revisit only if a concrete 'tag I never want to see' pain appears. `/` filter covers basic narrowing meanwhile."
**Category**: Enhancement to existing topic
**Affects**: Non-Goals (Tag exclusion bullet) and/or Filter Composition

**Details**:
The discussion explicitly records the rationale that tag exclusion is acceptable to defer *because* the existing `/` filter "covers basic narrowing meanwhile." The spec lists tag exclusion as a deferred non-goal ("a filter layer on top of grouping. Deferred power-feature.") but drops the load-bearing justification — that there is already an interim workaround. This is the kind of decision-rationale that prevents the deferral being re-litigated later, and it's a useful note for whoever builds/uses v1.

**Current**:
> - **Tag exclusion / hide-a-tag** — a filter layer on top of grouping. Deferred power-feature.

**Proposed Addition**:
Appended to the Tag exclusion non-goal bullet: "The deferral is acceptable because the existing `/` filter covers basic narrowing in the meantime; revisit only if a concrete 'tag I never want to see' pain appears."

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 2. `@portal-tags` reboot capture/restore is the cost line that v1 avoids

**Source**: discussion.md "Anchor → New option: tag at the tmux level" (lines 204–208) and "Decision — hybrid, both layers" (lines 240–242): the per-session `@portal-tags` layer requires capturing the option into `sessions.json` and re-applying on restore (a "modest, bounded schema addition" to the `Session` record, daemon capture, restore re-set), interacting with the `@portal-restoring` window.
**Category**: Enhancement to existing topic
**Affects**: Non-Goals (Per-session tags bullet)

**Details**:
The spec's Non-Goals bullet says per-session tags would be "captured/restored across reboot" but omits *what* that costs and *why it is the cut line*. The discussion is explicit that the directory-anchor v1 was chosen precisely to avoid the schema bump / daemon-capture / restore-re-set work (and the `@portal-restoring` interaction). Capturing the cost contrast preserves why v1 stops where it does and what the deferred layer entails — relevant context for the eventual second-layer work unit.

**Current**:
> - **Per-session tags + `portal open --tag=`** — the eventual hybrid model's second layer (per-session `@portal-tags` tmux option, captured/restored across reboot). Deferred.

**Proposed Addition**:


**Resolution**: Pending
**Notes**:

---

### 3. `--tag` flag also intended on the `x` alias

**Source**: discussion.md "Decision — hybrid, both layers" (line 241): "Settable at launch via a new flag: `portal open --tag=tag1,tag2` (and presumably `x --tag=`)."
**Category**: Enhancement to existing topic
**Affects**: Non-Goals (Per-session tags bullet)

**Details**:
The discussion notes the deferred launch flag is expected on both `portal open --tag=` and the `x` alias. The spec only names `portal open --tag=`. Minor, but since `x` is the user's habitual entry point (per the discussion's "real entry point is the directory / alias" framing), recording that the flag applies to `x` too matters for the deferred feature's scope.

**Current**:
> - **Per-session tags + `portal open --tag=`** — the eventual hybrid model's second layer (per-session `@portal-tags` tmux option, captured/restored across reboot). Deferred.

**Proposed Addition**:


**Resolution**: Pending
**Notes**:

---

### 4. Projects page has no projects → By Project mode is empty

**Source**: Specification analysis (gap surfaced from source blind spot). Discussion covers "By Tag with zero tags" empty state (lines 446–450) but never addresses the analogous "no projects / no resolvable sessions" state for **By Project** mode.
**Category**: Gap/Ambiguity
**Affects**: Mode Persistence & Empty States (Empty states)

**Details**:
The spec carefully specifies the By-Tag-with-zero-tags empty state (degrade-to-flat with a "No tags yet" signpost) but is silent on By Project mode when there are no projects/sessions to group, or when every session is on the lazy-stamp path and has not yet resolved a directory. Because By Project is sold as "delivers value with zero setup," its empty/degenerate rendering is worth pinning down (does it just show the flat list? a signpost? Untagged-equivalent bucket for un-resolvable sessions?). This is a minor blind spot the sources did not cover; flagging for completeness rather than as missed source content.

**Proposed Addition**:


**Resolution**: Pending
**Notes**:

---
