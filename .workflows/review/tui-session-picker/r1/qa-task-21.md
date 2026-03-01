TASK: Command-Pending Browse and N-Key with Command (tick-c5bbbb)

ACCEPTANCE CRITERIA:
- Browse directory selection in command-pending mode creates session with pending command
- Browse cancel in command-pending mode returns to locked Projects page
- n key creates session in cwd with pending command in command-pending mode
- n key creates session in cwd without command in normal mode
- n key works from both Sessions and Projects pages
- Command slice forwarded to CreateFromDir for all three paths (enter, browse, n-key)

STATUS: Complete

SPEC CONTEXT: The spec defines that in command-pending mode, `b` opens the file browser (same as normal mode) and `n` creates a session in cwd with the pending command. The `n` key works from both pages, and the command is forwarded to `CreateFromDir`. Browse cancel returns to Projects page. All session creation paths (enter, browse, n-key) go through `createSession` which always forwards `m.command`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `/Users/leeovery/Code/portal/internal/tui/model.go:554-558` -- BrowserDirSelectedMsg and BrowserCancelMsg handled at top-level Update, ensuring they work regardless of page state
  - `/Users/leeovery/Code/portal/internal/tui/model.go:602-610` -- `createSession` always passes `m.command` to `CreateFromDir`
  - `/Users/leeovery/Code/portal/internal/tui/model.go:657` -- `n` key on Projects page calls `handleNewInCWD()`
  - `/Users/leeovery/Code/portal/internal/tui/model.go:943` -- `n` key on Sessions page calls `handleNewInCWD()`
  - `/Users/leeovery/Code/portal/internal/tui/model.go:1080-1089` -- `handleNewInCWD` -> `createSessionInCWD` -> `createSession(m.cwd)`
  - `/Users/leeovery/Code/portal/internal/tui/model.go:689-698` -- `handleProjectEnter` also goes through `createSession`
  - `/Users/leeovery/Code/portal/internal/tui/model.go:669` -- `b` key handled on Projects page via `handleBrowseKey()`
- Notes: All three paths (enter, browse, n-key) converge on the single `createSession(dir)` method which always passes `m.command`. This is a clean, DRY design. BrowserCancelMsg always sets `activePage = PageProjects` (line 557), which is correct for both normal and command-pending modes.

TESTS:
- Status: Adequate
- Coverage: All 6 specified tests exist in `TestCommandPendingBrowseAndNKey`:
  1. "browse directory selection forwards command in command-pending mode" (line 6404) -- verifies command slice and dir passed to CreateFromDir
  2. "browse cancel returns to locked Projects page in command-pending mode" (line 6454) -- verifies page is PageProjects and command-pending banner still present
  3. "n-key creates session in cwd with command in command-pending mode" (line 6498) -- verifies command slice and cwd passed to CreateFromDir
  4. "n-key creates session in cwd without command in normal mode" (line 6540) -- verifies nil command in normal mode
  5. "n-key works from Sessions page" (line 6578) -- verifies SessionCreatedMsg returned with correct dir
  6. "n-key works from Projects page" (line 6621) -- verifies SessionCreatedMsg returned with correct dir
- Notes: Tests are well-structured with proper setup, execution, and verification. The mock (`mockSessionCreator`) captures both `createdDir` and `createdCommand`, enabling thorough verification. Tests appropriately verify both positive cases (command forwarded) and null cases (nil command in normal mode). No over-testing detected -- each test covers a distinct scenario.

CODE QUALITY:
- Project conventions: Followed -- table-driven subtests grouped under a parent test function, functional options pattern, value receiver on Model methods
- SOLID principles: Good -- `createSession` is the single point of session creation (SRP), all three paths delegate to it (DRY). The `SessionCreator` interface has a single method (ISP).
- Complexity: Low -- `createSession` is a 7-line function. `handleNewInCWD` is a 4-line guard-then-delegate. No branching on command-pending mode needed because `m.command` is naturally nil/empty in normal mode.
- Modern idioms: Yes -- closures for tea.Cmd, clean message-passing architecture
- Readability: Good -- the flow from key press to session creation is easy to trace. Method names are descriptive.
- Issues: None

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- (none)
