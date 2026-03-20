---
status: in-progress
created: 2026-03-20
cycle: 1
phase: Traceability Review
topic: Auto Start Tmux Server
---

# Review Tracking: Auto Start Tmux Server - Traceability

## Findings

### 1. TUI loading state only fetches sessions once — no recurring poll to detect sessions appearing after initial fetch

**Type**: Incomplete coverage
**Spec Reference**: "Timing" section — "The TUI's existing refresh cycle detects sessions" and "Sessions appear naturally via the TUI's refresh cycle as tmux/plugins do their thing" under "User Experience"
**Plan Reference**: Phase 3 / Task auto-start-tmux-server-3-2, `Init()` and `SessionsMsg` handler
**Change Type**: update-task

**Details**:
The spec says the TUI path detects sessions via its refresh cycle during the loading state, and transitions "as soon as sessions are detected." Task 3-2's `Init()` fires a single `fetchSessions` command plus the min/max tick commands. If the initial fetch returns no sessions (the primary use case — continuum hasn't restored them yet), there is no mechanism to re-fetch sessions before the 6-second max wait. The TUI would always wait the full 6 seconds after reboot, contradicting the spec's "Transition out of the loading state as soon as sessions are detected."

The fix adds a recurring session poll during the loading state using `tea.Tick` at the same 500ms interval defined in Phase 2. When `SessionsMsg` arrives during loading with no sessions and min wait hasn't elapsed, a new fetch is scheduled after `DefaultPollInterval`. When min wait has elapsed but no sessions yet, same behavior — keep polling until sessions appear or max wait fires.

**Current**:
```markdown
## auto-start-tmux-server-3-2 | approved

### Task 2: Timing messages and transition logic

**Problem**: Task 3-1 adds the loading page, but it has no way to transition to the normal view. The spec requires the TUI to transition out of the loading interstitial when (a) sessions are detected AND the minimum wait (1s) has elapsed, OR (b) the maximum wait (6s) elapses regardless. The TUI must remain responsive during loading — no blocking — so the timing must be message-based using Bubble Tea's `tea.Tick` pattern.

**Solution**: Implement two custom message types (`minWaitElapsedMsg` and `maxWaitElapsedMsg`) and two corresponding `tea.Tick` commands that fire after the min and max wait durations. Track timing state with `minWaitDone bool` and `sessionsReceived bool` fields on the model. On `Init()`, when on the loading page, batch the existing session fetch with the two tick commands. When `SessionsMsg` arrives during loading, set `sessionsReceived` and transition immediately if `minWaitDone` is also true. When `minWaitElapsedMsg` arrives, set `minWaitDone` and transition immediately if `sessionsReceived` is also true. When `maxWaitElapsedMsg` arrives, transition unconditionally. Transition means setting `activePage` to the appropriate page (sessions or projects) and running `evaluateDefaultPage`.

**Outcome**: The loading interstitial transitions to the normal TUI view when sessions appear (after min wait) or when the max wait elapses. The TUI remains responsive throughout — Ctrl+C works, window resizing works, and all timing is driven by Bubble Tea messages (no goroutine blocking).

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
  3. Modify `Init()` to batch tick commands when on the loading page:
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
     Add `"time"` and ensure `tmux` import is present. The `tmux.DefaultMinWait` and `tmux.DefaultMaxWait` constants are defined by Phase 2 Task 2-1 in `/Users/leeovery/Code/portal/internal/tmux/wait.go`.
  4. Add a `transitionFromLoading()` method that transitions away from the loading page:
     ```go
     func (m *Model) transitionFromLoading() {
         m.activePage = PageSessions
         m.evaluateDefaultPage()
     }
     ```
     This sets `activePage` to `PageSessions` as the default, then `evaluateDefaultPage()` may override it to `PageProjects` if there are no sessions. Note: `evaluateDefaultPage` checks `sessionsLoaded` and `projectsLoaded` — both should be true by the time transition happens (sessions were received, projects loaded in parallel). If projects haven't loaded yet, `evaluateDefaultPage` will be a no-op and the user sees the sessions page.
  5. Update the `SessionsMsg` handler in `Update()` to handle the loading page case. After the existing `m.sessionsLoaded = true` and `m.evaluateDefaultPage()` lines, add:
     ```go
     if m.activePage == PageLoading {
         m.sessionsReceived = true
         if m.minWaitDone {
             m.transitionFromLoading()
         }
         return m, nil
     }
     ```
     **Important**: This check must come AFTER the existing `SessionsMsg` handling (setting sessions, filtering, etc.) but the transition logic is new. The simplest approach is to add it right before the `return m, nil` at the end of the `SessionsMsg` case, checking if we're still on the loading page.
  6. Add handlers for the new message types in `Update()`. Add them to the cross-view message switch:
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
  7. The loading page's key handling should only support Ctrl+C (quit). Since the `Update()` method delegates to page-specific handlers at the bottom (`switch m.activePage`), add a `PageLoading` case to that switch that swallows all key input except Ctrl+C:
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
- [ ] When `SessionsMsg` arrives during loading AND `minWaitDone` is true, the model transitions to the normal view
- [ ] When `SessionsMsg` arrives during loading but `minWaitDone` is false, the model stays on `PageLoading` (waits for min)
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
- `"max wait transitions even with no sessions"` — create model on `PageLoading`, send `MaxWaitElapsedMsg{}` without any `SessionsMsg`, assert `ActivePage()` is no longer `PageLoading` (should be `PageProjects` since no sessions, via `evaluateDefaultPage`)
- `"max wait transitions even when min wait not elapsed"` — create model on `PageLoading`, send `MaxWaitElapsedMsg{}` (without `MinWaitElapsedMsg`), assert transition occurs
- `"Ctrl+C during loading quits"` — create model on `PageLoading`, send `tea.KeyMsg{Type: tea.KeyCtrlC}`, verify quit command returned
- `"regular keys during loading are swallowed"` — create model on `PageLoading`, send `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}`, assert model stays on `PageLoading` and no quit command
- `"transition goes to sessions page when sessions exist"` — create model on `PageLoading` with project store, send `SessionsMsg` with sessions, send `ProjectsLoadedMsg`, send `MinWaitElapsedMsg`, assert `ActivePage() == PageSessions`
- `"transition goes to projects page when no sessions"` — create model on `PageLoading` with project store, send `SessionsMsg` with empty sessions, send `ProjectsLoadedMsg`, send `MaxWaitElapsedMsg`, assert `ActivePage() == PageProjects`

**Edge Cases**:
- Sessions before minWait (still waits): If `SessionsMsg` arrives before `MinWaitElapsedMsg`, the model sets `sessionsReceived = true` but stays on `PageLoading`. The transition only happens when `MinWaitElapsedMsg` subsequently arrives. This prevents a jarring sub-second flash of the loading screen. Spec: "Minimum 1 second — prevents a jarring flash if sessions appear very quickly."
- No sessions by maxWait (transitions anyway): If `MaxWaitElapsedMsg` arrives and no sessions have appeared, the model transitions unconditionally. `evaluateDefaultPage` will route to `PageProjects` (empty state). Spec: "Maximum 6 seconds — proceed regardless after this."
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
```

**Proposed**:
```markdown
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
         m.activePage = PageSessions
         m.evaluateDefaultPage()
     }
     ```
     This sets `activePage` to `PageSessions` as the default, then `evaluateDefaultPage()` may override it to `PageProjects` if there are no sessions. Note: `evaluateDefaultPage` checks `sessionsLoaded` and `projectsLoaded` — both should be true by the time transition happens (sessions were received, projects loaded in parallel). If projects haven't loaded yet, `evaluateDefaultPage` will be a no-op and the user sees the sessions page.
  6. Update the `SessionsMsg` handler in `Update()` to handle the loading page case. After the existing `m.sessionsLoaded = true` and `m.evaluateDefaultPage()` lines, add:
     ```go
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
     ```
     **Important**: This check must come AFTER the existing `SessionsMsg` handling (setting sessions, filtering, etc.) but the transition logic is new. The simplest approach is to add it right before the `return m, nil` at the end of the `SessionsMsg` case, checking if we're still on the loading page. When sessions are empty, `pollSessionsCmd` schedules a re-fetch after 500ms. This continues until sessions appear or `maxWaitElapsedMsg` fires and transitions away from loading.
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
- [ ] Polling stops after transition (no orphaned poll commands — `pollSessionsCmd` returns a `SessionsMsg` which is handled as a normal refresh if the model is no longer on `PageLoading`)

**Tests**:
- `"Init batches tick commands when on loading page"` — create model with `WithServerStarted(true)` and a mock lister, call `Init()`, execute the batch, verify that among the returned messages there is a `SessionsMsg` (from the fetch) — the tick commands will fire after their duration, so just verify the batch contains more than 1 command
- `"Init does not batch tick commands when not on loading page"` — create model without `WithServerStarted`, call `Init()`, execute the command, verify only `SessionsMsg` is returned (no tick commands)
- `"sessions before min wait does not transition"` — create model on `PageLoading`, send `SessionsMsg` with sessions (before sending `MinWaitElapsedMsg`), assert `ActivePage()` is still `PageLoading`
- `"min wait elapsed with sessions transitions to normal view"` — create model on `PageLoading`, send `SessionsMsg` with sessions, then send `MinWaitElapsedMsg{}`, assert `ActivePage()` is `PageSessions`
- `"sessions after min wait transitions immediately"` — create model on `PageLoading`, send `MinWaitElapsedMsg{}` first, then send `SessionsMsg` with sessions, assert `ActivePage()` is `PageSessions`
- `"empty sessions during loading schedules another fetch"` — create model on `PageLoading`, send `SessionsMsg` with empty sessions, assert `ActivePage()` is still `PageLoading` and the returned `tea.Cmd` is non-nil (a poll command was scheduled)
- `"sessions appearing on subsequent poll are detected"` — create model on `PageLoading`, send `SessionsMsg` with empty sessions (triggers re-poll), then send `MinWaitElapsedMsg`, then send `SessionsMsg` with sessions (simulating the re-poll result), assert `ActivePage()` transitions to `PageSessions`
- `"max wait transitions even with no sessions"` — create model on `PageLoading`, send `MaxWaitElapsedMsg{}` without any `SessionsMsg` containing sessions, assert `ActivePage()` is no longer `PageLoading` (should be `PageProjects` since no sessions, via `evaluateDefaultPage`)
- `"max wait transitions even when min wait not elapsed"` — create model on `PageLoading`, send `MaxWaitElapsedMsg{}` (without `MinWaitElapsedMsg`), assert transition occurs
- `"Ctrl+C during loading quits"` — create model on `PageLoading`, send `tea.KeyMsg{Type: tea.KeyCtrlC}`, verify quit command returned
- `"regular keys during loading are swallowed"` — create model on `PageLoading`, send `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}`, assert model stays on `PageLoading` and no quit command
- `"transition goes to sessions page when sessions exist"` — create model on `PageLoading` with project store, send `SessionsMsg` with sessions, send `ProjectsLoadedMsg`, send `MinWaitElapsedMsg`, assert `ActivePage() == PageSessions`
- `"transition goes to projects page when no sessions"` — create model on `PageLoading` with project store, send `SessionsMsg` with empty sessions, send `ProjectsLoadedMsg`, send `MaxWaitElapsedMsg`, assert `ActivePage() == PageProjects`
- `"poll stops after transition from loading"` — create model on `PageLoading`, send `MaxWaitElapsedMsg` (transitions away), then send `SessionsMsg` (from an orphaned poll), assert model handles it as a normal session refresh without re-entering loading logic

**Edge Cases**:
- Sessions before minWait (still waits): If `SessionsMsg` arrives with sessions before `MinWaitElapsedMsg`, the model sets `sessionsReceived = true` but stays on `PageLoading`. The transition only happens when `MinWaitElapsedMsg` subsequently arrives. This prevents a jarring sub-second flash of the loading screen. Spec: "Minimum 1 second — prevents a jarring flash if sessions appear very quickly."
- No sessions by maxWait (transitions anyway): If `MaxWaitElapsedMsg` arrives and no sessions have appeared, the model transitions unconditionally. `evaluateDefaultPage` will route to `PageProjects` (empty state). Spec: "Maximum 6 seconds — proceed regardless after this."
- Sessions appearing after initial fetch: The primary use case — continuum restores sessions 1-3 seconds after server start. The initial `Init()` fetch returns empty. `pollSessionsCmd` re-fetches every 500ms. When sessions appear, the next `SessionsMsg` sets `sessionsReceived = true` and transitions if `minWaitDone`. This matches the spec's intent: "Transition out of the loading state as soon as sessions are detected."
- Orphaned poll commands after transition: If `maxWaitElapsedMsg` fires and transitions away from loading, a pending `pollSessionsCmd` may still deliver a `SessionsMsg`. This is handled safely — the `SessionsMsg` handler updates the session list normally (its existing behavior), and the `if m.activePage == PageLoading` guard is no longer true so no loading-specific logic runs.
- Ctrl+C during loading quits: The loading page handler must check for Ctrl+C and return `tea.Quit`. All other keys are swallowed — the user cannot navigate to sessions/projects during loading.
- Messages arriving after transition: If `MinWaitElapsedMsg` or `MaxWaitElapsedMsg` arrive when `activePage` is no longer `PageLoading` (because transition already happened), they are no-ops due to the `if m.activePage == PageLoading` guard.

**Context**:
> The spec says: "TUI path: The Bubble Tea model owns the wait. The interstitial is the model's initial state. The TUI's existing refresh cycle detects sessions; after min/max bounds are satisfied, transition to the normal view." And: "Session-detection with min/max bounds... Minimum 1 second... Maximum 6 seconds..." And: "Poll interval: 500ms. This applies to the CLI path directly; the TUI path uses its existing refresh cycle (which already polls session state) rather than a separate poll loop."
>
> The TUI does not have a pre-existing recurring session refresh cycle — it only fetches sessions once on `Init()` and on user actions (kill, rename). During the loading state, the `pollSessionsCmd` method provides the equivalent of the CLI's poll loop: it schedules a new `SessionsMsg` fetch after `DefaultPollInterval` (500ms) whenever the previous fetch returns no sessions. This ensures sessions are detected as they appear, matching the spec's intent.
>
> `tea.Tick(duration, func(time.Time) tea.Msg)` is Bubble Tea's mechanism for non-blocking timed messages. It schedules a function to be called after the given duration, returning the result as a message to `Update()`. This is the standard pattern for timers in Bubble Tea — no goroutines, no blocking.
>
> The timing constants `tmux.DefaultMinWait` (1s), `tmux.DefaultMaxWait` (6s), and `tmux.DefaultPollInterval` (500ms) are defined in Phase 2 Task 2-1 at `/Users/leeovery/Code/portal/internal/tmux/wait.go`. The TUI reuses these constants rather than defining its own, ensuring consistency between CLI and TUI paths.
>
> The `evaluateDefaultPage` method (model.go:496) already handles the logic of choosing between sessions and projects page based on loaded data. After transitioning from loading, calling this method ensures the correct default page is shown.

**Spec Reference**: `.workflows/auto-start-tmux-server/specification/auto-start-tmux-server/specification.md` — "Timing" section, "TUI path" under "User Experience", "Two-phase ownership" under "Bootstrap Mechanism"
```

**Resolution**: Pending
**Notes**:

---
