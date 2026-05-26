---
agent: standards
cycle: 1
status: findings
findings_count: 1
---

# Standards Analysis (Cycle 1)

STATUS: findings
FINDINGS_COUNT: 1

## Summary

Implementation conforms to the spec's core decisions — map entry added in alphabetical order (cmd/root.go:42), comment block rewritten with inclusion justification and the "intentionally NOT" paragraph dropped (cmd/root.go:17-37), and the required sub-test asserting `runner.calls == 0` for `hooks set` is present (cmd/hooks_test.go:381-412). Only minor drift is that the new sub-tests were placed in cmd/hooks_test.go rather than the spec-named cmd/root_test.go alongside the existing skipTmuxCheck allowlist assertion. No project convention violations: tests do not use t.Parallel(), all package-level mutable state (bootstrapDeps, hooksDeps) is restored via t.Cleanup, and the inverted Phase-4 sub-test (cmd/hooks_test.go:123-151) is the user-approved scope expansion.

## Findings

### Finding 1: New skip-bootstrap sub-test placed in cmd/hooks_test.go instead of cmd/root_test.go

- SEVERITY: low
- FILES: cmd/hooks_test.go:123, cmd/hooks_test.go:381, cmd/root_test.go:248
- DESCRIPTION: The spec's Scope section explicitly names `cmd/root_test.go` as the location for the new sub-test ("`cmd/root_test.go` — `skipTmuxCheck` behavior coverage: Add one sub-test asserting that `portal hooks set …` does not invoke the bootstrap orchestrator (mirroring the existing assertion pattern used for the other allowlisted commands)"). The existing allowlist assertion lives at root_test.go:248 inside `TestPersistentPreRunE_CallsEnsureServer` as the sub-test `orchestrator Run not called for skipTmuxCheck commands` and uses a single `version` argv. The implementation added the new coverage in cmd/hooks_test.go (`hooks list skips tmux bootstrap` at line 123, `hooks set skips tmux bootstrap` at line 381) rather than co-locating with the canonical allowlist assertion. Co-location matters because future readers grepping for skipTmuxCheck coverage from root_test.go will not discover the hooks-specific guard. Spec verification line 28 (`go test ./cmd -run TestSkipTmuxCheck`) further implies an aggregated test surface that does not currently exist under either name — implementation relies on the existing parent test continuing to cover the allowlist while delegating hooks-specific coverage to a sibling file.
- RECOMMENDATION: Either (a) extend root_test.go:248's existing `orchestrator Run not called for skipTmuxCheck commands` sub-test into a small table that iterates over the full allowlist including `hooks set`/`hooks list`, or (b) leave the hooks_test.go sub-tests in place but add a cross-reference comment in root_test.go's existing sub-test pointing to them. Option (a) more closely matches the spec wording.
