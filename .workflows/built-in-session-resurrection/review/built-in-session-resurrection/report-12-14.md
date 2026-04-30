# Review Report: built-in-session-resurrection-12-14

**TASK**: Idea — add binary-missing and projects.json-absence regression tests in `cmd/clean.go`

**ACCEPTANCE CRITERIA**:
- Use `cleanDeps` injection seam if present.
- Both binary-missing and `projects.json`-absent cases must NOT trigger staleness action.
- Add tests asserting these two paths do NOT trigger staleness action.

**STATUS**: Complete

**SPEC CONTEXT**:
Phase 4 acceptance pins that hook stale-detection is purely a structural-key mismatch against `tmux list-panes -a`. Two non-signals are explicitly called out: (1) the hook command's binary not existing on disk, and (2) absence of `projects.json`.

**IMPLEMENTATION**:
- Status: Implemented
- Location: `/Users/leeovery/Code/portal/cmd/clean_test.go:510-589`
  - Test 1 (lines 510-546): "keeps hook with missing-binary command when structural key is live" — writes hook pointing to `/nonexistent/no-such-binary --resume`, asserts hook is preserved when its structural key matches the live pane list.
  - Test 2 (lines 553-589): "keeps hook when projects.json absent and structural key is live" — intentionally does NOT create `projects.json`, asserts the hook is preserved.
- Notes: Both tests use the existing `cleanDeps` injection seam (preferred path), set `cleanDeps = nil` via `t.Cleanup`, and verify the hooks file's persisted contents (not just stdout).

**TESTS**:
- Status: Adequate
- Coverage: Both spec-pinned non-staleness signals are covered; assertions check both stdout (no removal message) AND hooks.json contents (entry retained). Doc-comments cite the phase-4 acceptance bullet.

**CODE QUALITY**:
- Project conventions: Followed — no `t.Parallel()`, uses `cleanDeps` package-level mutable seam with `t.Cleanup` reset, env vars via `t.Setenv`, helpers reused.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good — comment blocks above each test explain regression intent.
- Issues: None.

**BLOCKING ISSUES**:
- None

**NON-BLOCKING NOTES**:
- [idea] Two regression tests share substantial setup boilerplate; a small `setupCleanTest(t) (projectsFile, hooksFile string)` helper could DRY env-var setup across the file.
