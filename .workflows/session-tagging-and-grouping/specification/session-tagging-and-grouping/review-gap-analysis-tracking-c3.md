---
status: complete
created: 2026-06-07
cycle: 3
phase: Gap Analysis
topic: session-tagging-and-grouping
---

# Review Tracking: session-tagging-and-grouping - Gap Analysis

## Findings

### 1. Pattern B multi-membership contradicts "list items are session items only"

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Grouping Semantics → Pattern B* (lines 123, 130); *TUI Rendering & Toggle Behaviour → Group headers* (lines 176, 180); *Filter Composition → Build note* (line 249)

**Details**:
The spec asserts two requirements that cannot both hold literally, and an implementer must guess how to reconcile them:

- **Pattern B (By Tag):** "A session appears **once under each tag it has**" (lines 123, 130). The header-count rule (line 180) explicitly relies on this: "a multi-tag session is counted under each of its tag headings, so the sum of By-Tag header counts exceeds the live session count."
- **Build note (the load-bearing render-layer decision):** "the `bubbles/list` items are **session items only**, and group headings are injected at render time as visual separators — never as list items" (line 249), reinforced at lines 176 and 251.

If the underlying `bubbles/list` item slice is strictly one item per live session, a session with two tags physically cannot render under two different tag headings — there is only one list item for it, occupying one position in the (single) flat slice. To make a session appear under each of its tags, the flat item slice fed to `bubbles/list` in By Tag mode must contain **one entry per (session, tag) pair** (the same session materialised as N distinct list items), pre-sorted into grouped order so a header can be injected at each group boundary. That is a different item model from By Project / Flat (one item per session).

The spec never states this. The phrase "session items only" reads as "exactly one item per session," which directly conflicts with Pattern B. The unresolved questions an implementer is forced to guess on:

- In By Tag mode, is the item slice per-session (one item) or per-(session, tag) (N items)? Pattern B + the count rule require the latter, but the build note's wording implies the former.
- If per-(session, tag): what is the cursor/selection contract when the *same* session appears as multiple selectable list items (e.g. selecting either copy attaches the same session — presumably yes, but unstated)? Does `g`/`G`/initial-cursor land on a session-instance, and is that fine if the same session is reachable from two positions?
- How does header injection detect a group boundary — by comparing the current item's group key to the previous item's group key in the pre-sorted slice? (The "render time as visual separators" phrasing implies this but never states the ordering precondition that the slice must already be grouped, not the default `bubbles/list` order.)
- How does the Untagged/Unknown catch-all interact with this slice (a session with zero tags contributes exactly one item, pinned into the last group)?

This is the central rendering mechanism of the feature; leaving the item-model unstated would force a design decision at implementation time and risks an implementer building the literal "one item per session" model, which cannot satisfy Pattern B.

**Proposed Addition**:
{leave blank until discussed — likely a clarification in the Build note / Group headers section stating that the render-layer item slice is pre-sorted into grouped order, and that in By Tag mode the slice contains one item per (session, tag) pair (a session legitimately materialises as multiple list items, each selecting/attaching the same underlying session), while By Project / Flat keep one item per session; headers are injected at each group-key boundary in the pre-sorted slice.}

**Resolution**: Approved
**Notes**: Auto-approved (finding_gate_mode=auto). Added an 'Item model' subsection under Filter Composition: render-layer slice is pre-sorted into grouped order with headers injected at group-key boundaries; Flat/By-Project = one item per session, By-Tag = one item per (session,tag) pair; every instance attaches the same underlying session. Clarified the build-note wording ('session instance', invariant = no list item is a header) and cross-linked from Pattern B.

---
