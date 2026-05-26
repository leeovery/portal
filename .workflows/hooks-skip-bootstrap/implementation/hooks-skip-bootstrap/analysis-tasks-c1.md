---
topic: hooks-skip-bootstrap
cycle: 1
total_proposed: 1
---
# Analysis Tasks: Hooks Skip Bootstrap (Cycle 1)

## Task 1: Consolidate skipTmuxCheck coverage at canonical allowlist site and extend to full hooks chain
status: approved
severity: low
sources: standards, architecture

**Problem**: The new no-bootstrap sub-tests for `hooks set` (cmd/hooks_test.go:381-412) and `hooks list` (cmd/hooks_test.go:123-151) live in the hooks test file rather than alongside the canonical skipTmuxCheck allowlist assertion at cmd/root_test.go:248 (`orchestrator Run not called for skipTmuxCheck commands` inside `TestPersistentPreRunE_CallsEnsureServer`). The spec's Scope explicitly named `cmd/root_test.go` as the location, and the verification line (`go test ./cmd -run TestSkipTmuxCheck`) implies an aggregated surface that does not currently exist. Separately, `hooks rm` rides the same allowlist entry but has no assertion guarding it — a future refactor that special-cases `hooks rm --pane-key` through bootstrap would silently regress the cascading-bootstrap mitigation for that path. Future readers grepping skipTmuxCheck coverage from root_test.go will not discover the hooks-specific guards in their sibling file.

**Solution**: Extend the existing `orchestrator Run not called for skipTmuxCheck commands` sub-test at cmd/root_test.go:248 into a small table that iterates over the full allowlist (including `version`, `hooks list`, `hooks set`, `hooks rm`). Remove the now-redundant `hooks list skips tmux bootstrap` and `hooks set skips tmux bootstrap` sub-tests from cmd/hooks_test.go since the consolidated table covers them. Co-locating all skipTmuxCheck coverage at the canonical allowlist site matches the spec's stated location and produces symmetric coverage of all three `hooks` subcommands.

**Outcome**: Single tabular sub-test at cmd/root_test.go:248 asserts `runner.calls == 0` for every member of the skipTmuxCheck allowlist (version, hooks list, hooks set, hooks rm). cmd/hooks_test.go no longer contains skipTmuxCheck contract assertions. Grepping `skipTmuxCheck` from root_test.go locates the full coverage surface. `hooks rm` is protected against silent regression of the no-bootstrap contract.

**Do**:
1. Open cmd/root_test.go around line 248. Locate the existing sub-test `orchestrator Run not called for skipTmuxCheck commands` inside `TestPersistentPreRunE_CallsEnsureServer`.
2. Convert it into a table-driven sub-test. Each row supplies a `name` and an `argv` slice. Rows: `version` (existing), `hooks list`, `hooks set --on-resume 'true'`, `hooks rm --on-resume`. For each row, run the same `recordingRunner` + `bootstrapDeps`-injection + `runner.calls == 0` assertion currently used.
3. Where `hooks set`/`hooks rm` rows need it, inject the same `hooksDeps` / `mockKeyResolver` / seeded `hooks.json` scaffolding used by the existing cmd/hooks_test.go sub-tests at lines 123-151 and 381-412. Re-use t.Cleanup() to restore package-level mutable state per the project test convention (no t.Parallel()).
4. Delete the `hooks list skips tmux bootstrap` sub-test at cmd/hooks_test.go:123-151 and the `hooks set skips tmux bootstrap` sub-test at cmd/hooks_test.go:381-412 — coverage is now provided by the consolidated table.
5. Run `go test ./cmd -run TestPersistentPreRunE_CallsEnsureServer -v` to confirm all four rows pass.
6. Run `go test ./cmd/...` to confirm no regressions elsewhere in cmd tests.
7. Run `go build -o portal .` to confirm the build still succeeds.

**Acceptance Criteria**:
- The sub-test at cmd/root_test.go:248 is table-driven and contains rows for `version`, `hooks list`, `hooks set`, and `hooks rm`.
- Each row asserts `runner.calls == 0` after the command runs.
- cmd/hooks_test.go no longer contains sub-tests named `hooks list skips tmux bootstrap` or `hooks set skips tmux bootstrap`.
- No test uses `t.Parallel()`.
- All package-level mutable state mutations (bootstrapDeps, hooksDeps) are restored via t.Cleanup().
- `go test ./cmd/...` passes.
- `go build -o portal .` succeeds.

**Tests**:
- The consolidated table sub-test at cmd/root_test.go:248 — all four rows execute and assert no bootstrap orchestrator invocation.
- Existing TestHooksListCommand / TestHooksSetCommand / TestHooksRmCommand behaviour tests in cmd/hooks_test.go continue to pass unchanged (the deletions only remove the skipTmuxCheck-contract sub-tests, not the functional sub-tests).
