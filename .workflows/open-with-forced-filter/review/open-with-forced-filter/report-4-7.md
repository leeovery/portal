TASK: Fold the two loading-gate dismissal blocks into one helper (tick-cfc44d / open-with-forced-filter-4-7)

ACCEPTANCE CRITERIA:
- `resolveSearchDecision`, `transitionFromLoading` and the three-command batch each have exactly one call site in `internal/tui`, all inside `dismissLoadingGate`.
- Each gate is one call to the helper behind the guard it already owns; neither guard moves into the helper.
- The decision-before-`surfaceBufferedWarnings` rule is stated once, on the helper.
- Behaviour is unchanged: no test file is edited, no test is renamed, and no test semantics change.
- `go test ./...` is green.

STATUS: complete

SPEC CONTEXT: §7 (Cold-Boot Classification) makes a sigil invocation a picker invocation — it takes the concurrent bootstrap and the honest loading page, and its soft bootstrap warnings take the in-TUI route rather than stderr (§7.1). §7.5 is the ordering rule this task's comment encodes: when the search resolves to a direct attach (K = 1), the TUI tears down before the connector runs, so the notice band never surfaces and the accumulated warnings are written to the terminal after teardown instead. That is only true if the search decision is settled *before* `surfaceBufferedWarnings` empties the buffer — hence "the decision must precede surfaceBufferedWarnings". The two gates are the `LoadingMinDuration` minimum-display span and the concurrent orchestrator's terminal `BootstrapCompleteMsg`; which lands second is a race, so the sequence must be identical on both arms.

IMPLEMENTATION:
- Status: Implemented (delivered in `28af1b9d7`, subsequently reshaped by later approved tasks; the current tree satisfies the criteria in substance)
- Location:
  - `internal/tui/model.go:1503-1520` — `dismissLoadingGate`, carrying the single copy of the ordering comment.
  - `internal/tui/model.go:1522-1526` — `completeLoadingDismissal`, holding `transitionFromLoading()` plus the three-command `tea.Batch`.
  - `internal/tui/model.go:1641-1644` — `LoadingMinElapsedMsg` arm: guard `m.bootstrapComplete && m.activePage == PageLoading`, one call.
  - `internal/tui/model.go:1693-1696` — `BootstrapCompleteMsg` arm: guard `m.minElapsed && m.activePage == PageLoading`, one call.
  - `internal/tui/search_decision.go:51-63` — `applySearchDecision`, the second caller of `completeLoadingDismissal`.
- Notes:
  - Criterion-by-criterion against the current tree:
    - `transitionFromLoading` has exactly one production call site (`model.go:1523`); the only other mention is the explanatory comment at `model.go:1625` in the `SessionsMsg` arm. The three-command batch expression appears exactly once (`model.go:1524`). `resolveSearchDecision` no longer exists — phase 12's approved `tick-23141a` ("Take the search decision off the render goroutine") replaced the synchronous resolver with the `searchDecide`/`searchDecisionCmd`/`applySearchDecision` triple, so the criterion's named symbol is gone rather than duplicated.
    - Each gate is exactly one call behind the guard it already owned; neither guard moved into the helper. The helper does carry a third condition (`searchDecideInFlight`, `model.go:1510`), but that is phase 12's async single-shot guard, not one of the two gate guards.
    - The ordering rule is stated once, at `model.go:1503-1508`, and no copy survives in either arm.
    - The delivering commit `28af1b9d7` touched `internal/tui/model.go` alone (16 insertions, 13 deletions) — no test file edited, renamed, or re-semanticised, and the folded body is byte-for-byte the sequence the two arms ran before it.
  - Two divergences from the task's literal wording, both later-task provenance and both judged sound:
    - Shape: the helper is `func (m Model) dismissLoadingGate() (Model, tea.Cmd)` rather than the prescribed `func (m *Model) dismissLoadingGate() tea.Cmd`. Phase 11's `tick-390004` / `tick-61e334` moved the package onto the value-in/value-out shape its handlers already use; the arms consequently read `m, cmd = m.dismissLoadingGate()`, which is a tuple assignment and so carries none of the evaluation-order hazard the task's `cmd := …; return m, cmd` form was written to avoid.
    - Split: the transition-plus-batch tail now lives in `completeLoadingDismissal` with two callers — `dismissLoadingGate` (`model.go:1519`) and `applySearchDecision` (`search_decision.go:63`). That second caller is the same gate resuming once the asynchronous decision answers, not a second copy: the sequence and its ordering rule still exist exactly once, which is the outcome the task was written for. The ordering property survives the async move intact — the gate returns without transitioning while the decision is out, so nothing can reach `surfaceBufferedWarnings` before the decision has answered.

TESTS:
- Status: Adequate (existing tests only, as the task prescribes for a pure refactor)
- Coverage: All seven named tests exist and are unedited by the delivering commit:
  - `internal/tui/search_decision_test.go:137` `TestSearchDecision_HeldUntilMinimumSpanElapses` and `:155` `TestSearchDecision_HeldUntilBootstrapCompletes` drive each gate as the second to land, asserting no dispatch from the first and exactly one decision call after both.
  - `internal/tui/search_decision_test.go:214` `TestSearchDecision_IssuedExactlyOnce` drives a duplicated four-message gate sequence and pins dispatches == 1 and calls == 1, so the single-shot guard survives the fold.
  - `internal/tui/sessions_postload_warning_test.go:41` `TestColdTUIWarnings_SurfaceAsPostLoadNoticeBand` (complete-last arm) and `:64` `TestColdTUIWarnings_SurfaceWhenMinElapsedArmTransitions` (min-elapsed-last arm) assert the warning band on each arm — the cross-arm divergence this fold exists to prevent.
  - `internal/tui/coldboot_session_refetch_test.go:143` `TestColdBoot_PostCompleteRefetch_ReflectsRestoredSessions` and `:171` `TestColdBoot_PostCompleteRefetch_CompleteBeforeMinElapsed` assert the post-restore refetch on each arm, the latter explicitly checking no command is issued while only one gate is closed.
  - `internal/tui/search_decision_test.go:81` `TestSearchDecision_LeavesBufferedWarningsUnsurfacedOnAttach` directly observes the ordering rule the helper comment states: on an attach decision the buffer still holds its warning and no notice band owns the slot.
- Notes: No new tests were added and none altered, which is correct for a behaviour-preserving fold. Nothing is over-tested here — each named test covers a distinct arm or a distinct property, and the duplicated-gate test is the only one that overlaps another's subject (dispatch count), for a different reason.

CODE QUALITY:
- Project conventions: Followed. The value-in/value-out receiver shape matches the package's handler convention; the pointer-receiver `transitionFromLoading` is called on an addressable local and the mutated copy is returned, so no mutation is lost. The arms assign the named return `cmd`, which the `WindowSizeMsg` deferred composition at `model.go:1487-1489` (`defer func() { cmd = tea.Batch(closed, cmd) }()`) relies on — a `cmd :=` shadow would still have been correct, but the current form is the clearer of the two.
- SOLID principles: Good. `dismissLoadingGate` decides whether the gate may dismiss; `completeLoadingDismissal` performs the dismissal. That split is what lets the asynchronous decision re-enter the tail without restating it.
- Complexity: Low. Two guards and a tail call; the arms lost a branch each.
- Modern idioms: Yes.
- Readability: Good. The helper comment explains the constraint rather than restating the code, and names the mechanism (`surfaceBufferedWarnings` empties the buffer; an attach leaves the warnings owed to the caller).
- Issues: None. The comments in the changed region hold against the code: "Both loading gates dismiss through here" is true of both arms; "the page stays on PageLoading until the dispatched command answers" is true — `dismissLoadingGate` returns without touching `activePage` on the dispatch path; and no comment references a task id, phase, or spec section.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` is green." — settled only by running the unit lane; reading cannot establish it. `go test ./...` from the project root, with `internal/tui` (the only package this task touched) green in particular.
