TASK: Loading page state and view

ACCEPTANCE CRITERIA:
- PageLoading is a valid page constant
- WithServerStarted(true) sets activePage to PageLoading
- WithServerStarted(false) does not change activePage from default (PageSessions)
- Model without WithServerStarted starts on PageSessions (backward compat)
- View() returns centered "Starting tmux server..." on PageLoading
- Loading view is visually distinct from session/project views (no list chrome)
- Fallback to 80x24 when no WindowSizeMsg received
- Ctrl+C during loading quits
- WindowSizeMsg dimensions used for centering when available

STATUS: Complete

SPEC CONTEXT: The spec requires "A dedicated loading interstitial -- a blank screen with centered 'Starting tmux server...' text. Visibly different from the normal TUI so the user knows something is happening. No logo, no progress bar -- just a clean loading state." The loading page is shown when Portal has just started the tmux server and is waiting for plugins (e.g. continuum) to restore sessions.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/model.go:22-31` -- PageLoading added as iota value 0 in page enum, shifting PageSessions/PageProjects/pageFileBrowser
  - `internal/tui/model.go:143` -- `serverStarted bool` field on Model
  - `internal/tui/model.go:144-145` -- `minWaitDone bool`, `sessionsReceived bool` timing state fields
  - `internal/tui/model.go:367-375` -- `WithServerStarted` option sets `serverStarted` and conditionally sets `activePage = PageLoading`
  - `internal/tui/model.go:484` -- `New()` explicitly sets `activePage: PageSessions` (prevents zero-value defaulting to PageLoading after iota shift)
  - `internal/tui/model.go:503` -- `NewModelWithSessions()` also explicitly sets `activePage: PageSessions`
  - `internal/tui/model.go:1244-1248` -- `View()` switch dispatches to `viewLoading()` for PageLoading
  - `internal/tui/model.go:1267-1279` -- `viewLoading()` renders centered text with lipgloss.Place, fallback 80x24
  - `internal/tui/model.go:244-252` -- `ActivePage()` and `ServerStarted()` accessors for testing
  - `internal/tui/model.go:723-728` -- `Update()` swallows all keys during PageLoading except Ctrl+C
- Notes: Implementation matches the plan exactly. The iota shift is handled correctly -- both `New()` and `NewModelWithSessions()` explicitly set `activePage: PageSessions`. The `viewLoading()` method is clean and minimal, using `lipgloss.Place` for centering as specified. No drift detected.

TESTS:
- Status: Adequate
- Coverage:
  - `model_test.go:7362` -- WithServerStarted(true) starts on PageLoading (verifies ActivePage + ServerStarted accessor)
  - `model_test.go:7373` -- WithServerStarted(false) starts on PageSessions
  - `model_test.go:7381` -- Without WithServerStarted starts on PageSessions (backward compat)
  - `model_test.go:7389` -- Loading view contains "Starting tmux server..." text
  - `model_test.go:7400` -- Loading view centers text vertically (checks line position ~8-16 of 24 rows)
  - `model_test.go:7431` -- Loading view does not show session list chrome ("Sessions" title)
  - `model_test.go:7442` -- Fallback dimensions when no WindowSizeMsg received
  - `model_test.go:7457` -- Ctrl+C during loading produces QuitMsg
  - `model_test.go:7472` -- Init returns batch with tick commands on PageLoading
  - `model_test.go:7491` -- SessionsMsg with sessions + minWaitDone transitions away from loading
  - `model_test.go:7510` -- SessionsMsg with sessions before minWait does not transition
  - `model_test.go:7526` -- Empty sessions during loading schedules re-fetch
  - `model_test.go:7539` -- MinWaitElapsedMsg with sessionsReceived transitions
  - `model_test.go:7558` -- MaxWaitElapsedMsg transitions unconditionally
  - `model_test.go:7573` -- Other keys (q, p, enter, esc) are swallowed during loading
  - `model_test.go:7602` -- Orphaned MinWaitElapsedMsg after transition is harmless
  - `model_test.go:7621` -- Orphaned MaxWaitElapsedMsg after transition is harmless
  - `model_test.go:7642` -- Orphaned poll SessionsMsg after transition updates list normally
  - `model_test.go:7666` -- MaxWait with no sessions + projects loaded transitions to PageProjects
  - `model_test.go:7693` -- Transition with sessions + projects loaded stays on PageSessions
- Notes: Tests are thorough and well-structured. They cover all acceptance criteria, edge cases (orphaned messages, key swallowing, fallback dimensions), and transition logic. Tests beyond the scope of task 3-1 (timing/transitions from 3-2) are included here as well, which is fine since they naturally test the loading page behavior end-to-end. No over-testing detected -- each test verifies a distinct behavior. Tests would fail if the feature broke.

CODE QUALITY:
- Project conventions: Followed -- functional options pattern matches existing codebase; test accessors follow existing pattern (ActivePage, ServerStarted); Go doc comments on all exported types/functions
- SOLID principles: Good -- WithServerStarted follows the existing functional option pattern (Open/Closed); viewLoading has single responsibility; page enum cleanly extended
- Complexity: Low -- viewLoading is 10 lines; the switch in View() is a simple dispatch; Update() loading branch is straightforward
- Modern idioms: Yes -- uses lipgloss.Place for centering (idiomatic for Bubble Tea); iota for enum; functional options pattern
- Readability: Good -- clear field names (serverStarted, minWaitDone, sessionsReceived), well-commented message types, method names describe intent
- Issues: None

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The tests for timing/transition logic (lines 7472-7722) go beyond task 3-1 scope and overlap with task 3-2. This is acceptable since they test integrated behavior, but the 3-2 review should note this shared test coverage to avoid flagging missing tests.
- MinWaitElapsedMsg and MaxWaitElapsedMsg are exported types (uppercase). This makes them part of the package's public API. Given they are only used for testing message dispatch in tests, unexported types would be more conventional. However, this appears to be a deliberate choice to enable test assertions from the _test package.
