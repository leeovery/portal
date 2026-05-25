TASK: 9-5 — Rename NewIsolatedStateEnv to communicate parent-env mutation in its name

STATUS: Complete

SPEC CONTEXT: c3 standards — `NewIsolatedStateEnv` reads like constructor while in fact mutating caller's process env via `t.Setenv(...)`. Verb-shaped name `IsolateStateForTest` signals at call site. T8-9 was discussion-cycle precursor; T9-5 executes.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/portaltest/isolated_env.go:56` — `func IsolateStateForTest(t *testing.T) (env []string, stateDir string)`
  - `internal/portaltest/doc.go:5` — package doc updated
  - `CLAUDE.md` — references new name
- Godoc (19-24) explicitly documents SIDE EFFECT and verb-shaped naming rationale
- Zero remaining `NewIsolatedStateEnv` references in Go source (grep across *.go returns no matches)
- 18 Go files reference `IsolateStateForTest` across internal/portaltest, internal/restoretest, internal/tmux integration tests, cmd/state_daemon_* tests, cmd/bootstrap/* integration tests

TESTS:
- Status: Adequate
- `internal/portaltest/isolated_env_test.go` covers helper under new name
- Rename refactor; no behavioural change; no new tests warranted

CODE QUALITY:
- Project conventions: Followed; leaf-package discipline preserved
- SOLID: Good; verb-shaped name accurately describes side-effecting action
- Complexity: Low; pure rename
- Modern idioms: Yes; godoc surfaces SIDE EFFECT block in CAPS
- Readability: Good

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [quickfix] `audit-G-test-helpers.md` still references pre-rename `NewIsolatedStateEnv` in prose (already flagged by analysis-standards-c5); mechanical find-and-replace
- [idea] Prior review reports and phase task docs cite old name — historical records, should NOT be retro-rewritten
