---
status: in-progress
created: 2026-07-22
cycle: 1
phase: Input Review
topic: Persistent No Host Terminal Banner
---

# Review Tracking: Persistent No Host Terminal Banner - Input Review

## Findings

### 1. Causal-precision note — the detection cache is not the defect (banner permanence)

**Source**: Investigation — "Root Cause" § "Causal-precision note (from validation)" (lines 130–131), and "Contributing Factors" § "Async detection" (line 137). Explicitly preserved by the root-cause validation ("the two substantive ones are folded into the causal-precision note above", line 232).

**Category**: Enhancement to existing topic
**Affects**: §2 (Banner Split by Identity Shape) — and/or §8 (Risks)

**Details**:
The investigation carries an explicit validation-derived precision that the spec drops entirely: the banner's *permanence* is produced by the once-only detection cache (`detectDispatched` latch → cached `detectResolved`/`detectResolution`; nothing re-detects, and `rebuildSessionList` does not re-run detection), which is re-read every frame. The missing `IsNull()` gate decides only *whether* that cached unsupported resolution renders as the banner. The exact causal chain is "once-cached unsupported resolution × identity-blind gate", **not** "the gate makes it permanent." The note stresses the cache is **not itself a defect** and stays untouched — the `!IsNull()` gate change alone fully resolves the NULL symptom.

This matters because the spec's §1 describes the banner showing "permanently" / "for the whole picker session," which invites a reader to look for the permanence mechanism. Without the precision note, an implementer could misdiagnose the cache as needing a fix (e.g. re-detection on rebuild) rather than confining the change to the gate. It is a scope-guard: fix the gate, leave the once-only detection cache alone.

**Current**:
(from §2, "Why one gate covers both surfaces")
> The renderer already knows the NULL/named split (`renderUnsupportedHeader` / `unsupportedLeftCluster` branch on `bundleID == ""`); only the *gate* was blind to it. This sub-fix adds the missing discriminator at the gate — it does not change the renderers (the fate of the now-unreachable NULL render branch is Topic 6).

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 2. `WithInitialMultiSelect` is a construction/capture-harness path, deliberately not gated

**Source**: Investigation — H2 basis/evidence (lines 68, 102) and "Code Trace" Locus 2 (line 102: "Live entry point is unique: `WithInitialMultiSelect` (model.go:1006) is construction-time (capture harness), not a keypress").

**Category**: Enhancement to existing topic
**Affects**: §3 (Proactive Multi-Select Entry Block)

**Details**:
The investigation twice flags that the *only* live entry point for opening multi-select is the keypress handler `handleMultiSelectToggle`; `WithInitialMultiSelect` is a construction-time option used by the capture harness, **not** a keypress path. The spec's §3 correctly scopes the gate to "the entry branch of `handleMultiSelectToggle`" but never notes that `WithInitialMultiSelect` is a separate `multiSelectMode = true` setter that is deliberately *not* gated.

Two reasons this is worth capturing: (a) it prevents a scope error where an implementer, seeing another path that sets `multiSelectMode = true`, gates it too; and (b) it confirms the existing multi-select capture fixtures (e.g. `sessions-multi-select-active`) that construct the mode via `WithInitialMultiSelect` are unaffected by the entry block regardless of detection state — relevant to the §7 Visual/testing section, which discusses fixtures but does not mention the multi-select fixtures' interaction with the new gate.

**Current**:
(from §3, "### Change")
> Gate the entry branch of `handleMultiSelectToggle` (`internal/tui/model.go`) on `DetectUnsupported()`. Today the entry branch (`if !m.multiSelectMode { multiSelectMode = true; …mark-on-entry… }`) has **no** detection read; the only unsupported gate is downstream at `decideBurst`'s N≥2 Enter.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---
