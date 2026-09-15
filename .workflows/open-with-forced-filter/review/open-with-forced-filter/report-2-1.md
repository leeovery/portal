TASK: open-with-forced-filter-2-1 (tick-46dad8) — Decide a session match by case-folded containment over name and directory

ACCEPTANCE CRITERIA:
1. A term appearing as a contiguous run in the session name is a match, in either case direction
2. A term appearing as a contiguous run in the recorded directory's home-abbreviated form is a match, and the abbreviation is applied before the comparison
3. A directory outside the home directory is compared unabbreviated
4. A run spanning the end of the name and the start of the directory is NOT a match
5. A session with an empty recorded directory is judged on its name alone and never panics or matches vacuously
6. `*`, `?` and `[` in the term are literal
7. An unresolvable home directory degrades to comparing the raw recorded path
8. An empty term returns false (fail closed)
9. The predicate touches no filesystem beyond `AbbreviateHome`'s `$HOME` read and issues no tmux call
10. `go test ./...` passes

STATUS: complete

SPEC CONTEXT: §4.1 fixes the matched fields as the session name plus the *recorded* `@portal-dir` value (never a derived one), searched in its home-abbreviated form so "the searched form is the displayed form" — otherwise a term hitting the home prefix would match every session while no row showed the text. §4.3 fixes the sigil's rule as case-folded *containment* with the two fields tested separately and never joined, and the term as literal text (`*`, `?`, `[` are characters to find), explicitly because the picker's stock `sahilm/fuzzy` subsequence rule would let `/port` attach `~/Projects/rust-tools` unseen. §4.4 tabulates the divergence: containment for the sigil, the picker's fuzzy for `-f` and a hand-typed `/`, with the field set identical across all three.

IMPLEMENTATION:
- Status: Implemented (evolved past the task's literal wording by a later, recorded analysis task — see Notes)
- Location:
  - internal/resolver/search.go:9-14 — `SearchFields(name, recordedDir) []string`
  - internal/resolver/search.go:22-36 — `MatchesSearchTerm(term, name, recordedDir) bool`
  - internal/resolver/path.go:94-106 — `AbbreviateHome` (task 1-2), the abbreviation this rule takes the directory's matched form from
  - Delivered at 3f4bf8e4c; refactored onto `SearchFields` by tick-403463 ("Derive the searchable field list from one declaration")
- Notes:
  - Criterion-by-criterion the code holds. Empty term returns false before anything else (search.go:23-25). The term is folded once (search.go:27) and each field folded at comparison (search.go:30). `SearchFields` returns `[]string{name}` for an empty recorded directory, so the directory branch is structurally unreachable rather than guarded by a nil check — criterion 5 holds without a panic path. Only `strings` is imported; `AbbreviateHome`'s `os.UserHomeDir` is the sole environment read and there is no tmux call anywhere on the path (criterion 9, verified by reading the file's whole import set).
  - Drift, judged and sound: the task's **Do** prescribed "two separate `strings.Contains` calls against two separate folded strings". The shipped form is a `range` over `SearchFields(...)` calling `strings.Contains` once per field. This is the same substance — one `Contains` call per field against that field's own folded value, with no joined value ever built — and it is the product of a later plan task that made the field list one declaration shared with `SessionItem.FilterValue()` (internal/tui/session_item.go:88 joins the same `SearchFields` result for the fuzzy routes, which is exactly what §4.2/§4.3 require of the joined text). The intent the task names — "the text this rule searches is the same value `SessionItem.FilterValue()` joins" — is better served by the shipped form than by the prescribed one. Not a finding.
  - `filepath.Match` and `HasGlobMeta` appear nowhere on the path, so criterion 6 holds by construction rather than by special-casing.
  - "Wire the rule nowhere" was honoured at 3f4bf8e4c (the commit touches only search.go and its test). The two callers that exist today are the ones the task names as belonging to tasks 2-2 and 2-3: cmd/open_search.go:189 (the count, `searchMatches`) and internal/tui/search_filter.go:75 (`containmentFilter`). Both pass `Session.Dir` — the recorded value, not a derived one — so the "two deciders reach identical verdicts from one implementation" property the task's Problem statement demands is met: there is exactly one implementation and two call sites of it.

TESTS:
- Status: Adequate
- Coverage:
  - internal/resolver/search_match_test.go:10-137 — a 15-row table covering every criterion: name containment, both case directions (PORT/portal-a1b2 and port/PORTAL-A1B2), directory containment, the tilde the abbreviation introduces, the home directory's own last segment, `Users`, an unabbreviated `/opt/tools`, the name/directory spanning run `i /o`, empty-directory name-only match and empty-directory non-match, and `*`, `?`, `[` as literals in both directions.
  - internal/resolver/search_match_test.go:139-145 — `TestMatchesSearchTermUnresolvableHome` pins criterion 7 with `t.Setenv("HOME", "")`.
  - internal/resolver/search_fields_test.go:35-47 — restates the spanning case with a precondition assertion that the term *does* span the joined fields, so the test fails for the right reason rather than because the term happens not to occur.
  - internal/resolver/path_test.go:368-430 — `AbbreviateHome`'s own cases, including "home itself abbreviates to a bare tilde" and the lookalike-prefix guard, so the Edge Case about a directory equal to `$HOME` is covered at the level that owns it.
- Notes:
  - The abbreviation assertions are load-bearing rather than incidental, which is the failure mode this kind of table usually has. The `filepath.Base(home)` row (search_match_test.go:52-58) would pass trivially under a raw-path comparison only if the temp dir's last segment were absent from the raw path — it is not, so dropping the abbreviation turns that row red. The `~/code` row (46-51) is the positive counterpart: `~/code` occurs only in the abbreviated form. Criterion 2's "the abbreviation is applied before the comparison" is therefore genuinely pinned in both directions.
  - Every home-relative case is anchored on `t.Setenv("HOME", t.TempDir())` as the task required, so no verdict depends on the developer's account name. No `t.Parallel()` (required — `t.Setenv` forbids it, and the project bans it outright).
  - Not over-tested: the one duplicated subject is the spanning case, present in both files, and the two are not the same assertion — the fields test adds the precondition that the joined value contains the term, which the table row cannot express. No mocking, no setup beyond an env var, and nothing asserts on an implementation detail (the tests call only the exported predicate).
  - The `?` and `[` rows assert the *negative* only (`po?t` and `po[rs]t` miss `portal-a1b2`), where `*` gets both directions. That is the right asymmetry: a false there would mean the metacharacter was expanded, which is the only failure worth catching, and the positive direction is already carried by the `po*rt`/`po*rt-x` row.

CODE QUALITY:
- Project conventions: Followed. The package stays log-free (search.go imports `strings` alone; internal/resolver/log_free_test.go's guard covers it), the resolver remains a pure library as CLAUDE.md requires, and the subtests use the project's `it …` naming.
- SOLID principles: Good. One exported predicate with one reason to change, and the field list it depends on factored into `SearchFields` — which is what gives the fuzzy routes and the containment route a single declaration of "what is searchable" instead of two.
- Complexity: Low. One guard, one fold, one loop, one return.
- Modern idioms: Yes. Plain `strings.ToLower`/`strings.Contains` over a `range`; nothing here wants a generic or a `slices` helper.
- Readability: Good. Both doc comments state the contract and the reason for it (`never joined`, `matches nothing … narrows to nothing rather than to everything`) without restating the code, and neither references a task id, phase or spec section.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes" — requires an execution pass over the unit lane; reading cannot settle it. The package's tests are hermetic (no tmux, no daemon, no binary build — only `t.Setenv`/`t.TempDir`), so it belongs in the fast lane where it sits.
