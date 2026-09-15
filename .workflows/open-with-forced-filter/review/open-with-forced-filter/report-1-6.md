TASK: Open an empty focused filter for the term-less search form (tick-6d0e49 / open-with-forced-filter-1-6)

ACCEPTANCE CRITERIA:
- A model built with `Deps.Search = &SearchForm{Term: ""}` lands on `PageSessions` with `FilterState() == list.Filtering` and an empty `FilterValue()`
- Every live session is present in `VisibleItems()` under that empty focused filter — in Flat, By Project and By Tag
- The cursor is on a `SessionItem`, never a `HeaderItem`, in the grouped modes where headings are still present under the empty query
- `portal open /` exits through the picker rather than returning a usage error; `-f ""` is still refused
- The first typed character narrows the list, and `s` typed while the input is focused is a literal filter character rather than a grouping switch
- With zero live sessions the landing does not panic: the page is Sessions, the filter is focused and empty, and the body carries no rows
- `go test ./...` passes

STATUS: complete

SPEC CONTEXT:
§2.5 sets the bare slash: `x /` opens the picker with the filter open and empty, the cursor in it, ready to type — not an error, not a mint, and `-f`'s empty-value refusal explicitly does not transfer. §3.2 states a term-less sigil takes no count at all (no session read, no K). §3.3 makes the committed-filter landing the rule for a term-bearing form and names the bare slash as the deliberate exception (focused and empty). §4.4 states the term-less form supplies nothing for the containment test to hold, so the picker's own fuzzy rule applies from the first keystroke. §6.3 scopes the directory column to "the picker session a sigil opened — the term-less form of §2.5 included".

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/model.go:1318-1335` — `applySearchLanding`: the guard is now `!m.searchForm` alone; `state` is `list.FilterApplied` for a term and `list.Filtering` for an empty term; `SetFilterText(m.searchTerm)` runs before `SetFilterState(state)`, then `ensureSessionRowSelected()`.
  - `internal/tui/model.go:1303-1306` — a search form lands on `PageSessions` whatever the term; `1313-1315` sequences `applyInitialFilter` → `applySearchLanding` → `applyInitialCursor`.
  - `internal/tui/model.go:1269-1273` — `ensureSessionRowSelected` steps one row off a `HeaderItem`; nil-safe (a nil `SelectedItem()` fails the type assertion and no cursor move is attempted), which is what carries the zero-session case.
  - `cmd/open_search.go:221-224` — `runSearchForm` returns the picker for `term == ""` before any session source is built, so the term-less form reads nothing.
  - `cmd/open.go:242-243` — the `-f ""` refusal is untouched.
  - `internal/tui/search_filter.go:85-91` — `installSearchFilter` still no-ops for an empty term, so the list keeps `list.DefaultFilter` and the first typed character narrows by the picker's own rule (§4.4).
- Notes:
  - The ordering claim in the comment is correct against the dependency: `list.SetFilterText` (bubbles v2 v2.1.0, `list.go:280-294`) runs the filter pass and assigns `m.filteredItems`, and for an empty value `filterItems` (`list.go:1251-1255`) returns every item; `VisibleItems` (`list.go:459-464`) serves `filteredItems` in every state but `Unfiltered`. A bare `SetFilterState(Filtering)` would indeed leave the slice nil and hide every session.
  - The landing converges on the same internal state the picker's own `/` key produces (`list.go:883-894` populates `filteredItems` with all items when the input is empty, then flips to `Filtering` and focuses), so this is the native gesture reached from the shell rather than a bespoke state.
  - The cursor is visible from the first frame despite the `Focus()` blink command being discarded by `SetFilterState`: Portal pins `styles.Cursor.Blink = false` (`internal/tui/model.go:1058`), which puts the virtual cursor in `CursorStatic`, and `cursor.Focus` clears `IsBlinked` so the block renders immediately.
  - Render layer untouched, as the task required, and the two claims it rests on hold: `applySectionHeader` returns the list view unmodified while `Filtering` (`internal/tui/model.go:3539-3541`), so bubbles renders its own `/ ` input on the title row, and `renderSessionsFooterForFilterState` selects the focused-filter footer for `Filtering` (`internal/tui/model.go:3452-3454`).
  - Zero sessions: `sessionListEmpty()` requires `Unfiltered` and `sessionListNoMatches()` requires a non-empty query, so neither body claims the frame and the list renders with no rows — exactly as the task's edge-case note describes, and the spec says nothing about copy here.
  - The planned two-branch shape was folded into one shared call sequence with a `state` variable. That is a simplification over the plan's wording, not a loss — the term path already made the same three calls.
  - §6.3's "term-less form included" holds for free: the delegate takes `ShowDir: m.searchForm` (`internal/tui/model.go:938`), not a non-empty term.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/tui/search_landing_termless_test.go:13-134` covers all seven behavioural criteria: page + `Filtering` + empty value (20-32), every session visible under the empty filter (34-43), the same in By Project and By Tag (52-63), the cursor on a `SessionItem` with a self-checking headers-exist precondition (65-84), narrowing on the first typed character (87-101), `s` as a literal filter character with the mode asserted unswitched (103-114), and the zero-session landing plus a `View()` call (116-133).
  - `cmd/open_search_test.go:302-316` — `portal open /` reaches the picker seam with a search landing and no error (`executeOpen` at 180-188 fails the test on any returned error), and no other open branch runs.
  - `cmd/open_search_test.go:593-610` — the term-less form takes no count (`listCalls`/`currentCalls` both 0), which pins §3.2 at the cmd seam.
  - `cmd/open_picker_landing_test.go:49-59` — the same landing through the real `buildTUIModel` wiring, so the `Deps.Search` → `Filtering` path is covered end-to-end rather than only in-package.
  - `cmd/open_test.go:2342-2367` and `cmd/open_search_test.go:823-833` keep the `-f ""` refusal asserted.
  - `internal/tui/search_containment_test.go:352-370` pins §4.4 for this landing: no containment filter is installed for a term-less form, so `prt` widens to the fuzzy-only survivor.
- Notes:
  - The tests fail if the feature breaks in either of its two failure modes: dropping `SetFilterText` leaves `filteredItems` nil and the "keeps every live session visible" subtest fails; flipping the two calls ends in `FilterApplied` (because `SetFilterText` assigns that state last) and the first subtest fails.
  - `typeKeys`/`drainFilterCmd` (`internal/tui/filtering_reskin_test.go:33-53`) drain a single `FilterMatchesMsg`, which is sound here: with the cursor in `CursorStatic` the textinput returns no blink command, so `tea.Batch` collapses to the filter command alone (`compactCmds`, bubbletea v2 `commands.go:15-17`).
  - The superseded task 1-3 subtest ("it applies no filter for an empty search term", asserting `Unfiltered`) was removed rather than left contradicting the new expectation; no other test in the tree asserts `Unfiltered` for a search landing.
  - Not over-tested: each subtest names a distinct risk; the `s` subtest is not redundant with the narrowing one because `s` is the grouping-switch binding and exercises the `SettingFilter` guard (`internal/tui/model.go:2547-2549`).

CODE QUALITY:
- Project conventions: Followed — no `t.Parallel()`, cmd seams staged through `withFuncSeam`/`withOpenDeps`, subtest naming matches the package's "it …" style, no new logging or state.
- SOLID principles: Good — the landing stays a single private method on the model; the cmd layer decides nothing about filter state.
- Complexity: Low — one branch and three calls.
- Modern idioms: Yes.
- Readability: Good — the two comments say why (the ordering is load-bearing; the term-less form opens ready to type) rather than restating the calls.
- Issues: None. Comments verified against the dependency's behaviour and against `build.go:177` (the "Build wires no initial filter alongside a search form" claim holds — `WithInitialFilter` is applied only when `deps.Search == nil`).

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes" — requires running the unit lane (`go test ./...` from the project root); reading settles the behaviour of each assertion but not the suite's exit status.
