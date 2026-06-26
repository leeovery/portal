# Plan: Cold-Boot Restore Lands on Projects

## Phases

### Phase 1: Defer the Cold-Route Landing Decision
status: approved
approved_at: 2026-06-26

**Goal**: Fix the cold concurrent-bootstrap landing-page decision so the picker opens on Sessions (not Projects) when N>0 sessions are restored, and a cold-route `initialFilter` is routed to the session list — by gating `transitionFromLoading()` on the cold route (`progressReceiver != nil`) to set a valid interim `activePage = PageSessions` without setting `sessionsLoaded` or calling `evaluateDefaultPage()`, leaving the single landing decision to the post-restore refetch's `SessionsMsg`. The latch and `evaluateDefaultPage` decision logic stay untouched; the warm/CLI route stays byte-identical. Ships with regression coverage for all six required scenarios, every one of which delivers `ProjectsLoadedMsg` before the transition so the latch is actually exercised (the existing test's blind spot).

**Why this order**: This bug has a single root cause — the one-shot `evaluateDefaultPage()` call firing prematurely against the stale empty Init snapshot inside `transitionFromLoading()`. The fix is one cohesive Update-cycle change confined to `internal/tui/model.go`, and its regression tests share the same code path. There is no intermediate state with independent value to checkpoint, and splitting the production change from its tests (or the tests from each other) would create forward references and phases that aren't meaningful milestones. A single phase is the correct right-sizing.

**Acceptance**:
- [ ] AC1 — Cold route, `Init` `ListSessions` empty, `ProjectsLoadedMsg` delivered during loading, post-restore refetch returns N>0: active page is **Sessions** (asserting the page, not merely list contents).
- [ ] AC2 — Cold route, post-restore refetch genuinely returns zero sessions: active page is **Projects** (no over-correction to always-Sessions).
- [ ] AC3 — Cold route with `initialFilter` + N>0: lands on **Sessions** with the filter applied to the **session** list and `initialFilter` consumed there, not against Projects.
- [ ] AC4/AC5 — Warm route (`progressReceiver == nil`) lands on Sessions for N>0 and Projects for zero, byte-identical to today; `refetchSessionsAfterRestore()` returns `nil` (no extra enumeration).
- [ ] AC6 — `commandPending` launch lands on **Projects** as today; verified that `commandPending` never reaches the modified loading→picker transition.
- [ ] AC7 — After loading-page dismissal and before the post-restore refetch `SessionsMsg`, `activePage` is the valid interim **PageSessions** — never `PageLoading`, undefined, or blank — then the refetch `SessionsMsg` resolves the final landing per AC1.
- [ ] Ordering contract — `ProjectsLoadedMsg` delivered in the interim window (after the transition, before the refetch `SessionsMsg`) does not latch on Projects against the stale list; final page is **Sessions**.
- [ ] On the cold route `transitionFromLoading()` neither sets `sessionsLoaded` nor calls `evaluateDefaultPage()`; the deferral and the `refetchSessionsAfterRestore()` dispatch occur in the same handler return and stay coupled; `progressReceiver != nil` is the sole cold-route discriminator (no `serverStarted` / `tmux has-server` / `shouldRunConcurrentBootstrap` re-probe introduced).
- [ ] The `defaultPageEvaluated` latch and `evaluateDefaultPage`'s decision logic are unmodified; a failing refetch `SessionsMsg` still quits without stranding the interim page; the full suite (`go test ./...`) is green with no regressions and no `t.Parallel()` added.

#### Tasks
status: approved
approved_at: 2026-06-26

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| cold-boot-restore-lands-on-projects-1-1 | Gate transitionFromLoading on the cold route + reproduction test | refetch dispatch stays coupled in same handler return; warm route keeps sessionsLoaded=true + evaluateDefaultPage() byte-identical |
| cold-boot-restore-lands-on-projects-1-2 | Cold-route decision-correctness coverage (no over-correction + filter routing) | zero-session cold boot lands on Projects (over-correction guard); initialFilter routes to session list not project list and is zeroed there |
| cold-boot-restore-lands-on-projects-1-3 | Warm-route parity guard | warm route dispatches no post-complete refetch; zero-session warm boot lands on Projects |
| cold-boot-restore-lands-on-projects-1-4 | Interim-page and late-ProjectsLoadedMsg ordering invariants | interim render may briefly show Sessions empty-state (not special-cased); late ProjectsLoadedMsg must not latch Projects against the stale list |
