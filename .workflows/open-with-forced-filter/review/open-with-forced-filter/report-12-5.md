TASK: open-with-forced-filter-12-5 (tick-cb4c82) — Owe the terminal every warning nobody surfaced, however the picker exits

ACCEPTANCE CRITERIA:
1. `WarningsOwedAtTeardown` returns the concatenation of `bufferedWarnings` and `pendingBootstrapWarnings` and reads neither `searchAttached` nor `searchErr`.
2. Ctrl-C on the loading page after the bootstrap completed owes the warnings the gate took — both before the search decision is dispatched and while it is in flight.
3. The four unchanged exits keep today's answer: a search attach owes the buffered set (including a warning staged before the concurrent launch), a failed session-list read owes the buffered set, a warm picker owes the staged set, a painted picker owes nothing.
4. Every `TestWarningsOwedAtTeardown` subtest other than the cancel one passes with no edit.
5. `go test ./...` is green and `golangci-lint run` reports nothing new.

STATUS: complete

SPEC CONTEXT: §7.5 ("A sigil that resolves to a direct attach still delivers its warnings") establishes the principle the task extends: on the concurrent cold/TUI route the TUI can tear down before any surface shows a soft bootstrap warning, and in that case the warnings are written to the terminal after teardown and before the connect — the alternate screen is gone by then, so the corruption §7 exists to prevent cannot occur. §7.6 adds the command-pending member, which paints from frame one and has no loading gate, so its warnings also reach the terminal only at teardown. The spec does not name the Ctrl-C-on-loading-page exit; the task is the driver for that hole, and its remedy sits squarely inside §7.5's rule (a run that ends without a surface owes its warnings to the terminal).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/model.go:481-488` — the doc comment replaced and the body collapsed to `slices.Concat(m.bufferedWarnings, m.pendingBootstrapWarnings)`; the `searchAttached || searchErr != nil` branch is gone, and neither field is read anywhere in the function.
  - `internal/tui/model.go:8` — `slices` was already imported (also used at `:1669`), so no import churn.
  - `cmd/open.go:626` — the sole production caller, unchanged (`finishTUI` at `:621` is itself called only from `cmd/open.go:754`).
  - `internal/tui/model.go:471-479` (the `BufferedWarnings` / `PendingBootstrapWarnings` accessors) and `internal/tui/model.go:493-495` (`SetPendingBootstrapWarnings`) untouched, as the task required.
- Notes:
  - The union cannot double-count. The only two routes that put warnings on the model are `SetPendingBootstrapWarnings` (staged before Bubble Tea starts, `cmd/bootstrap_warnings.go:44-51`) and the `BootstrapCompleteMsg` arm (`internal/tui/model.go:1658-1681`). On the loading-page branch that arm assigns `bufferedWarnings = slices.Concat(pendingBootstrapWarnings, msg.Warnings)` and then nils `pendingBootstrapWarnings` (`:1669-1672`), so the two fields are disjoint by construction; on the command-pending branch it appends to `pendingBootstrapWarnings` only, and that route forces `PageProjects` from frame one so `bufferedWarnings` is never populated there. No path leaves the same warning in both fields.
  - Ordering is preserved on the one path where both halves could be non-empty in principle: the gate folds the staged set in *first* (`:1669`), so `concat(buffered, pending)` renders staged-before-orchestrator, which is what `TestWarningsOwedAtTeardown/"it still owes a staged warning …"` (`cmd/open_search_warnings_test.go:212-236`) pins.
  - The cancel path holds its buffer: `PageLoading`'s key arm returns `m, tea.Quit` with the model unmutated (`internal/tui/model.go:1812-1815`), so `bufferedWarnings` survives into the teardown read.
  - Behaviour on the fatal exit is unchanged in substance: `BootstrapFatalMsg` carries no warnings (`internal/tui/model.go:142-146`) and is mutually exclusive with `BootstrapCompleteMsg`, so `bufferedWarnings` is empty there and the union still answers with the staged set alone.
  - The change reaches every TUI teardown, not only the search route — a plain cold-boot picker cancelled on the loading page now writes its warnings too. That is the task's stated Outcome, and it is safe: `finishTUI` runs after Bubble Tea has returned, so the alternate screen is already gone.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/open_search_warnings_test.go:270-291` — the inverted subtest, renamed to `"it owes the buffered set when the loading page was cancelled"`, now asserting `warnings` through `assertOwed` with the reason re-written ("no surface ever showed what the gate took"). Both pre-assertions the task required are kept: the buffer is non-empty (`:283-285`) and neither an attach nor an error is recorded (`:286-288`).
  - `cmd/open_search_warnings_test.go:293-317` — the new in-flight sibling. It drives `WindowSizeMsg` → `LoadingMinElapsedMsg` → `BootstrapCompleteMsg{Warnings}`, asserts the dispatched decision command is non-nil while the page stays on `PageLoading` (`:307-309`), deliberately leaves that command unrun, sends Ctrl-C and asserts the same warnings are owed. Traced against the model: both gates satisfied route into `dismissLoadingGate` (`internal/tui/model.go:1503-1517`), which sets `searchDecideInFlight` and returns `searchDecisionCmd()` while holding the page — exactly the state the subtest claims to be in.
  - `cmd/open_search_warnings_test.go:479-501` — `TestFinishTUI_StagedBootstrapWarnings`'s cancel subtest inverted in the same commit, and it is the one end-to-end assertion of the new behaviour: it runs `finishTUI` and compares stderr byte-for-byte against `wantWarningOutput(warnings)`. It builds a model with **no** `Search` form, so it also covers the plain (non-sigil) loading-page cancel the union now reaches. The plan named only the `TestWarningsOwedAtTeardown` subtest, but this edit is a required consequence of the same behaviour change and encodes it correctly.
  - The four unchanged exits are all still covered and all still answer as before: attach (`:203-210`), attach with a pre-staged warning (`:212-236`), failed read (`:238-246`), warm picker (`:248-253`), painted picker owes nothing (`:255-261`), nothing accumulated (`:263-267`).
- Notes:
  - Would the tests fail if the feature broke? Yes — reverting to either single field breaks a cancel subtest (`nil` vs `warnings`), and reverting to the old discriminator breaks both.
  - Not over-tested: the two cancel subtests exercise genuinely different model states (pad not elapsed vs decision dispatched-and-outstanding); neither is reachable from the other's setup, and the shared `tui.Build` block is four lines. The existing `searchTeardownModel` / `driveLoadingGates` helpers cannot be reused here precisely because they drive the gates to completion, which is the state the cancel subtests must avoid.
  - No new mocking or seams were introduced; the subtests drive `Update` directly, as every sibling in the file does.

CODE QUALITY:
- Project conventions: Followed. `slices.Concat` is the `modernize`-linter-preferred form and matches the existing use at `:1669`; the accessor keeps the value-receiver shape of its neighbours; the doc comment states the rule rather than the mechanism, in the house voice.
- SOLID principles: Good. The function now has a single, stated rule and no knowledge of which exit occurred — the discriminator it used to carry duplicated state that the consuming sites already encode by emptying their own field.
- Complexity: Low. Two branches and two field reads replaced by one expression.
- Modern idioms: Yes.
- Readability: Good. The new comment names each field's consumer, which is what makes the union rule checkable at the call site rather than by tracing.
- Comment accuracy: Verified against the code. "the buffer by the picker's notice band or stderr flush" matches `surfaceBufferedWarnings` / `flushBufferedWarningsCmd` (`internal/tui/bootstrap_warnings.go:41-66`, both of which nil the field); "the staged set by a loading gate folding it into the buffer" matches `internal/tui/model.go:1669-1672`. The paragraph claiming a cancelled loading page is owed nothing is gone with the branch. No process-artifact references.
- Issues: None above the reporting bar.

Judged and dismissed (recorded for the orchestrator, not findings):
  - `Model.SearchAttached()` (`internal/tui/search_decision.go:17-21`) has no production reader. Measured: its last one was removed in commit `5e132cc7a` (task 9-3), which replaced `emitSearchTeardownWarnings(warnings, model)` in `cmd/open.go` with the `WarningsOwedAtTeardown` call — **not** by this task. What 12-5 removed was a read of the unexported *field* `m.searchAttached` from inside the same package, which never went through the accessor. Today the only non-test reference to `SearchAttached()` in the repo is its own declaration; it is read from four test files (`cmd/concurrent_search_decision_integration_test.go`, `cmd/open_picker_landing_test.go`, `cmd/open_search_warnings_test.go`, `internal/tui/search_decision_test.go`), where it asserts a real behavioural property — that a picker decision, a cancel and a nil closure each record no attach. It joins an established family on this model (`BufferedWarnings`, `PendingBootstrapWarnings`, `MinElapsed`, `BootstrapComplete` likewise have no production reader), so it is consistent with the package's own convention rather than orphaned, and deleting it would cost coverage. Nothing breaks by leaving it; there is no finding here.
  - `internal/tui/model.go:243-245` ("What is still pending at the end is written at teardown") is now an incomplete description of the pair rather than a false one — what is still *buffered* is written too. It is outside the changed lines and the code does not falsify it, so it is below the bar.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` is green and `golangci-lint run` reports nothing new." — a suite run and a lint run; settled only by executing them. Reading settles the parts that can be read: the change compiles in principle (`slices` already imported, no signature change, one production caller unchanged), the two edited test files reference only existing exported symbols (`tui.LoadingMinElapsedMsg`, `tui.BootstrapCompleteMsg`, `Model.ActivePage`, `tui.PageLoading`, `Model.SearchAttached`, `Model.SearchError`, `Model.BufferedWarnings`, `wantWarningOutput`), and no other test in the repo asserts on `WarningsOwedAtTeardown` (the only two files naming it are `cmd/open.go` and `cmd/open_search_warnings_test.go`).
- "Every `TestWarningsOwedAtTeardown` subtest other than the cancel one passes with no edit." — the "with no edit" half is settled by reading the commit diff (`54d9a9f36` touches only the cancel subtest inside that test, plus the new sibling); the "passes" half needs the suite run above.
