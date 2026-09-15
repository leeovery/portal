TASK: open-with-forced-filter-12-4 (tick-8ecd82) — Decide whether completion may offer a word for a line the validator will refuse. Settled to "completion completes the word in front of it"; the task lands the record of that decision (spec §8.1 correction + one corrigendum + doc comment + one sibling test pin), with no behaviour change.

ACCEPTANCE CRITERIA:
1. `completeOpenPositional` and `validateSearchFormCollisions` behaviourally unchanged — `portal __complete open api /po` still offers `/portal-a1b2`; `portal open api /port` still refused with `cannot use a /term search with another target`.
2. §8.1 states the `--` separator as the single bound on the sigil arm and names the validator as what refuses an illegal line.
3. `## Corrigenda` carries exactly one new entry, dated from this machine, attributed to `implementation/open-with-forced-filter`, quoting the claim it corrects.
4. The re-index ran against that specification path and the knowledge store's changes are committed with the task rather than left dirty.
5. `completeOpenPositional`'s doc comment accounts for a search form offered on a line the validator refuses.
6. The new test pins the `-f` case, so the rule is pinned at two arities rather than one.
7. `completingPreDashPositional`, `validateSearchFormCollisions`'s seventh derived arm and the plain session-name completion at a second positional are untouched, and `go test ./...` is green.

STATUS: complete

SPEC CONTEXT: §8.1 ("Completion looks past the sigil") states what the sigil form offers — the searched set's session names the typed term prefixes, sigil retained, directories and slash-bearing names excluded — and, since the earlier 2026-09-15 corrigendum, bounds the sigil arm at the `--` separator. §5.1 makes the search form non-composing, and `validateSearchFormCollisions` enforces that at parse time. The gap this task closes is a record gap, not a behaviour gap: the separator bound's stated reason ("offering one there would complete to a term the parser will not honour") read as though it extended to every other line §5.1 refuses, which §8.1 never took up, leaving `open api /po<TAB>` → `/portal-a1b2` → refused-on-Enter looking like an oversight rather than a decision.

IMPLEMENTATION:
- Status: Implemented (record-only; behaviour deliberately unchanged)
- Location:
  - `cmd/completion.go:95-103` — the added doc-comment paragraph on `completeOpenPositional` (`cmd/completion.go:104`).
  - `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md:332` — §8.1's separator paragraph, now "…and by nothing else."
  - `…/specification.md:334` — the new "Everywhere else a search form could be typed, one is offered" paragraph, naming `validateSearchFormCollisions` as what refuses the assembled line.
  - `…/specification.md:494` — the one new corrigendum, dated 2026-09-15 (confirmed against `date` on this machine: Tue Sep 15 2026), attributed to `implementation/open-with-forced-filter`, quoting both §8.1's unconditional offered-set sentence and the separator bound's stated reason.
  - Commits: `0af96d2ca` (code + test) and `cef2d5632` (spec + `.workflows/.knowledge/metadata.json` + `store.msp`), both dated 2026-09-15 18:56:46 +0100.
- Notes:
  - Criterion 1 holds by construction — `0af96d2ca` touched only a comment and a test — and holds against the code as it now stands after the later Task 12-6 restructure: `completeOpenPositional` (`cmd/completion.go:104-111`) gates on `completingPreDashPositional` alone, so `open api /po` (no separator) takes the sigil arm; `validateSearchFormCollisions` (`cmd/open_search.go:77-105`) returns "cannot use a /term search with another target" for two pre-dash positionals one of which is a sigil (`cmd/open_search.go:86-87`). Both halves carry standing pins: `cmd/completion_test.go:637-645` and `cmd/open_search_test.go:337-354`.
  - Criterion 4 verified rather than assumed: `grep -a` finds both "and by nothing else" and "Everywhere else a search form" present in `.workflows/.knowledge/store.msp`, and `git status` shows the knowledge store clean.
  - Criterion 7's "untouched" half verified against the diff of `0af96d2ca`: it adds 11 comment lines and 10 test lines and changes nothing else. (The later `756d46a23`/Task 12-6 restructure of `completeOpenPositional`'s body and the `4b6fbd18e` follow-up correction to §8.1:332 and to this task's corrigendum at :494 are that task's, not drift here — and the record is internally consistent after them.)
  - The doc comment's claims were checked against the code rather than taken on trust: all four domain pins plus `-f`, `-e` and `--ack` are string flags registered on `openCmd` (`cmd/open.go:774-781`), and `openCmd.ValidArgsFunction = completeOpenPositional` (`cmd/open.go:785`), so a positional typed beside any of them does reach the sigil arm exactly as the comment says.

TESTS:
- Status: Adequate
- Coverage: `cmd/completion_test.go:647-655` ("it still offers sigil completions for a pre-dash word on a line carrying -f") drives a real `__complete` through `rootCmd.Execute` (`completionResult`, `cmd/completion_test.go:296-325`) with `--filter api` set and `/po` as the word being completed, asserting the candidate set is exactly `[/portal-a1b2]`. It sits beside the unchanged positional pin at `:637-645`, so the rule is pinned once at a second positional and once at a set flag. The validator half of the pair is pinned independently at `cmd/open_search_test.go:381-385` (`-f`) and `:337-354` (second target).
- Notes:
  - It would fail if the behaviour broke in the one plausible way — a gate added to `completeOpenPositional` on `cmd.Flags().Changed("filter")` (or on any flag-shaped condition) — because the assertion is an exact set equality, not a non-empty check.
  - Not over-tested: nine lines, one assertion, no new mocking. It stages only `completionSessions`, matching every sibling in `TestCompleteOpenPositionalSeparatorBound`; the unstaged `completionCurrentSession` seam degrades to "" against the package `TestMain`'s poisoned `TMUX`, which is the file's existing shape rather than something this task introduced.

CODE QUALITY:
- Project conventions: Followed. Comment-only change on the production side; the test uses the package's `withFuncSeam`-backed `withCompletionSessions` staging helper rather than assigning the seam directly, so `cmd/seam_guard_test.go` stays satisfied.
- SOLID principles: Good — no structural change. The decision recorded is the single-responsibility one: the completer completes a word, the Args validator judges a line.
- Complexity: Low — no branch added.
- Modern idioms: Yes.
- Readability: Good. The doc comment now answers the question a reader meeting `x api /po<TAB>` actually has, and states the asymmetry between the two bounds rather than asserting the rule bare.
- Issues: None. The added comment carries no spec-section numbers, no task ids and no restatement of code; every symbol it names (`validateSearchFormCollisions`, `completingPreDashPositional`) exists and behaves as described.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`completingPreDashPositional`, `validateSearchFormCollisions`'s seventh derived arm and the plain session-name completion at a second positional are untouched, and `go test ./...` is green." — the "untouched" half is settled by reading the diff; the suite-green half needs `go test ./...` executed to settle.
