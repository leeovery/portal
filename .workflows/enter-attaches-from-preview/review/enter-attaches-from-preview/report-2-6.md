TASK: enter-attaches-from-preview-2-6 — Rapid-bail replacement resets text and supersedes prior tick via generation bump

ACCEPTANCE CRITERIA:
- Two successive bails: second replaces first; generation increments
- Prior in-flight tick does NOT clear newer flash when fired
- Newer flash's own tick still clears it at deadline
- N successive bails (N ≥ 5) preserve only the latest; all stale ticks silently discarded
- Manual keystroke clear interleaved with bails does not desynchronise generation
- No new production code if 2-1 / 2-3 / 2-5 are correct; documented scoped corrections otherwise

STATUS: Complete

SPEC CONTEXT: Spec § Replacement on rapid successive bails — "latest bail wins, prior pending tick must not clear the new flash early". Mechanism: monotonic uint64 generation counters: setFlash bumps; flashTickMsg self-discriminates; clearFlash leaves gen intact. Task 2-6 is the integration gate proving the three primitives compose.

IMPLEMENTATION:
- Status: Implemented (verification-only task; no new production code)
- Location: internal/tui/sessions_flash_replacement_test.go (new integration tests), supporting production code at internal/tui/sessions_flash.go and internal/tui/model.go:974-1036
- Notes: Bail handler at model.go:990-992 has the correct ordering — `m.setFlash(...)` runs first, then `flashTickCmd(m.flashGen)` captures the POST-bump generation. The flashTickMsg handler at model.go:1023-1036 includes the `if msg.Gen == m.flashGen` guard. setFlash at model.go:785 bumps gen monotonically. clearFlash at model.go:794 leaves flashGen untouched.

TESTS:
- Status: Adequate (exceeds plan's minimum scenarios with thoughtful additions)
- Coverage:
  - Scenario 1: TestFlashReplacement_TwoSuccessiveBailsReflectLatestText
  - Scenario 2: TestFlashReplacement_PriorTickDoesNotClearNewerFlash
  - Scenario 3: TestFlashReplacement_CurrentTickClearsItsOwnFlash
  - Scenario 4: TestFlashReplacement_FiveSuccessiveBailsOnlyLatestSurvives
  - Scenario 5: TestFlashReplacement_ManualClearBetweenBailsLeavesStaleTicksAsNoOps
  - Bonus: TestFlashReplacement_SetFlashBumpsGenByExactlyOnePerCall, TestFlashReplacement_SameNameBailsStillBumpGen
- Helper functions `applyBail` and `applyTick` are small and clearly purposed.

CODE QUALITY:
- Project conventions: Followed.
- SOLID principles: Good.
- Complexity: Low.
- Modern idioms: Good — uses `uint64` for the gen comparison space, raw-string literals for the spec-exact flash wording.
- Readability: Excellent — file header comment summarises the three invariants and points at the probable bug site if anything fails.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
