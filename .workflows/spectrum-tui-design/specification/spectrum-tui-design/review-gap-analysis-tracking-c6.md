---
status: in-progress
created: 2026-06-18
cycle: 6
phase: Gap Analysis
topic: spectrum-tui-design
---

# Review Tracking: spectrum-tui-design - Gap Analysis

## Findings

### 1. NO_COLOR vs the owned-canvas surfaces (modals, flash band, preview chrome) is under-defined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §2.5 (NO_COLOR carve-out), §8.1 / §13.5 (modal blank screen), §11.2 (inline flash band), §9.2 (preview chrome)

**Details**:
The canvas reversal added §2.5's carve-out: under `NO_COLOR` Portal "paints no canvas at all" and renders on the terminal's native fg/bg. But several revised sections now describe behaviour *in terms of* the owned canvas without stating the `NO_COLOR` fallback for those same surfaces:

- §8.1 / §13.5: a modal opens by clearing the page "to the owned canvas (mode-matched)." Under `NO_COLOR` there is no owned canvas — what does the blank-screen clear to? Presumably the terminal's native bg, but the spec never says, and "blank screen" plus "no canvas paint" could be read as conflicting (does the page-content still clear? yes — but the implementer is left to infer the surface).
- §11.2: the inline flash band is "on a `bg.warning` tint with `text.on-warning` message text." Under `NO_COLOR` there is no `bg.warning` tint. §2.2 guarantees the `⚠` glyph + bold/dim carries the state, so the band is presumably still legible — but the spec doesn't say the band degrades to glyph-only / no-tint, leaving the `NO_COLOR` rendering of the notice slot unspecified.
- §9.2: "On the owned canvas, the `canvas` colour paints the preview chrome (cyan frame + top bar) and surrounding margins." Under `NO_COLOR` neither the canvas nor the cyan exists; the chrome's `NO_COLOR` appearance is unstated (content is already covered as out-of-theme ANSI, but the *chrome* is not).

The §2.5 carve-out is stated once at the colour-system level but isn't propagated to the canvas-dependent surface descriptions, so an implementer building any of those three surfaces must guess how each degrades. This matters because `NO_COLOR` is now a *second distinct render path* (§1) — the spec asserts it exists but only fully specifies it for the base list, not for the modal/flash/preview surfaces.

**Proposed Addition**:
Appended to §2.5 carve-out paragraph: "This carve-out applies to **every** canvas-dependent surface, not just the base list: under `NO_COLOR` the **modal blank-screen** (§8.1 / §13.5) clears to the terminal's native bg (no painted canvas), **notice bands** (§11.2 inline flash, §11.3/§11.4 mode bands) drop their tint and lean on the glyph + `⚠`/`✓` + bold/dim to carry the state (§2.2), and the **preview chrome** (§9.2) renders colourless on the native bg. The captured preview *content* is already out-of-theme real ANSI regardless (§9.2)."

**Resolution**: Approved
**Notes**: Generalised the carve-out in §2.5 (single chokepoint) rather than editing the three surface sections.

---

### 2. Does NO_COLOR skip the startup detection-or-timeout first-paint gate?

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §2.6 (light/dark detection & first-paint gate), §10.2 (cold-path startup flip), §2.5 (NO_COLOR)

**Details**:
§2.6's "Flip avoidance" gates the first real paint on "detection resolved OR a short timeout (tens of ms)" — and §10.2 mirrors this for the cold-path loading page. The gate exists *solely* to choose which canvas to paint and avoid a visible canvas flip. Under `NO_COLOR` there is no canvas to choose (§2.5: "paints no canvas at all"), so the entire reason for the gate disappears.

§2.6 explicitly states that the `appearance: light | dark` override "pin[s] the mode and skip[s] detection (also skipping the startup detection wait)." `NO_COLOR` has an *even stronger* reason to skip the detection wait — there is no mode-dependent surface at all — yet the spec gives `NO_COLOR` no equivalent statement. An implementer wiring the first-paint gate must therefore decide on their own whether a `NO_COLOR` launch still incurs the (invisible, tens-of-ms) OSC 11 wait. The wait is harmless but pointless under `NO_COLOR`; more importantly the *omission* leaves a behaviour undefined that the appearance-override case took care to define. One sentence ("under `NO_COLOR`, detection and its first-paint wait are skipped — there is no canvas to select") would close it and keep §2.5/§2.6 mutually consistent.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 3. §1 cross-reference to "the one-row-per-delegate pagination invariant (§3.6)" points to the wrong section

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §1 (Canvas ownership bullet), §3.5 / §3.6

**Details**:
The revised §1 "Painted on every cell, two layers" bullet ends: "it must **not** perturb the one-row-per-delegate pagination invariant (§3.6)." But §3.6 is "Borders & framing" — it discusses the canvas being a flat fill rather than a frame and never states the one-row-per-delegate pagination invariant. The pagination invariant itself is defined in §3.5 (Pagination), §4.1 ("Each session is one delegate line — the load-bearing pagination invariant"), and §5.1 ("every list row ... is exactly one delegate line"). A reader following the §3.6 pointer to confirm the invariant the fill must not perturb lands on the wrong content.

This is the cross-ref the canvas fold-in introduced, so it is in-scope for this revisit. Minor, but it is a dangling-target reference precisely on the load-bearing invariant the new canvas fill interacts with — the kind of pointer an implementer will actually follow to confirm the fill is safe. Retarget to §3.5 (or §4.1/§5.1, where the invariant is actually stated).

**Current**:
The fill is an **outer-layer wrap** (not per-delegate-row painting) with the list's width/height budget unchanged — it must **not** perturb the one-row-per-delegate pagination invariant (§3.6).

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 4. Outer-layer canvas fill vs. the flash-band viewport recompute (§11.2 F10) — interaction unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §1 (outer-layer full-terminal fill), §11.2 (F10 flash vs pagination), §3.5 (pagination)

**Details**:
§1 introduces an outer-layer full-terminal fill sized `Height=termH` that "fills full height" and "pads every line to full width." §11.2 (F10) states the flash band is chrome, so when it appears/clears "the list viewport height is recomputed ... so the list never overflows or miscounts rows." These two mechanisms both touch vertical budget but the spec never relates them: the flash band consumes rows from the *list's* height budget (header + band + list + footer must sum to termH), while the outer fill is described as wrapping at the full terminal height with "the list's width/height budget unchanged."

An implementer must reconcile: when the band appears and the list viewport shrinks, the outer fill still paints the full termH (correct — it pads the now-shorter content region), but is the band itself painted by a leaf `.Background` or does it rely on the outer fill? And does inserting/removing the band's row(s) re-drive the same height recompute that pagination already mandates, *underneath* the outer fill? §1 asserts the fill "must not perturb the one-row-per-delegate pagination invariant" but says nothing about the band's dynamic height change, which is the one place the vertical budget actually moves at runtime. The ordering (recompute list height first, then wrap with the full-height fill) is implied but not stated, leaving the layering order an implementer design decision on a path the spec elsewhere treats as load-bearing.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---
