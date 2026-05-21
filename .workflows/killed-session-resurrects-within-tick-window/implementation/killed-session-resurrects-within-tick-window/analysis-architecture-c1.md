AGENT: architecture
STATUS: findings
FINDINGS_COUNT: 6

FINDINGS:

- FINDING: save.requested touch logic duplicated three times instead of being a single state-package primitive
  SEVERITY: medium
  FILES: cmd/state_commit_now.go:122-132, cmd/state_notify.go:50-59, cmd/state_commit_now_test.go:119-134
  DESCRIPTION: `defaultTouchSaveRequested` (commit-now), the inline body in `stateNotifyCmd`, and the fake `TouchSaveRequested` in `commitNowFixture` are byte-for-byte identical: `OpenFile(O_WRONLY|O_CREATE|O_TRUNC, 0o600)` → `Close` → `os.Chtimes(now, now)`. Two production sites that must stay in lock-step (daemon's dirty-flag contract depends on identical semantics) are coupled by convention rather than code. If notify ever needs to change, commit-now will silently drift. The state package already owns `SaveRequested(dir)` so it is the natural home.
  RECOMMENDATION: Promote the touch into the state package as `state.TouchSaveRequested(dir) error`, sibling to `state.SaveRequested`. Have both `stateNotifyCmd` and `defaultTouchSaveRequested` call it. The test fake can wrap with a counter rather than re-implementing.

- FINDING: IsRestoring query-failure mode silently bypasses the @portal-restoring short-circuit
  SEVERITY: medium
  FILES: cmd/state_commit_now.go:194-206
  DESCRIPTION: `restoring, err := isRestoring(); if err == nil && restoring { skip }`. A non-nil err falls through to the structural-commit happy path. The short-circuit exists to prevent corruption-during-restore; an err means we *cannot prove* the marker is clear, so the safer default is "treat unknown as set" — skip and touch `save.requested`. The current default optimises for "kill removes the session promptly" in a transient-failure scenario at the cost of "do not corrupt an in-flight restore", inverting the spec's stated risk priority in § @portal-restoring Defence.
  RECOMMENDATION: Treat `isRestoring` err as "marker presumed set" — log WARN, touch save.requested, exit 0. Cost is marginally-extended resurrection window on rare transient-tmux-query-failure, recovered on daemon's next tick.

- FINDING: errCommitNowFailed empty-message sentinel creates hidden cross-file coupling
  SEVERITY: low
  FILES: cmd/state_commit_now.go:14-22, cmd/state_commit_now.go:238-244
  DESCRIPTION: The non-zero exit relies on returning `errors.New("")` so that `main.go`'s `err.Error() == ""` guard suppresses stderr. The contract spans two files with no compile-time link.
  RECOMMENDATION: Introduce a named exit-code error type checked via `errors.Is`, or have RunE call `os.Exit(1)` directly after logging.

- FINDING: MigrationLogger interface in internal/tmux duplicates the *state.Logger nil-receiver idiom
  SEVERITY: low
  FILES: internal/tmux/hooks_register.go:174-185, internal/tmux/hooks_register.go:372-401
  DESCRIPTION: A new `MigrationLogger` interface is defined inside `internal/tmux` solely to avoid an import cycle on `*state.Logger`. *state.Logger satisfies it structurally. The `noopMigrationLogger` fallback is also reinvented locally despite `*state.Logger` already having documented nil-receiver no-op semantics. Two parallel logger seams now coexist.
  RECOMMENDATION: If the cycle is real, keep `MigrationLogger` but drop `noopMigrationLogger` — let callers pass `(*state.Logger)(nil)`. If the cycle can be broken, drop the interface entirely.

- FINDING: resolveCommitNowDeps named-return tuple-of-six departs from cmd-package DI idiom
  SEVERITY: low
  FILES: cmd/state_commit_now.go:76-113, cmd/state_commit_now.go:183
  DESCRIPTION: `resolveCommitNowDeps` returns six function values via named-return-with-naked-return. Compared to `bootstrapDeps` / `openDeps` / `hooksDeps` (which use the *Deps struct field directly with a per-field nil check at the call site), this tuple-shaped return is an outlier. A seventh seam adds a tuple slot every test must update.
  RECOMMENDATION: Have resolveCommitNowDeps return a fully-populated `*CommitNowDeps` and have RunE call sites read `deps.ReadIndex` / etc.

- FINDING: failCommitNow drops the underlying cause from the returned error chain
  SEVERITY: low
  FILES: cmd/state_commit_now.go:238-244
  DESCRIPTION: `failCommitNow` logs the cause through logger.Error then returns the empty-message `errCommitNowFailed` — the original error is preserved only in portal.log.
  RECOMMENDATION: Wrap via `fmt.Errorf("%w: %v", errCommitNowFailed, cause)` so `errors.Is` still drives silent-exit while `Unwrap` surfaces the cause. Low priority; bundle with the errCommitNowFailed fix.

SUMMARY: The fix is structurally sound. Two medium items merit attention: save.requested touch is duplicated across three sites in a way that will silently drift, and IsRestoring query-failure default favours kill-promptness over the spec's stated "do not corrupt an in-flight restore" priority.
