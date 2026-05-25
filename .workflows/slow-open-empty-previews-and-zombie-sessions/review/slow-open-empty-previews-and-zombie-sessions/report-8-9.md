TASK: 8-9 — Document or split NewIsolatedStateEnv to reflect parent-env mutation

STATUS: Complete

SPEC CONTEXT: c2#5 (re-asserted as c3#3) — prior constructor-shaped name masked `t.Setenv` on caller's process before returning subprocess env. Planning row 8-9 recommended option (a) rename.

IMPLEMENTATION:
- Status: Implemented (verb-shaped name; chose `IsolateStateForTest` rather than planning's literal `SetupIsolatedStateEnv`)
- Location: `internal/portaltest/isolated_env.go:56` — `func IsolateStateForTest(t *testing.T) (env []string, stateDir string)`
- Docstring (13-55) explicitly calls out parent-env mutation in `SIDE EFFECT:` paragraph
- Zero residual `NewIsolatedStateEnv` references across tree
- 63 `IsolateStateForTest` references across 14 files
- CLAUDE.md "Test isolation for daemon-spawning tests" already names helper correctly — impl now matches documented contract end-to-end
- Behaviour preserved (signature, ordering, error handling, `*testing.T` enforcement)

TESTS:
- Status: Adequate (rename-only — pre-existing test exercises under new name)
- 2 calls in dedicated test file; 61 other call sites act as integration coverage

CODE QUALITY:
- Project conventions: Followed; verb-shaped name aligns with golang-pro idiom
- SOLID: Good
- Complexity: Low; same control flow
- Modern idioms: `t.Helper()`, `t.Setenv`, `t.Cleanup`, `t.TempDir()`
- Readability: Good; generous docstring

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Planning row 8-9 specified `SetupIsolatedStateEnv`; impl chose `IsolateStateForTest`; both satisfy intent; refresh planning row for traceability
- [idea] `SIDE EFFECT:` docstring leader and verb-shaped name now overlap; once rename has bedded in, leader could be trimmed
