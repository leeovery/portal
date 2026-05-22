TASK: Add `commit-now` failure-path discipline (killed-session-resurrects-within-tick-window-1-3)

ACCEPTANCE CRITERIA:
- On `CaptureStructure` or `Commit` failure: log ERROR under `state.ComponentDaemon`, best-effort touch `save.requested`, exit non-zero, never panic, no Go stack trace.
- If `save.requested` touch itself fails on a failure exit: log WARN, preserve non-zero exit (original failure dominates).
- Edge cases: tmux unreachable, disk error during commit, `save.requested` touch also fails.

STATUS: Complete

SPEC CONTEXT:
Spec § commit-now Failure Behaviour and § save.requested Touch Failure Handling: touch `save.requested` before non-zero exit; touch-on-failure errors are best-effort with original failure dominating; failures must not block the kill. Diagnostics route exclusively through `portal.log`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/state_commit_now.go:209-215` — failure dispatch on Capture/Commit error.
  - `cmd/state_commit_now.go:250-256` — `failCommitNow` helper centralising the discipline.
  - `cmd/state_commit_now.go:22` — `errCommitNowFailed` sentinel (descriptive; identity via `errors.Is`).
  - `cmd/state_commit_now.go:30-35` — `IsSilentExitError` exported for `main.go` stderr suppression.
  - `main.go:26-32` — top-level handler wired to `cmd.IsSilentExitError`.
- Notes: Three discipline points (ERROR log, best-effort touch, wrap-and-return sentinel) encapsulated in one helper reused at both failure sites. `fmt.Errorf("%w: %s: %v", ...)` preserves cause via `errors.Unwrap` while keeping `errors.Is(err, errCommitNowFailed)` working.

TESTS:
- Status: Adequate
- Coverage (`cmd/state_commit_now_test.go`):
  - T17 `ExitsNonZeroWhenCaptureStructureFails` — tmux unreachable.
  - T18 `TouchesSaveRequestedWhenCaptureStructureFails`.
  - T19 `LogsErrorWhenCaptureStructureFails` — ERROR, ComponentDaemon, cause.
  - T20 `ExitsNonZeroWhenCommitFails` — disk error during commit.
  - T21 `TouchesSaveRequestedWhenCommitFails`.
  - T22 `LeavesSessionsJSONByteIdenticalWhenCommitFailsBeforeRename`.
  - T23 `LogsErrorWhenCommitFails` — ERROR with cause.
  - T24 `ExitsNonZeroWhenBothCommitAndTouchFail`.
  - T25 `LogsWarnForTouchFailureAlongsidePrimaryError`.
  - T26 `DoesNotPanicOnAnyFailurePath` — table-driven.
  - T27 `FailureExitErrorIsDetectableSentinel` — `errors.Is`; stderr silent.
  - T27b `FailureExitPreservesCauseViaUnwrap`.
- Notes: All three spec edge cases covered. Negative invariants (no panic, stderr silent, file byte-identical pre-rename) verified.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. `failCommitNow` is single-responsibility and reused at both failure sites.
- Complexity: Low.
- Modern idioms: Yes — `errors.Is`, `%w` wrap.
- Readability: Good. Doc comments cite spec sections and explain cross-package contract with `main.go`.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] `failCommitNow`'s `dir` parameter is only used for the touch call; pre-binding via a closure would shrink surface. Cosmetic.
- [idea] T17/T18/T19 (and T20/T21/T23) split one failure scenario across three tests. Consolidation would save ~60 lines but current granularity is defensible.
