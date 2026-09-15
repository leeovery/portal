TASK: open-with-forced-filter-12-3 (tick-23141a) — Take the search decision off the render goroutine

ACCEPTANCE CRITERIA:
1. No `cmd`-supplied closure is invoked from inside `Update`: the decision runs only from a `tea.Cmd`.
2. While the decision is in flight the page stays `PageLoading` and Ctrl-C quits with nothing selected.
3. The closure is invoked exactly once across a duplicated `LoadingMinElapsedMsg` / `BootstrapCompleteMsg` sequence, and not at all before both gates are satisfied or after a `BootstrapFatalMsg`.
4. An attach decision records `Selected()` and `SearchAttached()` and quits with no picker frame composed; a read failure records `SearchError()`, quits, and sets no fatal.
5. A picker decision dismisses through today's sequence with the decision still ahead of `surfaceBufferedWarnings`: `WarningsOwedAtTeardown` returns the buffered set on the attach and read-failure exits and nothing on the picker exit.
6. A decision message arriving after a fatal leaves the error frame standing.
7. A model with no decision closure dismisses exactly as it does today, and `go test ./...` is green.

STATUS: complete

SPEC CONTEXT:
§3.4 ("When the count is taken") requires K to be evaluated against the live session set only once the whole ten-step bootstrap has run — "the loading page stands until the count can be taken, and is then replaced by the attach when K turns out to be 1, or by the picker at any other count", with the minimum display span untouched. §3.7 requires a failed session-list read to be reported in tmux's own terms, not as a bootstrap fatal and with no in-TUI error frame. §7.1/§7.5 classify a sigil invocation as a picker invocation taking the concurrent bootstrap and the honest loading page, with the soft warnings still owed to the terminal on the K=1 attach exit (written after teardown, before the connector). §7.6 states the concurrent path's own machinery is unchanged for the sigil.

This task does not change any of that observable behaviour — it moves the classification from a synchronous call inside `Update` to a dispatched `tea.Cmd`, which makes §3.4's "the loading page stands until the count can be taken" literally true of the event loop as well as of the page.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/search_decision.go:29-34` — `searchDecisionMsg{name, err}`, the answered classification.
  - `internal/tui/search_decision.go:36-46` — `searchDecisionCmd`, a value-receiver `tea.Cmd` factory that captures the closure and returns the message; it does not invoke the closure.
  - `internal/tui/search_decision.go:48-64` — `applySearchDecision`: clears `searchDecide`/`searchDecideInFlight`, then read failure → `searchErr` + `tea.Quit`, named session → `selected` + `searchAttached` + `tea.Quit`, anything else → `completeLoadingDismissal()`.
  - `internal/tui/model.go:1503-1520` — `dismissLoadingGate` now holds the page on an in-flight decision (`:1510-1514`), dispatches once and returns with the page still `PageLoading` (`:1515-1518`), and otherwise falls through to the extracted `completeLoadingDismissal` (`:1522-1526`), which keeps the exact `transitionFromLoading` → `tea.Batch(surfaceBufferedWarnings, refetchSessionsAfterRestore, maybeDispatchDetectionCmd)` sequence.
  - `internal/tui/model.go:1646-1652` — the `searchDecisionMsg` arm in `Update`'s cross-view switch, ahead of `BootstrapProgressMsg` and with the `fatalActive` early return.
  - `internal/tui/model.go:192-199` — `searchDecide` plus the new `searchDecideInFlight`, with the field comment naming the gate's own state as the single-shot guard.
  - `cmd/testhelpers_test.go:258-287` — `driveLoadingGates`, the shared cmd-side driver that satisfies both gates and runs the dispatched command.
  - `cmd/concurrent_search_decision_integration_test.go:118-126` — the cold-boot pump now runs the command the terminal complete event returns and feeds its answer back in.
- Notes:
  - The single-shot property is now held by gate state rather than by clearing the closure field mid-call, exactly as the task prescribed. `searchDecide` is read in only two production places (`model.go:1515`, `search_decision.go:41`) and written in two (`search_decision.go:13`, `:52`); nothing else keys off it, so leaving it non-nil while the decision is out changes no other behaviour.
  - `resolveSearchDecision` is gone from the tree — no stale Go reference to it remains (the only hits are in `.workflows/` planning and review prose).
  - `dismissLoadingGate` and `applySearchDecision` are both value-in/value-out `(Model, tea.Cmd)`, which preserves phase 11-2's shape: every method that writes the decision state returns the model it wrote, and the losing `return m, m.method()` spelling cannot compile.
  - Ordering preserved: the `BootstrapCompleteMsg` arm still assigns `bufferedWarnings` before reaching the gate (`model.go:1666-1672`), and the attach/read-failure branches return before `completeLoadingDismissal`, so `surfaceBufferedWarnings` never empties the buffer on those exits.
  - AC2's "Ctrl-C quits" was settled by reading the dependency rather than assumed: `dismissLoadingGate` returns the decision as a bare `tea.Cmd`, which Bubble Tea v2 runs in a deliberately unwaited goroutine (`charm.land/bubbletea/v2@v2.0.7 tea.go:711-733`, "Don't wait on these goroutines, otherwise the shutdown latency would get too large"), and `Program.shutdown` (`tea.go:1241-1264`) waits only on `p.handlers`, not on that goroutine. A slow tmux read therefore cannot delay the quit. The loading page stays on `PageLoading`, so the `PageLoading` key arm (`model.go:1812-1816`) is the one that sees the keypress and returns `tea.Quit` with nothing selected.
  - No production behaviour drifts from §3.4/§3.7/§7: the decision still runs only after both gates, still attaches on K=1 without composing a picker frame, still reports a read failure without a fatal, and still leaves the warnings owed on the non-picker exits.

TESTS:
- Status: Adequate
- Coverage: All nine tests the task named exist and are non-vacuous.
  - `internal/tui/search_decision_test.go:280-300` `TestSearchDecision_HoldsLoadingPageWhileInFlight` — the direct AC1/AC2 proof: page is `PageLoading`, `d.calls == 0` before the returned command is run and `1` after, and the command's answer is asserted to be a `searchDecisionMsg`.
  - `:307-323` `TestSearchDecision_CtrlCWhileInFlightSelectsNothing` — Ctrl-C with the decision outstanding quits with empty `Selected()` and false `SearchAttached()`.
  - `:214-237` `TestSearchDecision_IssuedExactlyOnce` — drives `min → complete → complete → min` and asserts one dispatch and one closure call (AC3's duplication half).
  - `:137-191` `HeldUntilMinimumSpanElapses` / `HeldUntilBootstrapCompletes` / `NotIssuedOnProgress` — AC3's "not before both gates", in both gate orders plus the progress stream.
  - `:193-212` `NotIssuedAfterBootstrapFatal` — AC3's "not after a fatal".
  - `:63-80` and `:117-135` — AC4, attach and read failure, each asserting the page never left `PageLoading` and a quit was returned; the read-failure case additionally asserts `FatalError() == nil`.
  - `:96-115` `OpensPickerWhenNoSession` and `:82-94` `LeavesBufferedWarningsUnsurfacedOnAttach` — AC5's tui half (band raised on the picker exit, no band and the buffer intact on the attach). `cmd/open_search_warnings_test.go:204-262` covers AC5's `WarningsOwedAtTeardown` half across attach / staged+orchestrator / read failure / picker exits, plus `:293-318`, a new subtest pinning that a cancel with the decision deliberately left unrun still owes the buffered set — the escape route this task creates.
  - `:325-353` `MessageAfterFatalLeavesErrorFrameStanding` — AC6, with the fatal interleaved between the dispatch and the answer, asserting the fatal stands, the page stands, nothing was selected and no command was returned.
  - `:257-278` `AbsentClosureLeavesTransitionUnchanged` — AC7's no-closure half.
- Notes:
  - The tui-side driver `transitionThroughDecision` (`:43-61`) discriminates the decision command by message type (`isDecisionMsg`) and `t.Fatal`s if neither gate dispatched, so none of these tests can pass vacuously. The cmd-side `driveLoadingGates` cannot see the unexported message type and discriminates on "a command returned with the page still `PageLoading`" instead; each of its three call sites then asserts a post-decision property (`SearchAttached()`, `connectedTo`, `WarningsOwedAtTeardown`), so the weaker discriminator cannot hide a missed dispatch either.
  - Not over-tested: the pairs that look adjacent are distinct states — `CtrlCBeforeDecisionSelectsNothing` is pre-dispatch and `CtrlCWhileInFlightSelectsNothing` is post-dispatch; `NotIssuedAfterBootstrapFatal` is fatal-before-gates and `MessageAfterFatalLeavesErrorFrameStanding` is fatal-after-dispatch.
  - No other suite builds a model carrying a decision closure (`SearchForm.Decide` appears only in `internal/tui/build.go`, the three cmd test drivers and the integration pump), so the gate change cannot have silently degraded an unrelated gate-driving test into a no-op.

CODE QUALITY:
- Project conventions: Followed. The change moves the one exception back onto the package's established shape — every other tmux read on this path (`fetchSessionsCmd`, `loadProjects`, `refetchSessionsAfterRestore`) is a `tea.Cmd` returning a message, and this now is too. Value-in/value-out receivers match the surrounding handlers. No new log component, no new attr key, no test executes a real command body without its seams.
- SOLID principles: Good. `searchDecisionCmd` (dispatch) and `applySearchDecision` (act) split the old dual-purpose method along the one axis that mattered; `completeLoadingDismissal` gives the shared tail exactly one home, so both the gate and the decision's picker branch run the identical sequence.
- Complexity: Low. `dismissLoadingGate` is three guards; `applySearchDecision` is two.
- Modern idioms: Yes.
- Readability: Good. The three comments that stated the dropped synchronous rule were all rewritten to state the new one (`search_decision.go:36-39`, `model.go:1503-1508`, `model.go:192-198`), and `model.go:1511-1512` names why a repeat gate message holds rather than dismisses.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "A model with no decision closure dismisses exactly as it does today, and `go test ./...` is green." — The green-suite half cannot be settled by reading. Run `go test ./...` for the unit lane, and `go test -tags integration -p 1 ./cmd -run TestConcurrentColdBoot_SearchDecision` for the two cold-boot suites whose pump this task rewired (`cmd/concurrent_search_decision_integration_test.go`), which are the only coverage that drives the new dispatch against a real ten-step bootstrap and a real tmux server.
