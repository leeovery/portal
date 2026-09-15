TASK: Derive the searchable field list from one declaration (tick-403463 / open-with-forced-filter-6-2)

ACCEPTANCE CRITERIA:
- `SearchFields` returns the name alone for an empty recorded directory, and the name plus the home-abbreviated directory otherwise
- `MatchesSearchTerm` still tests each field separately: a term spanning the end of the name and the start of the directory is not a match, and an empty term matches nothing
- `FilterValue` for a session with no recorded directory is the bare name with no trailing space; with one it is `"<name> <home-abbreviated dir>"`, name first
- Neither consumer restates the field list: `internal/tui/session_item.go`'s `FilterValue` names no directory field of its own, and the only `AbbreviateHome` calls left in `internal/tui` are the two display ones
- `TestMatchesSearchTerm`, `TestMatchesSearchTermUnresolvableHome` and the existing `FilterValue` subtests pass with no edit
- `go test ./...` is green, `gofmt -l` reports nothing and `go vet ./...` is clean

STATUS: complete

SPEC CONTEXT:
§4.1 fixes the match domain as name + recorded directory (`@portal-dir`), never a derived one, and fixes the *form* as the home-abbreviated value ("the searched form is the displayed form"). §4.2 states the fields are shared across all three filter entry points and that the abbreviation "belongs to the value rather than to the sigil's rule". §4.3/§4.4 state what diverges: the sigil tests the fields separately by containment, while `-f` and hand-typed `/` join them ("the session name, one space, then the recorded directory in its home-abbreviated form … a session carrying no recorded directory joins to its name alone, with no trailing separator") and score by the picker's fuzzy rule. §4.5 names narrowing back to names alone as the anticipated cheap reversal — which is precisely the edit this task makes single-sited.

IMPLEMENTATION:
- Status: Implemented, matching the plan's `Do` list step for step, with no drift.
- Location:
  - `internal/resolver/search.go:9-14` — `SearchFields(name, recordedDir string) []string`: `[]string{name}` when `recordedDir == ""`, else `[]string{name, AbbreviateHome(recordedDir)}`.
  - `internal/resolver/search.go:22-36` — `MatchesSearchTerm` rewritten as the empty-term guard, one `strings.ToLower(term)` fold, then a `strings.Contains` loop over `SearchFields(...)`. Nothing joins the fields on this path.
  - `internal/tui/session_item.go:87-89` — `FilterValue` is `strings.Join(resolver.SearchFields(i.Session.Name, i.Session.Dir), " ")`; the `Dir == ""` branch is deleted and `strings` is imported (`internal/tui/session_item.go:6`, its only use is line 88).
  - `internal/tui/session_item.go:82-86` — doc comment trimmed: the "widening or narrowing what a filter matches means changing both" warning is gone, one line on the join order (name leads) and the grouping-derived-directory exclusion retained.
- Notes:
  - Behavioural equivalence holds on reading: field order is unchanged (name first), the abbreviation is pure, and the old `recordedDir == ""` early return is now expressed by the one-element slice. The one accepted difference — `SearchFields` abbreviates eagerly where the old code short-circuited on a name hit — is exactly what the plan instructed to accept, and `AbbreviateHome` (`internal/resolver/path.go:94-106`) is an env read plus at most one string slice, no filesystem access.
  - Single-declaration property verified across every consumer: `cmd/open_search.go:189` and `internal/tui/search_filter.go:75` are the only production callers of `MatchesSearchTerm` (both now inherit the field list), and `internal/tui/session_item.go:88` is the only site building the joined form. `internal/tui/search_filter.go` needed no edit — its `searchEntry{name, dir}` holds the *inputs* to `SearchFields`, not a restatement of the field list, and it delegates the rule to `MatchesSearchTerm`.
  - `AbbreviateHome` enumeration in `internal/tui`: exactly two call sites remain, both display-side — `internal/tui/session_dir_column.go:28` (`fitSessionDir`) and `internal/tui/session_item.go:268` (the unsized-list row render). None in `internal/tui` test files.
  - No later commit touches `internal/resolver/search.go`, `internal/tui/session_item.go` or either test file (`git log bb1522dfc..HEAD` on those paths is empty), so what the task delivered is what stands at HEAD.

TESTS:
- Status: Adequate.
- Coverage:
  - `internal/resolver/search_fields_test.go:12-47` — all three plan-named resolver tests, with `HOME` pinned to a `t.TempDir()` in every subtest so the abbreviation is environment-independent: name alone for an empty directory, `[]string{"api-work", "~/Code/portal"}` in that order, and the spanning-run case whose precondition first asserts the run really does span the *joined* fields before asserting `MatchesSearchTerm` still refuses it.
  - `internal/tui/session_item_test.go:38-66` — both plan-named `FilterValue` tests: derivation from `SearchFields` for a session with a directory, and the one-element join with an explicit `strings.HasSuffix(got, " ")` trailing-separator assertion.
  - The pre-existing literal-value subtests (`internal/tui/session_item_test.go:16-36`) are untouched and pin the concrete strings `"dev"` and `"api-work ~/Code/portal"`, so the two new derivation tests are not self-referential in aggregate: a wrong `SearchFields` fails the literal pair, a broken derivation fails the new pair.
  - `internal/resolver/search_match_test.go` is untouched by the commit (the diff spans four files, and it is not among them), keeping the 15-case containment table — empty term, case folding both directions, tilde matching, the replaced-home-segment refusals, glob literals, the spanning-run refusal, and the empty-directory cases — as the unedited regression baseline the task's acceptance asks for. `TestMatchesSearchTermUnresolvableHome` (`internal/resolver/search_match_test.go:139-145`) still reads correctly under the rewrite: `HOME=""` makes `AbbreviateHome` return the raw path, so the field is the unabbreviated `/Users/...` and `"Users"` matches.
- Notes:
  - The new spanning-run subtest overlaps the table case `"it does not match a run that spans the name and the directory"`. It is plan-prescribed and earns its place by asserting the precondition the table case leaves implicit (that the run genuinely spans the join), so it is not redundant in substance.
  - No `t.Parallel()` anywhere in the new tests, per the project rule.

CODE QUALITY:
- Project conventions: Followed. Exported function with a doc comment opening on its own name; `internal/resolver` stays a pure, log-free library; no new dependency edge (`internal/tui` already imported `internal/resolver`).
- SOLID principles: Good. The field set now has one owner and two derivations; the rule (containment vs fuzzy) stays with each entry point, which is the divergence the spec requires.
- Complexity: Low. The loop replaces a three-branch cascade; `SearchFields` is a single guard.
- Modern idioms: Yes. `strings.Join` over the slice is what makes the no-trailing-separator case fall out rather than be branched on.
- Readability: Good.
- Issues: None. Comment accuracy checked on both rewritten doc comments: `SearchFields`'s "in display order … name, then home-abbreviated directory" matches the row order in `renderSessionRow` (`internal/tui/session_item.go:311`, name then `dirCell`), and `FilterValue`'s retained claim that the grouping-derived directory is no part of it holds — the join reads `i.Session.Dir`, which §4.1 fixes as the recorded value, never `m.derivedDirs`. No process-artifact references in either.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`TestMatchesSearchTerm`, `TestMatchesSearchTermUnresolvableHome` and the existing `FilterValue` subtests pass with no edit" — the "no edit" half is settled by the commit diff (neither `internal/resolver/search_match_test.go` nor `internal/tui/session_item_test.go:16-36` is modified); the "pass" half needs `go test ./internal/resolver ./internal/tui` run, though reading shows the matched set unchanged case by case.
- "`go test ./...` is green, `gofmt -l` reports nothing and `go vet ./...` is clean" — settle by running `go test ./...`, `gofmt -l .` and `go vet ./...` from the project root.
