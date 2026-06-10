---
status: in-progress
created: 2026-06-10
cycle: 1
phase: Input Review
topic: file-browser-alias-breadcrumb
---

# Review Tracking: File Browser Alias Breadcrumb - Input Review

## Findings

### 1. Work-type categorization decision (keep `bugfix`) is absent from the spec

**Source**: investigation `file-browser-alias-breadcrumb.md` → "Fix Direction → Work-type categorization (decided)" (L346-355), reinforced in "Discussion (findings-review journey)" (L368-370) and "Notes" (L404-405)

**Category**: Enhancement to existing topic

**Affects**: "Problem & Chosen Approach → Decision — remove the file browser feature in full" (specifically alongside the alternatives-rejected discussion)

**Details**:
The investigation records an explicit, reasoned decision made with the user at findings review: keep `work_type: bugfix` even though the fix is a full removal. The rationale is load-bearing — bugfix is the **only** work type with an Investigation phase, which is already complete here; re-typing to `quick-fix` or `feature` would orphan this investigation (those pipelines never read it) and force re-seeding findings by hand. The removal's blast radius (two packages + TUI state-machine surgery) also wants the spec/planning/review rigor that quick-fix skips. The user "explicitly weighed re-typing the work unit and landed on keeping it a bugfix."

The spec captures the A/B/C fix-scope decision faithfully but says nothing about *why this remains a bugfix despite being a deletion*. This is a deliberate decision that a future reader (or planner surprised to find a "bugfix" that adds no behaviour and writes no tests) would otherwise have to re-derive or re-litigate. Worth a sentence in the spec so the framing is anchored.

**Current**:
> ### Decision — remove the file browser feature in full
>
> Decided with the user at findings review (2026-06-09). Because the reported bug sits on unreachable dead code, an in-place audit-fix would only polish code that never runs. The user confirmed they never use the file browser and want it gone. **The fix for this bug is to delete the file-browser feature** — this resolves the latent audit-bypass by removal and reclaims two dead packages. No `SetAndSave` rewiring is performed.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 2. Adjacent UX sharp edge (alias exact-match miss silently degrades to zoxide) not captured

**Source**: investigation `file-browser-alias-breadcrumb.md` → "Notes" (L410-412), with supporting context in "Discussion (findings-review journey)" (L365-367)

**Category**: New topic (out-of-scope / future-work note)

**Affects**: "Problem & Chosen Approach → Scope boundary" (as an explicitly-out-of-scope adjacent observation)

**Details**:
The investigation deliberately records an adjacent fact established during the findings-review side-investigation: the alias system **is** wired, functional, and out-prioritises zoxide in the resolver chain (the user simply had no matching alias for the names they tried). Crucially it also flags a UX sharp edge "for context, not part of this fix": an exact-match alias miss silently degrades to a fuzzy zoxide search, which can open a *different* directory than intended with **no indication the alias was skipped**.

This is explicitly out of scope for the removal — it must NOT become a requirement of this fix. But it is a captured-on-purpose observation that the spec drops entirely. The spec's scope boundary lists what stays unchanged (including the resolver chain) but omits this noted UX edge and the alias-priority-over-zoxide fact. Surfacing it as an explicit "noted, out of scope" line preserves the context for whoever revisits the resolver later and prevents the observation from being lost between investigation and any future planning.

**Proposed Addition**:

**Resolution**: Pending
**Notes**: Investigation marks this explicitly as "not part of this fix, captured for context." If logged, it must be framed as an out-of-scope / future-context note only — never as an acceptance criterion or requirement of the removal.

---
