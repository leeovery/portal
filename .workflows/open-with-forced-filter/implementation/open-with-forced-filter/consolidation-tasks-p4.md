# Consolidation Tasks: Open With Forced Filter (Phase 4)

## Task 1: Classify a command-only line with no target as a picker invocation
placement: phase 4
severity: behaviour

**Problem**: `portal open -- claude` — no target, a command after the separator — opens the picker, and `isTUIPath` answers false for it. Its first arm asks `len(args) == 0` against the raw slice, which still holds the post-dash words, so a line with no positional target of its own reads as a CLI line. On a cold server that produces both failures this classification exists to prevent: no loading page, so the user gets a blank terminal for the whole of server start, restore and scrollback replay; and the accumulated soft warnings are written straight to stderr immediately before the picker claims that terminal with its alternate screen, so a saver-down warning is painted over and lost. The flag spelling of the identical invocation, `portal open -e claude`, takes the loading page and buffers its warnings correctly — so which of the two documented ways to scope a command the user typed decides whether their cold boot looks like a hang. This is phase 4's to fix rather than merely adjacent to it: task 4-4 rewrote `isTUIPath` into two arms and brought `--` awareness into it through `searchFormPositionals` → `preDashPositionals`, leaving one arm dash-aware and one dash-blind inside a single four-line function, and its own new subtest pins the dash rule for the search arm only.

**Solution**: Take the first disjunct through the same helper as the second — `len(preDashPositionals(cmd, args)) == 0 || len(searchFormPositionals(cmd, args)) > 0` — so the function answers "which positionals are this invocation's own?" once, and both arms read alike. Verified against every case the phase's table already pins: bare `open`, `-f text`, `-e cmd`, `~/dir`, `api blog`, `~/Code/api -- ls /tmp`, `/port`, and `/port -s api` all keep their current verdict. The one verdict that moves is `open -- <cmd>`, from false to true — which is the defect. It carries its own regression row beside the existing `-e` row.

**Outcome**: `portal open -- claude` on a cold server shows the loading page and routes its soft warnings to the notice band, exactly as `portal open -e claude` already does.

**Do**:
- In `cmd/root.go`, rewrite `isTUIPath`'s return (`cmd/root.go:176`) as `return len(preDashPositionals(cmd, args)) == 0 || len(searchFormPositionals(cmd, args)) > 0`, leaving no read of the raw `args` slice anywhere in the function.
- Add a subtest to `TestIsTUIPath` (`cmd/concurrent_bootstrap_gate_test.go`), beside the `-e cmd` row at `:69-77`, that builds an `openProbeCmd()`, calls `c.ParseFlags([]string{"--", "claude"})` and asserts `isTUIPath(c, c.Flags().Args())` is true — the `ArgsLenAtDash() == 0` shape, which is the one the raw-slice read gets wrong (same construction as the existing dash subtest at `:122-130`, which parses a pre-dash target and must keep its `false`).
- Add the matching row to `TestShouldRunConcurrentBootstrap`, beside its `-e cmd` row at `:216-224`, asserting the same parsed command routes concurrent with `latchSatisfied=false`.
- Edit no existing subtest in either test and no consumer of `isTUIPath`: the stderr-drain gate (`cmd/root.go:145-147`) and `shouldRunConcurrentBootstrap` (`cmd/root.go:188-193`) take the corrected verdict as they stand.

**Acceptance Criteria**:
- [ ] `isTUIPath` answers both disjuncts from the same positional helper — `preDashPositionals` for the no-target arm, `searchFormPositionals` (itself routed through it) for the search arm.
- [ ] `portal open -- claude` classifies as the TUI path, so its cold boot takes the concurrent bootstrap and the loading page, and its accumulated soft warnings stay in the sink for the notice band rather than being written to the terminal the picker is about to claim.
- [ ] Every other classified line keeps the verdict the phase already pins: bare `open` true, `-f text` true, `-e cmd` true, `~/dir` false, `api blog` false, `~/Code/api -- ls /tmp` false (a pre-dash target survives the strip), `/port` true, `/port -s api` false.
- [ ] `TestOpenCommand_CommandNoTarget_DashDash_OpensProjectsPicker` (`cmd/open_test.go:2486-2503`) still passes unchanged — the line still reaches the picker with the zero landing and `["claude"]` threaded as the pending command; only its cold-path classification moved.
- [ ] `go test ./...` is green.

**Tests**:
- `"open -- cmd (command after the separator, no target) IS the TUI path"`
- `"it routes concurrent for open -- cmd (command after the separator, not satisfied)"`
- `"words after a -- separator are never inspected"` — the existing subtest, unedited and still green: a line carrying a pre-dash target stays off the TUI path.

## Task 2: Fold the two loading-gate dismissal blocks into one helper
placement: phase 4
severity: duplication

**Problem**: The two gates that dismiss the loading page run the same four-step sequence — resolve the search decision, transition, then batch the warning surfacing, the post-restore refetch and the detection dispatch — as two independent copies. Which copy executes is decided by a race the user cannot observe: whichever of the minimum-display span and the bootstrap's terminal event lands second. A later edit to one arm therefore produces a cold boot whose behaviour differs by machine speed and by how long restore took. Two concrete losses: move or drop the decision in one arm and a single-match search attaches directly on a slow boot and opens the picker on a fast one; reorder it against the warning surfacing in one arm and the saver-down warning a cold-boot attach is owed is emptied from the buffer before the teardown write can read it, so it reaches the user after some reboots and vanishes after others. Nothing fails in either case: both arms compile, and the ordering constraint is stated only on one copy, so an editor working in the other has no signal that the order is load-bearing at all.

**Solution**: Extract the sequence onto the model as one pointer-receiver helper returning the command — the search decision's quit when it pre-empts, otherwise the transition and the batch. Both arms collapse to one call behind their unchanged both-gates-closed guards. The ordering comment moves onto the helper, where it is the only copy and governs both gates. Behaviour-preserving: every callee is compatible with an addressable receiver, and each guard stays in the arm that owns it.

**Outcome**: The dismissal sequence and its ordering rule exist once, so the two gates cannot diverge.

**Do**:
- Add `func (m *Model) dismissLoadingGate() tea.Cmd` to `internal/tui/model.go` beside `transitionFromLoading` (`:1474`): return `m.resolveSearchDecision()`'s command when it is non-nil, otherwise call `m.transitionFromLoading()` and return `tea.Batch(m.surfaceBufferedWarnings(), m.refetchSessionsAfterRestore(), m.maybeDispatchDetectionCmd())` — the same four steps in the same order the arms run today.
- Move the ordering comment now at `:1621-1623` onto the helper, leaving no copy in either arm, so the one statement of the rule governs both gates.
- Collapse the `LoadingMinElapsedMsg` arm (`:1596-1602`) and the `BootstrapCompleteMsg` arm (`:1620-1629`) to that single call behind their unchanged `minElapsed && activePage == PageLoading` / `bootstrapComplete && activePage == PageLoading` guards, taking the returned command into a local first (`cmd := (&m).dismissLoadingGate()` then `return m, cmd`) — `m` is the per-`Update` copy the helper mutates, and the evaluation order of a non-call operand beside a call in one return statement is unspecified.
- Leave the rest of both arms exactly as they stand: the `fatalActive` early returns, `m.bootstrapComplete = true`, and the `m.bufferedWarnings = msg.Warnings` capture.

**Acceptance Criteria**:
- [ ] `resolveSearchDecision`, `transitionFromLoading` and the three-command batch each have exactly one call site in `internal/tui`, all inside `dismissLoadingGate`.
- [ ] Each gate is one call to the helper behind the guard it already owns; neither guard moves into the helper.
- [ ] The decision-before-`surfaceBufferedWarnings` rule is stated once, on the helper.
- [ ] Behaviour is unchanged: no test file is edited, no test is renamed, and no test semantics change.
- [ ] `go test ./...` is green.

**Tests**: existing tests only — this is a pure refactor, so nothing new is added and none is altered. These must stay green, and between them they drive both arms:
- `TestSearchDecision_HeldUntilMinimumSpanElapses` / `TestSearchDecision_HeldUntilBootstrapCompletes` — the decision resolves on whichever gate lands second, from either arm.
- `TestSearchDecision_IssuedExactlyOnce` — the single-shot guard survives the fold.
- `TestColdTUIWarnings_SurfaceAsPostLoadNoticeBand` / `TestColdTUIWarnings_SurfaceWhenMinElapsedArmTransitions` — the warning surfacing on each arm.
- `TestColdBoot_PostCompleteRefetch_ReflectsRestoredSessions` / `TestColdBoot_PostCompleteRefetch_CompleteBeforeMinElapsed` — the post-restore refetch on each arm.

## Task 3: Make openTUI's teardown ordering reachable by a test
placement: phase 4
severity: complexity

**Problem**: `openTUI` ends in three ordered statements — restore the terminal background, emit the search teardown warnings, then connect — and the order of all three is load-bearing, because outside tmux the connect reaches `syscall.Exec` and the process never returns. They sit inside a function that runs a real Bubble Tea program against the real terminal, so the unit lane cannot drive it and nothing does. Moving the warning emit below the connect, or letting a later edit interleave anything between them, leaves the whole suite green while a cold-boot single-match attach silently drops every soft bootstrap warning it accumulated — the one route on which those warnings have nowhere else to go, since no picker frame is ever painted for the notice band to carry them. The user notices it as a save daemon that is down and says nothing about it, for exactly the invocation this phase made routine. The same reach covers the canvas restore above it: moved below the connect, the terminal keeps Portal's canvas colour after the attach, again with nothing failing. The existing "it writes before connecting" test re-composes the two calls by hand rather than driving the production tail, so it stays green under any reordering of it.

**Solution**: Extract the tail into a function the unit lane can drive, holding the three statements in order and returning the result, with the two writers as parameters so a test can pass buffers and the package's existing observing connector and assert that both writes had happened by the time the connect ran. `openTUI` becomes a single call to it. Behaviour-preserving; the existing subtest then drives the production function instead of a hand-composed copy of it.

**Outcome**: A reordering of the teardown fails a test rather than silently dropping warnings or leaving the canvas colour behind.

**Do**:
- Add `func finishTUI(model tui.Model, connector SessionConnector, canvas, warnings io.Writer) error` to `cmd/open.go` beside `processTUIResult` (`:572`), holding the three statements in their current order — `tui.RestoreTerminalBackground(canvas, model)`, `emitSearchTeardownWarnings(warnings, model)`, `return processTUIResult(model, connector)` — and reading no writer of its own. The two ordering comments at `cmd/open.go:715-719` move with the statements.
- Replace `openTUI`'s tail (`cmd/open.go:715-721`) with `return finishTUI(model, connector, os.Stdout, cmd.ErrOrStderr())`, so the runtime writers are unchanged.
- Move the `"it writes before connecting"` subtest (`cmd/open_search_warnings_test.go:278-293`) out of `TestEmitSearchTeardownWarnings` into a new top-level `TestFinishTUI` and rewrite it to drive `finishTUI`: a buffer per writer, the existing `observingConnector` (`:296-305`) snapshotting both buffers in `onConnect`, and a model carrying both the buffered warnings and a captured original background — feed a `tea.BackgroundColorMsg{Color: color.RGBA{…}}` whose hex differs from the model's canvas through `searchTeardownModel`'s drive so `RestoreTerminalBackground` has a set-back to write. Assert the warnings buffer equals `wantWarningOutput(warnings)` and the canvas buffer is non-empty **at the moment `Connect` ran**.
- Prove the reach before finishing: move `emitSearchTeardownWarnings` below `processTUIResult` inside `finishTUI`, confirm `TestFinishTUI` fails, revert; repeat for `tui.RestoreTerminalBackground`.
- Leave the other subtests of `TestEmitSearchTeardownWarnings` untouched — they exercise that helper directly and keep doing so.

**Acceptance Criteria**:
- [ ] `openTUI` ends in a single `finishTUI` call, and the three teardown statements exist only inside `finishTUI`, in the order canvas → warnings → connect.
- [ ] `finishTUI` writes only through its two parameters and returns `processTUIResult`'s result unchanged; `os.Stdout` and `cmd.ErrOrStderr()` are named at the `openTUI` call site alone.
- [ ] `TestFinishTUI` drives the production function — no hand-composed `emitSearchTeardownWarnings` + `processTUIResult` sequence remains in any test.
- [ ] Moving either writer below `processTUIResult` inside `finishTUI` fails `TestFinishTUI` (demonstrated, then reverted).
- [ ] Behaviour is unchanged on every teardown route: a picker that opened still writes nothing, a cancelled loading page still writes nothing, and a decision attach or a failed session-list read still writes the buffered warnings before the connect.
- [ ] `go test ./...` is green.

**Tests**:
- `"it restores the canvas and writes the warnings before connecting"` — the rewritten subtest, now under `TestFinishTUI` and driving the production tail.
- `"it writes nothing after teardown when the picker opened"` and `"it writes nothing when the loading page was cancelled"` — existing subtests of `TestEmitSearchTeardownWarnings`, unedited and still green.
