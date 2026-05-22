TASK: Collapse pollSessionsJSON / pollSessionsJSONForKill duplication and sessionNames variants (killed-session-resurrects-within-tick-window-2-4)

ACCEPTANCE CRITERIA:
- `pollSessionsJSONForKill` no longer exists.
- Symptom test calls `pollSessionsJSON` with explicit present/absent slices.
- Poll constants declared once across `cmd_test`.
- Single `sessionNames`/name-set helper of shape `map[string]struct{}`.
- `go test ./cmd -run TestStateCommitNow` passes.

STATUS: Complete

SPEC CONTEXT: Internal-quality refactor from cycle-1 analysis. Two implementations of the same two-consecutive-consistent-reads poll loop existed in symptom/reentrancy files; kill-variant predicate was a strict subset. Parallel `sessionNames(idx) map[string]bool` and `indexSessionNameSet(idx) map[string]struct{}` differed only by value type.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/state_commit_now_symptom_integration_test.go:513-545` — single `pollSessionsJSON(ctx, stateDir, mustHave, mustOmit []string)` with `matchesShape` helper at lines 549-561.
  - `cmd/state_commit_now_reentrancy_integration_test.go:283-289` — canonical `sessionNames(idx state.Index) map[string]struct{}`.
  - Poll constants declared once at `cmd/state_commit_now_reentrancy_integration_test.go:70,77,87` (`reentrancyHookBudget`, `reentrancyPollInterval`, `reentrancyConsecutiveReads`) and reused across files.
  - Callers: symptom 3×, daemon-merge 1×, reentrancy 1×.
- Notes:
  - `pollSessionsJSONForKill` and `indexSessionNameSet` both deleted (grep confirms zero hits).
  - The `package cmd` (unit-test) `sessionNames(idx state.Index) []string` at `state_commit_now_test.go:153` is retained — different package (`cmd` vs `cmd_test`), returns `[]string` for ordered slice assertions, explicitly excluded from task scope.

TESTS:
- Status: Adequate
- Coverage: No new tests required per plan. Three integration files exercise the consolidated helper from every kill scenario.
- Notes: Refactor is behaviour-preserving; existing tests are the regression net.

CODE QUALITY:
- Project conventions: Followed. `package cmd_test` external-test pattern preserved; no `t.Parallel()`. Comments explain non-obvious invariants.
- SOLID: Good. `pollSessionsJSON` single responsibility; `matchesShape` factored as pure predicate.
- Complexity: Low.
- Modern idioms: `errors.Is(err, fs.ErrNotExist)`, comma-ok idiom, `context.WithTimeout`.
- Readability: Good. Canonical-helper doc-comment documents shape contract and cross-file ownership.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The `package cmd` unit-test `sessionNames(idx) []string` is now the only remaining variant — out of scope here, but a future rename (e.g. `sessionNamesSlice`) would clarify shape distinction.
- [idea] `matchesShape` is local to symptom file; if a fourth integration file ever needs it, promote alongside `sessionNames`.
