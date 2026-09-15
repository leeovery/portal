TASK: open-with-forced-filter-10-3 (tick-4c0f85) — Let the recorded search decision drive the teardown's warning branch

ACCEPTANCE CRITERIA:
1. `WarningsOwedAtTeardown` reads `m.searchAttached` and `m.searchErr` and nothing else — no `activePage` test and no `m.selected` test
2. `m.searchAttached` has a production consumer: written in the decision resolution, read by the teardown branch, exposed by `SearchAttached()`
3. A search attach and a failed search read each still owe the buffered set
4. A warm picker, a picker-decision search, a cancelled loading page and a command-pending run each still owe the staged set
5. A session selected by Enter on the picker still owes the staged set
6. `go test ./internal/tui/... ./cmd/...` passes with no edit to any test file

STATUS: complete

SPEC CONTEXT: §7.5 — on a K=1 sigil attach the TUI tears down before the connector runs, so the notice band never surfaces; the accumulated soft bootstrap warnings must be written to the terminal after teardown and before the attach. The task is the model-side half of that: making the teardown's "which set do I owe?" answer read the decision the gate actually recorded rather than re-deriving it from the page and the selection.

IMPLEMENTATION:
- Status: Implemented as written, then deliberately superseded by a later task in the same plan.
- Location: delivered at commit `fb1d3b442` (`internal/tui/model.go`, 4 insertions / 3 deletions, no test file touched). That commit replaced the `m.activePage == PageLoading && (m.selected != "" || m.searchErr != nil)` derivation with `m.searchAttached || m.searchErr != nil` and re-voiced the doc comment, exactly as the task's Do prescribed. Criteria 1 and 2 were met by that commit.
- Current HEAD: `internal/tui/model.go:486-488` now reads `return slices.Concat(m.bufferedWarnings, m.pendingBootstrapWarnings)` — no branch at all. This is commit `54d9a9f36`, task `open-with-forced-filter-12-5` (tick-cb4c82, "Owe the terminal every warning nobody surfaced, however the picker exits"), whose Do explicitly instructs "dropping the `searchAttached || searchErr != nil` branch entirely". That task's own record names the reason: phase 12's task 3 made a Ctrl-C on the loading page the designed escape from a slow tmux server, and under the 10-3 branch that exit answered with the (by then nil) staged set, silently dropping every accumulated warning. So the later task is a strict improvement on the same axis — it removes the discriminator the finding was about rather than re-keying it — and it is on the record, not drift.
- Consequence for the criteria as literally worded: criterion 1 is not met at HEAD (the function reads neither field); criterion 2 is not met at HEAD (`SearchAttached()` has 14 call sites across 4 files — `cmd/concurrent_search_decision_integration_test.go`, `cmd/open_search_warnings_test.go`, `cmd/open_picker_landing_test.go`, `internal/tui/search_decision_test.go` — and zero in a non-test file); criterion 4's cancelled-loading-page clause is deliberately inverted at HEAD (the cancel now owes the buffered set, which is 12-5's bug fix).
- Judged as a divergence rather than a finding: nothing the intent needed is lost. The failure mode 10-3 named — an assertion staying green while the teardown falls through to the wrong branch and a cold-boot `x /term` attach drops the user's warnings — is now structurally impossible, because there is no branch to fall through: every exit owes everything no surface consumed. The residue (a flag with test-only readers) is not a loss either: `m.searchAttached` is the only way any caller can distinguish "the decision took the attach arm" from "Enter selected a row", which `Selected()` alone cannot express, so the accessor records a real and distinct fact about the decision.
- Notes: `m.searchAttached` and `m.searchErr` are still written from exactly one place each (`internal/tui/search_decision.go:60` and `:55`, both inside `applySearchDecision`, renamed from `resolveSearchDecision` by task 12-3), so the equivalence premise 10-3 turned on held for the life of its branch.

TESTS:
- Status: Adequate.
- Coverage: `TestWarningsOwedAtTeardown` (`cmd/open_search_warnings_test.go:193-317`) carries eight subtests covering the attach, the staged-warning-plus-attach union, the failed read, the warm picker, the surfaced buffer, the no-warnings case, and both cancel windows (pre-dispatch and decision-in-flight). `internal/tui/search_decision_test.go` covers the attach, picker-decision, no-closure and cancel arms of the decision itself. `TestFinishTUI` pins the teardown ordering (warnings written before the connect) and `cmd/concurrent_search_decision_integration_test.go` pins the decision behind a real cold server.
- Notes: the task was a behaviour-preserving refactor requiring no test edit, and the delivering commit edited none. The `SearchAttached()` / `SearchError()` pre-checks the task cited as now guarding the branch survive at HEAD as pre-checks on the recorded decision itself; with 12-5's branch removal they no longer guard a production branch, but they are not redundant — they discriminate the decision arm the subtest is set up for, which the owed-set assertion below them cannot. Not over-tested: each subtest pins a distinct exit route.

CODE QUALITY:
- Project conventions: Followed. Model accessors stay value-receiver reads; the change touched one function and its doc comment.
- SOLID principles: Good — the teardown's answer stays in the model beside the state it reads, which is what cycle 3's approved task settled.
- Complexity: Low (the delivered form was a two-field disjunction; HEAD is branchless).
- Modern idioms: Yes — `slices.Concat` at HEAD, already imported for the `BootstrapCompleteMsg` arm.
- Readability: Good. The doc comment at `internal/tui/model.go:481-485` states the union rule and holds true against the code beneath it: each field is emptied by its consumer (`internal/tui/bootstrap_warnings.go:46` and `:60` for the buffer; `internal/tui/model.go:1672` for the staged set), so a set still sitting in either field is one nobody surfaced. No stale claim survives from the 10-3 wording.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./internal/tui/... ./cmd/...` passes with no edit to any test file" — the no-edit half is settled by reading (`git show --stat fb1d3b442` lists `internal/tui/model.go` alone); the passing half needs the unit lane run — `go test ./internal/tui/... ./cmd/...` — which reading cannot settle.
