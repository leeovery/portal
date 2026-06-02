TASK: Emit bootstrap per-step step complete and overall orchestration complete summaries (portal-observability-layer-5-2)

ACCEPTANCE CRITERIA:
1. Successful bootstrap emits eleven INFO bootstrap: step complete step=<StepName> took=T in step order, correct closed StepName.
2. Successful bootstrap emits one INFO bootstrap: orchestration complete steps=11 warnings=N took=T at Return boundary; warnings == len(warnings).
3. Fatal abort at step 1/2/3/8 returns *FatalError, emits neither the aborting step's step complete nor the orchestration summary.
4. Existing per-step Debug(step entering) breadcrumbs retained as DEBUG.
5. warnings reflects accumulated soft warnings; clean run warnings=0.

STATUS: Complete

SPEC CONTEXT:
Spec § Cycle-level summary — bootstrap is a Sequence cycle. Catalog: bootstrap: step complete step=<StepName> took=T (per step) + bootstrap: orchestration complete steps=11 warnings=N took=T (Return boundary), component bootstrap. step/steps/warnings/took closed keys. Cycle summaries INFO; entering breadcrumbs DEBUG.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/bootstrap/bootstrap.go:259-418 (Run); closed StepName consts :67-79; totalSteps :59; wiring bootstrap_production.go:147,197 (log.For("bootstrap")).
- Notes: orchestrationStart at top after OrDiscard; each step stepStart + Info("step complete","step",<StepName>,log.Took(stepStart)). Closed StepName set is single source for both entering DEBUG and step-complete INFO (normalized SetRestoring/ClearRestoring/SweepOrphanFIFOs). Four fatal steps (1,2,3,8) return o.fatalf immediately (no step-complete/summary for aborting step; ERROR is terminal). :416 Info("orchestration complete","steps",totalSteps,"warnings",len(warnings),log.Took(orchestrationStart)). DEBUG breadcrumbs retained. Only closed keys; log.Took helper.

TESTS:
- Status: Adequate
- Location: cmd/bootstrap/bootstrap_test.go
- Coverage: emitsStepCompletePerStepInOrder (eleven, closed names, order); emitsStepCompleteUnderBootstrapComponent; emitsOrchestrationCompleteOnCleanBootstrap (one, steps=11/warnings=0/took=); orchestrationCompleteReportsAccumulatedWarnings (saver-down + corrupt-index → warnings=2); fatalStep1ShortCircuitsBeforeSummaries; fatalStep8ShortCircuitsBeforeSummary (no ClearRestoring step-complete, preceding seven still fire); retainsEnteringDebugWithNormalizedNames (guards against legacy names).
- Notes: 1:1 with named tests. Behaviour-focused. Fatal steps 2/3 not individually exercised (structurally identical to 1/8). Not over-tested.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; log.Took/For/OrDiscard; closed vocab).
- SOLID: Good — closed StepName consts centralize step= so breadcrumb and summary can't drift.
- Complexity: Acceptable (eleven uniform linear blocks, deliberate readability choice over table-driven).
- Modern idioms: Yes (log.Took, structured attrs).
- Readability: Good — orchestrationStart-vs-stepStart + Return-boundary comments.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Eleven near-identical step blocks; a runStep helper could collapse boilerplate but steps differ enough (bool return, corrupt/err switch, fatal short-circuits) that premature abstraction would obscure per-step branch logic. Current explicit form defensible.
- [idea] Fatal steps 2/3 have no dedicated summary-suppression test (only 1 and 8); abort path structurally identical, low risk; a parametrized fatal-step assertion would close the gap.
