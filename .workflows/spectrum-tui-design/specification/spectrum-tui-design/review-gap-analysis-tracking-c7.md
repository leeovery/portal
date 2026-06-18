---
status: in-progress
created: 2026-06-18
cycle: 7
phase: Gap Analysis
topic: spectrum-tui-design
---

# Review Tracking: spectrum-tui-design - Gap Analysis

## Findings

### 1. §2.5 NO_COLOR carve-out attributes a `⚠`/`✓` glyph fallback the mode bands and success flash do not carry

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §2.5 (NO_COLOR carve-out, line 81); interacts with §11.2 (inline flash), §11.3 (no-tags signpost), §11.4 (command-pending banner), and the §2.9 token-table role description for `state.green` (line 128)

**Details**:
The cycle-6 generalisation of the NO_COLOR carve-out (§2.5, line 81) enumerates four notice surfaces and states they all "drop their tint and lean on the glyph + `⚠`/`✓` + bold/dim to carry the state (§2.2)." But the four surfaces do not uniformly carry a `⚠`/`✓` glyph:

- **§11.2 inline flash — warning variant** does carry `⚠` (line 442). Consistent with the carve-out.
- **§11.2 inline flash — success variant**: §11.2 prose says only "The success variant uses `state.green`" (line 442) and specifies **no glyph**. The §2.9 token table role cell for `state.green` (line 128) lists "`✓` done, success flash," implying the success flash shows `✓`, but the actual band spec in §11.2 never states the success band renders `✓`. Under NO_COLOR — where §2.5 explicitly leans on `✓` to carry the success state — this glyph is load-bearing, yet §11.2 doesn't establish it. An implementer wiring the colourless path has no §11.2 instruction to emit `✓`, so a success flash and a warning flash could be indistinguishable once `state.green`/`accent.orange` are stripped, unless the differing message text alone is relied upon.
- **§11.3 no-tags signpost** and **§11.4 command-pending banner** are `accent.violet` left-bar + text bands (lines 446, 448–449). Neither carries `⚠` or `✓` — their only non-text structural signal is the `▌` left-bar (catalogued in §2.2 as the *selector bar*, not a notice marker) plus the violet colour. §2.5's blanket "lean on the glyph + `⚠`/`✓`" over-attributes a warning/check glyph fallback these two bands do not have.

Why it matters (Minor, not blocking): §2.2's "state is never carried by hue alone" still holds for all four bands because each carries a distinct full-text message (e.g. "No tags yet — …", "Pick a project to run", "… closed externally — list updated"), so the colourless path remains usable. The defect is precision: §2.5 names a `⚠`/`✓`-glyph fallback as the NO_COLOR differentiator for surfaces that don't carry those glyphs, which would send an implementer looking for a `✓` on the success flash and `⚠`/`✓` on the mode bands and finding neither specified. The fix is to make §2.5's claim accurate to what each band actually carries (text + `▌` left-bar + the `⚠` only on the warning flash, plus deciding whether the success flash gets an explicit `✓` glyph in §11.2 to match the §2.9 role cell), not to add new behaviour.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---
