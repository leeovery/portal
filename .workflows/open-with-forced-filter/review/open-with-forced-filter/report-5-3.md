TASK: open-with-forced-filter-5-3 (tick-6827c1) — Describe the search form in `portal open --help`

ACCEPTANCE CRITERIA:
- `Long` states the recognition rule — a positional beginning with `/` and containing no further `/` — in terms a reader can apply to their own argument
- `Long` gives all three outcomes by match count: one match attaches, none or several open the pre-filtered picker, and neither of the latter is an error
- `Long` names the term-less form and says it opens the picker with the filter ready to type, not that it is an error and not that it mints at root
- `Long` states that the form composes with nothing, naming the colliding kinds rather than restating the whole refusal table
- `Long` distinguishes `-f` from `/term` by outcome and says which to reach for — `-f` for a script or a keybinding, `/term` interactively
- `Long` names the single-segment absolute-directory cost and the `-p` escape
- The `-f/--filter` registered usage string is unchanged and remains a single line
- `TestOpenHelpMetadata_DescribesRedesignedVerb` passes with its existing keywords plus the two new ones, and its `Args` admission of two positionals is untouched
- No behaviour changes: no flag is added, renamed or re-defaulted, and `RunE`, `Args` and the completer are untouched
- `go test ./...` passes

STATUS: complete

SPEC CONTEXT:
§9.2 splits the documentation deliverable: `portal open --help` (`cmd/open.go`'s `Long`) carries the form and its recognition rule (with the single-segment absolute-directory cost and the `-p` escape, §2.2/§2.4), the outcomes by match count (§3.2), the term-less form (§2.5), that it composes with nothing (§5.1), and the `-f` / `/term` distinction by outcome plus which to reach for (§9.1); the `-f` flag description stays a one-liner. The completion points and the rollout consequence (§8.1/§8.3/§8.5) belong to the README, not to `Long` — they are another task's subject and their absence here is correct, not an omission.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/open.go:136-153` (the inserted search-form block in `openCmd.Long`, immediately after the `-f, --filter` line at `cmd/open.go:133-134`); `cmd/open.go:775` (the untouched `filter` flag registration); `cmd/retired_surface_test.go:143-144` (the two added keywords); `cmd/open_help_test.go:1-114` (the new suite). Commit `a065e1b9b`, +18 lines in `cmd/open.go` and +2 in `cmd/retired_surface_test.go`, both purely additive.
- Notes: every criterion is met by the delivered copy, and each claim it makes is true of the code that implements it, checked rather than assumed:
  - Recognition rule — "A positional beginning with / and containing no further /" (`cmd/open.go:136`) is §2.2 verbatim in substance, and is the rule `resolver.IsSearchSigil` is consulted for at `cmd/open_search.go:20`.
  - Outcomes — "Exactly one match attaches that session outright; no match or several open the picker pre-filtered by the term — never an error" (`cmd/open.go:142-143`) matches §3.2's K table and `runSearchForm` (`cmd/open_search.go:213-247`).
  - Term-less form — "open /  open the picker with the filter empty and ready to type" (`cmd/open.go:139`) matches §2.5 and the implementation at `internal/tui/model.go:1324-1333`, which flips the list to `list.Filtering` (focused, empty) only for an empty term. It is neither called an error nor described as minting at root.
  - Composition — "A search composes with nothing — another target, a command, -f, a domain pin or a second search on the same line is a usage error" (`cmd/open.go:148-149`) names the colliding kinds and no more; the arms it summarises are `validateSearchFormCollisions` (`cmd/open_search.go:83-96`). It deliberately does not claim "any flag", which keeps §5.1's `--help` / persistent-flag carve-out intact.
  - `-f` vs `/term` — `cmd/open.go:151-153` separates them by outcome and assigns the uses §9.1 assigns (script-or-keybinding vs interactive), not by input shape.
  - Single-segment cost and escape — `cmd/open.go:144-145` ("mint there with -p /tmp"), per §2.4.
  - No behaviour change: the diff touches `Long` only. `Use`, `Short`, `Args` (`validateOpenArgs`), `RunE`, the flag registrations and the completer are unmodified in the commit.

TESTS:
- Status: Adequate
- Coverage: `cmd/open_help_test.go` covers each acceptance criterion as a named subtest of `TestOpenHelpMetadata_DescribesSearchForm` — the recognition rule (`open /term`, `beginning with /`, `no further /`), the term-less form (`open /` plus `ready to type`), the outcomes (`one match` / `attaches` / `picker pre-filtered` / `never an error`), the match domain (`case-folded`, `contiguous run`, `recorded directory`), the composition refusal (`composes with nothing`, `usage error`), the single-segment cost and `-p /tmp`, and the `-f` / `/term` split. That last one is the strongest assertion in the file: `searchFormHelpBlockNaming` (`cmd/open_help_test.go:82-89`) requires both tokens in one blank-line-separated paragraph and then requires `always opens the picker`, `keybinding` and `interactive` inside that same paragraph — which is what pins §9.1's "described by outcome in one place" rather than "both words appear somewhere". `TestOpenFilterFlagUsage_StaysAOneLiner` (`cmd/open_help_test.go:100-114`) pins the flag one-liner from both sides: no newline, and none of `/term` / `search` / `match`.
- Notes: the block-scoped assertion resolves unambiguously against the current text — no earlier paragraph of `Long` carries both `-f` and `/term`, so `searchFormHelpBlockNaming` cannot pass on the wrong paragraph. Each subtest would fail if the corresponding sentence were deleted or reworded past its keyword, which is the failure mode copy work actually has. Not over-tested: the keyword approach is the one the neighbouring `TestOpenHelpMetadata_DescribesRedesignedVerb` already states a reason for, and the two new keywords added there (`cmd/retired_surface_test.go:143-144`) overlap this suite only at the "the words exist" level while guarding a different subject (the full-surface enumeration). The `Args` two-positional admission at `cmd/retired_surface_test.go:101-105` is untouched, as required.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()` (CLAUDE.md forbids it in this tree); the file name derives from the source file under test and matches the package's existing `open_*_test.go` split-by-concern layout; the two new helpers (`searchFormHelpBlockNaming`, `namesAll`) are unique in package `cmd`, so nothing collides at compile time.
- SOLID principles: Good — the assertions are behaviour-shaped (what the text tells a reader) rather than golden-string coupled.
- Complexity: Low.
- Modern idioms: Yes — `strings.SplitSeq` range-over-func, valid under the module's `go 1.26.0` and the `modernize` linter the repo runs.
- Readability: Good. The help block reads in the order a user needs it (rule → examples → outcomes → cost → composition → which form to reach for), and sits directly under the `-f` line so the pair is read together, as §9.1 requires.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes" — settled only by running the unit lane; reading confirms the added file compiles against package `cmd` (unique helper names, imports used, `strings.SplitSeq` available at `go 1.26.0`) and that every literal each assertion requires is present in `openCmd.Long` and absent from the `filter` flag usage, but the suite itself was not executed.
