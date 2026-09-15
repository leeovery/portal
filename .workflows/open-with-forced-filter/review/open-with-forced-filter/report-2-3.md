TASK: Narrow a search-opened list by containment while the term stands untouched (tick-c771be / open-with-forced-filter-2-3)

ACCEPTANCE CRITERIA:
- A model built with `Deps.Search = &SearchForm{Term: t}` shows exactly the sessions matching `t` by containment over name and recorded directory; a session the fuzzy rule would have returned on a scattered-letter subsequence is absent
- Surviving rows keep the order the session list already gives them — nothing is re-ranked
- Group headers are absent under a non-empty term in By Project and By Tag, and the containment set is identical in all three grouping modes
- An `s` regroup, a `Space` preview and back, and a refresh that drops an externally-killed session each re-render the containment set
- Committing any other filter value returns the list to `list.DefaultFilter`; clearing shows every session; committing the identical term again restores containment
- The term-less form is on the picker's own rule from its first keystroke
- A picker opened by `-f`, or with no filter at all, narrows by `list.DefaultFilter` exactly as before
- A session whose name contains a space is matched on its own two fields, never by splitting the joined filter value
- The rebuilt item slice is what the ranks resolve against, so a row can never be matched against another row's fields after a regroup
- `go test ./...` passes

STATUS: complete

SPEC CONTEXT: §4.3–§4.4 fix the sigil's rule as case-folded containment over the two fields tested **separately** (never the joined filter value, since a session name may legally contain a space), and fix the divergence test as the *text*: while the committed filter value is character-identical to the term the sigil supplied, containment stands — through a `Space` preview, an `s` regroup and a refresh — and any other value, a cleared filter included, is the picker's own `list.DefaultFilter`. §4.4 also fixes that containment narrows but never reorders: rows keep whatever order the current grouping mode gives them. §4.2 keeps the matched *fields* shared across all three entry points, so only the rule diverges.

IMPLEMENTATION:
- Status: Implemented (drifted from the plan's prescribed mechanism, deliberately and soundly — see Notes)
- Location:
  - `internal/tui/search_filter.go:24` (`searchItemSource`), `:31` (`set`), `:47` (`current`), `:63` (`containmentFilter`), `:85` (`installSearchFilter`)
  - `internal/tui/model.go:203` (the `searchItems` field), `:895` (`installSearchFilter` at the end of `New`, after the options loop), `:1252` (`m.searchItems.set(items)` immediately before `m.sessionList.SetItems(items)` at `:1254`)
  - Matching rule: `internal/resolver/search.go` — `SearchFields` / `MatchesSearchTerm` (fields tested separately, case-folded, empty term matches nothing)
- Notes:
  - The plan's "Do" prescribed a slice-held source with ranks resolved by index into it; the delivered code holds a `map[filterValue]searchEntry` guarded by an `RWMutex` and resolves each target string to its own session's fields. That shape was reached by the later plan tasks 6-3 (`tick-7c6b83`, the mutex) and 12-2 (`tick-e44308029`, the per-target map), and it satisfies this task's criteria in substance while being strictly safer: `bubbles`' `filterItems` (`bubbles/v2@v2.1.0/list/list.go:1251-1275`) captures `m.items` by value in its `tea.Cmd` and resolves `items[r.Index]` inside that same closure, so ranks always index the generation the pass belongs to, and the map lookup hands that row its own fields however many rebuilds have landed since. Not a loss; no finding.
  - `m.sessionList.SetItems` has exactly one call site in the package (`internal/tui/model.go:1254`, inside `rebuildSessionList`), and the source is re-pointed on the line before it, so no rebuild path can leave the source behind. `newSessionList` has two call sites (`model.go:871` in `New`, `model.go:1065` in `NewModelWithSessions`); neither runs after `installSearchFilter`, so the installed `Filter` cannot be discarded by a later list reconstruction.
  - `installSearchFilter` is a no-op unless `m.searchForm && m.searchTerm != ""`, and `Build` (`internal/tui/build.go:132-133`, `:177`) wires `WithSearchForm` only for a non-nil `Deps.Search` and refuses `InitialFilter` alongside it — so `-f` and the plain picker keep `list.DefaultFilter`, and the term-less form keeps it too.
  - `Model` is copied on every `Update`; the source is a pointer allocated once in `New` and never reassigned (`model.go:203` declaration, `:1252` and `search_filter.go:89` are its only other mentions), so every copy shares it.
  - Headers contribute no entry (`set` skips non-`SessionItem`s) and `HeaderItem.FilterValue()` is `""`, so a header ranks nothing under a non-empty term — the criterion's "headers absent" falls out structurally rather than by a special case.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/tui/search_containment_test.go` (external `package tui_test`, models driven through `tui.Build` with `Deps.Search`) covers every behavioural criterion: the containment set vs. the `-f` fuzzy set over the same scattered-letter fixture (`rust-tools` under `port`); list order preserved against a fixture the fuzzy rule would invert; a directory-only match; a spaced name under a term contained only in the joined value; header absence plus set-identity across Flat / By Project / By Tag; reproduction after an `s` regroup (both hops), a `Space` preview and `Esc` back, and a `SessionsMsg` refresh dropping a session; edit-away / edit-back / clear; and the term-less form on the fuzzy rule from its first keystroke.
  - `internal/tui/search_filter_test.go` (internal `package tui`) covers the rule directly: ranks in the targets' own order with nil `MatchedIndexes`, a stale generation's targets ranked by containment, every row of a session repeated across By-Tag groups, a target the source does not hold, a header's empty filter value, fallback to `list.DefaultFilter` on any other text, a never-`set` source (empty and non-empty targets), and a concurrent `set` against a running filter pass.
  - The setup invariants are asserted rather than assumed — the scattered-letter fixtures fail loudly if the fuzzy rule ever stops ranking them, so the tests cannot pass by the fixture silently ceasing to discriminate.
- Notes: Not over-tested — the two files split cleanly between "what the user sees" and "what the rule returns", with no duplicated assertions between them. The one gap reading cannot close is the whole-suite run (recorded under UNSETTLED).

CODE QUALITY:
- Project conventions: Followed. Small pointer-held seam, accessor-only access to the guarded fields, no test-only export added to production (`SessionListVisibleItems` / `SetSessionListFilter` predate this task at `model.go:398` / `:419`), no `t.Parallel()`, external test package for the behavioural suite.
- SOLID principles: Good. The rule is a closure over a term and a source; the matching predicate stays in `internal/resolver` (single declaration of the field set and the containment rule, shared with the count that decides the shortcut), and `internal/tui` holds only the plumbing.
- Complexity: Low. `containmentFilter` is one guard, one map read and one loop; `set`/`current` are three lines each.
- Modern idioms: Yes (`sync.RWMutex` with a replace-don't-mutate map so readers need no hold; `slices`/`range N`/`WaitGroup.Go` in the tests, all legal under `go 1.26.0`).
- Readability: Good. Every non-obvious choice carries its reason — why the source is a pointer, why the mutex is load-bearing, why the re-point precedes `SetItems`, why `MatchedIndexes` is nil. All four claims check out against the code and against the `bubbles` source they describe.
- Issues: None. `gofmt -l` reports nothing over the four touched files.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes" — the unit lane was not executed (this review reads; it does not run). Settle it with `go test ./...`, and `go test -race ./internal/tui` for `TestContainmentFilterIsRaceFreeAgainstAConcurrentSet`, which only measures anything under `-race`.
