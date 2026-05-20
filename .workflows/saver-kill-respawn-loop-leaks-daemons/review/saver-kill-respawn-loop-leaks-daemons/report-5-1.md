TASK: Extract version-scenario and barrier-count test helpers in portal_saver_test.go

ACCEPTANCE CRITERIA:
- Helper-definition site colocated near existing versionScenario type definition
- 24 triplet call sites preserve their sessionPresent boolean per site
- 12 barrier-count sites switch downstream assertions to pointer deref

STATUS: Complete

SPEC CONTEXT: Phase 5 = Analysis Cycle 3 cleanups. Pure test-quality refactor reducing repeated boilerplate around the alive-check / version-mismatch matrix (Phase 1) and barrier-call counter scaffolding. No production-code/behaviour change.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmux/portal_saver_test.go:652-660 — newVersionScenarioClient(t, sessionPresent) colocated immediately after versionScenario type def (603-613) and its run method (615-650).
  - internal/tmux/portal_saver_test.go:662-673 — recordBarrierCalls(t) *int colocated in same helper cluster.
- Notes: recordBarrierCalls correctly delegates to existing installKillSaverFn(t, ...) for t.Cleanup LIFO restore — no duplicate cleanup wiring.

TESTS:
- Status: Adequate (helpers ARE the scaffolding; usage validates them).
- Coverage:
  - 24 newVersionScenarioClient call sites confirmed at lines 690, 714, 740, 761, 782, 804, 832, 940, 1343, 1378, 1666, 1693, 1717, 1737, 1756, 1785, 1805, 1825, 1865, 2141, 2167, 2196, 2225, 2341. sessionPresent booleans preserved per site — 22 use true, 2 use false (832, 1865) matching original &versionScenario{} zero-value semantics.
  - 12 recordBarrierCalls call sites confirmed: 1662, 1689, 1715, 1735, 1754, 1783, 1803, 1823, 1859, 2165, 2194, 2223. All downstream barrier-count assertions use *barrierCalls pointer deref.
- Notes: Five inline &versionScenario{...} literals remain at 856, 886, 1882, 2050 — these match the helper's documented escape hatch ("Tests that need a custom RunFunc wrapper around scenario.run still construct the pieces inline").

CODE QUALITY:
- Project conventions: Followed (t.Helper(), no t.Parallel(), mirrors existing install* seam pattern).
- SOLID: Good — single-responsibility helpers; correctly delegates cleanup.
- Complexity: Low (each ≤10 LOC).
- Modern idioms: Yes (triplet return, pointer-for-shared-counter).
- Readability: Good — both helpers carry contract docstrings.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
