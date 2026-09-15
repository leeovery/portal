TASK: open-with-forced-filter-4-2 (tick-700a3d) — Defer the search count to the loading page when a bootstrap is in flight

ACCEPTANCE CRITERIA:
1. With a deferred bootstrap on the context, `runSearchForm` calls neither source method before `openTUIFunc` runs, and the captured landing carries a non-nil `decide` alongside `{filter: term, search: true}`
2. Invoking the captured closure returns the single matching session's name; zero matches and two-plus matches both return `("", nil)`; an enumeration error is returned unchanged
3. The closure's candidate set is identical to the up-front count's — the same discriminating probe read, and inside tmux the current session excluded
4. Without a deferred bootstrap the count is taken up front exactly as task 2-2 leaves it, and the landing's `decide` is nil
5. The term-less form supplies no closure on either route and calls neither source method
6. `-f <text>` and the no-argument picker pass a nil `decide`
7. `processTUIResult` returns the model's search error without connecting anything, ahead of `Selected()` and behind `FatalError()`; the returned error is neither a `*UsageError` nor a `*bootstrap.FatalError`
8. A single match found by the closure is connected by the connector `openTUI` built before `p.Run()` — the closure constructs no connector and performs no connect of its own
9. `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

STATUS: complete

SPEC CONTEXT:
§3.4 fixes when K is taken: immediately on a warm server, and on a cold server only once the concurrent bootstrap has run to completion — every step, not merely restore, because acting at the end of restore would replace the process mid-bootstrap and abandon the clearing of `@portal-restoring`. §3.7 fixes two things this task must honour: an attach under K = 1 uses the connector the invocation already selects (no third connection mode), and a session list that could not be read is not a zero match — it is reported in tmux's own words, exits non-zero, is not a bootstrap fatal and takes no in-TUI error frame, reaching the user after teardown on a cold boot. §7.1/§7.6 classify the sigil as a picker invocation taking the concurrent bootstrap unchanged.

IMPLEMENTATION:
- Status: Implemented (with legitimate downstream drift from later tasks)
- Location:
  - `cmd/open_search.go:200-211` — `searchDecision(src, term)`, the single classification: `searchCandidates` → `searchMatches`, `(matches[0].Name, nil)` at exactly one, `("", nil)` otherwise, error returned unchanged.
  - `cmd/open_search.go:221-249` — `runSearchForm`: term-less early return at 223 (no source built, no closure), closure built once at 226, deferred branch at 229-232 handing the closure to the picker, warm route at 234-248 invoking the same closure.
  - `cmd/open.go:594-597` — `buildTUIModel` passes the landing's `*tui.SearchForm` (term + `Decide`) straight into `tui.Deps.Search`; `internal/tui/build.go:133` wires both through `WithSearchForm`/`WithSearchDecision`.
  - `cmd/open.go:602-615` — `processTUIResult`: `FatalError()` (603) → `SearchError()` (609) → `Selected()` (612) → `connector.Connect`.
- Notes:
  - Two shape changes from later plan tasks, both sound and neither a loss. The plan wrote `decide func() (string, error)` onto `pickerLanding`; task 9-2 moved the search form into `pickerLanding.search *tui.SearchForm`, so the closure now lives on `SearchForm.Decide` (`internal/tui/build.go:68-78`) and the term lives in `search.Term` rather than `landing.filter`. The substance of criterion 1 is unchanged — the picker still lands pre-filtered by the term via `WithSearchForm(deps.Search.Term)` — and the structure is strictly better: the pointer makes "search landing" and "-f filter" mutually exclusive by construction.
  - The plan's "Do" put `buildSearchSessionSource(cmd)` inside the deferred branch; the implementation hoists the closure above the branch so both routes run the identical one. That is what makes criterion 3 true by construction rather than by two implementations agreeing — an improvement on the written instruction.
  - Criterion 3 verified by reading: both routes call the same `searchDecision` closure, whose enumeration is `ListSessionsProbe` (`internal/tmux/tmux.go:143-149`, the discriminating variant that returns an error rather than an empty list) less `currentPickerSession` (`cmd/open_search.go:164-173`).
  - Criterion 8 verified by reading: `openTUI` builds `connector` at `cmd/open.go:686`, ahead of `p.Run()` at 744, and hands it to `finishTUI` at 754 → `processTUIResult` → `connector.Connect(selected)`. The closure returns a name only; it constructs nothing. The warm route's `openSession` (`cmd/open.go:112-114`) is `buildSessionConnector(tmuxClient(cmd)).Connect(name)` — the same connector construction, so the two routes attach identically and §3.7's "no third connection mode" holds.
  - The §3.7 error contract holds end to end: `rootCmd` sets `SilenceErrors: true` (`cmd/root.go:164`), so the returned read failure is printed once by `main.classify` (`main.go:62-77`) and exits 1, not 2.
  - §3.4's "once the bootstrap has run to completion" is honoured by the gate this task feeds: `dismissLoadingGate` (`internal/tui/model.go:1509-1519`) is only reached with both `minElapsed` and `bootstrapComplete` set (`internal/tui/model.go:1641`, `1693`), so the closure cannot run before the terminal event — which is also what makes the task's "the two never issue tmux commands concurrently" edge case true.

TESTS:
- Status: Adequate
- Coverage: `cmd/open_search_deferred_test.go` covers criteria 1-5 and 7 by name, one subtest per planned test: no source call before the picker seam (`readsAtTUI`, 43-56), the landing's captured shape including the `decided` bit (58-67), single match / zero / two-plus / enumeration error through the captured closure (69-125), current-session exclusion on the deferred route (127-141), the term-less form on both routes (143-168), and the warm route counting up front with a nil closure (171-187). `TestProcessTUIResult_SearchDecision` (244-294) drives a real `tui.Build` model to the loading gate and asserts the search error is returned un-wrapped, is neither a `*UsageError` nor a `*bootstrap.FatalError`, leaves the connector untouched, connects the single match, and loses to a bootstrap fatal. Criterion 6 is covered by the existing `-f` and no-arg landing assertions (`cmd/open_test.go:2185`, `2390`, `2437`, `2526`, `2552`), which this commit converted to exact `landingShape` comparisons — `decided` is part of that struct, so a stray closure on either path fails them.
- Notes:
  - The switch from comparing `pickerLanding` directly to `shapeOfLanding` (`cmd/testhelpers_test.go:238-256`) was forced by the func field making the struct non-comparable, and it preserves every property the old comparison pinned plus the new `decided` bit. Nothing was weakened.
  - Two subtests in `TestSearchForm_WarmInvocation_CountsUpFront` — "it attaches the single match up front" (189-203) and "it returns the enumeration error up front" (205-218) — restate `TestOpenCommand_SearchForm_AttachesTheSingleMatch` (`cmd/open_search_test.go:514`) and `TestOpenCommand_SearchForm_ReturnsAnEnumerationError` (`cmd/open_search_test.go:728`) one level lower (direct `runSearchForm` rather than `rootCmd.Execute`). They are the regression guard for hoisting the closure above the branch, so the duplication is deliberate and cheap; not reported.
  - The deferred current-session-exclusion subtest relies on the package-wide poisoned `TMUX` (`cmd/testmain_isolation_test.go:76`) to make `tmux.InsideTmux()` true, as the warm suite already does. Consistent with the package's convention.

CODE QUALITY:
- Project conventions: Followed. The classification stays in `cmd`, `internal/tui` takes a bare `func() (string, error)` seam and remains env-free; `SearchSessionSource` is a two-method interface resolved through `openDeps`, matching the package's DI pattern; the tests stage every seam through `withFuncSeam`/`withOpenDeps`/`withBootstrapDeps` rather than assigning directly, so `cmd/seam_guard_test.go` stays satisfied; no `t.Parallel()`.
- SOLID principles: Good. `searchDecision` composes `searchCandidates` and `searchMatches` rather than restating either — grep confirms one production caller each and no second implementation of the count.
- Complexity: Low. `runSearchForm` is one early return, one branch and a three-way result; `processTUIResult` gained one guard clause in a fixed order.
- Modern idioms: Yes.
- Readability: Good. The branch reads as "build the classification, then decide where it runs".
- Issues: None. The comments added by this task hold against the code: `cmd/open.go:606-608` ("ordinary error rather than a bootstrap fatal… no error frame and nothing attached") is true of both `processTUIResult`'s ordering and `main.classify`'s handling, and `cmd/open_search.go:218-220` ("hands the count to the picker, which takes it once that bootstrap completes") is true of the gate at `internal/tui/model.go:1509-1519`.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — settled only by running both lanes. Reading confirms the suites exist and are wired (`cmd/open_search_deferred_test.go`, `cmd/concurrent_search_decision_integration_test.go` under `//go:build integration`), but neither pass/fail nor the integration lane's real-cold-server timing can be judged by reading.
