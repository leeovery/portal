TASK: Make pickerLanding carry the search form itself (tick-251123 / open-with-forced-filter-9-2)

ACCEPTANCE CRITERIA:
- `pickerLanding` has exactly two fields — `filter string` and `search *tui.SearchForm` — with no bool discriminator and no free closure.
- `buildTUIModel` assigns `landing.search` straight through to `deps.Search` and constructs no `tui.SearchForm`.
- A search landing carries its term in `search.Term` with `filter` empty; a `-f` landing carries its text in `filter` with `search` nil.
- `deps.Search` and `deps.InitialFilter` receive what they receive today on all five routes: bare sigil, warm term, deferred term, `-f`, no-args picker.
- `cmd` takes no new import.
- Every converted assertion asserts the same fact it asserted before — the term changes field, not meaning; no case is dropped and none is added.

STATUS: complete

SPEC CONTEXT: The spec's §3.2/§3.3 fix what the landing must be: a term that opens the picker lands with the filter text *committed* (state applied, cursor re-anchored), exactly as `-f` lands; a term-less `/` sigil is the deliberate exception and lands with the filter input *focused and empty*. §3.4 adds the cold route: on a concurrent bootstrap the count cannot be taken up front, so the classification travels into the picker and is taken once bootstrap completes. This task is the internal-shape change behind those three landings — the landing value the `cmd` layer hands `tui.Build` — and is behaviour-preserving by intent.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/open_search.go:51-58` — `pickerLanding{filter string; search *tui.SearchForm}`; the `search bool` and the `decide func() (string, error)` field (with its comment) are gone, and the doc comment now states the nil-means-no-search rule.
  - `cmd/open_search.go:223` — term-less form: `pickerLanding{search: &tui.SearchForm{}}`.
  - `cmd/open_search.go:227` — term form: `pickerLanding{search: &tui.SearchForm{Term: term}}`; `filter` left empty.
  - `cmd/open_search.go:229-232` — deferred route sets `landing.search.Decide = decide`.
  - `cmd/open.go:594-598` — `if landing.search != nil { deps.Search = landing.search } else { deps.InitialFilter = landing.filter }`; no `tui.SearchForm` is constructed here.
  - `cmd/testhelpers_test.go:240-256` — `landingShape` gains `term`; `search` becomes `l.search != nil`, `decided` becomes `l.search.Decide != nil`.
- Notes:
  - All four production construction sites are converted and none was missed: `cmd/open_search.go:223`, `:227` (search) and `cmd/open.go:186` (`-f`), `:207` (bare open). `landing.filter` has exactly one reader in the whole package (`cmd/open.go:597`), so moving the term off it can reach nothing else; `openTUI` (`cmd/open.go:631-738`) only passes the landing through to `buildTUIModel`.
  - Route-by-route value equivalence holds. Before: `search==true` produced `&tui.SearchForm{Term: landing.filter, Decide: landing.decide}` with `InitialFilter` unset. After: bare sigil → `&SearchForm{}`; warm term → `&SearchForm{Term: term}` with `Decide` nil (the warm route spends its closure up front and never attached it before either); deferred term → the same plus `Decide`; `-f` → `InitialFilter = filter`, `Search` nil; no-args → both zero. `internal/tui/build.go:132-133,177` consumes `deps.Search` unchanged.
  - No new import: `cmd/open_search.go` already imported `internal/tui` before the change (verified against `1b61c15c9^`), and `cmd/open.go` still uses the package elsewhere.
  - The combination `{filter: "x", search: &SearchForm{...}}` is still constructible, with `filter` then silently unread. Deliberate and harmless rather than a residual defect: the task's shape keeps `filter` beside `search`, no production site sets both, the `else` arm makes the drop total, and `internal/tui/build.go:177` guards the same pairing a second time. Not raised as a finding.

TESTS:
- Status: Adequate
- Coverage: `cmd/open_picker_landing_test.go` carries all five prescribed cases, each named as the plan names it: search form with term (`InitialFilter()` empty, landed `FilterApplied` on "port"), term-less search form (landed `Filtering`, empty value), deferred decision closure (drives the loading gates and asserts `SearchAttached()` plus `Selected() == "portal-a1b2"`), `-f` text (landed `FilterApplied` with `InitialFilter() == "blog"`), and a bare open (`Unfiltered`, empty). The landed-state assertions are the spec's §3.3 committed-vs-focused distinction, so these are behaviour assertions rather than field readbacks.
- Notes:
  - The deferred subtest would genuinely fail if the wiring broke: `driveLoadingGates` (`cmd/testhelpers_test.go:262-287`) only runs a command dispatched while the page is still loading, so a `Decide` that never reached the model dispatches nothing, and both `SearchAttached()` and `Selected()` fail.
  - Assertion conversion is complete and count-preserving. `cmd/open_search_test.go` held five `landingShape` assertions before and holds five now (`:198`, `:310`, `:556`, `:573`, `:604`); the three carrying a term moved `filter: "port"` → `term: "port"`, the two term-less ones are untouched. `cmd/open_search_deferred_test.go` held three and holds three (`:63`, `:158`, `:183`), two converted. The `-f` assertions in `cmd/open_test.go` (`:2185`, `:2526`, `:2552`) and the three zero-value ones (`:2390`, `:2437`, `:2494`) are unchanged, which is correct — those landings never carried a search form. Each converted assertion is now marginally stronger, not weaker: `landingShape.filter` defaults to `""`, so it also pins "a search landing leaves `filter` empty".
  - Mild overlap with `TestBuildTUIModel`'s existing "filter creates model with initial filter" / "no command and no filter" subtests (`cmd/open_test.go:1663-1670`, `:1616-1637`) on the `InitialFilter()` readback alone. Not over-testing: the new cases add the landed filter *state*, which those older cases never drove.
- Not run: test adequacy was judged by reading, per this review's remit.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()` (cmd stages package-level seams); the seam staging in the converted suites is untouched; the new test file is unit-lane and touches no tmux server, no daemon and no binary — `defaultTestTUIConfig` wires stubs and Flat mode performs no pane reads.
- SOLID principles: Good. The change removes a representation the type could not keep honest and lets the domain type (`tui.SearchForm`) carry term and decision as one value; `cmd` stops unpacking and repacking a type one layer down.
- Complexity: Low. Net effect on `buildTUIModel` is a pointer test replacing a bool test and a literal construction.
- Modern idioms: Yes. Nil pointer as the "absent" discriminator matches `tui.Deps.Search`'s own documented convention (`internal/tui/build.go:52-53`).
- Readability: Good. The `pickerLanding` and `landingShape` comments both state the new rule accurately — `landingShape`'s note that "a pickerLanding compares by search-form pointer identity" is true of the new struct (it is comparable now that the func field is gone) and explains why the reduction helper survives.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
