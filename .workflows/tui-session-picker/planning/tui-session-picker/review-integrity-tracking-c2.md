---
status: complete
created: 2026-02-28
cycle: 2
phase: Plan Integrity Review
topic: tui-session-picker
---

# Review Tracking: TUI Session Picker - Integrity (Cycle 2)

## Applied Fix Verification

All 6 applied fixes from cycle 1 have been correctly incorporated:
- Task 1-5: CreateFromDir(m.cwd, nil) -- command references removed
- Task 3-1: sessionsLoaded/projectsLoaded synchronization added
- Task 2-2: createSession(path) method defined, createSessionInCWD delegates
- Task 2-6: Reduced to independent filter verification only
- Task 1-3: modalRename removed from enum
- Task 3-2: e/d added to disabled keybindings, p removed

No issues introduced by the applied fixes.

## Findings

### 1. Task 3-4 does not explicitly modify createSession to forward m.command

**Severity**: Important
**Plan Reference**: Phase 3 / tui-session-picker-3-4 (tick-e8fd08)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 2-2 defines `createSession(path string)` as calling `CreateFromDir(path, nil)` -- hardcoding nil for the command parameter. Task 3-4 (Command-Pending Enter Creates Session with Command) says "emit createSession(path) which passes m.command to CreateFromDir" but never explicitly modifies the createSession method. The same applies to task 3-5, which says "n key handler calls createSession(m.cwd) which passes m.command".

The actual change needed is a one-line modification to createSession: replace `CreateFromDir(path, nil)` with `CreateFromDir(path, m.command)`. This is backward-compatible (m.command is nil in normal mode). Task 3-4 should state this modification explicitly since it is the first task that requires command forwarding.

**Current**:
```
**Do**:
- In Projects page enter handler, emit createSession(path) which passes m.command to CreateFromDir
- Handle SessionCreatedMsg: set m.selected = msg.SessionName, return tea.Quit
- Handle sessionCreateErrMsg: stay on Projects page
```

**Proposed**:
```
**Do**:
- Modify createSession(path string) to pass m.command instead of nil: change CreateFromDir(path, nil) to CreateFromDir(path, m.command). This is backward-compatible -- m.command is nil in normal mode, so existing behavior is unchanged.
- In Projects page enter handler, enter calls createSession(item.Project.Path) which now forwards m.command automatically
- Handle SessionCreatedMsg: set m.selected = msg.SessionName, return tea.Quit
- Handle sessionCreateErrMsg: stay on Projects page
```

---

### 2. Task 3-7 does not explicitly remove initial filter from SessionsMsg handler

**Severity**: Minor
**Plan Reference**: Phase 3 / tui-session-picker-3-7 (tick-bd640d)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 1-6 adds initial filter application inside the SessionsMsg handler: "if m.initialFilter != '', call SetFilterText/SetFilterState, then clear initialFilter". Task 3-7 moves initial filter application to after the default page is determined (inside evaluateDefaultPage from task 3-1). However, task 3-7's Do section only describes adding the new filter application -- it does not mention removing the initial filter code from the SessionsMsg handler that task 1-6 added. Without this removal, the initial filter would be applied twice: once in the SessionsMsg handler and once in evaluateDefaultPage. The implementer would likely catch this, but explicit instruction avoids confusion.

**Current**:
```
**Do**:
- After default page determined and data loaded, if initialFilter != "", call SetFilterText(initialFilter) and SetFilterState(list.FilterApplied) on default page's list
- In command-pending mode, always apply to Projects page
- If initialFilter empty, do nothing (no-op)
- Consume initial filter after applying (set initialFilter = "")
```

**Proposed**:
```
**Do**:
- Remove the initial filter application from the SessionsMsg handler (added in task 1-6) -- this logic moves to evaluateDefaultPage
- In evaluateDefaultPage (from task 3-1), after default page is determined: if initialFilter != "", call SetFilterText(initialFilter) and SetFilterState(list.FilterApplied) on the default page's list
- In command-pending mode, always apply to Projects page's list
- If initialFilter empty, do nothing (no-op)
- Consume initial filter after applying (set initialFilter = "")
```

---
