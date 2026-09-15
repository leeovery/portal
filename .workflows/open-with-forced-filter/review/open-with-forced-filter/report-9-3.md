TASK: open-with-forced-filter-9-3 (tick-b8464e) — Let the model answer what the teardown still owes the terminal

ACCEPTANCE CRITERIA:
1. `finishTUI` makes exactly one `tui.WriteBootstrapWarnings` call and reads exactly one model accessor to decide its argument.
2. No predicate in `cmd` reads `SearchAttached()` or `SearchError()` to decide a warning write (`processTUIResult`'s `SearchError()` read, which returns the error, is untouched).
3. Route parity, each verified: search attach → buffered set written; search read failure → buffered set written; warm picker painted → pending set written; picker painted after a loading gate → nothing; cancelled loading page → nothing.
4. Nothing is written twice on any route, and the disjointness `finishTUI` asserted in prose is now a consequence of one function returning one set.
5. `surfaceBufferedWarnings` and the concurrent route's notice band are unchanged.
6. The teardown's lines still match the CLI path byte for byte.

STATUS: complete

SPEC CONTEXT:
§7.1/§7.2 make a sigil invocation a picker invocation, so its soft bootstrap warnings take the in-TUI route rather than stderr; §7.5 carves out the K=1 case — the TUI tears down before the connector runs, so the notice band never surfaces and the accumulated warnings are written to the terminal "after teardown, before the attach", which is where a warm-server sigil attach already puts them. The task is the architecture-side consequence: the question "what does this teardown still owe the terminal" is not search-specific, so the answer moves out of a search-named predicate in `cmd` and into the model that holds both buckets.

IMPLEMENTATION:
- Status: Implemented (and since deliberately evolved by two later plan tasks in the same plan — see Notes).
- Location:
  - `internal/tui/model.go:481-488` — `WarningsOwedAtTeardown()`, the single decider.
  - `cmd/open.go:621-629` — `finishTUI`, now a single `tui.WriteBootstrapWarnings(warnings, model.WarningsOwedAtTeardown())` in the slot between `tui.RestoreTerminalBackground` and `processTUIResult`.
  - `cmd/open_search.go` — `emitSearchTeardownWarnings` and the `io` import are gone (commit 5e132cc7a); no reference survives anywhere in the tree.
  - `internal/tui/model.go:1666-1679` — the gate's take (`bufferedWarnings = Concat(pending, msg.Warnings)`; `pendingBootstrapWarnings = nil`) and the command-pending append, which are what make the buckets disjoint at teardown.
- Notes on the criteria, each checked against the code at HEAD:
  - (1) Holds. `finishTUI` contains exactly one `WriteBootstrapWarnings` call and one accessor read to decide its argument; the only other production `WriteBootstrapWarnings` caller is `internal/tui/bootstrap_warnings.go:16` (the in-TUI flush), and the CLI path writes via `BootstrapWarningsSink.EmitTo` (`cmd/bootstrap_warnings.go:37`).
  - (2) Holds. Across non-test sources, `SearchError()` has exactly one reader — `cmd/open.go:609` in `processTUIResult`, the exempted one that returns the error — and `SearchAttached()` has no production reader at all.
  - (3) Holds on four of the five routes as written; the fifth (cancelled loading page) was deliberately inverted afterwards by plan task 12-5 (tick-cb4c82, "Owe the terminal every warning nobody surfaced, however the picker exits"), whose stated problem is precisely that the carve-out this task's criterion pinned drops every warning when Ctrl-C is the designed escape from a slow decision. That is a later approved plan task superseding an earlier one, not drift: the decider now returns `slices.Concat(bufferedWarnings, pendingBootstrapWarnings)`, and 12-5's own criteria pin the other four exits unchanged.
  - (4) Holds, and now structurally. The two fields cannot hold warnings simultaneously at teardown: the only writer of `bufferedWarnings` (`model.go:1669`) nils `pendingBootstrapWarnings` in the same arm, the only other pending writer (`:1679`) runs on the `activePage != PageLoading` branch a dismissal reaches only after `surfaceBufferedWarnings` has nil'd the buffer, and the sink is drained into the model by `stageBootstrapWarningsOnModel` so no third copy survives. So the union writes each warning once, and the prose invariant the old `finishTUI` asserted is a property of the one function that answers.
  - (5) Holds. The task commit (5e132cc7a) touched `cmd/open.go`, `cmd/open_search.go`, `internal/tui/model.go` and one test file only; `internal/tui/bootstrap_warnings.go` and `notice_band.go` are untouched by it.
  - (6) Holds. Both writers bottom out in `warning.WriteLines` (`internal/tui/bootstrap_warnings.go:21-23` and `cmd/bootstrap_warnings.go:37-39`), and two subtests assert the teardown's stderr equals the CLI sink's rendering of the same warnings.
  - Spec §7.5 ordering is preserved: the write sits after `RestoreTerminalBackground` and before `processTUIResult`, and `TestFinishTUI` captures stderr *at the connect* to prove the exec'd attach cannot outrun it.

TESTS:
- Status: Adequate.
- Coverage: `cmd/open_search_warnings_test.go:193-318` (`TestWarningsOwedAtTeardown`) drives the real `tui.Model` through each exit and asserts the owed set: gate-quit on a named session (:204), gate-quit on a failed read (:241), a warning staged before the concurrent launch surviving the gate's fold (:214), warm picker painted (:250), picker painted after the gate (:255), nothing accumulated (:264), cancelled loading page (:269) and cancelled with the decision still in flight (:293). `TestFinishTUI` (:320-366) pins the cmd-level wiring — written exactly once, present at the connect, canvas set-back first — and byte parity with the CLI sink; `TestFinishTUI_StagedBootstrapWarnings` (:419-520+) repeats the route set at the teardown boundary.
- Notes: The five test names the plan required are present, the cancelled one under 12-5's renamed-and-re-reasoned wording. The model-level and `finishTUI`-level suites overlap on route coverage, but they pin different properties (which set the model answers with, versus that the teardown writes it once in the right order against the real writer), so the overlap is justified rather than redundant. Assertions compare rendered output through the shared `wantWarningOutput` helper, so a break in either the decider or the renderer fails them. No mock stands in for the model — every case drives `tui.Build` and real `Update` messages.

CODE QUALITY:
- Project conventions: Followed. No new log component or attr, no new seam, the `cmd` `*Deps`/function-seam rules are untouched, and the test suite injects through the existing `installSearchFormSeams`/`warmSearchCommand` helpers rather than reaching a real tmux.
- SOLID principles: Good — this is the point of the task: the arbitration now sits with the type that owns both fields, and `cmd` states the policy-free "write what is owed".
- Complexity: Low. One accessor, one call site.
- Modern idioms: Yes. `slices.Concat` (already imported for the gate arm) returns nil for two empty inputs, so the no-warnings route needs no special case.
- Readability: Good. The doc comment on `WarningsOwedAtTeardown` names the two consumers that empty the fields, which is exactly the fact the union rule turns on.
- Comment accuracy: Verified. The decider's comment ("the buffer by the picker's notice band or stderr flush, the staged set by a loading gate folding it into the buffer") matches `bootstrap_warnings.go:55-66`/`:41-50` and `model.go:1669-1672`; the superseded paragraph claiming a cancelled page is owed nothing was removed with the branch. `finishTUI`'s remaining comments ("the outside-tmux attach execs and never returns") hold against the ordering the test pins. No process-artifact references.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
