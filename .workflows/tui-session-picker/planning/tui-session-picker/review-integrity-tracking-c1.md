---
status: complete
created: 2026-02-28
cycle: 1
phase: Plan Integrity Review
topic: tui-session-picker
---

# Review Tracking: TUI Session Picker - Integrity

## Findings

### 1. Task 1-5 N-Key references command-pending mode but command field doesn't exist until Phase 3

**Severity**: Important
**Plan Reference**: Phase 1 / tui-session-picker-1-5 (tick-01c27d)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 1-5 (N-Key New Session in CWD) says "Works in command-pending mode (command is passed to CreateFromDir)" in the acceptance criteria and "Session creation uses CreateFromDir(cwd, command) with the pending command if set" as a criterion. It also includes `m.command` references in the Do section. However, `m.command` and command-pending mode don't exist until Phase 3 (task 3-2). An implementer executing Phase 1 would not know what `m.command` is or how to handle it.

The n-key behavior in Phase 1 should create a session without command awareness. Phase 3 task 3-5 (Command-Pending Browse and N-Key with Command) explicitly handles wiring the command through the n-key path. Task 1-5 should focus only on the n-key creating a session in cwd without command forwarding.

**Current**:
```
**Do**:
- In internal/tui/model.go:
  - Add a cwd string field to Model (the current working directory)
  - Add a WithCWD(path string) Option to set cwd on the model
  - Handle n key in the sessions page update: if sessionCreator is not nil, return m.createSessionInCWD() command
  - func (m Model) createSessionInCWD() tea.Cmd calls m.sessionCreator.CreateFromDir(m.cwd, m.command), returns SessionCreatedMsg or sessionCreateErrMsg
  - Remove the "[n] new in project..." option from the view: remove the divider rendering, the "new in project" line, and the totalItems() method that counted sessions+1
  - Remove the n-key-jumps-to-new-option behavior
  - SessionCreatedMsg and sessionCreateErrMsg handling already exists in Update()
- Update cmd/open.go openTUI to pass cwd to the model (it already gets cwd via os.Getwd())
- Update tests in internal/tui/model_test.go

**Acceptance Criteria**:
- [ ] n key creates a session in the current working directory immediately
- [ ] Session creation uses CreateFromDir(cwd, command) with the pending command if set
- [ ] On success, Selected() returns the new session name and TUI quits
- [ ] On error (sessionCreateErrMsg), TUI handles gracefully (no crash)
- [ ] n with no session creator configured is a no-op
- [ ] The "[n] new in project..." option and divider are removed from the session list
- [ ] Works in command-pending mode (command is passed to CreateFromDir)

**Tests**:
- "n creates session in cwd and quits"
- "n with pending command passes command to CreateFromDir"
- "n with no session creator is no-op"
- "session creation error is handled gracefully"
- "session list no longer shows new in project option"
```

**Proposed**:
```
**Do**:
- In internal/tui/model.go:
  - Add a cwd string field to Model (the current working directory)
  - Add a WithCWD(path string) Option to set cwd on the model
  - Handle n key in the sessions page update: if sessionCreator is not nil, return m.createSessionInCWD() command
  - func (m Model) createSessionInCWD() tea.Cmd calls m.sessionCreator.CreateFromDir(m.cwd, nil), returns SessionCreatedMsg or sessionCreateErrMsg
  - Remove the "[n] new in project..." option from the view: remove the divider rendering, the "new in project" line, and the totalItems() method that counted sessions+1
  - Remove the n-key-jumps-to-new-option behavior
  - SessionCreatedMsg and sessionCreateErrMsg handling already exists in Update()
- Update cmd/open.go openTUI to pass cwd to the model (it already gets cwd via os.Getwd())
- Update tests in internal/tui/model_test.go

**Acceptance Criteria**:
- [ ] n key creates a session in the current working directory immediately
- [ ] Session creation uses CreateFromDir(cwd, nil) -- command forwarding is added in Phase 3 task 3-5
- [ ] On success, Selected() returns the new session name and TUI quits
- [ ] On error (sessionCreateErrMsg), TUI handles gracefully (no crash)
- [ ] n with no session creator configured is a no-op
- [ ] The "[n] new in project..." option and divider are removed from the session list

**Tests**:
- "n creates session in cwd and quits"
- "n with no session creator is no-op"
- "session creation error is handled gracefully"
- "session list no longer shows new in project option"
```

---

### 2. Task 1-6 initial filter applied only to sessions list but Phase 3 applies to default page

**Severity**: Important
**Plan Reference**: Phase 1 / tui-session-picker-1-6 (tick-b00429) and Phase 3 / tui-session-picker-3-7 (tick-bd640d)
**Category**: Scope and Granularity
**Change Type**: update-task

**Details**:
Task 1-6 (Built-in Filtering and Initial Filter) implements initial filter application on the sessions list during SessionsMsg handling. However, Task 3-7 (Initial Filter Applied to Default Page) later moves this logic to apply the initial filter to whichever page is the default. This means Task 1-6 implements initial filter logic that will be reworked in Phase 3. The initial filter should remain as task 1-6 implements it (on sessions), and task 3-7 refactors it to the default page. This is acceptable layering -- but task 1-6's Do section says "Update WithInitialFilter to just store the string (remove project picker filter forwarding for now)" which is fine.

However, task 1-6's acceptance criterion "Initial filter with no matches shows the list's built-in 'no matches' state" overlaps exactly with task 3-7's test "initial filter with no matches shows empty filtered state". This duplication is fine for Phase 1's self-contained testing but should be noted.

No change needed. Noting for awareness only.

**Current**:
N/A -- observation only.

**Proposed**:
N/A -- no change proposed. Task 1-6 correctly implements initial filter for sessions; task 3-7 correctly refactors it for the default page. Overlap in testing is acceptable.

---

### 3. Task 3-1 Default Page Selection lacks implementation detail for data readiness

**Severity**: Important
**Plan Reference**: Phase 3 / tui-session-picker-3-1 (tick-2f0ec0)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 3-1 (Default Page Selection on Launch) says "Modify Init() to fetch both sessions and projects concurrently (using tea.Batch)" and "Add logic block that determines the default page after initial data load completes." But it does not explain how to determine when both data loads are complete. Sessions come via SessionsMsg and projects via ProjectsLoadedMsg -- these arrive asynchronously. The task needs to specify the synchronization approach: e.g., track two booleans (sessionsLoaded, projectsLoaded) and evaluate the default page when both are true. Without this, the implementer must design the synchronization mechanism themselves.

Additionally, in Phase 1 (task 1-2), sessions data triggers immediate rendering. Now task 3-1 must defer page selection until both loads complete. This is a behavioral change to how SessionsMsg is handled that needs to be explicit.

**Current**:
```
**Do**:
- Add logic block that determines the default page after initial data load completes
- Modify Init() to fetch both sessions and projects concurrently (using tea.Batch)
- Default page rules: (1) sessions exist (after inside-tmux exclusion) -> Sessions, (2) no sessions -> Projects, (3) both empty -> Projects
```

**Proposed**:
```
**Do**:
- Add sessionsLoaded bool and projectsLoaded bool fields to Model
- Modify Init() to fetch both sessions and projects concurrently (using tea.Batch with session fetch and loadProjects commands)
- In SessionsMsg handler: set sessionsLoaded = true, populate session list items as before, then call evaluateDefaultPage()
- In ProjectsLoadedMsg handler: set projectsLoaded = true, populate project list items as before, then call evaluateDefaultPage()
- func evaluateDefaultPage(): if !sessionsLoaded || !projectsLoaded, return (wait for both). Otherwise apply default page rules: (1) session list has items (after inside-tmux exclusion) -> pageSessions, (2) no session items -> pageProjects, (3) both empty -> pageProjects
- evaluateDefaultPage also triggers initial filter application (deferred to task 3-7)
```

---

### 4. Task 2-4 Project Edit Modal is too large for a single TDD cycle

**Severity**: Important
**Plan Reference**: Phase 2 / tui-session-picker-2-4 (tick-fd7b3f)
**Category**: Scope and Granularity
**Change Type**: update-task

**Details**:
Task 2-4 (Project Edit Modal) is the most complex task in the plan. It introduces: (1) a new modal state, (2) ProjectEditor and AliasEditor interfaces with Option constructors, (3) edit-mode state fields (editProject, editName, editAliases, editRemoved, editNewAlias, editFocus, editAliasCursor, editError), (4) an editField type with focus-switching, (5) Tab navigation between name and alias sections, (6) alias listing with x-to-remove, (7) alias addition with collision detection, (8) save orchestration across rename + alias delete + alias set, (9) validation (empty name, alias collision), (10) rendering the modal content. This has 11 tests and 11 acceptance criteria.

This is at least 2-3 TDD cycles. The task should be split into: (a) a task for the edit modal scaffolding (open modal, name editing, save/cancel, empty name rejection) and (b) a task for alias management within the edit modal (alias listing, x-to-remove, add alias, collision detection). However, per the review rules I should flag this rather than redesign.

**Current**:
Task tick-fd7b3f with 11 acceptance criteria spanning modal scaffolding, name editing, alias management, collision detection, and save orchestration.

**Proposed**:
Flag only. Task 2-4 should be considered for splitting into two tasks: (a) edit modal with name editing (open, name field, save/cancel, empty name rejection, no-editor guard) and (b) alias management within edit modal (alias listing, removal, addition, collision detection). The current single task is too large for one TDD cycle.

---

### 5. Task 1-2 Sessions Page Core bundles too many concerns

**Severity**: Minor
**Plan Reference**: Phase 1 / tui-session-picker-1-2 (tick-c64e34)
**Category**: Scope and Granularity
**Change Type**: update-task

**Details**:
Task 1-2 (Sessions Page with bubbles/list Core) handles: replacing Model with list.Model, SessionsMsg handling, inside-tmux filtering and title, enter-to-attach, q/Ctrl+C quit, WindowSizeMsg, empty state, and help bar configuration. It has 10 tests. While the scope is logically coherent (setting up the core sessions page), it's on the large side for a single TDD cycle. The inside-tmux behavior (filtering + title) could be a separate task.

However, the inside-tmux behavior is integral to how sessions are loaded (it's part of the SessionsMsg handler), so splitting it would create an artificial boundary. Flagging as minor -- the task is acceptable but at the upper end of TDD cycle size.

**Current**:
N/A -- observation only.

**Proposed**:
N/A -- no change proposed. Task is at the upper bound but logically cohesive. Inside-tmux filtering is part of the SessionsMsg handler and splitting would be artificial.

---

### 6. Phase 3 tasks 3-3 through 3-8 share identical creation timestamp causing non-deterministic execution order

**Severity**: Minor
**Plan Reference**: Phase 3 / tasks tick-f8d97a, tick-e8fd08, tick-c5bbbb, tick-5c4639, tick-bd640d, tick-dc682b
**Category**: Dependencies and Ordering
**Change Type**: update-task

**Details**:
Tasks 3-3 through 3-8 in the plan.md all have creation timestamp 2026-02-28T09:21:50Z. Tick sorts by priority then creation time. When creation times are identical, the sort falls back to ID hash, producing this execution order: 3-6 (Esc), 3-7 (Filter), 3-5 (Browse), 3-8 (Wire), 3-4 (Enter), 3-3 (StatusLine). The plan.md intended order was: 3-3, 3-4, 3-5, 3-6, 3-7, 3-8.

In practice this does not block execution: these tasks are independent vertical slices that all build on task 3-2 (Command-Pending Mode Core) which has a distinct earlier timestamp. The wire task (3-8) executing before enter behavior (3-4) is fine because the wire task just configures option constructors, not internal behavior. However, the implementer may be confused by the execution order not matching the plan document.

No change required. The tasks are independent enough that execution order among them doesn't matter.

**Current**:
N/A -- observation only.

**Proposed**:
N/A -- no change proposed. Tasks 3-3 through 3-8 are independent vertical slices; execution order among them is immaterial.

---

### 7. Task 2-2 Projects Page Core references createSession but does not define it

**Severity**: Important
**Plan Reference**: Phase 2 / tui-session-picker-2-2 (tick-9184b3)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 2-2 says "Handle enter on projects page: get m.projectList.SelectedItem(), cast to ProjectItem, call m.createSession(item.Project.Path)". The method `createSession(path)` is not defined in any Phase 1 task. Phase 1 task 1-5 defines `createSessionInCWD()` which hardcodes cwd. Task 2-2 needs either: (a) to define `createSession(path string)` as a new method, or (b) to reference an existing method. Since `createSessionInCWD()` calls `CreateFromDir(m.cwd, m.command)`, the projects page needs a generalized `createSession(path)` that calls `CreateFromDir(path, m.command)`.

The task should make explicit that it creates the `createSession(path)` method (which `createSessionInCWD` can then delegate to).

**Current**:
```
**Do**:
- In internal/tui/model.go:
  - Define a ProjectsLoadedMsg struct carrying []project.Project and error
  - Replace the stub projectList initialization with: list.New(nil, ProjectDelegate{}, 0, 0), set title to "Projects", set empty text to "No saved projects", disable default quit keybinding
  - Add a loadProjects() tea.Cmd method that calls m.projectStore.CleanStale() then m.projectStore.List() and returns a ProjectsLoadedMsg
  - In Init(), fire the loadProjects command in addition to session fetching
  - Handle ProjectsLoadedMsg: convert to list items, call m.projectList.SetItems(items). On error, remain empty.
  - Handle enter on projects page: get m.projectList.SelectedItem(), cast to ProjectItem, call m.createSession(item.Project.Path)
  - Handle n on projects page: same as sessions page -- m.createSessionInCWD()
  - Handle tea.WindowSizeMsg: update m.projectList.SetSize() as well
  - Configure projects list AdditionalShortHelpKeys for projects-specific bindings
  - In View() for pageProjects: return m.projectList.View()
- Create or update internal/tui/model_test.go with projects page tests
```

**Proposed**:
```
**Do**:
- In internal/tui/model.go:
  - Define a ProjectsLoadedMsg struct carrying []project.Project and error
  - Replace the stub projectList initialization with: list.New(nil, ProjectDelegate{}, 0, 0), set title to "Projects", set empty text to "No saved projects", disable default quit keybinding
  - Add a loadProjects() tea.Cmd method that calls m.projectStore.CleanStale() then m.projectStore.List() and returns a ProjectsLoadedMsg
  - In Init(), fire the loadProjects command in addition to session fetching
  - Handle ProjectsLoadedMsg: convert to list items, call m.projectList.SetItems(items). On error, remain empty.
  - Create func (m Model) createSession(path string) tea.Cmd that calls m.sessionCreator.CreateFromDir(path, nil) and returns SessionCreatedMsg or sessionCreateErrMsg. Refactor createSessionInCWD to delegate: return m.createSession(m.cwd)
  - Handle enter on projects page: get m.projectList.SelectedItem(), cast to ProjectItem, call m.createSession(item.Project.Path)
  - Handle n on projects page: m.createSessionInCWD() (which delegates to createSession)
  - Handle tea.WindowSizeMsg: update m.projectList.SetSize() as well
  - Configure projects list AdditionalShortHelpKeys for projects-specific bindings
  - In View() for pageProjects: return m.projectList.View()
- Create or update internal/tui/model_test.go with projects page tests
```

---

### 8. Task 2-6 Two-Way Page Navigation largely duplicates Phase 1 task 1-7

**Severity**: Important
**Plan Reference**: Phase 2 / tui-session-picker-2-6 (tick-9aebe2) and Phase 1 / tui-session-picker-1-7 (tick-fe1e6f)
**Category**: Scope and Granularity
**Change Type**: update-task

**Details**:
Task 1-7 (Page-Switching Skeleton and Help Bar) implements: p on sessions -> projects, s on projects -> sessions, x toggle, help bar configuration, empty page messages, and state preservation. Task 2-6 (Two-Way Page Navigation and Independent Filters) has nearly identical acceptance criteria: p/s/x navigation, help bars, empty pages, state preservation. The only new content in task 2-6 is verifying independent filter state across pages.

Task 2-6's Do section says "Verify" for most items -- it's a verification task rather than an implementation task. This is not a TDD cycle; it's a confirmation that Phase 1's skeleton still works with the real projects page. The independent filter verification is the only new testable behavior.

Task 2-6 should be reduced to only the new behavior: independent filter state verification. The navigation verification tests are already covered by task 1-7.

**Current**:
```
**Acceptance Criteria**:
- [ ] p on sessions page switches to projects page
- [ ] s on projects page switches to sessions page
- [ ] x on sessions page toggles to projects page
- [ ] x on projects page toggles to sessions page
- [ ] Switching pages preserves filter state on the source page
- [ ] Switching pages does not carry filter text to the destination page
- [ ] Navigating to an empty page shows the empty message
- [ ] Sessions help bar shows [p] projects
- [ ] Projects help bar shows [s] sessions
- [ ] x does not appear in either help bar

**Tests**:
- "s on projects page switches to sessions page"
- "x on projects page toggles to sessions page"
- "switching pages does not carry filter text"
- "filter state preserved when switching back to source page"
- "navigating to empty sessions page shows empty message"
- "navigating to empty projects page shows empty message"
- "x is not shown in help bar"
- "sessions help bar includes p for projects"
- "projects help bar includes s for sessions"
```

**Proposed**:
```
**Acceptance Criteria**:
- [ ] Switching pages preserves filter state on the source page
- [ ] Switching pages does not carry filter text to the destination page
- [ ] Projects page help bar shows [s] sessions (verifies real page replaces stub)
- [ ] Projects page help bar shows correct project-specific keybindings ([e] edit, [d] delete, [b] browse)

**Tests**:
- "switching pages does not carry filter text"
- "filter state preserved when switching back to source page"
- "projects help bar includes s for sessions and project-specific keys"
- "sessions help bar still includes p for projects after projects page replacement"
```

---

### 9. Task 1-3 Modal Overlay System defines modalRename but task 1-4 re-implements it

**Severity**: Minor
**Plan Reference**: Phase 1 / tui-session-picker-1-3 (tick-b29c05) and Phase 1 / tui-session-picker-1-4 (tick-34ba3d)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 1-3 defines `modalRename` as a constant in the modalState enum ("Define type modalState int with constants: modalNone, modalKillConfirm, modalRename (rename used in Task 4)"). This is forward-declaring a value for a task that hasn't been implemented yet. While harmless (it's just an enum constant), it slightly violates task self-containment -- task 1-3 is adding code for task 1-4's scope.

The cleaner approach is for task 1-3 to define only `modalNone` and `modalKillConfirm`, and for task 1-4 to add `modalRename` to the enum. This is minor because the constant alone has no behavioral impact.

**Current**:
```
- Define type modalState int with constants: modalNone, modalKillConfirm, modalRename (rename used in Task 4)
```

**Proposed**:
```
- Define type modalState int with constants: modalNone, modalKillConfirm (additional modal states added by subsequent tasks as needed)
```

---

### 10. Task 2-5 File Browser references pageFileBrowser but task 1-7 only defines pageSessions and pageProjects

**Severity**: Minor
**Plan Reference**: Phase 2 / tui-session-picker-2-5 (tick-f14aa6) and Phase 1 / tui-session-picker-1-7 (tick-fe1e6f)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 1-7 defines the page type with "Replace viewState with type page int and constants pageSessions, pageProjects (keep pageFileBrowser for browser sub-view)". This says "keep pageFileBrowser" implying it already exists in the old viewState. Task 2-5 then uses pageFileBrowser to handle the browser sub-view. This is fine -- pageFileBrowser is carried over from the old code.

However, task 2-5's Do section says "set m.activePage = pageFileBrowser" and "Ensure existing cross-view message handlers work" implying the browser message handlers already exist. Task 2-5 should clarify whether it's creating these handlers or verifying existing ones. The old code uses `viewFileBrowser` (via viewState enum), so task 2-5 is actually migrating the browser integration from the old viewState to the new page system.

No change needed -- the task's acceptance criteria are clear enough. The implementer will see the existing browser code and adapt it.

**Current**:
N/A -- observation only.

**Proposed**:
N/A -- no change proposed. Task is sufficiently clear from context.

---
