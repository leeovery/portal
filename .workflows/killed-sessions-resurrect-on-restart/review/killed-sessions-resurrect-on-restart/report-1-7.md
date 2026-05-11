TASK: killed-sessions-resurrect-on-restart-1-7 — Update CLAUDE.md Server bootstrap section with renumbered 10-step list and EagerSignalHydrate paragraph

ACCEPTANCE CRITERIA:
- CLAUDE.md "Server bootstrap" section updated with renumbered step list and one-paragraph EagerSignalHydrate description.
- Edge cases: preserve "Return is the post-step boundary" framing; renumber subsequent steps; one-paragraph step description.
- Phase 5 task 5-3 supplementary: step-6 references post-Task-1 production primitive (DefaultFIFOSignaler / SendHydrateSignal), with retained mention of state.WriteFIFOSignal explicitly noting it is the seam-bearing variant.

STATUS: Complete

SPEC CONTEXT: Per spec § "Fix 1 → Bootstrap Step Numbering Update" (lines 103–120), orchestrator step list updated to 10 steps with EagerSignalHydrate at position 6. CLAUDE.md update made in same PR. Phase 5 task 5-3 refines step 6 to reference the production primitive (DefaultFIFOSignaler / SendHydrateSignal).

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/CLAUDE.md lines 69–86 ("Server bootstrap" section)
- Notes:
  - 10 spec-ordered steps with EagerSignalHydrate (6) inserted correctly.
  - Line 71 preserves "Return is the post-step boundary, not a numbered step" framing.
  - Line 71 lead-in correctly updated to "ten-step" Orchestrator.
  - Line 78 (EagerSignalHydrate) is a single paragraph covering: production primitive, WriteFIFOSignal-is-seam-only note (task 5-3), @portal-restoring placement invariant (AC8), per-session signaling gap closure, failure posture (WARN under ComponentBootstrap, swallowed, never fatal).
  - Line 84 trailer correctly updated to "After step 10".

TESTS:
- Status: N/A (documentation-only task)

CODE QUALITY:
- Project conventions: Followed.
- Readability: Good. Dense paragraph but consistent with section style.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Line 78 parenthetical bundles two distinct facts; splitting could improve scanability. Optional.
- [idea] "N−1 saved sessions" phrasing assumes spec context; forward-reference could help future readers. Optional.
