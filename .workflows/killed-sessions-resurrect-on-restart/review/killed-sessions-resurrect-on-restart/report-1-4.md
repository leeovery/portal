TASK: killed-sessions-resurrect-on-restart-1-4 — Insert EagerSignalHydrate into Orchestrator between Restore and Clear @portal-restoring

ACCEPTANCE CRITERIA:
- New `EagerSignalHydrate` step runs strictly after step 5 (Restore) and strictly before step 7 (Clear `@portal-restoring`) at position 6.
- Verified by an orchestrator ordering test.
- AC8 invariant preserved: step runs while `@portal-restoring` is still set so daemon `captureAndCommit` suppression remains in force during helper-driven scrollback replay.
- Step is best-effort: non-nil err logged via Warn under ComponentBootstrap, swallowed, never escalates to fatal.

STATUS: Complete

SPEC CONTEXT: Spec § Fix 1 → Placement and Ordering Invariant (must run while @portal-restoring is still set; after restore populates marker set). § Bootstrap Step Numbering Update pins canonical 10-step list with EagerSignalHydrate at position 6. § Failure Posture: never escalates to fatal.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/cmd/bootstrap/bootstrap.go:199 — `EagerSignaler EagerHydrateSignaler` field on Orchestrator
  - /Users/leeovery/Code/portal/cmd/bootstrap/bootstrap.go:289-300 — step 6 invocation between Restore (step 5, lines 275-287) and Clear (step 7, lines 302-306)
  - /Users/leeovery/Code/portal/cmd/bootstrap/bootstrap.go:5-27 — package-doc step list updated
  - /Users/leeovery/Code/portal/cmd/bootstrap/bootstrap.go:221-224 — Run-method docstring updated
  - /Users/leeovery/Code/portal/CLAUDE.md:78 — Server bootstrap section renumbered
- Notes: Logger.Debug step-entry diagnostic "step 6 (EagerSignalHydrate): entering"; soft-warn message "step 6 (EagerSignalHydrate) failed"; @portal-restoring window correctly brackets the step (Set at line 256, EagerSignalHydrate at line 297, Clear at line 304). AC8 invariant preserved.

TESTS:
- Status: Adequate
- Coverage:
  - /Users/leeovery/Code/portal/cmd/bootstrap/bootstrap_test.go:145-169 (TestOrchestratorRun_executesStepsInSpecOrder) asserts the exact 10-element call sequence with `EagerSignalHydrate` at index 5 (position 6).
  - /Users/leeovery/Code/portal/cmd/bootstrap/bootstrap_test.go:336-385 (TestOrchestratorRun_continuesPastEagerSignalHydrateFailure) pins the soft-warn contract.
  - /Users/leeovery/Code/portal/cmd/bootstrap/bootstrap_test.go:912-946 (TestOrchestratorRun_emitsDebugLinePerExecutedStep) pins per-step Debug emission.
  - Sibling tests at lines 285, 387, 465, 535, 622 include EagerSignalHydrate in expected ordering slice.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. Comment block at lines 289-295 explicitly calls out AC8 invariant.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
