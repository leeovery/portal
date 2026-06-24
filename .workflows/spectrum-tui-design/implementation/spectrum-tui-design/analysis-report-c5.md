---
topic: spectrum-tui-design
cycle: 5
total_findings: 7
deduplicated_findings: 7
proposed_tasks: 1
---
# Analysis Report: Spectrum TUI Design (Cycle 5)

## Summary
The fifth analysis cycle on a heavily-consolidated tree confirms the implementation is architecturally clean and spec-faithful: shared chrome (renderJoinedPanel, destructive-confirm, modal footer, row-style helpers, the keymap descriptor, the notice band, the appearance gate) already routes through single owners guarded by dedicated *_consolidation_test.go files, and standards conformance is high across the §2.9 token vocabulary, §12 keymap, §8 edit/destructive/rename modals, §9 peek preview, §10 loading, and §2.5/§2.6 colour gates. Of the seven low-severity findings, six are deliberate/justified/correct-as-is and are discarded; one genuine code-dedup — the byte-for-byte pad-right pair differing only by fill style — clears the bar and is proposed as a single same-package extraction.

## Discarded Findings
- Window-count pluralisation re-implemented across two renderers (duplication, low) — Only two production sites (windowLabel / killWindowCount), below the strict Rule-of-Three, and the agent itself flags the bar as borderline. The shared logic is a single singular/plural ternary; wrapping it in a helper plus two call-site wrappers adds more indirection than the duplication removes, and the only drift risk is a hypothetical future "0 windows" wording. The third instance (swatch.go:237) is acceptable test-fixture copy. Not worth the churn on a 5th-cycle consolidated tree.
- appearance→canvas-mode mapping authored in two packages (duplication, low) — Boundary-justified parallel: newAppearanceGate (internal/tui, unexported) vs swatch's modeFromAppearance (internal/capture, offline validation-only). internal/capture must not reach internal/tui internals; the swatch self-documents the mirror. Deliberate and acceptable; discarded per the boundary-justified-parallel rationale.
- swatch fillCanvas re-derives the owned-canvas outer-fill invariant (duplication, low) — Boundary-justified parallel and a simplified (non-verbatim) copy; the swatch deliberately does not route through tui.Build. The agent's own recommendation is "leave as-is — no action required this cycle." Discarded.
- Session-list title carries the inside-tmux "(current: %s)" suffix (standards, low) — Self-documented "DIVERGENCE FROM SPEC" that preserves pre-existing inside-tmux behaviour under the §1 "Reskin, not rebuild → preserve existing behaviour" mandate; the spec is silent on the inside-tmux case. Justified parity, no functional/contrast impact. The agent's recommendation is "No code change required." Discarded.
- stepLabelTable step-6 dual-mapping leaks into three derived functions (architecture, low) — The step-6 "Restoring sessions" vs "Replaying scrollback" dual-mapping is genuinely two labels the table can only hold one of; correct and guarded by TestMappingCoversAllElevenStepsNoGaps today. Lone crack in an otherwise clean single-source-of-truth design; correct-and-tested-as-is, not worth a data-model refactor this cycle. Discarded.
- keymap descriptor↔dispatch correspondence held by guard tests (architecture, low) — Accepted, documented seam: display derives from the keymapEntry descriptor while dispatch is a hand-coded switch (bubbles/list owns the actual key matching), with correspondence enforced by keymap_dispatch_guard_test.go across all three pages. The agent's recommendation is "No change required for this work unit." Discarded.
