---
agent: architecture
cycle: 1
status: findings
findings_count: 1
---

# Architecture Analysis (Cycle 1)

STATUS: findings
FINDINGS_COUNT: 1

## Summary

Architecturally clean — single map entry, comment rewrite, zero bootstrap-context coupling in cmd/hooks.go to break (no `tmuxClient`/`serverWasStarted` reads). Seam usage (`recordingRunner`/`nopRunner`/`bootstrapDeps`/`hooksDeps`) is consistent with cmd/root_test.go's pattern. Only gap is asymmetric test coverage: `hooks list` and `hooks set` assert the no-bootstrap contract but `hooks rm` does not.

## Findings

### Finding 1: hooks rm missing skipTmuxCheck contract assertion

- SEVERITY: low
- FILES: cmd/hooks_test.go:415-717
- DESCRIPTION: The skipTmuxCheck contract is asserted for `hooks list` (hooks_test.go:123-151) and `hooks set` (hooks_test.go:381-412) but not `hooks rm`. The spec's motivating burst pattern is `portal hooks set` from Claude Code's SessionStart, but the allowlist entry covers the entire `hooks` parent chain — `hooks rm` rides the same contract. Without an assertion, a future refactor that special-cases one subcommand (e.g. routing `hooks rm --pane-key` through bootstrap to get fresh live-pane data) would silently regress the cascading-bootstrap mitigation for that path. No abstraction needed — just symmetric coverage of the third public surface.
- RECOMMENDATION: Add one sub-test in `TestHooksRmCommand` mirroring the `hooks set skips tmux bootstrap` block — same `recordingRunner` + `bootstrapDeps` injection, asserts `runner.calls == 0` after `portal hooks rm --on-resume` (with seeded hooks.json and a `mockKeyResolver`).
