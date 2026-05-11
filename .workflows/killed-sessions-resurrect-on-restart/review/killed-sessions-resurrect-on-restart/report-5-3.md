TASK: killed-sessions-resurrect-on-restart-5-3 — Update CLAUDE.md step-6 to reference post-Task-1 production primitive

ACCEPTANCE CRITERIA:
- Step-6 must reference post-Task-1 production primitive instead of state.WriteFIFOSignal.
- Retained mention of state.WriteFIFOSignal must explicitly note it is the seam-bearing variant for retry-ladder unit tests.

STATUS: Complete

SPEC CONTEXT: Phase 5 cycle 2 — CLAUDE.md step-6 referenced state.WriteFIFOSignal as production primitive after Task 1-1 relocated helpers and Task 4-1 introduced state.DefaultFIFOSignaler / state.SendHydrateSignal as the no-seam production entry.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/CLAUDE.md line 78
- Notes: Reads "...via state.DefaultFIFOSignaler / state.SendHydrateSignal (the no-seam production entry point that pins OpenFIFOForSignal + time.Sleep over the seam-bearing state.WriteFIFOSignal retained for retry-ladder unit tests)." Primary AC met; edge case met. All four referenced symbols verified in internal/state/signal_hydrate.go.

TESTS:
- Status: N/A — doc-only edit.

CODE QUALITY:
- Readability: Good — single parenthetical resolves both production vs seam reference without disrupting surrounding paragraph.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
