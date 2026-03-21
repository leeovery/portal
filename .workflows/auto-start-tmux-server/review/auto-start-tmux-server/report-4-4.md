TASK: Eliminate package-level mutable DI vars to prevent test isolation risk

ACCEPTANCE CRITERIA:
- Option A: No package-level mutable dep vars exist in cmd/; all deps flow through context
- Option B: Every cmd test file has a documented no-parallel constraint
- All existing tests pass

STATUS: Complete

SPEC CONTEXT: The specification does not directly address DI patterns or test isolation. This task originated from Analysis Cycle 1 (architecture finding) identifying that package-level mutable vars (bootstrapDeps, attachDeps, killDeps, listDeps, openDeps, openTUIFunc) create latent data-race risk if t.Parallel is ever added to cmd tests. The plan explicitly offered two acceptable options: (A) refactor deps through context, or (B) document the constraint.

IMPLEMENTATION:
- Status: Implemented (Option B chosen)
- Location:
  - cmd/attach_test.go:3 — "Tests in this file mutate package-level state (bootstrapDeps, attachDeps) and MUST NOT use t.Parallel."
  - cmd/kill_test.go:3 — "Tests in this file mutate package-level state (bootstrapDeps, killDeps) and MUST NOT use t.Parallel."
  - cmd/list_test.go:3 — "Tests in this file mutate package-level state (bootstrapDeps, listDeps) and MUST NOT use t.Parallel."
  - cmd/open_test.go:3 — "Tests in this file mutate package-level state (bootstrapDeps, openDeps, openTUIFunc) and MUST NOT use t.Parallel."
  - cmd/root_test.go:3 — "Tests in this file mutate package-level state (bootstrapDeps, listDeps) and MUST NOT use t.Parallel."
  - cmd/bootstrap_wait_test.go:3 — "Tests in this file mutate package-level state (bootstrapDeps) and MUST NOT use t.Parallel."
- Notes: Option B was selected. All 6 test files that mutate package-level DI vars have the documented no-parallel constraint as a comment at the top. The 7 remaining test files in cmd/ (alias_test.go, init_test.go, root_integration_test.go, clean_test.go, version_test.go, config_test.go, bootstrap_context_test.go) do not mutate any DI vars and correctly omit the constraint comment. No t.Parallel() calls exist anywhere in cmd/ tests, confirming the constraint is being honored.

TESTS:
- Status: Adequate
- Coverage: The task's test requirement is "All existing cmd tests pass." This is a documentation/constraint task, not a functional change; there is no code behavior to test. The existing test suite validates that the DI var pattern works correctly with sequential execution.
- Notes: The stretch goal (Option A: verify with -race flag that tests can run with t.Parallel) does not apply since Option B was chosen. No new tests are needed for a documentation-only change.

CODE QUALITY:
- Project conventions: Followed — comment placement at file top (line 3, after package declaration) is consistent across all 6 files
- SOLID principles: N/A (no code change)
- Complexity: N/A (no code change)
- Modern idioms: N/A (no code change)
- Readability: Good — comments are clear, specific about which vars are affected, and state the constraint unambiguously
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The comments accurately enumerate which package-level vars each file mutates, which is helpful for maintenance. If a new DI var is added in the future, the relevant test file comments should be updated to include it.
- Option A (context-based DI) remains a valid future improvement if the number of package-level DI vars grows or if parallel test execution becomes desired. The current approach is pragmatic and sufficient.
- No linter or CI check was added to enforce the no-parallel constraint (the task mentioned this as a "consider" item for Option B). A staticcheck or custom lint rule could catch accidental t.Parallel additions in the future, but this is not required by the acceptance criteria.
