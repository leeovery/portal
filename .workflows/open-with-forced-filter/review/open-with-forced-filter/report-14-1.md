TASK: open-with-forced-filter-14-1 — Close the containment filter's cross-generation borrowing of a colliding target's verdict

ACCEPTANCE CRITERIA:
1. With the source holding only `{name: "api ~/Code/api", dir: ""}` and the pass handed targets built only from `{name: "api", dir: "<home>/Code/api"}`, the term `api ~` ranks no row; the same holds with the two generations exchanged.
2. A key whose accumulated pairs all contain the term still ranks every target carrying it, and every non-colliding key ranks exactly as it does today.
3. A pass handed the targets of a generation the source has since rebuilt over still ranks them by containment at the targets' own indexes (12-2's property preserved, not reverted).
4. Recording the same generation N times leaves each key holding one entry per distinct field pair.
5. Every row of a multi-tag session still ranks; a header row and a target no generation has produced rank nothing; ranks index the targets the pass was handed; `list.DefaultFilter` reached on exactly one condition; `MatchedIndexes` nil on every rank.
6. `set` still builds a fresh map and swaps it under the write hold; no map or slice `current()` handed out is mutated afterwards.
7. The type comment and `containmentFilter`'s comment state the rule the code holds.
8. `SessionItem.FilterValue()` / `HeaderItem.FilterValue()` byte-unchanged; no file outside `internal/tui/search_filter.go` and `internal/tui/search_filter_test.go` edited.
9. `go test ./internal/tui`, the `-race` run of the race pin, `go test ./...`, `gofmt -l .`, `go vet ./...` and `golangci-lint run` all clean.

STATUS: complete

SPEC CONTEXT:
`.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §4.3 separates the searched fields (name, home-abbreviated recorded directory) and never joins them, so a run spanning the two is not a match; §4.4 fixes the sigil's rule as containment for as long as the filter text stands, and carries the collision carve-out at :180 ("Where the sessions behind one target disagree about the term, every row behind that target is withheld … ranking both would surface a session neither of whose fields contains the term"). :182 is the text this task is written against: "The disagreement is judged over the picker run, not over the list as it stands. The test is taken across every distinct pair of fields any generation of the list has produced for that target since the picker opened — the record accumulates for the run's life and nothing is ever evicted from it", with the killed/renamed-collider residual recorded as accepted and eviction explicitly refused. The corrigendum at :504 records this task as the amendment that made the record cumulative. Omission is the sanctioned failure direction; surfacing a non-matching row is the forbidden one.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tui/search_filter.go:40-61` (`set`, now cumulative), `:18-32` (restated type comment), `:71-84` (restated `containmentFilter` guarantee); task commit `ff1079d1b` (2 files), comment follow-ups `1719dfe6a` and `e85f89e6a` (same file).
- Notes:
  - `set` takes the write hold for the whole call (`:41-42`), builds a fresh map sized `len(s.entries)+len(items)` (`:45`), seeds it with `slices.Clone` of each held slice (`:46-48`), folds this generation's `SessionItem` rows in through the existing `slices.Contains` dedupe (`:49-59`), then assigns (`:60`). `s.entries` is read directly inside the write hold rather than through `current()` — correct, since `sync.RWMutex` is not upgradable/re-entrant (the `golang-concurrency` skill states the same rule, SKILL.md:77).
  - No published slice is ever appended into: every value in the fresh map is either a fresh `slices.Clone` allocation or a slice first created by `append` to a nil entry in this call, so a reader holding the previous map (which `set` replaces rather than mutates) sees a stable array. `current()` (`:65-69`) and `containmentFilter` (`:85-99`) only read.
  - The ranking rule is untouched: `len(held) > 0 && everyEntryMatches(held, query)` at `:93`, ranks indexing the pass's own targets at `:94`, `query != term` the single route to `list.DefaultFilter` at `:87-89`, `MatchedIndexes` never set. `current`, `everyEntryMatches`, `installSearchFilter` unedited.
  - The comments' load-bearing claim checks out against the code: `searchItemSource` is constructed exactly once per picker, in `installSearchFilter` (`:118`) called from `New` (`internal/tui/model.go:895`); the session list is created empty (`newSessionList(nil)`, `model.go:871`, `:827`); the only `SetItems` on `m.sessionList` is `model.go:1254`, preceded by `m.searchItems.set(items)` at `:1251-1252`; nothing calls `InsertItem`/`RemoveItem` on it (grep over `internal/tui` non-test sources). So every generation a filter pass can be handed is one the source recorded first.
  - Accumulation cannot create a spurious collision from grouping: `buildByProject`/`buildByTag` carry `Session` through unchanged (`internal/tui/grouping.go:46-50`, `:67-72`, `:98-111`), and derived directories live in `m.derivedDirs`, never in `Session.Dir` — so a session's filter value is identical in every mode and only a genuine two-session collision ever puts two pairs under one key.
  - Retention cost is the one the spec sanctions (:182) and is bounded by distinct field pairs seen in one picker run; a refresh that drops a killed session is unaffected because the departed session's key is no longer among the targets (`internal/tui/search_containment_test.go:293-309` still describes the delivered behaviour).

TESTS:
- Status: Adequate
- Coverage: `internal/tui/search_filter_test.go:236-295` adds the four named tests under `TestContainmentFilterAcrossGenerationsOfOneCollidingFilterValue`, reusing `collidingSessions` (`:192-198`) and driving both assignments of the two sessions across held/handed generations (`:237-249`, `:251-263`), the union-matching case (`:265-277`) and the repeated-record dedupe (`:279-294`). No existing test in the file was edited (the task commit's diff to the test file is additive only). The six existing `TestContainmentFilter*` pins carry criteria 3 and 5: stale-generation ranking at `:39-56`, multi-tag rows at `:58-74`, unheld target at `:76-86`, header row at `:88-101`, ordering + nil `MatchedIndexes` at `:103-123`, `DefaultFilter` fall-through at `:125-143`, never-set source at `:145-166`, race pin at `:168-188`.
- Notes:
  - The two withholding tests are genuine regression pins: under the pre-change last-generation-wins `set`, the held generation in each is the one whose pair contains `api ~`, so both would rank a row and fail.
  - The dedupe test asserts `len(entries) == 1` and two entries under the shared key, which doubles as the collision setup invariant for its siblings; the sibling test at `:200-220` additionally asserts `targets[0] == targets[1]` explicitly.
  - Not over-tested: `:265-277` overlaps `:222-233` in outcome but differs in construction (two separate `set` calls versus one generation), which is the property this task installed.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`; `t.Setenv` inside the subtests; no new dependency; `slices`/`sync` already imported and still used.
- SOLID principles: Good — the source keeps its single responsibility (record field pairs per filter value) and the ranking rule was not touched.
- Complexity: Low — one added clone loop; `set` remains a single linear pass with an early `continue` for non-session rows.
- Modern idioms: Yes — `slices.Clone`/`slices.Contains`, `for value, held := range`. `maps.Copy` would be wrong here (it would share slice headers), so the explicit clone loop is the correct shape.
- Readability: Good — the in-body comment at `:43-44` states why the clone exists rather than restating the code.
- Comment accuracy: The type comment (`:18-32`) and `containmentFilter`'s guarantee (`:71-84`) now state a rule the code holds; the `set` comment (`:38-39`) matches the body after `e85f89e6a` dropped the header clause. I found no claim the code falsifies.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./internal/tui` is green with the six existing `TestContainmentFilter*` pins unedited, `go test -race ./internal/tui -run TestContainmentFilterIsRaceFreeAgainstAConcurrentSet` is green, `go test ./...` is green, and `gofmt -l .`, `go vet ./...` and `golangci-lint run` are clean." — settled only by execution. Reading confirms the code compiles in shape (no symbol added or removed, imports unchanged and all still used), that the new tests' expectations follow from the delivered `set`/`containmentFilter`, and that the write-hold-plus-clone discipline leaves no read of a mutated slice for `-race` to catch; the actual suite, race and lint runs are for the executing pass.
