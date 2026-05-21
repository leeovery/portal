---
topic: killed-session-resurrects-within-tick-window
cycle: 1
total_proposed: 7
---
# Analysis Tasks: killed-session-resurrects-within-tick-window (Cycle 1)

## Task 1: Promote save.requested touch into state package
status: pending
severity: medium
sources: duplication, architecture

**Problem**: The `O_WRONLY|O_CREATE|O_TRUNC` + `Close` + `os.Chtimes(now, now)` sequence on `state.SaveRequested(dir)` is duplicated byte-for-byte across three production-shape sites — `defaultTouchSaveRequested` in `cmd/state_commit_now.go:122-132`, the inline body of `stateNotifyCmd.RunE` in `cmd/state_notify.go:50-60`, and the `TouchSaveRequested` fake body in `cmd/state_commit_now_test.go:119-134`. A near-variant (`os.WriteFile(state.SaveRequested(...), nil, 0o600)`) also exists at `cmd/state_commit_now_daemon_merge_integration_test.go:151`. The two production sites must stay in lock-step (the daemon's dirty-flag contract depends on identical semantics) but are coupled by convention rather than code. The `state` package already owns `SaveRequested(dir)`, making it the natural home.

**Solution**: Promote the touch sequence to a `state.TouchSaveRequested(dir string) error` helper sibling to `state.SaveRequested`. Have both `cmd/state_notify.go` and `cmd/state_commit_now.go`'s `defaultTouchSaveRequested` delegate to it. Have the test fake wrap the helper with a counter rather than re-implementing the body.

**Outcome**: Single source of truth for the save.requested touch contract. The two production sites cannot drift.

**Do**:
1. Add `state.TouchSaveRequested(dir string) error` to the `state` package alongside `SaveRequested` (likely `internal/state/paths.go` or sibling). Implementation: `OpenFile(SaveRequested(dir), O_WRONLY|O_CREATE|O_TRUNC, 0o600)` → `Close` → `os.Chtimes(SaveRequested(dir), now, now)`. Return wrapped error on any step.
2. In `cmd/state_commit_now.go:122-132`, replace `defaultTouchSaveRequested` body with `return state.TouchSaveRequested(dir)`. Keep the function name as a seam if tests still need injection.
3. In `cmd/state_notify.go:50-60`, replace the inline `OpenFile/Close/Chtimes` block with `state.TouchSaveRequested(dir)`.
4. In `cmd/state_commit_now_test.go:119-134`, change the `TouchSaveRequested` fake to either call the real `state.TouchSaveRequested` and increment a counter, or record the call and assert against the side effect on disk.
5. In `cmd/state_commit_now_daemon_merge_integration_test.go:151`, replace the `os.WriteFile` near-variant with `state.TouchSaveRequested(dir)`.

**Acceptance Criteria**:
- `state.TouchSaveRequested` exists and is exported from `internal/state`
- `cmd/state_commit_now.go` and `cmd/state_notify.go` both call `state.TouchSaveRequested` (no duplicated body in either)
- The cmd-package test fake no longer re-implements the touch body
- The daemon-merge integration test uses `state.TouchSaveRequested`
- All existing tests still pass; on-disk behaviour unchanged

**Tests**:
- New unit test in `internal/state` covering `TouchSaveRequested` happy path (file created, mtime equals now within tolerance) and ENOENT-parent-dir error path
- Existing cmd-package tests continue to pass without modification beyond the fake's body

---

## Task 2: Treat IsRestoring query failure as "marker presumed set"
status: pending
severity: medium
sources: architecture

**Problem**: At `cmd/state_commit_now.go:194-206`, the `@portal-restoring` short-circuit reads `restoring, err := isRestoring(); if err == nil && restoring { skip }`. A non-nil err falls through to the structural-commit happy path. The short-circuit exists to prevent corruption-during-restore; an err means we cannot prove the marker is clear, so the safer default is to treat unknown as set — skip commit and touch `save.requested`. The current default optimises for "kill removes the session promptly" in a transient-failure scenario at the cost of "do not corrupt an in-flight restore", inverting the spec's stated risk priority in § `@portal-restoring Defence`.

**Solution**: On `isRestoring` query error, log WARN, touch `save.requested`, and exit 0 — same path as the `(true, nil)` short-circuit. Cost is a marginally-extended resurrection window on rare transient-tmux-query failures, recovered on the daemon's next tick.

**Outcome**: The `@portal-restoring` defence holds even when the marker query itself fails. Risk priority matches the spec.

**Do**:
1. In `cmd/state_commit_now.go` around lines 194-206, change the condition so `err != nil` is handled symmetrically to `restoring == true`:
   - Log WARN under the surrounding component with the error attached (e.g. `"isRestoring query failed; presuming marker set to protect in-flight restore"`).
   - Call the same `touchSaveRequested(dir)` path as the `restoring == true` branch.
   - Return nil (exit 0).
2. Confirm the `(true, nil)` branch's log/touch/exit sequence is shared with the new err branch — extract a small inline helper if duplication becomes visible.

**Acceptance Criteria**:
- A non-nil `isRestoring` error skips the structural commit, touches `save.requested`, returns nil
- The error is logged at WARN with the underlying cause attached
- The `(true, nil)` short-circuit path is unchanged
- Spec § `@portal-restoring Defence` priority order is honoured

**Tests**:
- New unit test injecting an `isRestoring` fake returning `(false, errors.New("tmux unreachable"))` — assert: structural commit NOT invoked, `save.requested` IS touched, RunE returns nil, WARN log contains the error string
- Existing tests for `(true, nil)` and `(false, nil)` branches continue to pass

---

## Task 3: Collapse dumpStateDir / dumpStateDirRaw duplication
status: pending
severity: medium
sources: duplication

**Problem**: `dumpStateDir` at `cmd/state_commit_now_reentrancy_integration_test.go:354-397` and `dumpStateDirRaw` at `cmd/state_commit_now_symptom_integration_test.go:657-695` are line-by-line equivalents: identical ReadDir loop, one-level recursion, `(size=%d, mode=%s)` formatting, 2048-byte `sessions.json` truncation, and `--- sessions.json contents ---` banner. Both live in `cmd_test`; the symptom file already calls `sessionNames` defined in the reentrancy file, contradicting the "idiomatically self-contained" rationale.

**Solution**: Delete `dumpStateDirRaw` and have `symptomFixture.diagnostic` call `dumpStateDir` directly. Identical signature — name-only collapse.

**Outcome**: One implementation of the state-dir dump callable from any `cmd_test` file.

**Do**:
1. Delete `dumpStateDirRaw` (and any justifying header comment) from `cmd/state_commit_now_symptom_integration_test.go:657-695`.
2. Update the single caller in `symptomFixture.diagnostic` to call `dumpStateDir`.
3. Verify no other callsite references `dumpStateDirRaw`.

**Acceptance Criteria**:
- `dumpStateDirRaw` no longer exists
- `dumpStateDir` is called from both integration tests
- Diagnostic output identical to pre-change
- `go test ./cmd -run TestStateCommitNow` passes

**Tests**:
- No new tests required; existing integration tests exercise the dump on failure paths

---

## Task 4: Collapse pollSessionsJSON / pollSessionsJSONForKill duplication and sessionNames variants
status: pending
severity: medium
sources: duplication

**Problem**: `pollSessionsJSON` at `cmd/state_commit_now_reentrancy_integration_test.go:294-334` and `pollSessionsJSONForKill` at `cmd/state_commit_now_symptom_integration_test.go:518-550` are two implementations of the same two-consecutive-consistent-reads poll loop. The only difference is the shape predicate; the kill variant is a strict subset of the general variant. Additionally, the duplicate `sessionNames` shapes (low-severity finding) live alongside — `cmd_test`'s `sessionNames(idx) map[string]bool` and `indexSessionNameSet(idx) map[string]struct{}` differ only by value type.

**Solution**: Delete `pollSessionsJSONForKill` and update its single caller to invoke `pollSessionsJSON(ctx, stateDir, []string{kept}, []string{killed})`. While editing, collapse `sessionNames` / `indexSessionNameSet` onto a single `map[string]struct{}` helper.

**Outcome**: One poll-loop implementation, parameterised by present/absent name sets. One canonical name-set helper.

**Do**:
1. Identify the single caller of `pollSessionsJSONForKill` in the symptom file.
2. Replace the call with `pollSessionsJSON(ctx, stateDir, []string{<keptName>}, []string{<killedName>})`.
3. Delete `pollSessionsJSONForKill`.
4. Audit nearby constants (poll interval, consecutive threshold) — fold duplicates.
5. Collapse `sessionNames` and `indexSessionNameSet` onto `map[string]struct{}`; update callers in both files.

**Acceptance Criteria**:
- `pollSessionsJSONForKill` no longer exists
- The symptom test calls `pollSessionsJSON` with explicit present/absent slices
- Poll constants declared once across `cmd_test`
- Single `sessionNames`/name-set helper of shape `map[string]struct{}`
- `go test ./cmd -run TestStateCommitNow` passes with no flakiness regression

**Tests**:
- No new tests required — existing symptom integration test exercises the kill scenario

---

## Task 5: Replace errCommitNowFailed empty-message sentinel and preserve cause
status: pending
severity: low
sources: architecture

**Problem**: At `cmd/state_commit_now.go:14-22` and `:238-244`, the non-zero exit relies on `errCommitNowFailed = errors.New("")` so that `main.go`'s `err.Error() == ""` guard suppresses stderr. The contract spans two files with no compile-time link. Additionally, `failCommitNow` logs the cause via `logger.Error` then returns the empty sentinel — the cause is preserved only in `portal.log`, lost from the returned error chain.

**Solution**: Introduce a named exit-code error type detectable via `errors.Is` in `main.go`. Have `failCommitNow` wrap the cause via `fmt.Errorf("%w: %v", errCommitNowFailed, cause)`.

**Outcome**: Silent-exit contract is compile-time-linked across `cmd` and `main`. Underlying cause preserved.

**Do**:
1. In `cmd/state_commit_now.go`, change `errCommitNowFailed` message to descriptive (e.g., `errors.New("commit-now failed")`).
2. Update `main.go`'s top-level error handler: replace the `err.Error() == ""` guard with `errors.Is(err, cmd.ErrCommitNowFailed)`.
3. In `failCommitNow`, wrap the cause: `return fmt.Errorf("%w: %v", errCommitNowFailed, cause)`. Keep the existing `logger.Error` write.
4. Verify exit code from `portal state commit-now` on failure is still non-zero with no stderr noise.

**Acceptance Criteria**:
- `errCommitNowFailed` no longer relies on empty message
- `main.go` detects silent-exit via `errors.Is`/typed check
- `failCommitNow` returns a wrapped error preserving cause
- Failure path: non-zero exit, empty stderr
- All existing tests pass

**Tests**:
- New unit: `errors.Is(failCommitNow(...), errCommitNowFailed) == true`
- New unit: `errors.Unwrap(...)` surfaces the cause
- Existing subprocess integration tests asserting exit code 1 with empty stderr continue to pass

---

## Task 6: Replace resolveCommitNowDeps tuple-of-six with *Deps struct
status: pending
severity: low
sources: architecture

**Problem**: `resolveCommitNowDeps` at `cmd/state_commit_now.go:76-113` returns six function values via named-return-with-naked-return. This is an outlier compared to `bootstrapDeps` / `openDeps` / `hooksDeps` which use a `*Deps` struct with per-field nil checks. A seventh seam adds a tuple slot every test must update.

**Solution**: Have `resolveCommitNowDeps` return a fully-populated `*CommitNowDeps`. Update RunE to read `deps.ReadIndex` / `deps.CaptureStructure` / etc. directly.

**Outcome**: `state_commit_now.go` matches the rest of the `cmd` package's DI shape.

**Do**:
1. Confirm `CommitNowDeps` struct exists; if not, define one field per current tuple slot.
2. Rewrite `resolveCommitNowDeps` to return `*CommitNowDeps` populated from the package-level `commitNowDeps` var with nil-field fallbacks.
3. Update call site at line 183: replace destructure with `deps := resolveCommitNowDeps()`, then `deps.ReadIndex(...)` etc.
4. Update tests that previously stubbed via tuple shape to set struct fields on `commitNowDeps`.

**Acceptance Criteria**:
- `resolveCommitNowDeps` returns `*CommitNowDeps`
- RunE reads via `deps.<Field>`
- All existing tests pass

**Tests**:
- No new tests required — existing unit tests exercise all seams

---

## Task 7: Remove redundant MigrationLogger noop fallback
status: pending
severity: low
sources: architecture

**Problem**: At `internal/tmux/hooks_register.go:174-185` and `:372-401`, `MigrationLogger` is defined inside `internal/tmux` solely to avoid an import cycle on `*state.Logger`. `*state.Logger` satisfies it structurally. The `noopMigrationLogger` fallback is reinvented locally despite `*state.Logger`'s documented nil-receiver no-op semantics.

**Solution**: If the cycle is real, keep `MigrationLogger` but drop `noopMigrationLogger` — let callers pass `(*state.Logger)(nil)`. If the cycle can be broken, drop the interface entirely.

**Outcome**: One logger seam in `hooks_register.go`. No reinvented no-op type.

**Do**:
1. Attempt to import `internal/state` from `internal/tmux/hooks_register.go` — confirm whether the cycle is real.
2. If cycle exists: delete `noopMigrationLogger`. Update default-construction sites to pass `(*state.Logger)(nil)`. Verify `*state.Logger`'s nil-receiver methods all no-op.
3. If no cycle: delete `MigrationLogger` and change signatures to take `*state.Logger` directly.
4. Run `go build ./...` and `go test ./internal/tmux/...`.

**Acceptance Criteria**:
- `noopMigrationLogger` no longer exists
- Either `MigrationLogger` remains as a structural seam with `(*state.Logger)(nil)` no-op callers, OR `MigrationLogger` is deleted and `*state.Logger` is consumed directly
- `go build ./...` succeeds with no new import cycles
- `go test ./internal/tmux/...` passes
- Existing migration-logging behaviour unchanged

**Tests**:
- No new tests required — existing `hooks_register_test.go` exercises the migration path
