TASK: Command-Pending Enter Creates Session with Command (tick-e8fd08)

ACCEPTANCE CRITERIA:
- enter on a project in command-pending mode calls CreateFromDir(projectPath, command)
- Command slice forwarded exactly as stored
- On success, Selected() returns the created session name
- On success, TUI quits
- On session creation error, TUI does not crash and stays on Projects page
- When no command set (normal mode), nil passed as command (backward-compatible)

STATUS: Complete

SPEC CONTEXT:
The specification defines command-pending mode behavior under "Command-Pending Mode > Actions":
"`enter` -- creates a session in the selected project's directory with the pending command, then attaches."
The `createSession` method is the shared path for session creation across all modes (enter, browse, n-key), making it the natural place to forward `m.command`. The spec also states that normal mode enter on Projects page "creates a new session in the selected project's directory and attaches," meaning `m.command` being nil in normal mode must be backward-compatible.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `/Users/leeovery/Code/portal/internal/tui/model.go:602-610` -- `createSession(dir)` passes `m.command` to `CreateFromDir`
  - `/Users/leeovery/Code/portal/internal/tui/model.go:689-698` -- `handleProjectEnter()` calls `createSession(pi.Project.Path)`
  - `/Users/leeovery/Code/portal/internal/tui/model.go:583-585` -- `SessionCreatedMsg` handler sets `m.selected` and returns `tea.Quit`
  - `/Users/leeovery/Code/portal/internal/tui/model.go:586-588` -- `sessionCreateErrMsg` handler returns to current page (no crash, no quit)
  - `/Users/leeovery/Code/portal/internal/tui/model.go:129` -- `command []string` field on Model
- Notes: The implementation is clean and backward-compatible. `m.command` is nil by default and only set via `WithCommand()`. The `createSession` method is a shared helper used by enter, browse, and n-key paths, so forwarding `m.command` in one place covers all three consistently. No drift from plan.

TESTS:
- Status: Adequate
- Coverage:
  - "enter on project in command-pending mode creates session with command" (line 6160) -- verifies CreateFromDir called with correct dir and non-nil command
  - "command slice forwarded exactly to CreateFromDir" (line 6201) -- verifies each element of a multi-arg command slice is forwarded verbatim
  - "selected returns session name after creation" (line 6238) -- verifies Selected() returns the created session name after SessionCreatedMsg
  - "TUI quits after successful session creation" (line 6272) -- verifies tea.Quit returned after SessionCreatedMsg
  - "session creation error keeps TUI on Projects page" (line 6309) -- verifies no crash, stays on PageProjects, Selected() is empty, view renders
  - "normal mode enter on project passes nil command" (line 6366) -- verifies backward compatibility with nil command in normal mode
- Notes: All six tests from the task spec are present and well-structured. Each test verifies a distinct acceptance criterion. The error case test also checks that Selected() remains empty and the view still renders, which is thorough. The mock (`mockSessionCreator`) captures both `createdDir` and `createdCommand`, enabling precise verification. No over-testing -- each test covers a unique behavioral aspect.

CODE QUALITY:
- Project conventions: Followed. Table-driven-style subtests within a named test function. Uses the established mock pattern. Follows the functional options pattern for dependency injection.
- SOLID principles: Good. `createSession` has a single responsibility (build and return a tea.Cmd). The `SessionCreator` interface is minimal (one method). Dependency inversion via interfaces.
- Complexity: Low. `createSession` is 8 lines. `handleProjectEnter` is 8 lines with two early returns. The `SessionCreatedMsg`/`sessionCreateErrMsg` handlers in `Update` are 2-3 lines each.
- Modern idioms: Yes. Uses Go closures for tea.Cmd, value receivers on Model following Bubble Tea conventions.
- Readability: Good. The intent of `createSession` forwarding `m.command` is immediately clear. The separation between `handleProjectEnter` (selects project, calls createSession) and `createSession` (builds the command closure) is clean.
- Issues: None identified.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- (none)
