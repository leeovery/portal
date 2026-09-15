TASK: open-with-forced-filter-1-2 (tick-d5998c) — Match sessions on their home-abbreviated recorded directory as well as their name

ACCEPTANCE CRITERIA:
1. `AbbreviateHome` abbreviates a path under the home directory to `~/<rest>`, returns a bare tilde for the home directory itself, and returns any other path byte-identical — including a home-prefix lookalike (`/Users/leeoveryX/Code` under home `/Users/leeovery`)
2. `AbbreviateHome` returns its input unchanged when the home directory cannot be resolved, and touches the filesystem on no path
3. `SessionItem.FilterValue()` for a session with a recorded directory is the name, one space, then the home-abbreviated directory; for one without, the name alone with no trailing space
4. `-f <directory fragment>` narrows the sessions list to sessions whose recorded directory carries that fragment
5. A `/` filter typed by hand inside the picker narrows on the same widened text
6. A term matching the user's account name or home prefix matches no session on the strength of a path the row does not display
7. `HeaderItem.FilterValue()` is still empty, so headings still vanish the moment a query is typed
8. A session whose directory is known only to grouping (recorded empty, derived present) is not matchable by that directory
9. `go test ./...` passes

STATUS: complete

SPEC CONTEXT:
§4.1 fixes the matched fields as the session name plus its **recorded** `@portal-dir` directory — never a grouping-derived one — and fixes the searched form as the displayed form: a directory under the user's home is matched home-abbreviated, so a term hitting the home prefix cannot match every session on characters no row shows. §4.2 widens those fields for all three filter entry points (`-f`, the sigil, a hand-typed `/`) deliberately; only the *rule* diverges by entry point (§4.3/§4.4), and on the two fuzzy routes the fields are joined as "name, one space, home-abbreviated directory", a session with no recorded directory joining to its name alone with no trailing separator. §4.5 accepts that the widening reaches pickers this feature does not open, where a row may surface on a directory it does not display.

IMPLEMENTATION:
- Status: Implemented (with a later, sound consolidation on top)
- Location:
  - `internal/resolver/path.go:88-106` — `AbbreviateHome`: `os.UserHomeDir()`, degrade-to-input on error or empty home, bare `~` for the home itself, `"~/" + path[len(home)+1:]` under the `home + "/"` prefix, otherwise the path unchanged. Only `os` (env read) and `strings`; no `filepath.EvalSymlinks`, no `os.Stat`, no filesystem access on any branch.
  - `internal/tui/session_item.go:82-89` — `FilterValue()` now returns `strings.Join(resolver.SearchFields(i.Session.Name, i.Session.Dir), " ")`.
  - `internal/resolver/search.go:5-15` — `SearchFields` returns `[name]` for an empty recorded dir and `[name, AbbreviateHome(dir)]` otherwise.
  - `internal/tui/session_item.go:100` — `HeaderItem.FilterValue()` still `""`.
  - `internal/tui/model.go:1207-1228` — the derived directory lands in `m.derivedDirs` only; `Session.Dir` is never written, so the derived value is structurally outside the matched text.
- Notes: The task as authored prescribed the literal `i.Session.Name + " " + resolver.AbbreviateHome(i.Session.Dir)`, and the commit that delivered this task (b46f51e41) wrote exactly that. A later phase-6 task (tick-403463, "Derive the searchable field list from one declaration") re-pointed it at `resolver.SearchFields`, which the sigil's containment rule (`resolver.MatchesSearchTerm`) also reads. The observable value is byte-identical for both shapes, and the field list now has one declaration rather than two — a divergence from the task's wording that is a gain, not a loss. `resolver.AbbreviateHome` is also the single abbreviation used by the directory column (`internal/tui/session_dir_column.go:28`), so what is matched and what is rendered come from one rule.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/resolver/path_test.go:368-429` — `TestAbbreviateHome`: under-home abbreviation, home itself → `~`, the `/Users/leeoveryX/Code` lookalike, an unrelated `/opt/tools`, unresolvable home (`t.Setenv("HOME", "")` — Go's `os.UserHomeDir` errors on an empty `$HOME`), and an `ExpandTilde` round trip. Criteria 1 and 2.
  - `internal/resolver/search_fields_test.go:12-45` — `SearchFields` shapes, plus the separate-fields property for the containment rule.
  - `internal/tui/session_item_test.go:16` and `:26` — name alone (no trailing separator) and `"api-work ~/Code/portal"`; `:38` and `:52` pin the value as the `SearchFields` slice joined, which is the drift guard the consolidation wanted. `:88`/`:101` are the two group-field subtests the task named, both extended to the with-directory shape. Criterion 3.
  - `internal/tui/session_item_test.go:419-424` — `HeaderItem.FilterValue()` empty. Criterion 7.
  - `internal/tui/model_test.go:2825` (`-f portal` over `api-work` at `/Users/leeovery/Code/portal`), `:2846` (`/` plus `portal` driven through `Update`), `:2873` (`-f leeovery` returns zero rows). Criteria 4, 5, 6. All three discriminate: under the old name-only filter value, `portal` is not even a fuzzy subsequence of `api-work` (no `t` after the `r`), so 2825/2846 would fail; and `leeovery` matches the raw path but not the abbreviated one, so 2873 would fail if the abbreviation were dropped.
  - `internal/tui/rebuild_dir_resolution_test.go:233-261` — a By-Project rebuild populates `derivedDirs`, then asserts `FilterValue()` is the bare name and that filtering on the derived key leaves zero visible items. Criterion 8.
- Notes: No over-testing found. The two `SearchFields`-derived subtests (`:38`, `:52`) overlap the literal-value subtests above them, but they are pinning a different property — that `FilterValue` reads the shared declaration rather than restating the rule — so they are not redundant assertions of the same thing.

CODE QUALITY:
- Project conventions: Followed. `internal/resolver` stays stdlib-only and log-free; `internal/tui` already read `$HOME` transitively via `project.CanonicalDirKey` → `ExpandTilde`, so no new kind of dependency. Doc comments lead with the identifier, no test uses `t.Parallel()`, no spec-section or task-id references in source comments.
- SOLID principles: Good. `AbbreviateHome` is the exact inverse of `ExpandTilde` and sits beside it; the field list has one home (`SearchFields`) consumed by both the fuzzy join and the containment rule.
- Complexity: Low — three guarded returns, no branching beyond them.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None. Comments checked against the code: `AbbreviateHome`'s "never touches the filesystem" and "degrades to the path as given" both hold; `FilterValue`'s "the grouping-derived directory is no part of it" holds against `model.go:1207-1228`, which writes the derived value only into `m.derivedDirs`.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes" — not settled by reading. The widening changes `SessionItem.FilterValue()` for every picker in the tree, so any pre-existing suite that filters a session list whose sessions carry a `Dir` could shift its visible-row count; the unit lane (`go test ./...`) has to be run to settle it.
