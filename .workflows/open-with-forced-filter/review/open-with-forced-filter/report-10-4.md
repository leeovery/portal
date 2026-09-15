TASK: Make the constructor enforce the picker landing's exclusivity (open-with-forced-filter-10-4, tick-623658)

ACCEPTANCE CRITERIA:
- `Build` applies `WithInitialFilter` only when `deps.Search == nil`
- A `Deps` with both fields populated produces a model whose session-list filter value is the search term, in `FilterApplied` state, landed on the Sessions page
- A `Deps` with `InitialFilter` alone is unchanged — `-f` still commits its text and still decides its own landing page
- A `Deps` with `Search` alone is unchanged — term committed, or focused-and-empty for a term-less form
- `cmd/open.go`'s `buildTUIModel` is untouched

STATUS: complete

SPEC CONTEXT: The specification treats `-f/--filter` and the `/term` search sigil as two distinct whole-invocation forms that never compose (§5.1, §5.5, §3.3): a search form declares the session domain and always lands on Sessions with its term committed, while `-f` keeps its own contract including the Projects redirect under a pending command. §5.1's composition rule and `validateOpenArgs` already refuse `portal open /term -f <text>` at the argv layer, so no production line can reach `Build` carrying both. This task closes the same hole one layer down, at the `pickerLanding` union's flattening boundary into `tui.Deps`, so the exclusivity the spec states is enforced by the constructor rather than by the order two `apply*` calls happen to run in.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tui/build.go:177-179` (the gate: `if deps.Search == nil && deps.InitialFilter != ""`); `internal/tui/model.go:1318-1319` (the re-voiced comment above `applySearchLanding`); commit `0fce684d7`
- Notes: The gate is the exact `else` half of the `deps.Search != nil` arm at `build.go:132-134`, as the task prescribed. It holds for a term-less form too: `WithSearchForm` (`internal/tui/model.go:565-570`) sets `searchForm = true` regardless of term, and the new gate keys on `deps.Search == nil` rather than on the term, so a `Search: &SearchForm{Term: ""}` beside an `InitialFilter` still lands focused-and-empty rather than leaking the `-f` text into the input — the fourth criterion's harder half. The comment now states what `Build` guarantees ("Build wires no initial filter alongside a search form, so exactly one of the two applies") rather than what a caller is trusted to do, and it holds true against the code. `cmd/open.go:595-599`'s `buildTUIModel` if/else and the `pickerLanding` union (`cmd/open_search.go:51-58`) are untouched — the commit's stat is three files, `build.go`, `model.go` and `search_landing_test.go`, with `cmd/` absent entirely. `internal/tui/build.go:178` is the only non-test caller of `WithInitialFilter` in the tree, so the constructor is the whole boundary and nothing routes around it. `internal/capture`'s fixture `Deps` (`internal/capture/fixtures.go:75-110`) populates `Search` and never `InitialFilter`, so the capture harness the task named as the motivating hypothetical caller is unaffected by the precedence.

TESTS:
- Status: Adequate
- Coverage: `internal/tui/search_landing_test.go:214-245` adds `TestSearchFormPrecedenceOverInitialFilter` with the prescribed sub-test name verbatim. It pins the criterion at two levels: the constructor's own decision (`m.initialFilter == ""` read straight off the built model at :229-231, before any ingestion — this is what would fail if the gate at build.go:177 were reverted, and it fails independently of anything the model does later) and the landed outcome (Sessions page, `list.FilterApplied`, filter value `"myapp"` not `"other"`, at :235-243). The unchanged-route criteria are carried by the existing cases left untouched: `-f` alone at :159-183 (page, state, value, `initialFilter` consumed, project list empty) and the command-pending Projects redirect at :185-211; `Search` alone by `TestSearchFormLanding` (:42-157) and `TestSearchFormLanding_TermlessForm` (`internal/tui/search_landing_termless_test.go`), neither of which the commit touched.
- Notes: Not over-tested — one sub-test, four assertions, each a distinct property named by a criterion, and no second case restating the same thing through a different mode. The constructor-level assertion is the one that earns its place: without it the test would pass on the pre-change tree too, since `applySearchLanding` already overwrote the filter text by statement order. Would the test fail if the feature broke? Yes — reverting build.go:177 to `if deps.InitialFilter != ""` fails the :229 assertion.

CODE QUALITY:
- Project conventions: Followed — the change is one conjunct in an existing option-assembly block, in the same shape as the surrounding `deps.X != nil` gates; the test is `package tui` beside the `searchLanding`/`ingestLanding` helpers it reuses, uses the package's `t.Run` naming voice, and adds no helper.
- SOLID principles: Good — the invariant moves to the type's own constructor, which is where the `cmd` union is flattened; nothing else gained a responsibility.
- Complexity: Low — one added conjunct.
- Modern idioms: Yes.
- Readability: Good — the re-voiced `applySearchLanding` comment names `Build` as the guarantor, so a reader of `evaluateDefaultPage` (`internal/tui/model.go:1313-1314`) can see why the two `apply*` calls' order is no longer load-bearing.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
