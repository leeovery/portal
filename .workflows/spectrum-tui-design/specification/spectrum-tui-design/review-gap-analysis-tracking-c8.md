---
status: complete
created: 2026-06-18
cycle: 8
phase: Gap Analysis
topic: spectrum-tui-design
---

# Review Tracking: spectrum-tui-design - Gap Analysis

## Findings

### 1. Notice-band left-bar glyph is asserted in §2.5 but never pinned in §11 — NO_COLOR legibility leans on an unspecified glyph

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §2.5 (NO_COLOR carve-out), §11 intro, §11.2, §11.3, §11.4 (and §2.2 glyph catalogue)

**Details**:
The cycle-7-reworded §2.5 sentence (line 81) asserts that under `NO_COLOR` the notice bands "carry the state through the message text, **their `▌` left-bar**, and their glyph where they have one." This is load-bearing for the NO_COLOR legibility-by-construction argument: with colour stripped, the band's state must be conveyed by non-colour means, and §2.5 names the `▌` left-bar as one of those means.

However, §11 — the section that actually owns the notice bands — never pins the left-bar glyph. The §11 intro (line 433) calls it a "left-bar accent line"; §11.2 (line 442) calls it an "`accent.orange` left-bar"; §11.3 (line 446) and §11.4 (line 449) call it an "`accent.violet` left-bar." In every case the bar is described by colour and position only, with no glyph specified.

Across the rest of the spec the `▌` glyph is pinned exclusively to selection/selector/caret/cursor roles (§2.2 "`▌` selector bar", §3.3 selection bar, §6.2 selected-row bar, §3.1 caret, §8.4 rename cursor, §10.3 `PORTAL ▌`). So §2.5 is the *only* place that ties the notice-band left-bar to the `▌` glyph, and it does so for a section (§11) that treats the bar as a colour fill, not a glyph.

The consequence: an implementer building the notice bands from §11 would render a solid-colour left bar with no glyph. Under `NO_COLOR` (colour stripped), a glyphless solid bar carries no information — defeating the §2.5 claim that the bar carries state "by construction." Either §11 should pin the notice-band left-bar as a `▌` glyph (so the NO_COLOR argument holds and §2.5/§2.2 are consistent), or §2.5 should not cite the left-bar as a NO_COLOR state-carrier. This is the only mechanism §2.5 relies on that the owning section doesn't establish.

**Proposed Addition**:
(a) §11 intro: pin the bar as a glyph — "Inline notices use a **`▌` left-bar accent line**: ...".
(b) §2.5: "**notice bands** (§11.2 inline flash, §11.3/§11.4 mode bands) drop their tint and bar colour — the band stays present via its `▌` left-bar and position — and carry the **state** through the message text plus, on the flashes, the `⚠`/`✓` glyph and bold/dim (§2.2)".

**Resolution**: Approved
**Notes**: §11 now pins the notice left-bar as `▌` (consistent with §3.3); §2.5 separates structure (bar + position) from state (message text + flash glyph), so the NO_COLOR carrier is accurate.

---

### 2. §2.2 glyph catalogue omits the success `✓` while §11.2 and §2.5 cite §2.2 as the authority for it

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §2.2 (glyph catalogue), §11.2 (success-variant flash), §2.5 (NO_COLOR carve-out)

**Details**:
The cycle-7 edit added a `✓` success glyph to the §11.2 flash band (line 442): "The success variant uses `state.green` with a `✓` glyph (so success stays glyph-distinct from the warning `⚠`, not colour-only — **§2.2**, matching the §2.9 `state.green` role)." §2.5 (line 81) likewise enumerates "the warning flash's `⚠`, the success flash's `✓`" and then cites "(§2.2)."

Both references point to §2.2 as the authority for the principle that success must be glyph-distinct from warning. But §2.2's own enumerated glyph list (line 64) is `` `●` attached, `▌` selector bar, `✕` removable/destructive, spaced uppercase headers, `⚠` warning `` — it includes the warning `⚠` but **omits its paired success `✓`**. So the two sections that establish the success glyph and lean on it for the NO_COLOR/colour-blind guarantee both cite a section whose example list doesn't contain that glyph.

This is consistent at the principle level (§2.2 states "Every state is conveyed by glyph + colour," and §2.9 line 128 correctly anchors `✓` to the `state.green` role + success flash), so it is not a contradiction. But the §2.2 list is presented as a representative catalogue and is the cross-referenced anchor for the warning-vs-success distinction; omitting `✓` from it while including `⚠` makes the §11.2/§2.5 cross-references slightly dangling — an implementer building the glyph-backed state catalogue from §2.2 alone would not include the success tick. Adding `✓` success to §2.2's list would make the catalogue self-consistent with the references that point at it.

**Proposed Addition**:
§2.2 list: "... `⚠` warning, `✓` success."

**Resolution**: Approved
**Notes**:

---
