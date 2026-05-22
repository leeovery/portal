TASK: Treat IsRestoring query failure as marker presumed set (killed-session-resurrects-within-tick-window-2-2)

ACCEPTANCE CRITERIA:
- `(false, err)` treated symmetric to `(true, nil)` — short-circuit, touch `save.requested`, exit 0.
- WARN log with cause emitted under `ComponentDaemon`.
- Existing branches (`(true, nil)` short-circuit and `(false, nil)` proceed) remain unchanged.

STATUS: Complete

SPEC CONTEXT: Spec § `@portal-restoring` Defence mandates `commit-now` short-circuits as no-op when marker is set, to protect bootstrap step 5 Restore / step 4 version-upgrade from partial-skeleton write. Cycle-1 standards review flagged original `IsRestoringSet` query-error semantics as spec gap (fail-open vs fail-closed). Cycle-2 resolution: presume-set on query failure protects in-flight restore at cost of marginally-extended resurrection window, recoverable on daemon's next tick via `save.requested` touch.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/state_commit_now.go:193-203` (switch dispatch with three arms: `err != nil`, `restoring`, fallthrough); shared helper `touchAfterShortCircuit` at lines 226-230.
- Notes:
  - `switch { case err != nil: … case restoring: … }` symmetric. Both short-circuit arms call shared `touchAfterShortCircuit` helper and `return nil` — byte-equivalent post-state.
  - WARN message at line 196: `"isRestoring query failed; presuming @portal-restoring marker set to protect in-flight restore: %v"`.
  - Comment at lines 187-192 documents risk-priority decision, references spec § `@portal-restoring` Defence.
  - `(false, nil)` arm falls through unchanged.

TESTS:
- Status: Adequate
- Coverage: `TestStateCommitNow_TreatsIsRestoringErrorAsMarkerPresumedSet` at `cmd/state_commit_now_test.go:714-780`:
  - exit 0 on `IsRestoring` returning `(false, errors.New("tmux unreachable"))`.
  - `CaptureStructure` and `Commit` call counts both 0.
  - `TouchSaveRequested` called exactly once and `save.requested` exists.
  - `sessions.json` byte-identical to seed.
  - WARN log under `ComponentDaemon` containing underlying error string.
- Notes:
  - Touch-failure-on-error-path not explicitly tested for new arm, but shared `touchAfterShortCircuit` means existing coverage applies structurally.
  - `TestStateCommitNow_ProceedsNormallyWhenRestoringClear` confirms `(false, nil)` happy path unchanged.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`, deps struct nil-field fallback honoured.
- SOLID: Good. Shared `touchAfterShortCircuit` applies DRY across both short-circuit branches without over-abstracting.
- Complexity: Low. Single `switch` adds one branch.
- Modern idioms: Yes. `switch { case … }` dispatch, `%v` for cause inclusion.
- Readability: Good. Comment block explains risk-priority decision before code, references spec section.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Error-branch WARN uses `%v` not `%w` for cause. Since function returns `nil` (exit 0), cause is not propagated up error chain — only logged. Correct given hook subprocess context.
- [idea] Test could also assert `f.restoringCals == 1` to lock the contract that IsRestoring is queried exactly once per invocation.
