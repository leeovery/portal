---
topic: enter-attaches-from-preview
cycle: 2
total_proposed: 3
---
# Analysis Tasks: enter-attaches-from-preview (Cycle 2)

## Task 1: Close previewLogger to remove lifecycle asymmetry
status: pending
severity: low
sources: architecture

**Problem**: `openTUI` in `cmd/open.go` calls `state.OpenLogger` to back the preview-attach pipeline but never closes the returned `*state.Logger`. Today the fd is reclaimed by `syscall.Exec` (outside-tmux) or normal process exit (inside-tmux SwitchClient path), so no observable victim — but the logger is the only resource opened in `openTUI` without a matching `defer Close()`. The asymmetry becomes a real fd leak if `openTUI` is ever invoked repeatedly in the same process.

**Solution**: Add `defer previewLogger.Close()` immediately after the `state.OpenLogger` call in `openTUI`. `Logger.Close` is already nil-safe.

**Outcome**: previewLogger's lifecycle is scoped to `openTUI`, matching every other resource in the function.

**Do**:
1. Open `cmd/open.go` and locate the `state.OpenLogger` call (around lines 431-435) inside `openTUI`.
2. Immediately after the assignment to `previewLogger`, add `defer previewLogger.Close()`.
3. Run `go build -o portal .` and `go test ./cmd/...`.

**Acceptance Criteria**:
- `cmd/open.go` contains `defer previewLogger.Close()` directly after the `state.OpenLogger` assignment in `openTUI`.
- No other call sites or signatures change.
- Build passes; existing `cmd` tests pass unchanged.

**Tests**:
- Existing tests continue to pass; no new test required.

## Task 2: Delete redundant flashModelOnSessionsPage alias
status: pending
severity: low
sources: duplication

**Problem**: `flashModelOnSessionsPage(names ...string) Model` is a one-line wrapper that calls `flashModelWithSessions(names...)` verbatim. The rename adds an indirection hop with no disambiguation benefit.

**Solution**: Delete `flashModelOnSessionsPage` and update its callers to call `flashModelWithSessions` directly.

**Do**:
1. Locate `flashModelOnSessionsPage` in `internal/tui/`.
2. Grep for every reference and rewrite each to `flashModelWithSessions`.
3. Delete the function definition.
4. Run `go test ./internal/tui/...`.

**Acceptance Criteria**:
- `flashModelOnSessionsPage` is not defined anywhere.
- Tests pass unchanged.

## Task 3: Extract singlePaneGroups() test helper for repeated stubEnumerator fixture
status: pending
severity: low
sources: duplication

**Problem**: The literal `&stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}}` is repeated 21 times across `preview_attach_bail_test.go` (8), `preview_attach_bail_flash_test.go` (10), and `preview_attach_selected_test.go` (3).

**Solution**: Add a helper (prefer `newSinglePaneEnumerator() *stubEnumerator`) and replace each of the 21 literal sites.

**Do**:
1. Add the helper to an existing tui test helpers file.
2. Replace all 21 occurrences with the helper call.
3. Run `go test ./internal/tui/...`.

**Acceptance Criteria**:
- A single helper exists.
- The literal `WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}` appears at most once outside the helper.
- Tests pass unchanged.
