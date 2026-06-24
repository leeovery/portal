---
topic: spectrum-tui-design
cycle: 2
total_findings: 10
deduplicated_findings: 7
proposed_tasks: 4
---
# Analysis Report: Spectrum TUI Design (Cycle 2)

## Summary
The MV reskin is exceptionally well-consolidated at the leaf layer; cycle-2 findings are residue and one concrete behaviour drift. One high-severity bug stands out: the Projects list omits the `pinArrowOnlyNav` call its Sessions sibling makes, so the vim/uppercase/page-jump keys §12.2 mandates dropping still navigate live (and the existing descriptor-level test checks the wrong layer). The remaining actionable items are consolidation/enforcement: dead per-modal footer/key-hint wrappers retained after a prior consolidation, a keymap descriptor that advertises a "single source of truth" guarantee it only delivers for display (dispatch is an unlinked switch), and a cluster of small enforcement/DRY gaps (colour-literal guard coverage, the command-pending off-descriptor footer path, duplicate separator constants).

## Discarded Findings
- Contrast-validation swatch reimplements the owned-canvas fill + on-canvas styling (duplication, low) — explicitly flagged "leave as-is"; the package boundary is deliberate (swatch must stay independent of `tui.Build`), the duplicated logic is genuinely small, and the agent recommends NOT forcing a shared abstraction now. No code-certain action; treat as a known parallel copy only if `Model.fillCanvas` geometry is ever revised.
- Sessions footer renders word forms where §3.4's copy shows ⏎/␣ glyphs (standards, low) — resolution depends on what the committed `Sessions — Modern Vivid v2` Paper reference frame actually shows (glyph vs word), which is the authoritative §15 visual-verification gate, not a code-certain fix. The word forms were a deliberate, documented decision to keep the footer byte-identical to pre-reskin. Requires user visual sign-off against the frame rather than an automated task; deferred out of this cycle.
