TASK: Replace resolveCommitNowDeps tuple-of-six with *Deps struct (killed-session-resurrects-within-tick-window-2-6)

ACCEPTANCE CRITERIA:
- `resolveCommitNowDeps` returns `*CommitNowDeps`.
- RunE reads via `deps.<Field>`.
- All existing tests pass.
- Edge cases: nil-field fallback; struct-shape test stubs.

STATUS: Complete

SPEC CONTEXT: Architectural cleanup from cycle 1. Original `resolveCommitNowDeps` returned six function values via named-return-with-naked-return, an outlier vs. `bootstrapDeps`/`openDeps`/`hooksDeps` idiom. Adding a seventh seam would have required updating every tuple-shape test.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/state_commit_now.go:65-84` — `CommitNowDeps` struct with six function fields (`ReadIndex`, `CaptureStructure`, `Commit`, `NewClient`, `IsRestoring`, `TouchSaveRequested`), each documented.
  - `cmd/state_commit_now.go:92-124` — `resolveCommitNowDeps() *CommitNowDeps` populates a fully-defaulted struct, overrides each field if non-nil. Always returns non-nil struct with all fields guaranteed non-nil.
  - `cmd/state_commit_now.go:175` — single call site: `deps := resolveCommitNowDeps()`, then `deps.<Field>` consumed.
- Notes:
  - No tuple-shaped return survives; grep shows one call site and one definition.
  - Doc-comment on `commitNowDeps` (lines 37-48) states "non-nil deps struct does NOT have to populate every field".

TESTS:
- Status: Adequate
- Coverage:
  - `installCommitNowDeps` (`state_commit_now_test.go:92-135`) constructs struct-shape `*CommitNowDeps` directly. Many tests leave `ReadIndex` unset, exercising fallback.
  - `TestStateCommitNow_OmitsUnderscorePrefixedSessions` (358-395) constructs partial `*CommitNowDeps` with only `NewClient`, `CaptureStructure`, `Commit` set — strongest nil-fallback regression test.
- Notes: No new tests required per plan. Existing suite uses struct-shape stubs throughout.

CODE QUALITY:
- Project conventions: Followed. Mirrors `bootstrapDeps`/`openDeps`/`hooksDeps`. Package-level `var commitNowDeps *CommitNowDeps` with `t.Cleanup` pattern.
- SOLID: Good. Single-responsibility seams; OCP-friendly (new seam = new field).
- Complexity: Low. `resolveCommitNowDeps` is six trivial if-statements.
- Modern idioms: Function-field struct DI — canonical Go pattern for this codebase.
- Readability: Good. Comprehensive per-field doc comments.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Six-line nil-fallback ladder duplicates the same shape across sibling `*Deps`. Generic `coalesce[T]` helper would DRY this; defer until duplication grows.
- [idea] `IsRestoring` default at line 98 constructs fresh `tmux.DefaultClient()` per call; `NewClient` also returns `tmux.DefaultClient()`. Could share, but `DefaultClient()` is cheap/stateless — harmless.
