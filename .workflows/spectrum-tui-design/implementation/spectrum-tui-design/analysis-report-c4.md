---
topic: spectrum-tui-design
cycle: 4
total_findings: 8
deduplicated_findings: 8
proposed_tasks: 4
---
# Analysis Report: Spectrum TUI Design (Cycle 4)

## Summary
Cycle 4 found the MV reskin to be in strong shape — duplication discipline and spec conformance are both high, with one shared joined-panel primitive backing every modal and the preview. The actionable debt is incremental-reskin residue: two dead/unconsumed rendering layers (a modal-box layer and a bubbles/list help-styling layer) that carry comments describing a transitional state that no longer exists, one leaf-style fork where a delegate re-implements canonical header helpers instead of delegating, and one low-priority mechanical rename of misnamed shared frame primitives. The remaining findings are non-actionable reference-frame-vs-spec-prose reconciliations already resolved at the §15.1 visual gate.

## Discarded Findings
- §10.4 friendly label drops the inline `(N/M)` form (standards, low) — reference-frame-governed per §15.1; behaviour matches spec (counter on active row only, suppressed at M=0). No code defect; resolves at the visual gate, not a code/spec change.
- §7.3 no-matches glyph uses `∅` (U+2205) where spec prose writes `⌀` (U+2300) (standards, low) — decorative glyph choice governed by the authoritative `Filtering — no matches (MV)` reference frame; token/placement/copy all spec-correct. No behavioural defect.
- §11.3/§11.4 info bands sit on a `bg.selection` tint not in spec prose (standards, low) — surface-treatment decision governed by the authoritative `no tags signpost` / `command pending` frames per §15.1; contrast-validated. No correctness risk; resolves at the visual gate.
- Solid-block fg==bg fill style authored inline in the loading bar (duplication, low) — explicitly flagged as a watch-item only; Rule of Three not yet met (two sites, the bar fill and the command chip, which are genuinely distinct fg==bg vs fg!=bg pairings). Not a mandatory extraction.
