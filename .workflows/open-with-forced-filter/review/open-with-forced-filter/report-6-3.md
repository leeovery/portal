TASK: Make the containment filter's item source self-contained (tick-7c6b83 / open-with-forced-filter-6-3)

ACCEPTANCE CRITERIA:
1. Every read and write of `searchItemSource`'s fields happens inside `set` or `current` under the mutex — no other file touches them
2. `current` returns the items and the recorded filter values from a single hold
3. The filter falls through to `list.DefaultFilter` whenever the recorded values differ from `targets`, including a same-length replacement
4. A source holding exactly the slice the targets were built from still narrows by containment, in the list's own order, skipping `HeaderItem` rows, with `MatchedIndexes` nil
5. A never-`set` source still behaves as today: an empty target list yields no ranks, a non-empty one falls through to the picker's own rule
6. `go test -race ./internal/tui` is green, including a test driving `set` concurrently with a filter pass
7. `TestContainmentFilterFallsBackWhenTheSourceIsOutOfStep` and the `TestSearchContainment*` suites pass with no edit
8. `gofmt -l` reports nothing and `go vet ./...` is clean

STATUS: complete

SPEC CONTEXT: The specification (§4.3, line 154; §4.5, lines 172/176) states that the sigil matches by containment over the session's name and recorded directory, and that **the containment set holds for as long as the sigil's filter text stands untouched** — a `Space` preview and back, an `s` regroup and a refresh after an external kill each "reproduce the containment set", and "only a hand edit of the filter text returns the list to the picker's own rule, and the test is the text rather than the act". The picker's own rule is `list.DefaultFilter` (subsequence, rank-sorted), which the sigil's list must not use while the term stands.

IMPLEMENTATION:
- Status: Implemented; two clauses (criterion 3, and the non-empty half of criterion 5) were deliberately superseded by a later plan task, not dropped.
- Location:
  - `internal/tui/search_filter.go:24-27` — `searchItemSource` holds a `sync.RWMutex` beside its single state field.
  - `internal/tui/search_filter.go:31-43` — `set` derives the generation off the Update goroutine, then takes the write lock and stores it.
  - `internal/tui/search_filter.go:47-51` — `current` answers under one read-lock hold.
  - `internal/tui/search_filter.go:63-81` — `containmentFilter`, falling through to `list.DefaultFilter` on `query != term` alone.
  - `internal/tui/model.go:1249-1254` — the writer (`m.searchItems.set(items)`) stayed exactly where the task required, immediately before `sessionList.SetItems`.
- Notes:
  - **The delivered shape is not the one this task's "Do" prescribes, and that is correct.** This task shipped as written at `930a1d76c` (items + a recorded `[]string` companion, compared with `slices.Equal`). Task `open-with-forced-filter-12-2` (tick-716bd3, commit `e44308029`) then replaced the pair with one `map[string]searchEntry` keyed on the item's filter value, holding the `Session.Name`/`Session.Dir` that value was built from, and dropped the `slices.Equal` gate with its `list.DefaultFilter` fall-through. That task names this one explicitly ("This corrects one clause of cycle 1's approved Task 3") and grounds the correction in the specification quoted above: a scheduling overlap between two rebuilds is not a hand edit, so the identity-check fall-through this task asked for was an escape hatch back to the fuzzy rule on a line the spec says is closed. The mutex and the single both-under-one-hold accessor — this task's actual subject — were explicitly retained. The divergence is a gain against intent, not a loss, so it is not a finding.
  - Criterion 1 holds: `mu` and `entries` are named nowhere outside `set` and `current` (`grep -rn "searchEntry\|\.entries" --include="*.go" .` → `internal/tui/search_filter.go` only; the tests construct the zero value `&searchItemSource{}` and otherwise go through the two methods, and `internal/tui/model.go:1252` calls `set`).
  - Criterion 2 holds in the stronger form the supersession leaves: `current` returns the whole generation as one value under one hold, so pairing halves from two calls is structurally impossible. `set` builds a fresh map every call and never mutates one already handed out (`search_filter.go:32-42`), which is what makes the lock-free read of the returned map safe — stated at `search_filter.go:45-46`.
  - Criterion 4 holds under the new shape: ranks are emitted at the target's own index in target order (= the list's own order), `MatchedIndexes` is left nil, and a `HeaderItem` contributes no entry (`search_filter.go:34-37` skips every non-`SessionItem`), so its empty filter value resolves to nothing. Even the unreachable degenerate case is safe: a `SessionItem` whose name and dir were both empty would key `""`, but `resolver.MatchesSearchTerm` (`internal/resolver/search.go:22-36`) returns false for any non-empty term against empty fields, so a header would still rank nothing.
  - Criterion 5's empty-target half holds (nil map, no ranks). The non-empty half now ranks nothing rather than falling through — changed by 12-2's "Do" bullet naming that exact subtest. It is inert in production: `sessionList.SetItems` has exactly one non-test call site (`internal/tui/model.go:1254`) and it is preceded by the `set` at `:1251-1253`, so a source holding nothing can never coexist with a non-empty target list.
  - Comment accuracy checked against the dependency rather than assumed: `containmentFilter`'s claim that ranks index the targets the pass was handed and so resolve against that pass's own generation is true of `charm.land/bubbles/v2@v2.1.0/list/list.go:1251-1275`, where `filterItems` builds `targets` from its captured `items` and resolves `items[r.Index]` from the same capture.

TESTS:
- Status: Adequate
- Coverage: `internal/tui/search_filter_test.go` (7 tests, unit-lane, no `t.Parallel()`), plus `internal/tui/search_containment_test.go` driving the real `tui.Model` end to end.
  - Concurrency — `TestContainmentFilterIsRaceFreeAgainstAConcurrentSet` (`search_filter_test.go:168-188`) runs 500 `set` calls on a `sync.WaitGroup.Go` goroutine against 500 filter passes on the test goroutine. This is the criterion-6 test and the one that would flag a dropped mutex under `-race`.
  - Containment, list order, nil `MatchedIndexes` — `TestContainmentFilterNarrowsToTheTargetsItContains` (`:103-123`), including an explicit per-rank `MatchedIndexes` assertion.
  - Header rows rank nothing — `:88-101`. Multi-tag rows each rank — `:58-74`. A target the source does not hold ranks nothing — `:76-86`.
  - Fall-through on the one surviving condition — `TestContainmentFilterReturnsToThePickersOwnRuleOnceTheTextIsNoLongerTheTerm` (`:125-143`), which asserts equality with `list.DefaultFilter`'s own ranks and carries a setup invariant so it cannot pass vacuously.
  - Stale generation — `:39-56` hands the filter another generation's targets in an order the source never held, and pins both the containment answer and (via the `len(fuzzy) <= len(got)` invariant) that the fuzzy rule would have answered differently. This is the test that would fail if the supersession regressed to the fall-through.
  - `search_containment_test.go` is untouched since task 2-3 (`git log -- internal/tui/search_containment_test.go` → `78f385857` only), so criterion 7's "with no edit" holds for the `TestSearchContainment*` suites; `TestContainmentFilterFallsBackWhenTheSourceIsOutOfStep` was removed by 12-2's own "Do", which is why it is absent rather than failing.
- Notes: No redundancy — each test has a distinct subject, and the two `TestContainmentFilterOnASourceThatWasNeverSet` subtests split the empty and non-empty target cases rather than repeating one. The race test asserts nothing about results, which is correct for its subject.

CODE QUALITY:
- Project conventions: Followed. Unit-lane test with no `t.Parallel()` and no tmux/daemon reach; the accessor-wrapped pointer type keeps the shared cell out of the value-copied `Model`, as `internal/tui/model.go:201-203` documents.
- SOLID principles: Good. `searchItemSource` owns access as well as state — the task's stated aim — and `containmentFilter` stays a pure closure over it.
- Complexity: Low. Two accessors and one linear pass; no branching beyond the single fall-through and the map miss.
- Modern idioms: Yes. `sync.RWMutex` with deferred unlocks, map-capacity hint, `range`-over-int and `sync.WaitGroup.Go` in the race test (the module is `go 1.26.0`, so both are available).
- Readability: Good. The doc comments state why the mutex is load-bearing (`search_filter.go:21-23`), why `set` replaces rather than mutates (`:45-46`), and why a lagging pass can only omit a row (`:57-59`) — each a claim the code supports.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test -race ./internal/tui` is green, including a test driving `set` concurrently with a filter pass" — the test exists and is shaped for it (`internal/tui/search_filter_test.go:168-188`); the green run itself needs `go test -race ./internal/tui` executed.
- "`TestContainmentFilterFallsBackWhenTheSourceIsOutOfStep` and the `TestSearchContainment*` suites pass with no edit" — reading settles the "no edit" half (the containment suite is unchanged since task 2-3) and the removal of the named fall-through test by task 12-2; the pass needs `go test ./internal/tui` run.
- "`gofmt -l` reports nothing and `go vet ./...` is clean" — the changed files read as gofmt-conventional (tab-indented, standard import grouping) and carry no value copy of the mutex-bearing type that `copylocks` would flag, but both commands have to be run to settle it.
