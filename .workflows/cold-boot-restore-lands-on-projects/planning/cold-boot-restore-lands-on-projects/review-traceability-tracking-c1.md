---
status: complete
created: 2026-06-26
cycle: 1
phase: Traceability Review
topic: Cold-Boot Restore Lands on Projects
---

# Review Tracking: Cold-Boot Restore Lands on Projects - Traceability

## Findings

### 1. AC6 (commandPending → Projects) is committed in the phase acceptance but no task verifies it

**Type**: Incomplete coverage
**Spec Reference**: §Acceptance Criteria AC6; §Constraints & Invariants ("`commandPending` branch preserved"; "`commandPending` does not intersect the deferral")
**Plan Reference**: Phase 1 Acceptance (line 18 of planning.md commits "AC6 — `commandPending` launch lands on **Projects** as today; verified that `commandPending` never reaches the modified loading→picker transition"), but Tasks 1-1 through 1-4 contain no Do step, Acceptance Criterion, or Test for AC6 / `commandPending`.
**Change Type**: add-to-task

**Details**:
AC6 is one of the seven required acceptance rows in the spec ("the fix is correct only if every row holds"), and the spec carries two dedicated constraints about it: the `commandPending → Projects` arm of `evaluateDefaultPage()` must remain correct, and a `commandPending` launch must never reach the modified loading→picker transition (no interim Sessions flash). The phase header explicitly promises this verification, yet no task delivers it — the spec's six required test cases (1-6) do not include a `commandPending` case, so AC6's verification fell through the gap between the phase acceptance and the task layer.

The risk this guards against is real and specific to *this* fix: the deferral changes `transitionFromLoading()`, and the spec's safety argument for AC6 is that `Init`'s `commandPending` branch returns before wiring the loading-dismissal machinery (no `loadingPadTick`, no `progressReceiver` re-issue) so `transitionFromLoading()` is never invoked for it. That non-intersection is a behaviour the fix relies on but does not test. A regression here (e.g. a future change wiring the loading page for `commandPending`) would silently break AC6.

The model already exposes the constructor seam needed: `WithCommand([]string{...})` sets `commandPending = true` and `activePage = PageProjects` (model.go:632-639), and `evaluateDefaultPage()` lands on Projects unconditionally when `commandPending` (model.go:1619-1633). Task 1-3 ("Warm-route parity guard") is the natural home — it already owns the "preserve today's behaviour, no new risk" parity tests built without a `progressReceiver`. Adding the AC6 verification there keeps the preservation guards together.

**Current** (Task 1-3, `cold-boot-restore-lands-on-projects-1-3`, in `phase-1-tasks.md` and tick `tick-7f37e3`):

The task's **Do**, **Acceptance Criteria**, and **Tests** sections currently cover only AC4 (N>0 warm → Sessions), AC5 (zero warm → Projects), `refetchSessionsAfterRestore()` nil/non-nil symmetry, and the no-refetch assertion. There is no `commandPending` / AC6 coverage.

**Proposed** (add to Task 1-3 — `add-to-task`):

Append to **Do**:
- AC6 (commandPending preservation), e.g. `TestCommandPending_LandsOnProjects_NoInterimFlash`: build with `New(lister, WithServerStarted(true), WithProgressReceiver(func() tea.Msg { return nil })).WithCommand([]string{"echo", "hi"})` — `WithCommand` sets `commandPending = true` and `activePage = PageProjects` (model.go:632-639). Drive `WindowSizeMsg`, then `ProjectsLoadedMsg` (≥1 project) so the `commandPending` arm of `evaluateDefaultPage()` (model.go:1619-1622) can resolve. Assert `final.ActivePage() == PageProjects`. The test verifies that a `commandPending` launch lands on Projects regardless of session count and never sits on `PageSessions` (the interim page the deferral introduces). Add a comment recording the spec invariant: `Init`'s `commandPending` branch (model.go:1882-1883) returns before wiring `loadingPadTick` / `progressReceiver`, so `transitionFromLoading()` is never invoked for a `commandPending` launch and no interim Sessions flash occurs — even though `WithProgressReceiver` is wired, the `commandPending` short-circuit takes precedence. No `t.Parallel()`.

Append to **Acceptance Criteria**:
- [ ] AC6: a `commandPending` launch lands on `PageProjects` regardless of session count and is never observed on the interim `PageSessions`; the `commandPending → Projects` arm of `evaluateDefaultPage()` is unchanged.
- [ ] Verified that the `commandPending` path never reaches `transitionFromLoading()` (the modified loading→picker transition): `Init`'s `commandPending` branch returns before wiring the loading-dismissal machinery, so no interim Sessions flash occurs on a `commandPending` launch.

Append to **Tests**:
- `"it lands a commandPending launch on Projects regardless of session count"`
- `"it never flashes the interim Sessions page on a commandPending launch (transitionFromLoading is not invoked)"`

Append to **Edge Cases**:
- `commandPending` does not intersect the deferral: `Init`'s `commandPending` branch returns before wiring `loadingPadTick` / `progressReceiver` re-issue, so `transitionFromLoading()` is never invoked and the interim Sessions page is never reached (spec §Constraints: "`commandPending` does not intersect the deferral"). AC6 therefore holds unchanged and is independent of the cold-route deferral.

Append to **Spec Reference** (so it reads):
`.workflows/cold-boot-restore-lands-on-projects/specification/cold-boot-restore-lands-on-projects/specification.md` — AC4, AC5, AC6, §Testing Requirements case 4, §Constraints (Warm / CLI / direct-path untouched, Canonical cold-route predicate, `commandPending` branch preserved, `commandPending` does not intersect the deferral).

**Resolution**: Fixed
**Notes**: Applied to Task 1-3 (tick-7f37e3) — appended AC6 commandPending preservation test, acceptance criteria, tests, edge case, and spec reference to both phase-1-tasks.md and the tick description.

---

### 2. "Failing refetch degrades to today's quit" is committed in the phase acceptance but no task verifies it

**Type**: Incomplete coverage
**Spec Reference**: §Constraints & Invariants ("Failing refetch degrades to today's quit"); §Constraints ("Decision always resolves on the cold route" — "A `SessionsMsg` carrying an error continues to quit, exactly as today")
**Plan Reference**: Phase 1 Acceptance (line 22 of planning.md commits "a failing refetch `SessionsMsg` still quits without stranding the interim page"), but Tasks 1-1 through 1-4 contain no Do step, Acceptance Criterion, or Test for an error-carrying post-restore refetch `SessionsMsg`.
**Change Type**: add-to-task

**Details**:
The spec carries an explicit invariant: if the post-restore refetch's `SessionsMsg` carries an error, the handler runs `tea.Quit` *before* the deferred decision — exactly as a failing `Init` fetch would — and the cold route must **not** strand the picker on the interim page. The phase acceptance (line 22) commits to this behaviour, but no task verifies it. The spec's six required test cases (1-6) do not include an error-refetch case, so this invariant fell through the gap between the phase acceptance and the task layer.

This is load-bearing for *this* fix specifically: the deferral leaves the cold route sitting on the interim `PageSessions` until the refetch `SessionsMsg` resolves the decision. The failure mode the fix must avoid is stranding the user on that interim page when the refetch errors. Without a test, a refactor of the `SessionsMsg` handler's error arm relative to the deferral could silently leave the picker stuck on the empty interim Sessions page instead of quitting.

Task 1-4 ("Interim-page and late-`ProjectsLoadedMsg` ordering invariants") is the natural home — it already owns the interim-window invariants (the interim page must be valid and never stranded). Adding the failing-refetch-quit case there keeps the "what happens in the interim window" guarantees together.

**Current** (Task 1-4, `cold-boot-restore-lands-on-projects-1-4`, in `phase-1-tasks.md` and tick `tick-6fee61`):

The task's **Do**, **Acceptance Criteria**, and **Tests** sections currently cover test case 5 (AC7 interim page) and test case 6 (late-`ProjectsLoadedMsg` ordering). There is no coverage of an error-carrying refetch `SessionsMsg`.

**Proposed** (add to Task 1-4 — `add-to-task`):

Append to **Do**:
- Failing-refetch quit guard (spec §Constraints: "Failing refetch degrades to today's quit"), e.g. `TestColdBoot_RefetchError_QuitsWithoutStrandingInterim`: build the cold model (`WithProgressReceiver(...)`, `WithServerStarted(true)`, project store). Drive `WindowSizeMsg`, stale empty `SessionsMsg` on `PageLoading`, `ProjectsLoadedMsg` on `PageLoading`, `LoadingMinElapsedMsg`, then `BootstrapCompleteMsg` (assert the interim page is `PageSessions`). Then deliver the post-restore refetch result as an **error-carrying** `SessionsMsg{Err: <non-nil error>}` directly through `Update` and assert the returned cmd is (or batches) `tea.Quit` — mirror the existing `SessionsMsg`-error assertion style already used elsewhere in the tui test surface for a failing fetch. Assert the model did NOT strand on the interim page silently waiting for a further decision (the error path quits; it does not run the deferred `evaluateDefaultPage()` decision before quitting). Add a comment recording the spec invariant: a `SessionsMsg` carrying an error continues to quit exactly as today, and a single interim frame rendering before quit is acceptable. No `t.Parallel()`.

Append to **Acceptance Criteria**:
- [ ] Failing refetch: a post-restore refetch `SessionsMsg` carrying an error runs `tea.Quit` before the deferred decision and does NOT strand the picker on the interim `PageSessions`; the cold route exits with the same error UX as the warm route (a single interim frame may render before quit, which is acceptable).

Append to **Tests**:
- `"it quits on a failing post-restore refetch SessionsMsg without stranding the interim Sessions page"`

Append to **Edge Cases**:
- Failing refetch degrades to today's quit: if the refetch's `SessionsMsg` carries an error, the handler runs `tea.Quit` before the deferred `evaluateDefaultPage()` decision — exactly as a failing `Init` fetch would (spec §Constraints: "Failing refetch degrades to today's quit"; "Decision always resolves on the cold route" — "A `SessionsMsg` carrying an error continues to quit, exactly as today"). A single interim frame may render before quit and is acceptable; the cold route must not strand the picker on the interim page.

Append to **Spec Reference** (so it reads):
`.workflows/cold-boot-restore-lands-on-projects/specification/cold-boot-restore-lands-on-projects/specification.md` — AC7, §Testing Requirements cases 5 & 6, §Fix Approach (Ordering contract), §Constraints (Valid interim page, Decision always resolves on the cold route, Interim render content, Failing refetch degrades to today's quit).

**Resolution**: Fixed
**Notes**: Applied to Task 1-4 (tick-6fee61) — appended failing-refetch quit guard test, acceptance criterion, test, edge case, and spec reference to both phase-1-tasks.md and the tick description. The exact error-path assertion shape (`tea.Quit` returned directly vs. batched, and whether the handler exposes a quit via a sentinel msg) should match the existing failing-`SessionsMsg`/`Init`-fetch error handling already present in `internal/tui` — the implementer should mirror that established pattern rather than introduce a new one.

---
