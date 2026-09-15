TASK: open-with-forced-filter-5-4 (tick-cd4c35) — Document the search form and the completion correction in the README

ACCEPTANCE CRITERIA:
- The example block carries `x /port` and `x /`, each with a one-line comment
- The section states the recognition rule as a test of the argument's shape, with no suggestion that the filesystem is consulted
- The section states what is matched — session name and recorded directory, home-abbreviated — and that the match is case-folded containment while the picker's own filter stays fuzzy
- The section gives all three outcomes by match count and says a zero count is not an error
- The section documents `x /` as opening the picker with an empty, ready-to-type filter
- The section names the single-segment absolute-directory cost and the `x -p /tmp` escape
- The section states that the form composes with nothing
- The section distinguishes `-f` from `/term` by outcome and says which to reach for
- The resolution table carries a row for the form that reads as a positional and not as a domain pin, and the table intro no longer claims every row hard-fails without popping the picker
- The section states that Tab after the session-opening function now offers live session names, and that `x /po<TAB>` completes the term after the slash against those same names
- The section documents the `portal init` correction and that an existing install picks it up only on a new shell or a re-run of `portal init` while the form itself works with the new binary
- The section notes that a path argument after the function no longer completes filenames
- No CHANGELOG entry is written, and no file outside `README.md` and the new test is modified
- `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

STATUS: issues_found

SPEC CONTEXT: §9.1–§9.3 make documentation a deliverable because `-f` and `/term` read as duplicates when described by input shape, so the pair must be described by outcome and the README must say which to reach for. §9.2 enumerates what the README's `x (open)` section must carry — the form and its recognition rule with the single-segment cost and the `-p` escape (§2.2, §2.4), the three outcomes (§3.2), the term-less form (§2.5), the matched fields and the containment-versus-fuzzy divergence (§4), that the form composes with nothing (§5.1), completion after the slash (§8.1), the corrected completion's offered set and the loss of filename fallback (§8.3), and the rollout consequence (§8.5) — and additionally puts the sigil row in the README's resolution table beside the pins and `-f`. §9.3 forbids a CHANGELOG entry.

IMPLEMENTATION:
- Status: Implemented (with later, sound refinements from sibling tasks 5-6 and 8-2)
- Location: README.md:128-129 (examples), README.md:143-159 (Session search block), README.md:161-171 (table intro, header `Flag / form`, `/<term>` row at :170), README.md:175 (Tab completion paragraph); guard at cmd/open_docs_test.go:1-92
- Notes:
  - Every factual claim in the block was walked against the code, not the spec: the recognition rule and "never asks the filesystem anything" against `resolver.IsSearchSigil` (internal/resolver/path.go:26-28); the containment/case-folding and the two separately-tested fields against `resolver.MatchesSearchTerm` and `SearchFields` (internal/resolver/search.go:10-35); the home-abbreviated form against `resolver.AbbreviateHome` as the row renders it (internal/tui/session_dir_column.go:29); the three outcomes against `runSearchForm` (cmd/open_search.go:196-224); "filter empty, focused" for `x /` and the committed landing for a term against `applySearchLanding` (internal/tui/model.go:1320-1335); "composes with nothing" against `validateSearchFormCollisions` (cmd/open_search.go:76-113); the `--` carve-out against `preDashPositionals` (cmd/open_search.go:28-34). All hold.
  - The completion paragraph's claims hold too: the `x <TAB>` arm against `completeSessionNames` (cmd/completion.go:33-40) reached via the emitted shims (cmd/init.go:65-79, :102-108, :168-171), and the search arm's offered set — "every live session but the one you are attached to" — against `completeSearchTerm` (cmd/completion.go:68-82) → `tui.PickerSessions` (internal/tui/picker_sessions.go:12-23) and `currentPickerSession` (cmd/open_search.go:164-173).
  - Divergence from the task text, deliberate and sound: the task asked the completion paragraph to open "this release corrects `portal init` …" and to say `x /po<TAB>` completes "against those same names". Later sibling tasks (287c6c51e for 5-6, 936f540cb for 8-2) replaced the release-relative framing with a durable statement of the behaviour and its `portal init` dependency, and stated the search completer's offered set separately and more precisely. Both readings still satisfy §9.2 and the criteria in substance; the second is strictly more accurate against the code. Not reported as a loss.
  - Scope held: the task commit (a8fc419d3) touched only README.md and cmd/open_docs_test.go. No commit in the feature's history touches CHANGELOG.md (the file exists and is untouched).

TESTS:
- Status: Adequate, with one weakened assertion (see FINDINGS)
- Coverage: `TestReadmeDocumentsSearchForm` (cmd/open_docs_test.go:68) fatals when the `### \`x\` (open)` heading is gone or its section is empty, so a rename fails loudly rather than passing vacuously; `readmeOpenExamples`/`hasCommentedExample` (cmd/open_docs_test.go:41-66) anchor the two example assertions to the fenced block and require a `#` comment, so prose carrying `/port` cannot satisfy them; the token assertions cover the `-p /tmp` escape, `portal init`, and the pre-existing `-f, --filter` row, each of which occurs exactly once in the section and so genuinely fails on removal.
- Notes: Not over-tested — the guard asserts literal tokens a user types rather than prose, so accurate copy edits do not churn it, which is what the task asked for. The prose criteria (recognition rule, outcomes, composition, `-f`/`/term`) are deliberately unguarded and should stay that way.

CODE QUALITY:
- Project conventions: Followed — no `t.Parallel()`, `*testing.T`-first helpers with `t.Helper()`, README located through `sourceguardtest.ProjectRoot` rather than a hand-rolled path, and the file compiles nothing so it correctly stays in the unit lane.
- SOLID principles: Good — the section slice, the example slice and the shape predicate are three small helpers with one job each.
- Complexity: Low.
- Modern idioms: Yes — `strings.Cut` for both slices, `strings.Fields` for the example shape.
- Readability: Good. The comments state why each helper exists (the whole-section match a prose sentence would satisfy; the heading rename reported once), and they are accurate against the code.
- Issues: None beyond the finding below. The map-ranged assertion loops give nondeterministic failure ordering where a table-driven slice would not — a preference, not reported.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/open_docs_test.go:82 — the assertion named "the resolution-table row" tests `strings.Contains(section, "`/<term>`")`, but that literal now occurs twice in the section: at README.md:161 in the table intro ("`-f`, `/<term>` and `-e`/`--` are not pins") as well as at README.md:170 in the row itself. Anchor it to the row — assert a line in the section whose trimmed form begins with the literal "| `/<term>`" — rather than to the bare token. FAILS: deleting the table row (README.md:170) leaves the test green, so the one guard the task asked for over that row no longer fails when it vanishes; the same class of unanchored-token weakness the attempt-1 fix round already corrected for the two example lines, reintroduced for the row when task 5-6 added `/<term>` to the intro sentence.

UNSETTLED:
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — both suites would have to be run; this review reads only. Read statically, `TestReadmeDocumentsSearchForm` passes against the README's current text (all four tokens present, both fenced examples present with `#` comments, section non-empty).
