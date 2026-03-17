TASK: File Browser Integration from Projects Page (tick-f14aa6)

ACCEPTANCE CRITERIA:
- [ ] b on projects page opens the file browser sub-view
- [ ] File browser shows the starting path (cwd) and directory entries
- [ ] Selecting a directory in the browser creates a session and exits TUI
- [ ] Cancelling the browser (Esc with no filter) returns to the projects page
- [ ] Esc in the browser with a filter active clears the filter (browser's own behavior)
- [ ] b works regardless of filter state on the projects page
- [ ] The projects page state is preserved when returning from the browser

STATUS: Complete

SPEC CONTEXT: The spec defines that `b` opens the custom file browser (`internal/ui/browser.go`) as a separate sub-view from the Projects page. It is not a modal -- it replaces the projects page view entirely. The browser emits `BrowserDirSelectedMsg{Path}` on directory selection (creating a session and exiting TUI) and `BrowserCancelMsg` on cancel (returning to Projects page). The `b` key is always accessible regardless of filter state and appears in the help bar. Esc behavior in the browser is handled by the browser itself: Esc with a filter clears the filter, Esc without a filter emits `BrowserCancelMsg`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/model.go:17-27` -- `pageFileBrowser` page constant defined
  - `internal/tui/model.go:119` -- `fileBrowser ui.FileBrowserModel` field on Model
  - `internal/tui/model.go:668-669` -- `b` key handler in `updateProjectsPage` calls `handleBrowseKey()`
  - `internal/tui/model.go:680-687` -- `handleBrowseKey()` creates `ui.NewFileBrowser(m.startPath, m.dirLister)`, sets `m.activePage = pageFileBrowser`
  - `internal/tui/model.go:554-558` -- Cross-view handlers: `BrowserDirSelectedMsg` calls `m.createSession(msg.Path)`; `BrowserCancelMsg` sets `m.activePage = PageProjects`
  - `internal/tui/model.go:595-596` -- `Update()` delegates to `updateFileBrowser(msg)` when `m.activePage == pageFileBrowser`
  - `internal/tui/model.go:895-901` -- `updateFileBrowser()` delegates to `m.fileBrowser.Update(msg)`
  - `internal/tui/model.go:1114-1115` -- `View()` returns `m.fileBrowser.View()` for `pageFileBrowser`
  - `internal/tui/model.go:384` -- `b` included in `projectHelpKeys()`
  - `internal/tui/model.go:395` -- `b` included in `commandPendingHelpKeys()`
- Notes: Implementation matches the task description precisely. The `b` key handler is outside the `m.projectList.SettingFilter()` guard (line 633), so it works when no filter prompt is active. The guard only prevents action keys while the user is actively typing a filter query, which is correct behavior. When a filter is *applied* (not being typed), the `b` key still works as expected.

TESTS:
- Status: Adequate
- Coverage:
  - `TestFileBrowserFromProjectsPage` (model_test.go:5218-5425) -- Dedicated test suite with 7 subtests covering all acceptance criteria:
    1. "b on projects page opens file browser" (5248) -- Verifies view shows starting path and entries, hides projects list
    2. "BrowserDirSelectedMsg creates session and quits" (5273) -- Verifies session creation command is emitted with correct path
    3. "BrowserCancelMsg returns to projects page" (5298) -- Verifies page returns to PageProjects and projects are visible
    4. "Esc in browser with no filter returns to projects page" (5320) -- Verifies Esc triggers BrowserCancelMsg flow
    5. "Esc in browser with filter clears filter" (5343) -- Verifies filter is cleared, not cancelled; still in browser
    6. "b works when projects list has active filter" (5376) -- Verifies browser opens even with applied project filter
    7. "projects page state preserved when returning from browser" (5399) -- Verifies project list items and page after round-trip
  - `TestFileBrowserIntegration` (model_test.go:782-1029) -- Additional integration tests including:
    - "browse option opens file browser" (783) -- from command-pending mode
    - "selection creates session with browsed path" (826) -- verifies CreateFromDir called with correct path
    - "selection registers project in store via session creator" (866)
    - "file browser selection forwards command to session creator" (903) -- verifies command is forwarded
    - "cancel in file browser returns to project picker" (944) -- from command-pending mode
    - "browse works from empty project list" (982)
- Notes: Tests are well-structured, each test verifying a distinct behavior. No redundant tests detected. The acceptance criteria are thoroughly covered with both dedicated tests and integration tests providing overlapping but distinct verification angles.

CODE QUALITY:
- Project conventions: Followed -- uses functional options pattern, interfaces for dependencies, consistent naming
- SOLID principles: Good -- `handleBrowseKey()` has single responsibility; browser integration uses the existing `ui.FileBrowserModel` via composition; `DirLister` interface enables testability via dependency inversion
- Complexity: Low -- `handleBrowseKey()` is 7 lines with a nil-guard and two assignments. Cross-view message handling is a clean switch case.
- Modern idioms: Yes -- uses type assertion for tea.Model interface, Bubble Tea patterns correctly
- Readability: Good -- clear method names, straightforward control flow, comments where helpful
- Issues: None

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The `handleBrowseKey()` nil-guard on `m.dirLister` (line 681-683) silently does nothing if no dir lister is configured. This is consistent with how other optional dependencies are handled (e.g., `handleNewInCWD()` at line 1081), so it follows the established pattern.
- The `TestFileBrowserIntegration` test suite (line 782) has some overlap with `TestFileBrowserFromProjectsPage` (line 5218). The former tests from command-pending mode and the latter from normal mode, so the overlap is justified and tests different contexts.
