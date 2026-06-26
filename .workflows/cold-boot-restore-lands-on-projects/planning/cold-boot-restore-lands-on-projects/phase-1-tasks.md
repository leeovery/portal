---
phase: 1
phase_name: Defer the Cold-Route Landing Decision
total: 4
---

## cold-boot-restore-lands-on-projects-1-1 | approved

### Task 1-1: Gate transitionFromLoading on the cold route + reproduction test

**Problem**: On the cold concurrent-bootstrap route (`portal open` against a fresh tmux server, TUI picker) the picker opens on **Projects** instead of **Sessions** even though N>0 sessions were just restored — the user must press `x` to reach them. `transitionFromLoading()` unconditionally calls `evaluateDefaultPage()` against the stale empty Init snapshot (enumerated at frame one, before Restore created any session), which lands on Projects and **permanently latches** the decision via `defaultPageEvaluated`. The post-restore refetch then repairs the list contents but `evaluateDefaultPage()` is by then a guarded no-op, so the page is never re-decided.

**Solution**: Gate `transitionFromLoading()` on the cold route (`m.progressReceiver != nil`): set a valid interim `activePage = PageSessions` but do **not** set `m.sessionsLoaded` and do **not** call `m.evaluateDefaultPage()`, leaving `sessionsLoaded` false so every `evaluateDefaultPage()` invocation early-returns until the post-restore refetch's `SessionsMsg` lands and makes the one true decision against the repaired list. The warm route (`progressReceiver == nil`) keeps today's synchronous `sessionsLoaded = true` + `evaluateDefaultPage()` exactly as-is. The `refetchSessionsAfterRestore()` dispatch must stay in the SAME handler return as the transition (already the case in both arms — do not move it).

**Outcome**: A cold boot with N>0 restored sessions opens the picker directly on the Sessions page listing all N sessions, with no `x` keypress required; the warm route is byte-identical to today. A regression test reproduces the exact bug ordering and now passes.

**Do**:
- Edit `internal/tui/model.go`, function `transitionFromLoading()` (currently ~line 1828, a `*Model` pointer receiver). Replace the unconditional body:
  ```go
  func (m *Model) transitionFromLoading() {
      m.activePage = PageSessions
      m.sessionsLoaded = true
      m.evaluateDefaultPage()
  }
  ```
  with a cold-route branch keyed on `m.progressReceiver != nil`:
  ```go
  func (m *Model) transitionFromLoading() {
      // Always move off the loading page onto a valid interim picker page.
      m.activePage = PageSessions
      if m.progressReceiver != nil {
          // Cold concurrent route: the Init snapshot is stale/empty (Restore ran
          // in a goroutine AFTER frame-one enumeration). Defer the landing decision
          // to the post-restore refetch's SessionsMsg: leave sessionsLoaded false so
          // every evaluateDefaultPage() caller early-returns (cannot latch against
          // the stale interim list) until the refetch lands. The refetch is
          // dispatched in the SAME handler return as this transition (see the
          // LoadingMinElapsedMsg / BootstrapCompleteMsg arms), so its SessionsMsg
          // necessarily arrives with activePage != PageLoading and makes the one
          // decision against the post-restore list.
          return
      }
      // Warm / CLI route: the Init snapshot is already post-restore
      // (PersistentPreRunE ran the orchestrator synchronously). Make the decision
      // now, exactly as today.
      m.sessionsLoaded = true
      m.evaluateDefaultPage()
  }
  ```
- Do NOT touch `evaluateDefaultPage()` (~line 1615), the `defaultPageEvaluated` latch, the `LoadingMinElapsedMsg` arm (~line 2005), or the `BootstrapCompleteMsg` arm (~line 2046). Confirm both arms still `return m, tea.Batch(m.surfaceBufferedWarnings(), m.refetchSessionsAfterRestore())` immediately after `transitionFromLoading()` so the refetch stays coupled in the same return.
- Add test case 1 (the exact bug-ordering reproduction) to `internal/tui/coldboot_session_refetch_test.go`, white-box `package tui`. Name it `TestColdBoot_NPositive_LandsOnSessions` (or similar). Build the model with `New(lister, WithServerStarted(true), WithProgressReceiver(func() tea.Msg { return nil }), WithProjectStore(...))` where the lister is a `coldBootStepLister` returning the post-restore N>0 snapshot on its single (refetch) call. Drive the cold lifecycle **delivering `ProjectsLoadedMsg` while on `PageLoading` BEFORE the transition** (this is mandatory — without it `projectsLoaded` stays false and the latch never fires, so the test would pass for the wrong reason as the old test did), then deliver `LoadingMinElapsedMsg` + `BootstrapCompleteMsg` to close both gates, then drain the resulting batch (which carries the refetch). Assert `final.ActivePage() == PageSessions` AND `visibleSessionNames(final)` equals the N restored names.
- Because the existing `driveColdBootToSessions` driver does NOT deliver `ProjectsLoadedMsg` and `t.Fatalf`s if the post-transition page is not `PageSessions`, either extend the driver to accept/inject a `ProjectsLoadedMsg` before the gate-closing step, or write the lifecycle inline in the new test (deliver `WindowSizeMsg`, stale `SessionsMsg` on `PageLoading`, `ProjectsLoadedMsg` on `PageLoading`, `LoadingMinElapsedMsg`, `BootstrapCompleteMsg`, then `drainBatchToModel` the complete cmd). Prefer the inline form so the existing driver and its callers stay untouched. Reuse `drainBatchToModel` and `coldBootStepLister` as-is.

**Acceptance Criteria**:
- [ ] AC1: A cold boot (`progressReceiver != nil`, `ProjectsLoadedMsg` delivered during loading, refetch returns N>0) lands with `ActivePage() == PageSessions` and the list shows all N restored session names — no `x` required.
- [ ] `transitionFromLoading()` on the cold route sets `activePage = PageSessions`, does NOT set `sessionsLoaded`, and does NOT call `evaluateDefaultPage()`.
- [ ] The new reproduction test fails against the pre-fix `transitionFromLoading()` (lands on Projects) and passes against the post-fix code — verified by running the test before applying the production edit, or by reasoning that the asserted `PageSessions` is unreachable pre-fix because the latch fires on the stale empty list.
- [ ] The refetch dispatch (`refetchSessionsAfterRestore()`) remains in the same handler return as the transition in both the `LoadingMinElapsedMsg` and `BootstrapCompleteMsg` arms (unmoved, still `tea.Batch`'d with `surfaceBufferedWarnings()`).
- [ ] `go test ./internal/tui/...` passes, including the pre-existing `TestColdBoot_PostCompleteRefetch_ReflectsRestoredSessions`, `TestColdBoot_PostCompleteRefetch_CompleteBeforeMinElapsed`, and `TestWarmRoute_NoPostCompleteRefetch`.

**Tests**:
- `"it lands the cold-route picker on Sessions when N>0 sessions are restored (ProjectsLoadedMsg delivered during loading, refetch returns N>0)"`
- `"it leaves sessionsLoaded false and does not call evaluateDefaultPage on the cold route at transition time"` (assert via the observable: page is interim Sessions and only flips to its final decision after the refetch SessionsMsg — covered jointly with 1-4 test case 5).
- `"it keeps the refetch coupled in the same handler return so exactly one decision-bearing SessionsMsg follows the transition"` (the lister's `calls` count is exactly 1 after the drain).

**Edge Cases**:
- The refetch dispatch MUST stay coupled in the same handler return — both the `LoadingMinElapsedMsg` and `BootstrapCompleteMsg` arms already `return m, tea.Batch(m.surfaceBufferedWarnings(), m.refetchSessionsAfterRestore())` immediately after `transitionFromLoading()`. Do not split the deferral from the refetch dispatch; the spec's ordering contract requires the interim page to be followed by exactly one decision-bearing `SessionsMsg`.
- The warm route (`progressReceiver == nil`) must keep `sessionsLoaded = true` + `evaluateDefaultPage()` byte-identical — verified by the pre-existing `TestWarmRoute_NoPostCompleteRefetch` and by Task 1-3.
- The new test MUST deliver `ProjectsLoadedMsg` during loading; omitting it leaves `projectsLoaded` false and the test passes vacuously (the old-test trap the spec calls out).

**Context**:
> Root cause (spec §Context): `evaluateDefaultPage()` lands on Sessions only when `len(m.sessionList.Items()) > 0`, else Projects, and is one-shot latched by `defaultPageEvaluated`. On the cold route Init's frame-one `fetchSessions` enumerates tmux before Restore creates any session, so the snapshot is empty; the premature `evaluateDefaultPage()` inside `transitionFromLoading()` latches Projects against that stale list before the post-restore refetch can repair it.
> Fix mechanism (spec §Fix Approach, do not re-derive): on the cold route set interim `activePage = PageSessions`, do NOT set `sessionsLoaded`, do NOT call `evaluateDefaultPage()`; the refetch's `SessionsMsg` (same handler return) then sets `sessionsLoaded = true` and runs the one decision against the post-restore list. `sessionsLoaded` is already false at transition time because the frame-one stale `SessionsMsg` ingests contents on `PageLoading` but deliberately does not flip `sessionsLoaded` — the fix only needs to *not set* it, never to reset it.
> Canonical predicate (spec §Constraints): `progressReceiver != nil` is the single authoritative cold-route discriminator, identical to the existing `refetchSessionsAfterRestore()` guard. Do NOT introduce an alternative (no re-probing `serverStarted`, `tmux has-server`, or `shouldRunConcurrentBootstrap`).
> Testing requirement (spec §Testing Requirements): every cold-route landing test MUST deliver `ProjectsLoadedMsg` before the loading-page transition and MUST assert the **active page**, not merely list contents — the existing `TestColdBoot_PostCompleteRefetch_ReflectsRestoredSessions` passes for the wrong reason precisely because it never delivers `ProjectsLoadedMsg`.

**Spec Reference**: `.workflows/cold-boot-restore-lands-on-projects/specification/cold-boot-restore-lands-on-projects/specification.md` — §Fix Approach, AC1, §Testing Requirements case 1, §Constraints (Canonical cold-route predicate, Latch preserved).

## cold-boot-restore-lands-on-projects-1-2 | approved

### Task 1-2: Cold-route decision-correctness coverage (no over-correction + filter routing)

**Problem**: The fix in Task 1-1 must not over-correct: a genuine zero-session cold boot must still land on **Projects** (AC2), and a cold boot carrying an `initialFilter` must route that filter to the **session** list — and consume it there — once the page resolves to Sessions (AC3, the filter co-defect). Both behaviours flow from deferring the single `evaluateDefaultPage()` call (which owns both the `len(Items())>0` test and the `initialFilter` application), so they need explicit regression coverage to lock the deferral's correctness.

**Solution**: Add two cold-route tests to `internal/tui/coldboot_session_refetch_test.go` that exercise `evaluateDefaultPage()` running at the post-restore `SessionsMsg` (not at transition time): one where the refetch returns zero sessions (must land on Projects), and one with N>0 and an `initialFilter` (must land on Sessions with the filter applied to and consumed by the session list, not the project list). No production change beyond Task 1-1.

**Outcome**: Two passing tests prove the deferred decision runs the same `len(Items())>0` test against the post-restore list (so zero sessions → Projects) and that the deferred `initialFilter` application lands on the Sessions list when the page resolves to Sessions, zeroing `m.initialFilter`.

**Do**:
- Test case 2 (AC2, over-correction guard), e.g. `TestColdBoot_ZeroSessions_LandsOnProjects`: build with `New(lister, WithServerStarted(true), WithProgressReceiver(func() tea.Msg { return nil }), WithProjectStore(...))` where the `coldBootStepLister` returns an EMPTY slice on the refetch call. Deliver `WindowSizeMsg`, a stale empty `SessionsMsg` on `PageLoading`, `ProjectsLoadedMsg` (with at least one project so Projects is a meaningful landing) on `PageLoading`, then `LoadingMinElapsedMsg` + `BootstrapCompleteMsg`, then `drainBatchToModel` the refetch. Assert `final.ActivePage() == PageProjects` and `visibleSessionNames(final)` is empty.
- Test case 3 (AC3, filter routing), e.g. `TestColdBoot_InitialFilter_RoutesToSessions`: build the model and apply `.WithInitialFilter("alpha")` (a post-construction `Model` method — chain it onto the `New(...)` result before driving). The `coldBootStepLister` returns an N>0 snapshot on the refetch call where at least one session name matches the filter (e.g. `{Name: "restored-alpha"}`, `{Name: "restored-bravo"}`). Drive the cold lifecycle delivering `ProjectsLoadedMsg` during loading, close both gates, drain the refetch. Assert:
  - `final.ActivePage() == PageSessions`
  - `final.SessionListFilterValue() == "alpha"` and `final.SessionListFilterState() == list.FilterApplied`
  - `final.ProjectListFilterValue() == ""` and `final.ProjectListFilterState()` is NOT `list.FilterApplied` (the filter did NOT route to Projects)
  - `final.InitialFilter() == ""` (the filter was consumed in the decision)
- Import `charm.land/bubbletea/v2/.../list` as needed for `list.FilterApplied` — check the existing import alias used in the tui test files (`bubbles/list`; e.g. other tui tests import it as `list`). Match the existing import path used in `model.go` for `list`.
- Use a real or stub `ProjectStore` that returns the desired projects via `List()`/`CleanStale()` — but note the tests deliver `ProjectsLoadedMsg` directly through `Update`, so the store contents only matter if you choose to drive projects through the store's command rather than a direct message. Prefer delivering `ProjectsLoadedMsg{Projects: []project.Project{{Path: "/p/one", Name: "one"}}}` directly for determinism (mirror the `stubProjectStore` / `smProjectStore` patterns already in the package).

**Acceptance Criteria**:
- [ ] AC2: cold boot whose post-restore refetch returns zero sessions lands on `PageProjects` (over-correction guard) — the deferred decision runs the same `len(Items())>0` test, so empty → Projects.
- [ ] AC3: cold boot with `initialFilter` and N>0 lands on `PageSessions` with the filter applied to the session list (`SessionListFilterValue()` equals the filter, state `FilterApplied`), the project list filter untouched (empty value, not `FilterApplied`), and `InitialFilter()` zeroed.
- [ ] Both tests deliver `ProjectsLoadedMsg` before the loading-page transition and assert the **active page** (not just list contents).
- [ ] Both tests pass under `go test ./internal/tui/...`.

**Tests**:
- `"it lands a zero-session cold boot on Projects (no over-correction to always-Sessions)"`
- `"it routes a cold-route initialFilter to the session list and consumes it when the page resolves to Sessions"`
- `"it does not apply the initialFilter to the project list on the cold route"`

**Edge Cases**:
- Zero-session cold boot must land on Projects: the fix changes *when* the `len(Items())>0` test runs, never *what* it tests (spec §Constraints: No over-correction). The refetch returning empty is the genuine zero-session case — distinct from the stale empty Init snapshot.
- The `initialFilter` is applied inside `evaluateDefaultPage()` only when `m.activePage == PageSessions && !m.commandPending`, then `m.initialFilter` is zeroed unconditionally — so on the cold route the filter is routed to Sessions because the deferred decision resolves to Sessions, never against the stale interim list.
- A filter that matches zero sessions would still land on Sessions (the page decision is on raw `len(Items())`, not filtered count) — keep the chosen session names matching the filter so the test asserts a non-empty visible list; matching-vs-non-matching filtered count is not part of this fix's scope.

**Context**:
> Filter co-defect (spec §Context): the `initialFilter` application lives inside `evaluateDefaultPage()` and is gated on the chosen page being Sessions; because the pre-fix latched decision is Projects, the filter is routed to the project list and `initialFilter` is zeroed in the same one-shot call so it is never re-applied to Sessions. Deferring the single `evaluateDefaultPage()` call resolves this with no extra code (spec §Fix Approach): on the cold route the filter is applied during the post-restore decision, so when the page resolves to Sessions the filter is routed to — and consumed by — the session list.
> No over-correction (spec §Constraints): the deferred decision runs the same `len(Items()) > 0` test against the post-restore list, so a genuine zero-session cold boot still lands on Projects (AC2).
> `evaluateDefaultPage()` filter block (model.go ~lines 1635-1646): applies `m.initialFilter` to `m.sessionList` when `activePage == PageSessions && !commandPending`, else to `m.projectList`, then sets `m.initialFilter = ""`.

**Spec Reference**: `.workflows/cold-boot-restore-lands-on-projects/specification/cold-boot-restore-lands-on-projects/specification.md` — AC2, AC3, §Testing Requirements cases 2 & 3, §Constraints (No over-correction).

## cold-boot-restore-lands-on-projects-1-3 | approved

### Task 1-3: Warm-route parity guard

**Problem**: The fix must not perturb the warm / CLI / synchronous route — the zero-new-risk contract from `spectrum-tui-design` (the warm-path startup sequence has prior-incident history: slow-open / zombie-session). The warm route (`progressReceiver == nil`) must still make the landing decision synchronously at `transitionFromLoading()` against its already-post-restore Init snapshot, dispatch no post-complete refetch, land on Sessions for N>0, and land on Projects for zero sessions — all byte-identical to today.

**Solution**: Add warm-route parity tests to `internal/tui/coldboot_session_refetch_test.go` (or the existing warm-route test neighbourhood) that build the model WITHOUT a `progressReceiver` and assert: (a) `refetchSessionsAfterRestore()` returns `nil` on the warm route, (b) the transition handler dispatches no post-complete refetch cmd, (c) N>0 warm boot lands on Sessions, and (d) zero-session warm boot lands on Projects. No production change.

**Outcome**: Tests prove the warm route's landing decision is made synchronously at transition (no deferral, no refetch) and lands correctly for both N>0 and zero-session cases — confirming Task 1-1's `progressReceiver == nil` branch preserves today's behaviour.

**Do**:
- The existing `TestWarmRoute_NoPostCompleteRefetch` already covers N>0 warm + no-refetch. Extend coverage with the missing rows:
- Test for AC5 (zero-session warm boot), e.g. `TestWarmRoute_ZeroSessions_LandsOnProjects`: build with `New(lister, WithServerStarted(true), WithProjectStore(...))` — NO `WithProgressReceiver`. The lister returns an EMPTY snapshot. Drive `WindowSizeMsg`, empty `SessionsMsg` on `PageLoading`, `ProjectsLoadedMsg` (with ≥1 project) on `PageLoading`, `LoadingMinElapsedMsg`, `BootstrapCompleteMsg`. Assert `final.ActivePage() == PageProjects`. Assert the complete handler's returned cmd does NOT trigger a refetch (lister call count unchanged across the transition — mirror the assertion style in `TestWarmRoute_NoPostCompleteRefetch`).
- Add a direct unit assertion that `refetchSessionsAfterRestore()` returns `nil` when `progressReceiver == nil`, e.g. `TestWarmRoute_RefetchSessionsAfterRestore_Nil`: construct a warm model (no `WithProgressReceiver`) and assert `m.refetchSessionsAfterRestore() == nil` (white-box, direct method call — the function is a `Model` value receiver). Pair with a cold model (`WithProgressReceiver(...)`) asserting it returns non-nil, to lock the predicate symmetry.
- Confirm (assertion in a test, or by inspection captured in a test comment) that on the warm route `transitionFromLoading()` still sets `sessionsLoaded = true` and runs `evaluateDefaultPage()`: the N>0 warm test landing on Sessions and the zero-session warm test landing on Projects jointly prove the synchronous decision fires at transition.
- AC6 (commandPending preservation), e.g. `TestCommandPending_LandsOnProjects_NoInterimFlash`: build with `New(lister, WithServerStarted(true), WithProgressReceiver(func() tea.Msg { return nil })).WithCommand([]string{"echo", "hi"})` — `WithCommand` sets `commandPending = true` and `activePage = PageProjects` (model.go:632-639). Drive `WindowSizeMsg`, then `ProjectsLoadedMsg` (≥1 project) so the `commandPending` arm of `evaluateDefaultPage()` (model.go:1619-1622) can resolve. Assert `final.ActivePage() == PageProjects`. The test verifies that a `commandPending` launch lands on Projects regardless of session count and never sits on `PageSessions` (the interim page the deferral introduces). Add a comment recording the spec invariant: `Init`'s `commandPending` branch (model.go:1882-1883) returns before wiring `loadingPadTick` / `progressReceiver`, so `transitionFromLoading()` is never invoked for a `commandPending` launch and no interim Sessions flash occurs — even though `WithProgressReceiver` is wired, the `commandPending` short-circuit takes precedence. No `t.Parallel()`.

**Acceptance Criteria**:
- [ ] AC4: warm route (`progressReceiver == nil`), N>0 sessions → lands on `PageSessions`, byte-identical to today (covered by the pre-existing `TestWarmRoute_NoPostCompleteRefetch` plus the no-refetch assertion).
- [ ] AC5: warm route, zero sessions → lands on `PageProjects`.
- [ ] `refetchSessionsAfterRestore()` returns `nil` on the warm route (no extra enumeration); returns non-nil on the cold route.
- [ ] The warm-route transition handler dispatches no post-complete refetch cmd (lister call count does not bump across the transition).
- [ ] AC6: a `commandPending` launch lands on `PageProjects` regardless of session count and is never observed on the interim `PageSessions`; the `commandPending → Projects` arm of `evaluateDefaultPage()` is unchanged.
- [ ] Verified that the `commandPending` path never reaches `transitionFromLoading()` (the modified loading→picker transition): `Init`'s `commandPending` branch returns before wiring the loading-dismissal machinery, so no interim Sessions flash occurs on a `commandPending` launch.
- [ ] All warm-route tests pass under `go test ./internal/tui/...`.

**Tests**:
- `"it lands the warm-route picker on Projects when zero sessions exist"`
- `"it returns nil from refetchSessionsAfterRestore on the warm route and non-nil on the cold route"`
- `"it dispatches no post-complete refetch on the warm route"` (existing `TestWarmRoute_NoPostCompleteRefetch` — keep green)
- `"it lands a commandPending launch on Projects regardless of session count"`
- `"it never flashes the interim Sessions page on a commandPending launch (transitionFromLoading is not invoked)"`

**Edge Cases**:
- Warm route must dispatch no post-complete refetch: the synchronous route ran the orchestrator before the model was built, so its Init snapshot is already post-restore — a refetch would be wasted work and a behaviour change (spec §Constraints: Warm / CLI / direct-path untouched).
- Zero-session warm boot lands on Projects: the warm route's synchronous `evaluateDefaultPage()` at transition runs the same `len(Items())>0` test against the already-post-restore Init snapshot (empty → Projects), unchanged from today.
- The `progressReceiver == nil` predicate is the sole discriminator; do not assert against `serverStarted` or any other probe (the warm test sets `WithServerStarted(true)` to force the loading page yet omits the receiver — proving the receiver, not `serverStarted`, gates the deferral).
- `commandPending` does not intersect the deferral: `Init`'s `commandPending` branch returns before wiring `loadingPadTick` / `progressReceiver` re-issue, so `transitionFromLoading()` is never invoked and the interim Sessions page is never reached (spec §Constraints: "`commandPending` does not intersect the deferral"). AC6 therefore holds unchanged and is independent of the cold-route deferral.

**Context**:
> Warm / CLI / direct-path untouched (spec §Constraints): the synchronous route keeps making the landing decision at `transitionFromLoading()` time against its already-post-restore Init snapshot. No new enumeration, no behaviour change, no new ordering dependency. This is the zero-new-risk contract — the warm-path startup sequence has prior-incident history (slow-open / zombie-session) and must not be perturbed.
> `refetchSessionsAfterRestore()` (model.go ~line 1818) returns `nil` when `m.progressReceiver == nil`, else `m.fetchSessionsCmd()`. The warm route never refetches.
> AC4/AC5 (spec §Acceptance Criteria): warm path N>0 → Sessions (byte-identical to today); warm path zero → Projects (byte-identical to today).

**Spec Reference**: `.workflows/cold-boot-restore-lands-on-projects/specification/cold-boot-restore-lands-on-projects/specification.md` — AC4, AC5, AC6, §Testing Requirements case 4, §Constraints (Warm / CLI / direct-path untouched, Canonical cold-route predicate, `commandPending` branch preserved, `commandPending` does not intersect the deferral).

## cold-boot-restore-lands-on-projects-1-4 | approved

### Task 1-4: Interim-page and late-ProjectsLoadedMsg ordering invariants

**Problem**: The deferral introduces a one-Update-cycle interim window between loading-page dismissal and the post-restore refetch's `SessionsMsg`. Two invariants must hold deterministically: (a) the interim page is a valid picker page — interim `PageSessions`, never `PageLoading`, blank, or undefined — even though it briefly renders the not-yet-repaired empty session list (AC7); and (b) the landing decision is independent of `ProjectsLoadedMsg` arrival order — a `ProjectsLoadedMsg` that arrives in the interim window (after the transition, before the refetch `SessionsMsg`) must NOT latch Projects against the stale interim list, because `sessionsLoaded` is still false and `evaluateDefaultPage()` early-returns.

**Solution**: Add two cold-route ordering tests to `internal/tui/coldboot_session_refetch_test.go`: one asserting the interim page is `PageSessions` immediately after the transition and BEFORE the refetch `SessionsMsg`, then asserting the final landing per AC1; and one exercising the adversarial late-`ProjectsLoadedMsg` interleave (transition with projects NOT yet loaded, deliver `ProjectsLoadedMsg` in the interim window, then deliver the refetch `SessionsMsg` with N>0) asserting the final page is Sessions. No production change beyond Task 1-1.

**Outcome**: Two passing tests make the transient interim invariant and the order-independence of the landing decision deterministic assertions rather than implementer discretion, proving the `sessionsLoaded`-stays-false correction survives both orderings.

**Do**:
- Test case 5 (AC7, interim page), e.g. `TestColdBoot_InterimPage_IsValidSessions`: build the cold model (`WithProgressReceiver(...)`, `WithServerStarted(true)`, project store). Drive `WindowSizeMsg`, stale empty `SessionsMsg` on `PageLoading`, `ProjectsLoadedMsg` on `PageLoading`, `LoadingMinElapsedMsg`, then `BootstrapCompleteMsg`. CAPTURE the model returned by the `BootstrapCompleteMsg` Update **before draining the batched refetch cmd** and assert `interim.ActivePage() == PageSessions` (a valid picker page — not `PageLoading`, not undefined). Optionally assert `visibleSessionNames(interim)` is empty (the accepted briefly-empty interim render — do NOT assert it is special-cased away). THEN drain the refetch cmd and assert the final landing per AC1 (`ActivePage() == PageSessions`, list shows all N restored names).
- Test case 6 (ordering contract, late `ProjectsLoadedMsg`), e.g. `TestColdBoot_LateProjectsLoadedMsg_StillLandsOnSessions`: build the cold model. Drive `WindowSizeMsg`, stale empty `SessionsMsg` on `PageLoading`, `LoadingMinElapsedMsg`, `BootstrapCompleteMsg` — WITHOUT delivering `ProjectsLoadedMsg` first (so `projectsLoaded` is false at transition). Assert the interim page is `PageSessions`. Then deliver `ProjectsLoadedMsg` (N≥1 project) in the interim window and assert it did NOT latch Projects (page stays `PageSessions` — its `evaluateDefaultPage()` early-returns on `!sessionsLoaded`). Then deliver the refetch's `SessionsMsg` with N>0 sessions and assert the final page is `PageSessions` with all N names visible. (Drive the refetch result either by draining the `BootstrapCompleteMsg` batch — which carries the refetch cmd — at the appropriate point, or by feeding a `SessionsMsg{Sessions: restored}` directly after the late `ProjectsLoadedMsg`; choose the ordering that places `ProjectsLoadedMsg` strictly between the transition and the decision-bearing `SessionsMsg`. Delivering the `SessionsMsg` directly is acceptable and clearer for asserting the strict interleave, since `drainBatchToModel` would otherwise resolve the refetch before you can inject the late `ProjectsLoadedMsg`.)
- For test case 6, note that under this ordering the decision is taken by the **post-restore `SessionsMsg` handler** (it lands second, after the late `ProjectsLoadedMsg` already set `projectsLoaded = true`), so `sessionsLoaded && projectsLoaded` are both true when the `SessionsMsg` arm calls `evaluateDefaultPage()` off `PageLoading`. Add a comment in the test capturing which handler is the decision point so the intent survives.
- Both tests MUST assert the **active page** at each checkpoint, and both MUST deliver `ProjectsLoadedMsg` (case 5 during loading; case 6 in the interim window) so the latch machinery is genuinely exercised.

**Acceptance Criteria**:
- [ ] AC7: immediately after the cold-route transition and BEFORE the refetch `SessionsMsg`, `ActivePage() == PageSessions` (a valid picker page — never `PageLoading` or undefined); the final landing after the refetch is `PageSessions` per AC1.
- [ ] Ordering contract: a `ProjectsLoadedMsg` delivered in the interim window (after transition, before the decision-bearing `SessionsMsg`) does NOT latch Projects against the stale list — the final page is `PageSessions` with all N restored sessions.
- [ ] The interim render is NOT special-cased: the briefly-empty interim Sessions list is asserted as the accepted state (the test does not require empty-state suppression).
- [ ] Both tests deliver `ProjectsLoadedMsg` and assert the active page at each checkpoint; both pass under `go test ./internal/tui/...`.

**Tests**:
- `"it holds a valid interim PageSessions between loading dismissal and the refetch SessionsMsg (never PageLoading or undefined)"`
- `"it may briefly render the empty session list in the interim window without special-casing the empty-state"`
- `"it does not latch Projects when ProjectsLoadedMsg arrives in the interim window (late-projects ordering still lands on Sessions)"`

**Edge Cases**:
- Interim render may briefly show the Sessions empty-state signpost (the stale empty list before the refetch populates it) — this is an accepted, valid (non-blank) page and must NOT be special-cased to suppress the empty-state (spec §Constraints: Interim render content). The window is a single Update cycle because the refetch is dispatched in the same handler return.
- Late `ProjectsLoadedMsg` must not latch Projects: the `ProjectsLoadedMsg` handler calls `evaluateDefaultPage()` **unconditionally** (no page guard), so premature latching is prevented solely by the `!sessionsLoaded` early-return inside `evaluateDefaultPage()`. Leaving `sessionsLoaded` false until the refetch's `SessionsMsg` is the load-bearing correction (spec §Constraints: Decision always resolves on the cold route).
- The landing decision is taken by whichever of the post-restore `SessionsMsg` and `ProjectsLoadedMsg` lands second; under the adversarial late-`ProjectsLoadedMsg` ordering the `SessionsMsg` handler is the decision point (it lands after projects). The latch stays unset until then, so no path strands the picker on the interim page.
- Interim-window user input is not special-cased and is out of scope; do not test mid-interim toggles.

**Context**:
> Valid interim page (spec §Constraints): between loading-page dismissal and the post-restore refetch landing, the cold route must hold a valid `activePage` (interim Sessions). It must never sit on an undefined page, re-enter the loading page, or render a blank frame.
> Ordering contract (spec §Fix Approach): the landing decision is independent of `ProjectsLoadedMsg` arrival order — whether projects load before or after the refetch, the first call that finds `sessionsLoaded && projectsLoaded` both true runs against the already-repaired post-restore list. The deferral and the refetch dispatch are a single unit (same handler return) and must stay coupled so the interim page is always followed by exactly one decision-bearing `SessionsMsg`.
> Decision always resolves (spec §Constraints): the `SessionsMsg` arm is gated by an `activePage == PageLoading` early-return; the `ProjectsLoadedMsg` arm calls `evaluateDefaultPage()` unconditionally — premature latching in the interim is prevented solely by the `!sessionsLoaded` early-return. Under the late-`ProjectsLoadedMsg` ordering (test case 6) the `SessionsMsg` handler is the decision point because it lands second.
> Interim render content (spec §Constraints): during the one-frame interim window the cold route renders `PageSessions` against the not-yet-repaired (empty) session list, which may briefly show the Sessions empty-state signpost before the refetch populates it. The interim render must NOT be special-cased to suppress the empty-state.

**Spec Reference**: `.workflows/cold-boot-restore-lands-on-projects/specification/cold-boot-restore-lands-on-projects/specification.md` — AC7, §Testing Requirements cases 5 & 6, §Fix Approach (Ordering contract), §Constraints (Valid interim page, Decision always resolves on the cold route, Interim render content).
