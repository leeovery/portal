---
status: complete
created: 2026-03-20
cycle: 1
phase: Plan Integrity Review
topic: auto-start-tmux-server
---

# Review Tracking: auto-start-tmux-server - Integrity

## Findings

### 1. Task 3-2 transitionFromLoading will not correctly evaluate default page

**Severity**: Critical
**Plan Reference**: Phase 3 / auto-start-tmux-server-3-2 (Timing messages and transition logic)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
The `transitionFromLoading()` method sets `activePage = PageSessions` then calls `evaluateDefaultPage()`. However, `evaluateDefaultPage()` has a guard: `if m.defaultPageEvaluated { return }`. During loading, `SessionsMsg` arrives from the initial fetch or re-polls, and the existing `SessionsMsg` handler (model.go:607) calls `m.evaluateDefaultPage()` — setting `defaultPageEvaluated = true`. When `transitionFromLoading()` later calls `evaluateDefaultPage()`, it returns immediately (no-op).

Result: `activePage` is always set to `PageSessions` by `transitionFromLoading`, even when there are zero sessions and the correct page would be `PageProjects`.

Two fixes needed: (1) `transitionFromLoading` must reset `defaultPageEvaluated` before calling `evaluateDefaultPage`. (2) The `SessionsMsg` handler's loading-page branch must be placed before the `evaluateDefaultPage()` call to prevent premature page switching during loading.

**Resolution**: Fixed

**Current**:
In the Do section, step 5:
```go
func (m *Model) transitionFromLoading() {
    m.activePage = PageSessions
    m.evaluateDefaultPage()
}
```

In the Do section, step 6:
> **Important**: This check must come AFTER the existing `SessionsMsg` handling (setting sessions, filtering, etc.) but the transition logic is new. The simplest approach is to add it right before the `return m, nil` at the end of the `SessionsMsg` case, checking if we're still on the loading page. When sessions are empty, `pollSessionsCmd` schedules a re-fetch after 500ms. This continues until sessions appear or `maxWaitElapsedMsg` fires and transitions away from loading.

**Proposed**:
In the Do section, step 5 — replace `transitionFromLoading`:
```go
func (m *Model) transitionFromLoading() {
    m.defaultPageEvaluated = false
    m.activePage = PageSessions
    m.evaluateDefaultPage()
}
```

In the Do section, step 6 — replace the instruction and code placement guidance:

6. Update the `SessionsMsg` handler in `Update()` to handle the loading page case. Insert the loading-page branch *after* the existing session data processing (setting sessions, filtering, items, size, title, `sessionsLoaded = true`) but *before* the `evaluateDefaultPage()` call. When on the loading page, the branch returns early — skipping `evaluateDefaultPage` so it does not prematurely set `defaultPageEvaluated = true`:
   ```go
   case SessionsMsg:
       if msg.Err != nil {
           return m, tea.Quit
       }
       m.sessions = msg.Sessions
       filtered := m.filteredSessions()
       items := ToListItems(filtered)
       m.sessionList.SetItems(items)
       if m.termWidth > 0 || m.termHeight > 0 {
           m.sessionList.SetSize(m.termWidth, m.termHeight)
       }
       if m.insideTmux && m.currentSession != "" {
           m.sessionList.Title = fmt.Sprintf("Sessions (current: %s)", m.currentSession)
       }
       m.sessionsLoaded = true

       if m.activePage == PageLoading {
           if len(msg.Sessions) > 0 {
               m.sessionsReceived = true
               if m.minWaitDone {
                   m.transitionFromLoading()
               }
               return m, nil
           }
           // No sessions yet — schedule another fetch after poll interval
           return m, m.pollSessionsCmd()
       }

       m.evaluateDefaultPage()
       return m, nil
   ```
   When on the loading page, `evaluateDefaultPage` is never called. When `transitionFromLoading` eventually fires (via min+sessions or max timeout), it resets `defaultPageEvaluated = false` and calls `evaluateDefaultPage` fresh, correctly choosing between `PageSessions` and `PageProjects`.

**Resolution**: Pending
**Notes**:

---

### 2. Task 3-1 missing explicit activePage set in NewModelWithSessions test helper

**Severity**: Important
**Plan Reference**: Phase 3 / auto-start-tmux-server-3-1 (Loading page state and view)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 3-1 correctly identifies that adding `PageLoading` as `iota` value 0 shifts existing constants, and that `New()` must explicitly set `activePage = PageSessions`. However, `NewModelWithSessions` (model.go:462) also constructs a `Model` struct literal without setting `activePage`. After the iota change, its zero-value `activePage` would be `PageLoading` (value 0), breaking all existing tests that use this helper.

The Edge Cases section mentions this ("The `NewModelWithSessions` test helper also needs to explicitly set `activePage: PageSessions` if it doesn't already") but the Do section does not include a step to fix it. An implementer following only the Do steps would miss this, causing test failures.

**Resolution**: Fixed
**Notes**:

---

### 3. Task 3-2 has a malformed test entry combining two separate tests

**Severity**: Minor
**Plan Reference**: Phase 3 / auto-start-tmux-server-3-2 (Timing messages and transition logic)
**Category**: Task Template Compliance
**Change Type**: update-task

**Details**:
The last test entry in the Tests section concatenates two separate test cases into one bullet.

**Resolution**: Fixed
**Notes**:
