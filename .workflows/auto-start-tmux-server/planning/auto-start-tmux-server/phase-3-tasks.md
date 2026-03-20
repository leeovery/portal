# Phase 3: TUI Loading Interstitial

---
phase: 3
phase_name: TUI Loading Interstitial
total: 3
---

## auto-start-tmux-server-3-1 | approved

### Task 1: Loading page state and view

**Problem**: When Portal starts the tmux server via bootstrap (Phase 1), the TUI currently opens directly to the session list — which will be empty because plugins have not had time to restore sessions. The user sees a blank/empty session list with no indication that anything is happening. The spec requires a visually distinct loading interstitial that shows "Starting tmux server..." while sessions are being restored.

**Solution**: Add a new `PageLoading` page constant to the `page` enum in the TUI model. Add a `serverStarted` boolean field to the `Model` struct, set via a new `WithServerStarted` option. When `serverStarted` is true, the model's initial `activePage` is `PageLoading`. The `View()` method renders a centered "Starting tmux server..." message for this page — no logo, no progress bar, just clean centered text. When `serverStarted` is false, the model behaves exactly as it does today (no loading page).

**Outcome**: A model created with `WithServerStarted(true)` starts on `PageLoading` and renders centered "Starting tmux server..." text. A model created without this option (or with `false`) starts on the default page as before. The loading view uses the cached terminal dimensions for centering, falling back to 80x24 if no `WindowSizeMsg` has been received yet.

**Do**:
- In `/Users/leeovery/Code/portal/internal/tui/model.go`:
  1. Add `PageLoading` to the `page` enum. Place it before `PageSessions` so it gets the `iota` value 0, and shift the others. Alternatively, place it after the existing values — either works, but putting it first is cleaner since it's the initial state when `serverStarted` is true:
     ```go
     const (
         PageLoading  page = iota  // Loading interstitial during bootstrap
         PageSessions              // Sessions list
         PageProjects              // Projects list
         pageFileBrowser           // File browser sub-view
     )
     ```
     **Important**: Since existing code checks `m.activePage` via the default `page` zero value, and `PageLoading` would now be 0, we must ensure the model's `activePage` is explicitly set. The `New()` constructor currently relies on the zero value being `PageSessions`. After this change, `New()` must explicitly set `activePage = PageSessions` so non-bootstrap models skip loading.
  2. Add a `serverStarted bool` field to the `Model` struct.
  3. Add a `WithServerStarted(started bool) Option` function:
     ```go
     func WithServerStarted(started bool) Option {
         return func(m *Model) {
             m.serverStarted = started
             if started {
                 m.activePage = PageLoading
             }
         }
     }
     ```
  4. Update `New()` to explicitly set `activePage: PageSessions`:
     ```go
     m := Model{
         sessionLister: lister,
         sessionList:   newSessionList(nil),
         projectList:   newProjectList(),
         activePage:    PageSessions,
     }
     ```
     Note: Options are applied after construction, so `WithServerStarted(true)` will override `activePage` to `PageLoading`.
  5. Update `NewModelWithSessions()` to explicitly set `activePage: PageSessions`:
     ```go
     func NewModelWithSessions(sessions []tmux.Session) Model {
         items := ToListItems(sessions)
         l := newSessionList(items)
         l.SetSize(80, 24)
         pl := newProjectList()
         pl.SetSize(80, 24)
         m := Model{
             sessions:    sessions,
             sessionList: l,
             projectList: pl,
             activePage:  PageSessions,
         }
         return m
     }
     ```
     This prevents existing tests from defaulting to `PageLoading` after the iota shift.
  6. Add a `ServerStarted() bool` accessor for testing:
     ```go
     func (m Model) ServerStarted() bool {
         return m.serverStarted
     }
     ```
  7. Update `View()` to handle `PageLoading`:
     ```go
     func (m Model) View() string {
         switch m.activePage {
         case PageLoading:
             return m.viewLoading()
         case PageProjects:
             // ... existing code
         }
     }
     ```
  8. Implement `viewLoading()`:
     ```go
     func (m Model) viewLoading() string {
         w := m.termWidth
         h := m.termHeight
         if w == 0 {
             w = 80
         }
         if h == 0 {
             h = 24
         }
         text := "Starting tmux server..."
         return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, text)
     }
     ```
     This uses `lipgloss.Place` for centering, which is the standard Lip Gloss utility for placing content within a bounding box.
  9. Update the `Update()` method's `WindowSizeMsg` handler — no changes needed; it already caches `termWidth`/`termHeight`. But verify the loading page does NOT forward `WindowSizeMsg` to the session/project lists (it does currently, which is fine — it pre-warms their dimensions for when we transition).

- In `/Users/leeovery/Code/portal/internal/tui/model_test.go`:
  - Add tests for the new loading page state and view.

**Acceptance Criteria**:
- [ ] `PageLoading` is a valid `page` constant
- [ ] `WithServerStarted(true)` sets `activePage` to `PageLoading`
- [ ] `WithServerStarted(false)` does not change `activePage` from its default (`PageSessions`)
- [ ] A model created without `WithServerStarted` starts on `PageSessions` (backward compatibility)
- [ ] `View()` returns centered "Starting tmux server..." text when `activePage` is `PageLoading`
- [ ] The loading view is visually distinct from the session/project list (no list chrome, no title, no help bar)
- [ ] When terminal dimensions are 0x0 (no `WindowSizeMsg` received), the loading view falls back to 80x24 for centering
- [ ] When a `WindowSizeMsg` has been received, the loading view uses those dimensions for centering
- [ ] `Ctrl+C` during the loading page quits the TUI (existing handler in `Update` already handles this for all pages — verify in test)

**Tests**:
- `"model with WithServerStarted(true) starts on PageLoading"` — create model with `WithServerStarted(true)`, assert `ActivePage() == PageLoading`
- `"model with WithServerStarted(false) starts on PageSessions"` — create model with `WithServerStarted(false)`, assert `ActivePage() == PageSessions`
- `"model without WithServerStarted starts on PageSessions"` — create model without the option, assert `ActivePage() == PageSessions`
- `"loading view shows Starting tmux server text"` — create model with `WithServerStarted(true)`, send `WindowSizeMsg{Width: 80, Height: 24}`, call `View()`, assert output contains "Starting tmux server..."
- `"loading view centers text in terminal"` — create model with `WithServerStarted(true)`, send `WindowSizeMsg{Width: 80, Height: 24}`, call `View()`, assert the text is not on the first line (it should be roughly centered vertically)
- `"loading view does not show session list chrome"` — create model with `WithServerStarted(true)`, send `WindowSizeMsg`, call `View()`, assert output does NOT contain "Sessions" title
- `"loading view uses fallback dimensions when no WindowSizeMsg received"` — create model with `WithServerStarted(true)`, do NOT send `WindowSizeMsg`, call `View()`, assert output contains "Starting tmux server..." (doesn't crash, renders with 80x24 fallback)
- `"Ctrl+C during loading page quits"` — create model with `WithServerStarted(true)`, send `tea.KeyMsg{Type: tea.KeyCtrlC}`, assert the returned `tea.Cmd` produces a `tea.QuitMsg`

**Edge Cases**:
- Terminal dimensions not yet received (fallback 80x24): Before the first `WindowSizeMsg`, `termWidth` and `termHeight` are 0. The `viewLoading()` method must detect this and use 80x24 as fallback dimensions for centering. This mirrors the fallback already used in `renderListWithModal` in `modal.go`.
- `serverStarted=false` skips loading page: When the server was already running, the model is created without `WithServerStarted` (or with `false`). The `activePage` defaults to `PageSessions` and the loading page is never shown. This is the fast path for normal operation.
- Existing `page` enum change: Adding `PageLoading` as `iota` value 0 shifts the existing constants. The `New()` constructor must explicitly set `activePage = PageSessions` to avoid all models defaulting to the loading page. The `NewModelWithSessions` test helper also needs to explicitly set `activePage: PageSessions` if it doesn't already.

**Context**:
> The spec says: "TUI path: A dedicated loading interstitial — a blank screen with centered 'Starting tmux server...' text. Visibly different from the normal TUI so the user knows something is happening. No logo, no progress bar — just a clean loading state."
>
> The existing `page` enum at model.go:21 uses `iota` starting with `PageSessions = 0`. Adding `PageLoading` as the new first constant changes the zero value. Since Go initializes `int` fields to 0, any model that doesn't explicitly set `activePage` would default to `PageLoading`. This must be addressed by setting `activePage: PageSessions` explicitly in constructors.
>
> `lipgloss.Place(w, h, hPos, vPos, text)` is the idiomatic way to center text in a Bubble Tea/Lip Gloss TUI. It pads the text to fill the given width/height with the content positioned at the specified horizontal and vertical alignment.

**Spec Reference**: `.workflows/auto-start-tmux-server/specification/auto-start-tmux-server/specification.md` — "TUI path" under "User Experience"

## auto-start-tmux-server-3-2 | approved

### Task 2: Timing messages and transition logic

**Problem**: Task 3-1 adds the loading page, but it has no way to transition to the normal view. The spec requires the TUI to transition out of the loading interstitial when (a) sessions are detected AND the minimum wait (1s) has elapsed, OR (b) the maximum wait (6s) elapses regardless. The TUI must remain responsive during loading — no blocking — so the timing must be message-based using Bubble Tea's `tea.Tick` pattern. Additionally, sessions may not exist on the initial fetch (continuum hasn't restored them yet), so the TUI needs to periodically re-fetch sessions during the loading state to detect them as they appear.

**Solution**: Implement two custom message types (`minWaitElapsedMsg` and `maxWaitElapsedMsg`) and two corresponding `tea.Tick` commands that fire after the min and max wait durations. Add a `pollSessionsCmd` that schedules a new session fetch after `DefaultPollInterval` (500ms). Track timing state with `minWaitDone bool` and `sessionsReceived bool` fields on the model. On `Init()`, when on the loading page, batch the existing session fetch with the two tick commands. When `SessionsMsg` arrives during loading with sessions, set `sessionsReceived` and transition immediately if `minWaitDone` is also true. When `SessionsMsg` arrives during loading with no sessions (empty or error), schedule another fetch after `DefaultPollInterval` to keep polling. When `minWaitElapsedMsg` arrives, set `minWaitDone` and transition immediately if `sessionsReceived` is also true. When `maxWaitElapsedMsg` arrives, transition unconditionally. Transition means setting `activePage` to the appropriate page (sessions or projects) and running `evaluateDefaultPage`.

**Outcome**: The loading interstitial transitions to the normal TUI view when sessions appear (after min wait) or when the max wait elapses. Sessions are detected as they appear via periodic re-fetching during the loading state. The TUI remains responsive throughout — Ctrl+C works, window resizing works, and all timing is driven by Bubble Tea messages (no goroutine blocking).

**Do**:
- In `/Users/leeovery/Code/portal/internal/tui/model.go`:
  1. Add new message types:
     ```go
     // minWaitElapsedMsg signals that the minimum loading wait has elapsed.
     type minWaitElapsedMsg struct{}

     // maxWaitElapsedMsg signals that the maximum loading wait has elapsed.
     type maxWaitElapsedMsg struct{}
     ```
  2. Add fields to `Model`:
     ```go
     // Loading interstitial timing state
     minWaitDone      bool
     sessionsReceived bool
     ```
  3. Add a `pollSessionsCmd` method that schedules a new session fetch after `DefaultPollInterval`:
     ```go
     func (m Model) pollSessionsCmd() tea.Cmd {
         return tea.Tick(tmux.DefaultPollInterval, func(time.Time) tea.Msg {
             sessions, err := m.sessionLister.ListSessions()
             return SessionsMsg{Sessions: sessions, Err: err}
         })
     }
     ```
     This uses `tea.Tick` to delay the fetch by 500ms, then executes the session list. The result arrives as a `SessionsMsg` which the existing handler processes. This is the TUI's equivalent of the CLI path's 500ms poll loop — it detects sessions as they appear after continuum restores them.
  4. Modify `Init()` to batch tick commands when on the loading page:
     ```go
     func (m Model) Init() tea.Cmd {
         if m.commandPending {
             return m.loadProjects()
         }

         fetchSessions := func() tea.Msg {
             sessions, err := m.sessionLister.ListSessions()
             return SessionsMsg{Sessions: sessions, Err: err}
         }

         cmds := []tea.Cmd{fetchSessions}

         if loadProjects := m.loadProjects(); loadProjects != nil {
             cmds = append(cmds, loadProjects)
         }

         if m.activePage == PageLoading {
             cmds = append(cmds,
                 tea.Tick(tmux.DefaultMinWait, func(time.Time) tea.Msg {
                     return minWaitElapsedMsg{}
                 }),
                 tea.Tick(tmux.DefaultMaxWait, func(time.Time) tea.Msg {
                     return maxWaitElapsedMsg{}
                 }),
             )
         }

         return tea.Batch(cmds...)
     }
     ```
     Add `"time"` and ensure `tmux` import is present. The `tmux.DefaultMinWait`, `tmux.DefaultMaxWait`, and `tmux.DefaultPollInterval` constants are defined by Phase 2 Task 2-1 in `/Users/leeovery/Code/portal/internal/tmux/wait.go`.
  5. Add a `transitionFromLoading()` method that transitions away from the loading page:
     ```go
     func (m *Model) transitionFromLoading() {
         m.defaultPageEvaluated = false
         m.activePage = PageSessions
         m.evaluateDefaultPage()
     }
     ```
     This sets `activePage` to `PageSessions` as the default, then `evaluateDefaultPage()` may override it to `PageProjects` if there are no sessions. Note: `evaluateDefaultPage` checks `sessionsLoaded` and `projectsLoaded` — both should be true by the time transition happens (sessions were received, projects loaded in parallel). If projects haven't loaded yet, `evaluateDefaultPage` will be a no-op and the user sees the sessions page.
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
  7. Add handlers for the new message types in `Update()`. Add them to the cross-view message switch:
     ```go
     case minWaitElapsedMsg:
         if m.activePage == PageLoading {
             m.minWaitDone = true
             if m.sessionsReceived {
                 m.transitionFromLoading()
             }
         }
         return m, nil
     case maxWaitElapsedMsg:
         if m.activePage == PageLoading {
             m.transitionFromLoading()
         }
         return m, nil
     ```
  8. The loading page's key handling should only support Ctrl+C (quit). Since the `Update()` method delegates to page-specific handlers at the bottom (`switch m.activePage`), add a `PageLoading` case to that switch that swallows all key input except Ctrl+C:
     ```go
     switch m.activePage {
     case PageLoading:
         if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyCtrlC {
             return m, tea.Quit
         }
         return m, nil
     case PageProjects:
         // ... existing
     }
     ```

- In `/Users/leeovery/Code/portal/internal/tui/model_test.go`:
  - Export the message types for testing. Since they're unexported (`minWaitElapsedMsg`, `maxWaitElapsedMsg`), either: (a) make them exported (`MinWaitElapsedMsg`, `MaxWaitElapsedMsg`), or (b) add helper functions that return them. Option (a) is cleaner for testing. The spec has no public API constraint, and test files in `tui_test` package need access. Export them:
    ```go
    type MinWaitElapsedMsg struct{}
    type MaxWaitElapsedMsg struct{}
    ```

**Acceptance Criteria**:
- [ ] `Init()` returns tick commands for min and max wait when `activePage` is `PageLoading`
- [ ] `Init()` does NOT return tick commands when `activePage` is NOT `PageLoading`
- [ ] When `SessionsMsg` arrives during loading with sessions AND `minWaitDone` is true, the model transitions to the normal view
- [ ] When `SessionsMsg` arrives during loading with sessions but `minWaitDone` is false, the model stays on `PageLoading` (waits for min)
- [ ] When `SessionsMsg` arrives during loading with no sessions, a new session fetch is scheduled after `DefaultPollInterval` (500ms)
- [ ] When `MinWaitElapsedMsg` arrives AND `sessionsReceived` is true, the model transitions to the normal view
- [ ] When `MinWaitElapsedMsg` arrives but `sessionsReceived` is false, the model stays on `PageLoading` (waits for sessions or max)
- [ ] When `MaxWaitElapsedMsg` arrives, the model transitions to the normal view unconditionally (even if no sessions)
- [ ] After transition, the model is on the appropriate page (sessions if sessions exist, projects if no sessions — via `evaluateDefaultPage`)
- [ ] `Ctrl+C` during loading quits the TUI
- [ ] Other key presses during loading are swallowed (no page navigation, no filtering)
- [ ] The TUI remains responsive during loading (messages are processed, not blocked)

**Tests**:
- `"Init batches tick commands when on loading page"` — create model with `WithServerStarted(true)` and a mock lister, call `Init()`, execute the batch, verify that among the returned messages there is a `SessionsMsg` (from the fetch) — the tick commands will fire after their duration, so just verify the batch contains more than 1 command
- `"Init does not batch tick commands when not on loading page"` — create model without `WithServerStarted`, call `Init()`, execute the command, verify only `SessionsMsg` is returned (no tick commands)
- `"sessions before min wait does not transition"` — create model on `PageLoading`, send `SessionsMsg` with sessions (before sending `MinWaitElapsedMsg`), assert `ActivePage()` is still `PageLoading`
- `"min wait elapsed with sessions transitions to normal view"` — create model on `PageLoading`, send `SessionsMsg` with sessions, then send `MinWaitElapsedMsg{}`, assert `ActivePage()` is `PageSessions`
- `"sessions after min wait transitions immediately"` — create model on `PageLoading`, send `MinWaitElapsedMsg{}` first, then send `SessionsMsg` with sessions, assert `ActivePage()` is `PageSessions`
- `"empty sessions during loading schedules another fetch"` — create model on `PageLoading`, send `SessionsMsg` with empty sessions, assert `ActivePage()` is still `PageLoading` and the returned `tea.Cmd` is non-nil (a poll command was scheduled)
- `"sessions appearing on subsequent poll are detected"` — create model on `PageLoading`, send `SessionsMsg` with empty sessions (triggers re-poll), then send `MinWaitElapsedMsg`, then send `SessionsMsg` with sessions (simulating the re-poll result), assert `ActivePage()` transitions to `PageSessions`
- `"max wait transitions even with no sessions"` — create model on `PageLoading`, send `MaxWaitElapsedMsg{}` without any `SessionsMsg`, assert `ActivePage()` is no longer `PageLoading` (should be `PageProjects` since no sessions, via `evaluateDefaultPage`)
- `"max wait transitions even when min wait not elapsed"` — create model on `PageLoading`, send `MaxWaitElapsedMsg{}` (without `MinWaitElapsedMsg`), assert transition occurs
- `"Ctrl+C during loading quits"` — create model on `PageLoading`, send `tea.KeyMsg{Type: tea.KeyCtrlC}`, verify quit command returned
- `"regular keys during loading are swallowed"` — create model on `PageLoading`, send `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}`, assert model stays on `PageLoading` and no quit command
- `"transition goes to sessions page when sessions exist"` — create model on `PageLoading` with project store, send `SessionsMsg` with sessions, send `ProjectsLoadedMsg`, send `MinWaitElapsedMsg`, assert `ActivePage() == PageSessions`
- `"transition goes to projects page when no sessions"`
- `"poll stops after transition from loading"` — create model on `PageLoading`, send `MaxWaitElapsedMsg` (transitions away), then send `SessionsMsg` (from an orphaned poll), assert model handles it as a normal session refresh without re-entering loading logic

**Edge Cases**:
- Sessions before minWait (still waits): If `SessionsMsg` arrives before `MinWaitElapsedMsg`, the model sets `sessionsReceived = true` but stays on `PageLoading`. The transition only happens when `MinWaitElapsedMsg` subsequently arrives. This prevents a jarring sub-second flash of the loading screen. Spec: "Minimum 1 second — prevents a jarring flash if sessions appear very quickly."
- No sessions by maxWait (transitions anyway): If `MaxWaitElapsedMsg` arrives and no sessions have appeared, the model transitions unconditionally. `evaluateDefaultPage` will route to `PageProjects` (empty state). Spec: "Maximum 6 seconds — proceed regardless after this."
- Sessions appearing after initial fetch: The primary use case — continuum restores sessions 1-3 seconds after server start. The initial `Init()` fetch returns empty. `pollSessionsCmd` re-fetches every 500ms. When sessions appear, the next `SessionsMsg` sets `sessionsReceived = true` and transitions if `minWaitDone`. This matches the spec's intent: "Transition out of the loading state as soon as sessions are detected."
- Orphaned poll commands after transition: If `maxWaitElapsedMsg` fires and transitions away from loading, a pending `pollSessionsCmd` may still deliver a `SessionsMsg`. This is handled safely — the `SessionsMsg` handler updates the session list normally, and the `if m.activePage == PageLoading` guard is no longer true so no loading-specific logic runs.
- Ctrl+C during loading quits: The loading page handler must check for Ctrl+C and return `tea.Quit`. All other keys are swallowed — the user cannot navigate to sessions/projects during loading.
- Messages arriving after transition: If `MinWaitElapsedMsg` or `MaxWaitElapsedMsg` arrive when `activePage` is no longer `PageLoading` (because transition already happened), they are no-ops due to the `if m.activePage == PageLoading` guard.

**Context**:
> The spec says: "TUI path: The Bubble Tea model owns the wait. The interstitial is the model's initial state. The TUI's existing refresh cycle detects sessions; after min/max bounds are satisfied, transition to the normal view." And: "Session-detection with min/max bounds... Minimum 1 second... Maximum 6 seconds..."
>
> `tea.Tick(duration, func(time.Time) tea.Msg)` is Bubble Tea's mechanism for non-blocking timed messages. It schedules a function to be called after the given duration, returning the result as a message to `Update()`. This is the standard pattern for timers in Bubble Tea — no goroutines, no blocking.
>
> The timing constants `tmux.DefaultMinWait` (1s) and `tmux.DefaultMaxWait` (6s) are defined in Phase 2 Task 2-1 at `/Users/leeovery/Code/portal/internal/tmux/wait.go`. The TUI reuses these constants rather than defining its own, ensuring consistency between CLI and TUI paths.
>
> The `evaluateDefaultPage` method (model.go:496) already handles the logic of choosing between sessions and projects page based on loaded data. After transitioning from loading, calling this method ensures the correct default page is shown.

**Spec Reference**: `.workflows/auto-start-tmux-server/specification/auto-start-tmux-server/specification.md` — "Timing" section, "TUI path" under "User Experience", "Two-phase ownership" under "Bootstrap Mechanism"

## auto-start-tmux-server-3-3 | approved

### Task 3: Wire serverStarted into TUI launch path

**Problem**: Tasks 3-1 and 3-2 add the loading page and transition logic to the TUI model, but the `openTUI` function in `/Users/leeovery/Code/portal/cmd/open.go` does not pass `serverStarted` to the model. Without this wiring, the loading interstitial is never shown even when bootstrap started the server. The `open` command's `RunE` needs to read `serverStarted` from the command context (Phase 2 Task 2-2) and pass it through to `buildTUIModel` via the `tuiConfig`.

**Solution**: Add a `serverStarted bool` field to `tuiConfig`. Modify `buildTUIModel` to call `tui.WithServerStarted(cfg.serverStarted)` when building the model. Modify `openTUI` to accept `serverStarted` as a parameter. In `open`'s `RunE`, read `serverWasStarted(cmd)` and pass it to `openTUI` for the TUI path (destination is empty). For non-TUI paths (destination is not empty), `bootstrapWait` from Phase 2 Task 2-3 already handles the CLI wait.

**Outcome**: When bootstrap starts the server and the user enters the TUI path (`portal open` with no args), the TUI shows the loading interstitial. When the server was already running, the TUI opens directly to the normal view. When a destination is provided (non-TUI path), the CLI wait handles it (Phase 2) and the TUI is not involved.

**Do**:
- In `/Users/leeovery/Code/portal/cmd/open.go`:
  1. Add `serverStarted bool` field to `tuiConfig`:
     ```go
     type tuiConfig struct {
         // ... existing fields
         serverStarted  bool
     }
     ```
  2. Update `buildTUIModel` to pass `serverStarted` to the model:
     ```go
     func buildTUIModel(cfg tuiConfig, initialFilter string, command []string) tui.Model {
         opts := []tui.Option{
             // ... existing options
         }
         if cfg.serverStarted {
             opts = append(opts, tui.WithServerStarted(true))
         }
         m := tui.New(cfg.lister, opts...)
         // ... existing With* calls
         return m
     }
     ```
     Place the `WithServerStarted` option in the `opts` slice (before `tui.New`) rather than as a post-construction `With*` call — this ensures it is applied during `New()` option processing.
  3. Change `openTUI` signature to accept `serverStarted`:
     ```go
     func openTUI(initialFilter string, command []string, serverStarted bool) error {
     ```
  4. In `openTUI`, pass it through to the config:
     ```go
     cfg := tuiConfig{
         // ... existing fields
         serverStarted: serverStarted,
     }
     ```
  5. Update all call sites of `openTUI` in `RunE`:
     - The TUI path (`destination == ""`): `openTUI("", command, serverWasStarted(cmd))` — passes the bootstrap flag
     - The fallback path (`FallbackResult`): `openTUI(r.Query, command, serverWasStarted(cmd))` — also passes the flag, since the TUI will show loading if server was started
  6. Verify that the non-TUI path (`destination != ""`) calls `bootstrapWait(cmd, nil)` (from Phase 2 Task 2-3) and does NOT pass `serverStarted` to the TUI — it uses the CLI wait instead.

- In `/Users/leeovery/Code/portal/cmd/open.go` `RunE`:
  The updated code should look like:
  ```go
  RunE: func(cmd *cobra.Command, args []string) error {
      command, destination, err := parseCommandArgs(cmd, args)
      if err != nil {
          return err
      }

      if destination == "" {
          return openTUI("", command, serverWasStarted(cmd))
      }

      bootstrapWait(cmd, nil)

      query := destination
      qr, err := buildQueryResolver()
      if err != nil {
          return err
      }

      result, err := qr.Resolve(query)
      if err != nil {
          return err
      }

      switch r := result.(type) {
      case *resolver.PathResult:
          return openPath(r.Path, command)
      case *resolver.FallbackResult:
          return openTUI(r.Query, command, serverWasStarted(cmd))
      default:
          return fmt.Errorf("unexpected resolution result: %T", result)
      }
  },
  ```

- Tests in `/Users/leeovery/Code/portal/cmd/open_test.go` (or appropriate test file):
  - Test `buildTUIModel` with `serverStarted: true` and verify the model starts on `PageLoading`
  - Test `buildTUIModel` with `serverStarted: false` and verify the model starts on `PageSessions`

**Acceptance Criteria**:
- [ ] `tuiConfig` has a `serverStarted` field
- [ ] `buildTUIModel` passes `WithServerStarted(true)` to the model when `cfg.serverStarted` is true
- [ ] `buildTUIModel` does NOT pass `WithServerStarted` when `cfg.serverStarted` is false
- [ ] `openTUI` accepts a `serverStarted` parameter and sets it on the config
- [ ] TUI path (`destination == ""`) passes `serverWasStarted(cmd)` to `openTUI`
- [ ] Fallback path (`FallbackResult`) passes `serverWasStarted(cmd)` to `openTUI`
- [ ] Non-TUI path (`PathResult`) does NOT call `openTUI` — uses `bootstrapWait` and `openPath` instead
- [ ] When server was already running (`serverWasStarted` returns false), the TUI opens directly to its normal view (no interstitial)
- [ ] When server was started (`serverWasStarted` returns true), the TUI opens to the loading interstitial
- [ ] All existing tests continue to pass (especially `open` command tests)

**Tests**:
- `"buildTUIModel with serverStarted true starts on loading page"` — create `tuiConfig` with `serverStarted: true` and a mock lister, call `buildTUIModel`, assert `model.ActivePage() == tui.PageLoading`
- `"buildTUIModel with serverStarted false starts on sessions page"` — create `tuiConfig` with `serverStarted: false` and a mock lister, call `buildTUIModel`, assert `model.ActivePage() == tui.PageSessions`
- `"buildTUIModel without serverStarted starts on sessions page"` — create `tuiConfig` with default values and a mock lister, call `buildTUIModel`, assert `model.ActivePage() == tui.PageSessions`
- `"buildTUIModel with serverStarted true preserves other options"` — create `tuiConfig` with `serverStarted: true`, `insideTmux: true`, and `currentSession: "dev"`, call `buildTUIModel`, assert model has `ServerStarted() == true` AND `InsideTmux() == true` AND `CurrentSession() == "dev"`

**Edge Cases**:
- Server already running (no interstitial): When the server was already running, `PersistentPreRunE` stores `serverStarted=false` in the context. `serverWasStarted(cmd)` returns `false`. `openTUI` receives `false` and does not set `WithServerStarted`. The model starts on `PageSessions` — no loading interstitial. This is the normal fast path.
- Open with destination skips TUI (Phase 2 CLI wait): When `destination != ""`, the command calls `bootstrapWait(cmd, nil)` to handle the CLI wait (stderr message + polling), then proceeds to `openPath` or the fallback TUI. If the fallback routes to `openTUI`, the `serverStarted` flag is still passed through so the TUI can show loading if needed. If it routes to `openPath`, no TUI is involved and the CLI wait already handled the delay.
- `openTUI` signature change: All call sites must be updated. There are exactly two calls to `openTUI` in `RunE`: the direct TUI path and the `FallbackResult` case. Both must pass `serverWasStarted(cmd)`.

**Context**:
> The spec's "Two-phase ownership" says: "TUI path: The Bubble Tea model owns the wait. The interstitial is the model's initial state." This task completes the wiring so the model receives the `serverStarted` signal from the CLI layer. The `serverWasStarted(cmd)` function (Phase 2 Task 2-2) reads the bool from the command's `context.Context`. The `WithServerStarted` option (Task 3-1) configures the model. This task bridges the two.
>
> Phase 2 Task 2-3 placed `bootstrapWait(cmd, nil)` in the `open` command's `RunE` for the non-TUI path only (when `destination != ""`). The TUI path was explicitly excluded from the CLI wait because it handles its own wait via the loading interstitial. This task ensures that exclusion is properly wired — the TUI path gets `serverStarted` instead of `bootstrapWait`.

**Spec Reference**: `.workflows/auto-start-tmux-server/specification/auto-start-tmux-server/specification.md` — "Two-phase ownership" under "Bootstrap Mechanism", "TUI path" under "User Experience"
