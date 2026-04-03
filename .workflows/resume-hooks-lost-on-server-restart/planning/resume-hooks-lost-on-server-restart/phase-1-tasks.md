---
phase: 1
phase_name: Empty-Pane Guard
total: 1
---

## resume-hooks-lost-on-server-restart-1-1 | approved

### Task 1: Guard CleanStale from empty pane list in ExecuteHooks

**Problem**: When a tmux server restarts, `ExecuteHooks` in `internal/hooks/executor.go` calls `ListAllPanes()` which returns an empty slice (no error -- the tmux command simply finds no panes). This empty slice is passed directly to `store.CleanStale()`, which treats every stored hook as stale (none match the empty live-panes set) and deletes them all. The user loses all registered resume hooks on every server restart. The `clean` command already guards against this at `cmd/clean.go:77-80` with a `len(livePanes) == 0` check, but that guard was never replicated in `ExecuteHooks`.

**Solution**: Add a `len(livePanes) > 0` guard in `ExecuteHooks` before calling `store.CleanStale()`, matching the existing pattern in `cmd/clean.go:77-80`. Also fix the existing test at `executor_test.go:537-568` ("no tmux server running skips cleanup gracefully") which currently asserts the buggy behavior -- it asserts `CleanStale` IS called with an empty list. After the fix, `CleanStale` must NOT be called when `livePanes` is empty.

**Outcome**: After a tmux server restart, `ExecuteHooks` skips the `CleanStale` call when `ListAllPanes` returns an empty slice, preserving all stored hooks on disk. Hooks survive the restart and fire correctly once tmux-resurrect restores sessions. The existing test is corrected to assert the safe behavior, and a new test verifies hooks survive the empty-pane scenario end-to-end.

**Do**:
1. **Fix the existing test first (TDD red step):** In `internal/hooks/executor_test.go`, update the test at line 537 ("no tmux server running skips cleanup gracefully") to assert `CleanStale` is NOT called when `ListAllPanes` returns an empty slice:
   - Change the assertion on line 558 from `if !store.called` to `if store.called` (expect `CleanStale` NOT to be called)
   - Update the error message from `"expected CleanStale to be called with empty list"` to `"expected CleanStale NOT to be called when live panes is empty"`
   - Remove the assertion at line 560-562 that checks `len(store.livePanesReceived) != 0` (no longer relevant since `CleanStale` should not be called at all)
   - Run `go test ./internal/hooks/...` -- confirm this test FAILS against the current code (the red step)

2. **Add a new test for nil return from ListAllPanes:** In the same `TestExecuteHooks_Cleanup` test group, add a test `"ListAllPanes returns nil skips cleanup gracefully"` that sets `mockAllPaneLister{panes: nil}` (as opposed to the empty slice `[]string{}`). Assert:
   - `ListAllPanes` was called (`tmux.called == true`)
   - `CleanStale` was NOT called (`store.called == false`)
   - Hook execution still proceeds (`len(tmux.sent) == 1`)
   - Run `go test ./internal/hooks/...` -- confirm this also FAILS (same bug path)

3. **Apply the fix in `ExecuteHooks`:** In `internal/hooks/executor.go`, modify lines 66-68. Change:
   ```go
   if livePanes, err := tmux.ListAllPanes(); err == nil {
       _, _ = store.CleanStale(livePanes)
   }
   ```
   To:
   ```go
   if livePanes, err := tmux.ListAllPanes(); err == nil && len(livePanes) > 0 {
       _, _ = store.CleanStale(livePanes)
   }
   ```
   This is a single-line change -- adding `&& len(livePanes) > 0` to the existing `if` condition. This matches the guard pattern in `cmd/clean.go:77-80`.

4. **Run all tests:** `go test ./internal/hooks/...` -- confirm both the updated test and the new nil-return test pass. Then run `go test ./...` to confirm no regressions anywhere.

**Acceptance Criteria**:
- [ ] `ExecuteHooks` does NOT call `CleanStale` when `ListAllPanes` returns an empty slice `[]string{}`
- [ ] `ExecuteHooks` does NOT call `CleanStale` when `ListAllPanes` returns `nil`
- [ ] `ExecuteHooks` still calls `CleanStale` when `ListAllPanes` returns a non-empty slice (existing test "cleanup calls ListAllPanes and CleanStale before hook execution" still passes)
- [ ] `ExecuteHooks` still skips `CleanStale` (but does not crash) when `ListAllPanes` returns an error (existing test "ListAllPanes error skips cleanup and continues" still passes)
- [ ] Hook execution proceeds normally regardless of whether cleanup was skipped -- hooks still fire for matching panes
- [ ] All existing tests in `go test ./...` pass with no regressions

**Tests**:
- `"no tmux server running skips cleanup gracefully"` -- UPDATED: `ListAllPanes` returns `[]string{}`, assert `CleanStale` NOT called, assert hooks still execute for matching panes
- `"ListAllPanes returns nil skips cleanup gracefully"` -- NEW: `ListAllPanes` returns `nil`, assert `CleanStale` NOT called, assert hooks still execute for matching panes
- `"cleanup calls ListAllPanes and CleanStale before hook execution"` -- EXISTING (no changes): confirms `CleanStale` IS called when live panes exist
- `"ListAllPanes error skips cleanup and continues"` -- EXISTING (no changes): confirms error path still skips cleanup
- `"CleanStale error skips cleanup and continues"` -- EXISTING (no changes): confirms `CleanStale` error does not block execution

**Edge Cases**:
- `ListAllPanes` returns `nil` vs empty slice `[]string{}`: both must skip `CleanStale`. Go's `len(nil)` returns 0, so `len(livePanes) > 0` handles both cases identically. The nil case gets its own test for explicitness.
- `ListAllPanes` error path: no regression -- the existing `err == nil` check in the `if` condition handles this before the `len` check is reached. The existing test "ListAllPanes error skips cleanup and continues" covers this.

**Context**:
> The specification identifies two distinct problems. This task addresses only Problem 1 (hook deletion on restart). Problem 2 (pane ID instability / structural keys) is handled in Phases 2 and 3. The empty-pane guard is the minimal surgical fix that prevents data loss immediately, even before structural keys are introduced.
>
> The guard pattern already exists in `cmd/clean.go:77-80`:
> ```go
> // Empty pane list with existing hooks means no tmux server is running.
> // Skip cleanup to avoid destroying hooks needed after next reboot.
> if len(livePanes) == 0 {
>     return nil
> }
> ```
> The `ExecuteHooks` implementation mirrors this by adding the `len` check to the existing `if` condition rather than using a separate guard block, since `ExecuteHooks` uses silent best-effort error handling (no early returns for cleanup issues).

**Spec Reference**: `.workflows/resume-hooks-lost-on-server-restart/specification/resume-hooks-lost-on-server-restart/specification.md` -- "Problem 1 -- Hook deletion on restart" in Problem Statement; "Empty-pane guard" in Solution Overview; "Fix existing test" in Testing Requirements.
