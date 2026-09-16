TASK: open-with-forced-filter-13-1 — Close the containment filter's map-key collision on two distinct rows

ACCEPTANCE CRITERIA:
1. With that pair of sessions in the source, the term `api ~` ranks neither row — the row whose name contains the run is not surfaced on a verdict borrowed from the other, and the row neither of whose fields contains it is not surfaced at all.
2. A key whose entries all match the term ranks every target carrying it, so a collision whose entries agree suppresses nothing.
3. Every row of a session listed under more than one tag still ranks, and a header row's empty filter value still ranks nothing.
4. A target the source does not hold still ranks nothing, ranks still index the targets the pass was handed, and `list.DefaultFilter` is still reached on exactly one condition.
5. `SessionItem.FilterValue()` and `HeaderItem.FilterValue()` are byte-unchanged, and no file outside `internal/tui/search_filter.go` and `internal/tui/search_filter_test.go` is edited.
6. `go test ./internal/tui` is green, `go test -race ./internal/tui -run TestContainmentFilterIsRaceFreeAgainstAConcurrentSet` is green, and `go test ./...` is green.

STATUS: issues_found

SPEC CONTEXT:
The specification's §4.4 (and the 2026-09-16 corrigendum at `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md:500`) names this exact carve-out: "A matching session is withheld where two sessions render to one filter value… Where the sessions behind one target disagree about the term, every row behind that target is withheld, so the narrowed list is a subset of the K sessions §3.2 counts" (spec:180). The sanctioned failure direction is omission; the forbidden one is surfacing a row neither of whose fields contains the term, which is what §4.3's field separation exists to prevent (`internal/resolver/search.go` — `MatchesSearchTerm` tests `SearchFields` separately and never joins them). The corrigendum points at this task's commit (`2fe01e014`) as the settlement, so the spec text and the delivered code agree.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tui/search_filter.go:28-31` (`entries map[string][]searchEntry`), `:35-51` (`set` appends a distinct `searchEntry{name, dir}` per key, `slices.Contains` collapsing an identical pair), `:55-59` (`current` unchanged in shape, retyped), `:74-88` (`containmentFilter` ranks only on `len(held) > 0 && everyEntryMatches(held, query)`), `:92-99` (`everyEntryMatches`).
- Notes:
  - Criterion 1 holds by construction and by the fixture: a session named `api ~/Code/api` with no recorded directory yields `SearchFields` → `["api ~/Code/api"]`, and `api` at `<home>/Code/api` yields `["api", "~/Code/api"]` (`internal/resolver/path.go:94-106` abbreviates against `$HOME`), which `SessionItem.FilterValue()` joins to the same `"api ~/Code/api"`. Under `api ~` the first matches and the second does not, so the key's entries disagree and nothing ranks.
  - Criterion 2: `everyEntryMatches` is guarded by `len(held) > 0`, so the vacuous-truth trap (an empty entry list ranking everything) cannot fire; a key whose entries all match ranks every target carrying it.
  - Criterion 3: the multi-tag repeat contributes one entry (identical field pair, deduped at `:44`), and both its rows still rank; a `HeaderItem`'s `""` filter value is never written by `set` (the `SessionItem` type assertion at `:38-41` skips it) so it resolves to a zero-length slice and ranks nothing.
  - Criterion 4: `entries[target]` on an unheld key (and on the nil map a never-`set` source hands back) is a nil slice → skipped; ranks are `list.Rank{Index: i}` over the `targets` slice the pass was handed (`:81-84`); `query != term` at `:76` remains the sole route to `list.DefaultFilter`.
  - Criterion 5: `git show --stat 2fe01e014` touches exactly `internal/tui/search_filter.go` and `internal/tui/search_filter_test.go`; `git diff 2fe01e014 HEAD` over those two paths is empty, so the reviewed state is the task's state. `SessionItem.FilterValue()` (`internal/tui/session_item.go:87-89`) and `HeaderItem.FilterValue()` (`:100`) are untouched, so the grouped-view flatten-on-filter pin is undisturbed.
  - The race design is unchanged and still sound by reading: `set` builds a fresh map and swaps it under the write hold, and nothing mutates a published map or its slices afterwards, so the map `current()` hands back stays safe to read unheld.

TESTS:
- Status: Adequate
- Coverage: `internal/tui/search_filter_test.go:190-234` adds `collidingSessions` plus `TestContainmentFilterOnTwoSessionsSharingOneFilterValue` with both prescribed subtests. The disagreement case runs **both slice orders** (`:203-206`), which is what makes it a genuine regression guard: the superseded last-wins map ranked nothing for the `{named, recorded}` order (the surviving entry was the non-matching one) and would only fail on `{recorded, named}`. The subtest also asserts its own setup invariant — that the two rows really do produce one target (`:210-212`) — so the fixture cannot silently stop colliding if `AbbreviateHome` or `SearchFields` changes.
- Notes: the five existing pins named in the plan are byte-unchanged in the diff (stale-generation ranking, multi-tag rows, unheld target, header row, race). No redundancy: the agree-case subtest is the only cover for criterion 2, the disagree-case the only cover for criterion 1. End-to-end field separation stays covered outside this file by `TestSearchContainmentMatchesASpacedNameOnItsOwnFields` (`internal/tui/search_containment_test.go:161-172`), which still ranks nothing under the new rule (single entry, neither field containing the spanning term).

CODE QUALITY:
- Project conventions: Followed — stdlib-only test, plain `t.Run("it …")` subtests matching the file's style, no `t.Parallel()`, helper takes `*testing.T` and calls `t.Helper()`.
- SOLID principles: Good — `everyEntryMatches` is a single-purpose unexported predicate and the source keeps its one accessor.
- Complexity: Low. The `slices.Contains` dedupe is O(entries-under-one-key) per row, which is one on an ordinary install and the tag count for a repeated session — no measurable cost.
- Modern idioms: Yes (`slices.Contains`, map-of-slice append with the nil zero value doing the work, the `if held := …;` init form).
- Readability: Good. One inaccuracy in the comments — see FINDINGS.
- Issues: none beyond the finding below.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/tui/search_filter.go:68-70 — the doc comment states as an unqualified guarantee that "a pass running behind, or one answering for a target two distinct sessions produced, can only omit a row, never surface one the term does not match". The second half of that is true within one generation but not across a stale one that carries a colliding key: the withholding rule is applied to the entries the *source* holds, not to the session the *target* came from. Narrow the clause to the generation it holds for — e.g. "…so within one generation a colliding target can only omit a row; a pass running behind can still answer for a target a since-departed session produced" — rather than restating an absolute. — FAILS: the code falsifies the claim in the compound case, so a maintainer reading it as the file's contract would conclude no residue remains. Concretely: the source holds a generation containing only `{name: "api ~/Code/api", dir: ""}` while the pass is handed targets from a generation containing only `{name: "api", dir: "<home>/Code/api"}`; both produce the target `"api ~/Code/api"`, so under the term `api ~` the held entry matches (`:82`) and the live row ranks — surfacing a row neither of whose fields contains the term, which is exactly the outcome `resolver.MatchesSearchTerm`'s field separation and this task's withholding rule exist to prevent. The remedy is comment text only: the code shape is the one the plan fixed (returning to a positional slice would give up the overlapping-rebuild property task 12-2 installed), so nothing here is blocking.

UNSETTLED:
- "`go test ./internal/tui` is green, `go test -race ./internal/tui -run TestContainmentFilterIsRaceFreeAgainstAConcurrentSet` is green, and `go test ./...` is green." — settled only by running those three commands from the project root; reading confirms the package compiles by inspection (the `slices` import is added and used at `:44`, `resolver` still used at `:94`, `searchEntry` is comparable so `slices.Contains` type-checks) and that the publish-and-swap design is race-free, but greenness and the race detector's verdict are measurements.
