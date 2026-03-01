---
topic: tui-session-picker
cycle: 2
total_proposed: 2
---
# Analysis Tasks: tui-session-picker (Cycle 2)

## Task 1: Guard evaluateDefaultPage against command-pending page selection
status: pending
severity: medium
sources: architecture

**Problem**: `evaluateDefaultPage` in `internal/tui/model.go:460-476` uses the command-pending flag only to control the readiness gate (skip waiting for sessions), but the page-selection logic at lines 472-476 unconditionally checks `sessionList.Items()`. If a `SessionsMsg` ever arrives before `evaluateDefaultPage` runs in command-pending mode, it would set `activePage` to `PageSessions`, violating the command-pending contract that locks the user to the Projects page.

**Solution**: Add an explicit branch for command-pending mode that always sets `activePage = PageProjects`, bypassing the session-items check entirely.

**Outcome**: The command-pending invariant (always land on Projects page) is self-contained within `evaluateDefaultPage` rather than relying on `Init()` not fetching sessions.

**Do**:
1. In `internal/tui/model.go`, in the `evaluateDefaultPage` method, after line 471 (`m.defaultPageEvaluated = true`), add a command-pending branch:
   - If `m.commandPending` is true, set `m.activePage = PageProjects` and skip the session-items check.
   - Otherwise, keep the existing `if len(m.sessionList.Items()) > 0` logic.
2. The resulting structure should be:
   ```go
   m.defaultPageEvaluated = true
   if m.commandPending {
       m.activePage = PageProjects
   } else if len(m.sessionList.Items()) > 0 {
       m.activePage = PageSessions
   } else {
       m.activePage = PageProjects
   }
   ```

**Acceptance Criteria**:
- When `commandPending` is true, `evaluateDefaultPage` always sets `activePage` to `PageProjects` regardless of session list contents.
- Existing non-command-pending behavior is unchanged.

**Tests**:
- Test that `evaluateDefaultPage` in command-pending mode sets `activePage` to `PageProjects` even when `sessionList.Items()` is non-empty.
- Test that `evaluateDefaultPage` in normal mode still defaults to `PageSessions` when sessions exist.

## Task 2: Add [q] quit binding to all help bars
status: pending
severity: low
sources: standards

**Problem**: The spec defines `[q] quit` as the last entry in all three help bar layouts (sessions, projects, command-pending). The implementation calls `DisableQuitKeybindings()` on both list models to handle `q` manually, but this also removes `q` from the help bar display. The `sessionHelpKeys`, `projectHelpKeys`, and `commandPendingHelpKeys` functions do not include a `q` binding, so `[q] quit` never appears in the help bar despite working correctly.

**Solution**: Add a display-only `q`/`quit` `key.Binding` to each of the three help key functions.

**Outcome**: All three help bars match the spec layout, showing `[q] quit` as the last entry.

**Do**:
1. In `internal/tui/model.go`, add a `q`/`quit` binding to the end of the slice returned by `sessionHelpKeys()` (line ~353):
   ```go
   key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
   ```
2. Add the same binding to the end of `projectHelpKeys()` (line ~377).
3. Add the same binding to the end of `commandPendingHelpKeys()` (line ~390).

**Acceptance Criteria**:
- `[q] quit` appears in the help bar on the Sessions page.
- `[q] quit` appears in the help bar on the Projects page.
- `[q] quit` appears in the help bar in command-pending mode.
- No change to actual quit behavior (already handled elsewhere).

**Tests**:
- Verify each help key function includes a binding with key "q" and help text "quit".
