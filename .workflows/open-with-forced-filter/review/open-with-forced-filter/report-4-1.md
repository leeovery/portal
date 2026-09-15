TASK: Hold the loading page until the search decision resolves (tick-00f27b, open-with-forced-filter-4-1)

ACCEPTANCE CRITERIA:
- A decision returning a session name leaves the model on `PageLoading`, sets `Selected()` to that name, sets `SearchAttached()`, and returns a quit command — no picker frame is ever composed
- On that attach `BufferedWarnings()` is still populated and no flash/notice band was set — `surfaceBufferedWarnings` did not run
- A decision returning `("", nil)` transitions to the picker exactly as today, warnings surfacing through the existing path
- A decision returning an error sets `SearchError()`, leaves `FatalError()` nil and the fatal state unset, stays on `PageLoading` and returns a quit command
- The decision fires only when both gates have closed: complete-then-min and min-then-complete each invoke it exactly once, and neither gate alone invokes it
- No `BootstrapProgressMsg` — including a restore-counter event — invokes the decision
- A `BootstrapFatalMsg` followed by `LoadingMinElapsedMsg` never invokes the decision and keeps the error frame
- The closure is invoked at most once across any message sequence, including a duplicate `BootstrapCompleteMsg`
- `Ctrl-C` on the loading page before the gates close quits with `Selected()` empty and the decision uninvoked
- With no decision closure supplied, every existing loading-page transition, warning surfacing and refetch behaviour is byte-identical
- `go test ./...` passes

STATUS: complete

SPEC CONTEXT:
§3.4 requires K to be evaluated only once the tmux server can answer for it — on a cold server, after the *whole* concurrent bootstrap, not merely restore — with the loading page standing until then and its minimum display span untouched. §3.7 separates a failed session-list read from a zero match: it is reported in tmux's own terms, exits non-zero, and is explicitly "not a bootstrap fatal and takes no in-TUI error frame". §7.4 accepts the flicker of a loading page that is replaced by an attach. §7.5 requires that on K = 1 the TUI tears down before the connector runs, so the accumulated soft warnings must still be owed to the terminal at teardown rather than surfaced in a notice band. The task implements exactly the seam those four sections need inside `internal/tui`.

IMPLEMENTATION:
- Status: Implemented (with a deliberate, recorded shape change from the task's written Do)
- Location:
  - `internal/tui/search_decision.go:11` `WithSearchDecision` (nil-tolerant option)
  - `internal/tui/search_decision.go:19` `SearchAttached()`, `:25` `SearchError()`
  - `internal/tui/search_decision.go:33` `searchDecisionMsg`, `:40` `searchDecisionCmd`, `:51` `applySearchDecision`
  - `internal/tui/model.go:192-200` `searchDecide` / `searchDecideInFlight` / `searchAttached` / `searchErr`
  - `internal/tui/model.go:1509` `dismissLoadingGate` (the gate), `:1522` `completeLoadingDismissal` (today's sequence)
  - `internal/tui/model.go:1634` `LoadingMinElapsedMsg` arm, `:1646` `searchDecisionMsg` arm, `:1658` `BootstrapCompleteMsg` arm
  - `internal/tui/build.go:71-78` `SearchForm.Decide`, `:133` the `Build` wiring
- Notes:
  - **Drift, deliberate and sound.** The task's Do said the closure "must call the closure synchronously rather than wrapping it in a `tea.Cmd`". The delivered code dispatches it as a `tea.Cmd` (`search_decision.go:40`) and acts on `searchDecisionMsg` (`model.go:1646`). That reversal is plan task 12-3 (tick-23141a, "Take the search decision off the render goroutine"), whose stated reason is that the closure performs blocking tmux round-trips and would freeze the loading page and its Ctrl-C. The single-shot property moved from a mid-call field clear to the gate's own `searchDecideInFlight` state, and the ordering the task required (decision before `surfaceBufferedWarnings`) is preserved as gate ordering: `dismissLoadingGate` returns with the page still on `PageLoading` while the decision is out, and only `applySearchDecision`'s `("", nil)` branch reaches `completeLoadingDismissal`. Every acceptance criterion of 4-1 still holds in substance. This is a gain, not a loss.
  - The two call sites the task named were folded into one `dismissLoadingGate` helper by plan task 5-7 (tick-cfc44d); the required "decision must precede `surfaceBufferedWarnings`" comment lives on that single helper (`model.go:1503-1508`) rather than at one of two arms, which is the correct home after the fold.
  - `fatalActive` / `fatalStep` / `fatalMessage` / `fatalErr` are set nowhere on this path; the `searchDecisionMsg` arm early-returns under `fatalActive` (`model.go:1647-1650`), so a decision answering after a fatal cannot dismiss the error frame. Matches §3.7's "takes no in-TUI error frame".
  - The seam is armed only on the concurrent route: `cmd/open_search.go:228-232` sets `Decide` only when `deferredBootstrapFromContext(cmd) != nil`; the warm route decides before the TUI runs and hands `Decide` nil, so the gate stays inert there.
  - `transitionFromLoading` (`model.go:1494`) is untouched and still leaves `sessionsLoaded` false on the concurrent route, so the picker branch lands its filter through the post-restore refetch as the task's edge case required.
  - `BootstrapProgressMsg` (`model.go:1653`) is untouched — it re-issues the receiver and returns, reaching no gate.
  - `SearchAttached()` has no production consumer today (plan task 12-5 replaced the `searchAttached`-driven teardown branch with the exit-agnostic `WarningsOwedAtTeardown`, `model.go:486`). It remains the observation seam five `cmd` and `internal/tui` suites discriminate the decision outcome through, and this task's criteria require it, so it is not orphaned code.

TESTS:
- Status: Adequate
- Coverage: `internal/tui/search_decision_test.go` carries one test per acceptance criterion and nothing surplus:
  - attach without painting (`:63`) — page, `Selected()`, `SearchAttached()`, quit
  - warnings left unsurfaced on attach (`:82`) — `BufferedWarnings()` non-empty *and* `activeNoticeBand()` empty, which is what proves `surfaceBufferedWarnings` did not run
  - picker on `("", nil)` (`:96`) — `PageSessions`, `bandWarning` surfaced
  - read failure without fatal (`:114`) — `SearchError()` by `errors.Is`, `FatalError()` nil, `PageLoading`, quit
  - both gate orders (`:132`, `:150`) — each asserts zero dispatch on the first gate alone and exactly one call after the second
  - progress events including a restore counter (`:168`)
  - fatal then both gates (`:186`)
  - exactly once across a duplicated gate sequence (`:206`)
  - Ctrl-C before the gates (`:232`) and while the decision is in flight (`:308`)
  - nil closure leaves the transition unchanged (`:262`), driven through the pre-existing `coldTUIModel` / `transitionWithWarnings` helpers so the comparison is against the untouched path
  - in-flight hold (`:283`) — asserts `d.calls == 0` after the gate returns, which is the assertion that would fail if the closure were ever called back on the update goroutine
  - decision message after a fatal (`:326`)
- Notes: The tests observe behaviour through exported accessors and `activeNoticeBand`, not internal fields, and each would fail if its behaviour broke — e.g. removing the `fatalActive` early return at `model.go:1647` fails `TestSearchDecision_MessageAfterFatalLeavesErrorFrameStanding`, and moving the closure back inline fails `TestSearchDecision_HoldsLoadingPageWhileInFlight`. `transitionThroughDecision` (`:41`) examines both gates' returned commands rather than assuming which one dispatches, so the helper does not bake in an ordering the production code is free to change. No redundant assertions, no over-mocking: the only fake is a three-field `decisionCounter`.

CODE QUALITY:
- Project conventions: Followed. The value-in/value-out method shape (`dismissLoadingGate`, `completeLoadingDismissal`, `applySearchDecision` all `func (m Model) ... (Model, tea.Cmd)`) matches the shape the package adopted in plan task 11-2. The tmux read stays behind a caller-supplied closure, so `internal/tui` takes no new dependency and remains env-free. The new file is small and single-purpose, consistent with the package's one-concern-per-file layout.
- SOLID principles: Good. The classification rule lives in `cmd/open_search.go`; `internal/tui` holds only the gate and the seam. The option is nil-tolerant, so the model has one code path whether or not a search armed it.
- Complexity: Low. `dismissLoadingGate` is three branches; `applySearchDecision` is three.
- Modern idioms: Yes. The message/command pair is the idiomatic Bubble Tea shape and matches every other tmux read on this path (`fetchSessionsCmd`, `loadProjects`, `refetchSessionsAfterRestore`).
- Readability: Good.
- Issues: None. Comment accuracy was checked line by line and every claim holds: `search_decision.go:8-10` ("nil fn leaves the seam inert") matches the `!= nil` gate at `model.go:1515`; `:42-45` ("running it inline would freeze the loading page — and its Ctrl-C") matches the inert-but-Ctrl-C-live loading arm at `model.go:1812-1815`; `model.go:1503-1508` ("a third condition on the gate rather than a step within it … keeps the decision ahead of `surfaceBufferedWarnings`") matches the dispatch-and-hold at `:1515-1517` and the fact that `surfaceBufferedWarnings` is reached only from `completeLoadingDismissal` at `:1524`; `model.go:195-197` ("the gate's own state, not the closure field, is the single-shot guard") matches `searchDecideInFlight` being what `:1510` tests. No comment references a task id, phase or spec section.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes" — settleable only by executing the unit lane; the verifier reads and does not run. Reading confirms the new tests are consistent with the code under test and that the nil-closure path is unchanged, but a green suite is an observation, not a reading.
