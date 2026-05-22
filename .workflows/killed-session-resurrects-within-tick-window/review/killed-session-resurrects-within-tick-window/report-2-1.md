TASK: Promote save.requested touch into state package (killed-session-resurrects-within-tick-window-2-1)

ACCEPTANCE CRITERIA:
- ENOENT parent dir handling.
- mtime tolerance in tests.
- Test fake delegation.
- Near-variant `os.WriteFile` callsite migration.

STATUS: Complete

SPEC CONTEXT: Analysis cycle 1 duplication finding noted save.requested touch sequence (`os.OpenFile` with `O_WRONLY|O_CREATE|O_TRUNC` + close + `Chtimes`) reached three production-shape copies plus a near-variant. Recommendation: promote to package-level helper in `internal/state` and have callers / test mocks delegate.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/state/paths.go:87-99` (`TouchSaveRequested`)
- Production callsites delegated:
  - `cmd/state_notify.go:48` — `state.TouchSaveRequested(dir)`
  - `cmd/state_commit_now.go:99` — production default in `resolveCommitNowDeps`
  - `cmd/state_commit_now.go:197,201,210,214` — short-circuit + failure-path callsites use `deps.TouchSaveRequested`
- No remaining inline `os.OpenFile(..., O_WRONLY|O_CREATE|O_TRUNC, ...)` + `Chtimes` near-variants outside the helper.
- Doc comment is precise: load-bearing open+truncate vs best-effort Chtimes, error-wrap prefix contract, swallowed Chtimes errors.

TESTS:
- Status: Adequate
- Location: `internal/state/paths_test.go:198-271` — `TestTouchSaveRequested`
- Coverage:
  - Happy path: creates file, size 0, mtime within 2s tolerance bracket.
  - Re-touch: bumps mtime on already-present save.requested, asserts truncation to 0 bytes.
  - ENOENT parent dir: error returned, file not created.
- Test fake delegation: `cmd/state_commit_now_test.go:119-126` — `TouchSaveRequested` fake field increments counter, captures dirs, delegates to `state.TouchSaveRequested(dir)`. Matches duplication-finding recommendation.

CODE QUALITY:
- Project conventions: Followed. Helper lives alongside `SaveRequested(dir)` in `paths.go`. Tests in `paths_test.go` under `state_test` external package.
- SOLID: Single responsibility. DI seam preserved at cmd layer via `CommitNowDeps.TouchSaveRequested`.
- Complexity: Trivially low (12 LOC, linear).
- Modern idioms: `os.OpenFile` + `os.Chtimes` correctly; `_ = os.Chtimes(...)` standard intentional-swallow pattern.
- Readability: Excellent doc comment — explains daemon invariant, asymmetric error handling, wrap prefix.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] `cmd/state_notify.go:50` wraps helper's error as `fmt.Errorf("touch save.requested: %w", err)`, but `state.TouchSaveRequested` already prefixes its errors with `"touch save.requested: "`. Result: `"touch save.requested: touch save.requested: <cause>"`. Drop the outer wrap or change context (e.g., `"notify: %w"`).
- [idea] Consider whether a debug-level log via injected logger would aid diagnostics when mtime updates fail silently on exotic filesystems. Out of scope here; flag for future hardening.
