TASK: open-with-forced-filter-1-3 (tick-d3537a) — Land a search-form term as a committed sessions filter in the picker

ACCEPTANCE CRITERIA:
- A model built with `Deps.Search = &SearchForm{Term: "…"}` lands on `PageSessions` with `FilterState() == list.FilterApplied` and `FilterValue()` equal to the term
- The visible set is the sessions matching the term and the cursor is on a `SessionItem`, never a `HeaderItem`, in Flat, By Project and By Tag
- With zero live sessions the page is still Sessions, the term is on the session list, and the project list's filter is empty
- With live sessions but nothing surviving the term the page is still Sessions and the no-matches state renders; the project list's filter is empty
- `Deps.Search == nil` changes nothing: the `-f` landing, the no-filter launch and the command-pending Projects redirect behave exactly as before (existing suites green unmodified)
- A search form never routes its term to the project list, on any page-choice path
- `go test ./...` passes

STATUS: complete

SPEC CONTEXT: §3.1 makes the sigil session-domain by declaration — its term is a search over live sessions and nothing else. §3.2 sets K = 0 and K >= 2 both opening the picker on the Sessions page. §3.3 is the landing this task delivers: "filter text set and filter state committed rather than focused, cursor re-anchored onto the post-filter visible set, so arrows, `Space` and `Enter` work immediately on the narrowed list. The user is not left inside a live filter input", with the bare slash as the deliberate exception (focused and empty, §2.5). §5.2 is why the sigil must not inherit `-f`'s Projects redirect — landing a session-search on the mint page would have the form contradict itself.

IMPLEMENTATION:
- Status: Implemented (with two sound post-task movements, both from later tasks in the same plan)
- Location:
  - `internal/tui/build.go:52-53` (`Deps.Search`, documented as the plan prescribes), `internal/tui/build.go:68-71` (`SearchForm`), `internal/tui/build.go:132-134` (option wiring), `internal/tui/build.go:177-178` (the initial-filter exclusion)
  - `internal/tui/model.go:188-191` (`searchForm` / `searchTerm`, with the "describe the invocation" note the plan asked for), `internal/tui/model.go:562-570` (`WithSearchForm`), `internal/tui/model.go:1301-1311` (the page pin, ahead of the item-count branch and behind `commandPending`), `internal/tui/model.go:1318-1335` (`applySearchLanding`)
  - Delivered by commit ef1599943, which touched `build.go`, `model.go` and the new `search_landing_test.go` only — no existing test modified.
- Notes: two divergences from the task's written text, both later tasks of this plan and both judged sound.
  1. `applySearchLanding` (`internal/tui/model.go:1324-1333`) now has a term-less arm setting `list.Filtering` rather than returning early. That is task 1-6's (tick-6d0e49) delivery of spec §2.5/§3.3's bare-slash exception, and it supersedes this task's "it applies no filter for an empty search term" test, which the term-less suite (`internal/tui/search_landing_termless_test.go`) replaces with a fuller set. No loss.
  2. The plan said `applySearchLanding` takes `applyInitialFilter`'s slot; it is called after it (`internal/tui/model.go:1313-1315`) with exclusivity enforced at the constructor (`internal/tui/build.go:177`, from tick-623658) instead. Same guarantee, enforced where a second caller cannot get past it, and pinned by `TestSearchFormPrecedenceOverInitialFilter`.
- The `SetFilterText` → `SetFilterState` order and the comment justifying it (`internal/tui/model.go:1329-1333`) are correct against bubbles v2.1.0: `list.SetFilterText` is what runs the filter pass (list.go:280-292) and `VisibleItems` serves `filteredItems` in every state but `Unfiltered` (list.go:459-464), so the reverse order would hide every row. For the empty term, `filterItems` returns every item (list.go:1251-1255), which is why the term-less landing keeps the whole list visible.
- The zero-live-sessions edge is safe: `ensureSessionRowSelected` (`internal/tui/model.go:1269-1274`) type-asserts a nil `SelectedItem()`, which is false rather than a panic.

TESTS:
- Status: Adequate
- Coverage: `internal/tui/search_landing_test.go` carries one sub-test per prescribed test — page/state/value/visible set; cursor on the first surviving row in Flat; cursor off a `HeaderItem` in By Project and By Tag (table-driven over the two modes); Sessions pinned with zero live sessions and the project filter asserted empty; Sessions pinned with nothing surviving plus `sessionListNoMatches()` and an empty project filter; the `-f` landing re-asserted with `Search` nil (including `initialFilter` consumed); the command-pending Projects redirect re-asserted with the session filter empty. `TestSearchFormPrecedenceOverInitialFilter` covers the both-set case the constructor now forecloses. `internal/tui/search_landing_termless_test.go` covers the empty-term arm.
- Notes: the tests bite. Dropping the `m.searchForm` branch at model.go:1303-1306 fails "it pins the Sessions page when no sessions are live"; dropping `applySearchLanding` fails the filter-state, filter-value and cursor assertions. Not over-tested: the grouped-mode cases are table-driven rather than copied, and each sub-test asserts only what its criterion names.

CODE QUALITY:
- Project conventions: Followed. Nil-tolerant optional `Deps` field with a one-line doc stating what nil means, Option-based wiring, no new logging in `internal/tui` (the package stays log-free on this path).
- SOLID principles: Good. `applySearchLanding` is one named step beside `applyInitialFilter` / `applyInitialCursor`, on the same shape; `evaluateDefaultPage` keeps the page decision and delegates the landing.
- Complexity: Low. One added branch in the page decision, one guard-clause function.
- Modern idioms: Yes.
- Readability: Good. Comments state why rather than what, and each one holds against the code — the "session-domain by declaration" note at model.go:1304-1305, the "describe the invocation" note at model.go:188-189 (nothing clears the two fields; the `defaultPageEvaluated` latch is what makes the landing single-shot), the exclusivity note at model.go:1318-1319 (true at build.go:177), and the ordering note at model.go:1329-1331 (verified against the bubbles source). No process-artifact references.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes" — needs the unit lane run (`go test ./...` from the project root); reading the suites settles their adequacy, not their outcome.
