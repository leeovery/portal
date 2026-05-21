---
topic: killed-session-resurrects-within-tick-window
cycle: 2
total_proposed: 3
---
# Analysis Tasks: killed-session-resurrects-within-tick-window (Cycle 2)

## Task 1: Delete defaultTouchSaveRequested wrapper
status: approved
severity: low
sources: duplication, architecture

**Problem**: `defaultTouchSaveRequested` in `cmd/state_commit_now.go:126-133` is a one-line trampoline (`func(dir string) error { return state.TouchSaveRequested(dir) }`) whose signature matches `state.TouchSaveRequested` exactly. The wrapper is residue from the cycle-1 promotion of `TouchSaveRequested` into the `state` package. It is asymmetric with the four sibling `CommitNowDeps` defaults (`ReadIndex` / `CaptureStructure` / `Commit` / etc.) which reference `state.X` directly. Tests do not substitute `defaultTouchSaveRequested`; they replace the whole `TouchSaveRequested` field.

**Solution**: Delete `defaultTouchSaveRequested` and assign `state.TouchSaveRequested` directly to the `CommitNowDeps.TouchSaveRequested` field default at `cmd/state_commit_now.go:99`.

**Outcome**: One fewer hop on the production path. `CommitNowDeps` default initialisation reads symmetrically across all fields. Observable behaviour unchanged.

**Do**:
1. Open `cmd/state_commit_now.go`.
2. Delete the `defaultTouchSaveRequested` function declaration at lines 126-133.
3. At line 99, change `TouchSaveRequested: defaultTouchSaveRequested,` to `TouchSaveRequested: state.TouchSaveRequested,`.
4. Run `go build ./...` to confirm compilation.
5. Run `go test ./cmd/...` to confirm no test references break.

**Acceptance Criteria**:
- `defaultTouchSaveRequested` no longer exists in `cmd/state_commit_now.go`
- `CommitNowDeps`'s `TouchSaveRequested` default is `state.TouchSaveRequested` (a direct reference)
- All existing cmd-package tests pass without modification
- The other four `CommitNowDeps` field defaults remain unchanged

**Tests**:
- No new tests required; behaviour is unchanged. Verify existing `cmd/state_commit_now*_test.go` continues to pass.
- Verify any test that replaces `deps.TouchSaveRequested` still does so against the field (not the wrapper symbol).

---

## Task 2: Replace ErrStatusUnhealthy empty-string sentinel with a descriptive message
status: approved
severity: low
sources: architecture

**Problem**: `ErrStatusUnhealthy` in `cmd/state_status.go:13-19` is declared as `errors.New("")` with a doc-comment justifying the empty message. Cycle 1 introduced `IsSilentExitError` to compile-time-link the silent-exit contract, retiring the brittle `err.Error() == ""` guard in `main.go`. The empty message is now load-bearing nowhere — it survives only as a dead convention that invites future readers to re-introduce the brittle string-compare pattern by analogy. `errCommitNowFailed` correctly received a descriptive message in cycle 1; `ErrStatusUnhealthy` was missed.

**Solution**: Change `errors.New("")` to `errors.New("status unhealthy")` and update the doc-comment so it cites `IsSilentExitError` as the suppression mechanism. Sentinel identity is preserved by `errors.Is`, so no call-site changes are needed.

**Outcome**: The empty-string sentinel convention is fully retired. Both silent-exit sentinels carry descriptive messages and document `IsSilentExitError` as the suppression contract.

**Do**:
1. Open `cmd/state_status.go`.
2. At lines 13-19, change `errors.New("")` to `errors.New("status unhealthy")`.
3. Update the adjacent doc-comment to reference `IsSilentExitError` as the silent-exit mechanism (mirroring the doc-comment style used on `errCommitNowFailed` in `cmd/state_commit_now.go:30-35`).
4. Run `go build ./...` to confirm compilation.
5. Run `go test ./cmd/...` and confirm any test asserting on `ErrStatusUnhealthy` identity (via `errors.Is`) still passes; if any test asserts on `err.Error() == ""`, update it to either `errors.Is(err, ErrStatusUnhealthy)` or the new descriptive message.

**Acceptance Criteria**:
- `ErrStatusUnhealthy` is declared with a non-empty descriptive message (`"status unhealthy"`)
- Its doc-comment references `IsSilentExitError` as the silent-exit contract
- All callers using `errors.Is(err, ErrStatusUnhealthy)` continue to match
- `main.go`'s silent-exit path (via `IsSilentExitError`) continues to suppress stderr for status-unhealthy returns

**Tests**:
- No new tests required; existing `state_status*_test.go` and integration tests cover the path.
- If a regression test for `IsSilentExitError(ErrStatusUnhealthy)` does not already exist, add a one-line assertion alongside the cycle-1 `errCommitNowFailed` test confirming `IsSilentExitError(ErrStatusUnhealthy)` returns true.

---

## Task 3: Extract runPortalSubprocess helper to consolidate runPortalCommitNow and runPortalList
status: approved
severity: low
sources: duplication

**Problem**: `runPortalCommitNow` (`cmd/state_commit_now_symptom_integration_test.go:453-467`) and `runPortalList` (`cmd/state_commit_now_symptom_integration_test.go:485-499`) are structurally identical subprocess shells: same `exec.Command(binary, ...)` shape, same three-line env append, same `CombinedOutput`, same `t.Fatalf` shape. Only positional args differ. This is the third cycle the duplication agent has flagged the pair. Still below strict Rule-of-Three with two direct call sites, but the structural identity is high and the pattern has stabilised.

**Solution**: Extract `runPortalSubprocess(t *testing.T, binary, stateDir string, args ...string) []byte` (or equivalent signature). Rewrite `runPortalCommitNow` and `runPortalList` as one-line trampolines forwarding to the helper. Preserve `t.Helper()` and existing `t.Fatalf` failure format.

**Outcome**: One subprocess-shell implementation. Two thin trampolines retain their names so call sites remain readable.

**Do**:
1. Open `cmd/state_commit_now_symptom_integration_test.go`.
2. Add `runPortalSubprocess(t *testing.T, binary, stateDir string, args ...string) []byte` near existing helpers. Body: build `exec.Command(binary, args...)`, append the same env vars, call `CombinedOutput`, call `t.Fatalf` with same format on error, return captured bytes.
3. Mark it with `t.Helper()`.
4. Rewrite `runPortalCommitNow` as a one-line trampoline returning `runPortalSubprocess(t, binary, stateDir, "state", "commit-now")`.
5. Rewrite `runPortalList` similarly.
6. Run `go test ./cmd -run TestStateCommitNow` to confirm the test still drives the same subprocess invocations.

**Acceptance Criteria**:
- `runPortalSubprocess` exists as a single helper carrying the `exec.Command` + env + `CombinedOutput` + `t.Fatalf` body
- `runPortalCommitNow` and `runPortalList` are one-line trampolines
- All existing tests using either trampoline pass unchanged
- `t.Helper()` propagation is preserved (failure lines report at the original call site)
- Failure-message format is byte-equivalent to the pre-extraction format

**Tests**:
- No new tests required; this is a non-behavioural test-helper consolidation.
- Verify by running the full `cmd` integration suite: `go test ./cmd/...`.
