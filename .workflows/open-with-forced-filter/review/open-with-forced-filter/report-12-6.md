TASK: open-with-forced-filter-12-6 (tick-e538df) — Let a word past the separator complete as the trailing command's own

ACCEPTANCE CRITERIA:
1. `portal __complete open ~/Code/api -- ls src/` answers with no candidates and `cobra.ShellCompDirectiveDefault`, so the shell's filename completion runs.
2. Every pre-separator positional is unchanged: session-name candidates with `ShellCompDirectiveNoFileComp` for a plain word, a word beside another target, and a word on a line carrying `-f`.
3. The boundary word immediately after the separator is unchanged: `open -- /po` and `open api -- /po` still offer sigil candidates.
4. `completeSessionNames` and `completeSearchTerm` are untouched and still return `NoFileComp`, and `completeOpenPositional`'s doc comment no longer claims a post-separator word is completed against session names.
5. §8.1's separator paragraph states the post-separator answer as no candidates plus the shell's filename completion; the 2026-09-15 corrigendum no longer claims the arm the bound selects suppresses filenames; one new dated corrigendum entry records the correction; the knowledge re-index has run and its store changes are committed with the task rather than left dirty.
6. `go test ./...` is green and `golangci-lint run` reports nothing new.

STATUS: complete

SPEC CONTEXT: §8.1 ("Completion looks past the sigil") bounds sigil completion at the `--` separator, as §2.3 bounds recognition, and names the cost of getting the post-separator answer wrong twice: at `specification.md:332` ("would suppress the filenames the user actually wanted in the same breath") and `:334` ("offering sigil candidates costs the user their filenames, so that bound corrects two wrongs where this one would correct none"). §2.3 states the words after a separator are the trailing command's own, passed to that command untouched. The 2026-09-15 corrigendum at `:494` previously asserted that the arm the bound *selects* — the session-name arm — also suppressed those filenames, which made the "corrects two wrongs" conclusion self-contradictory. This task makes the behaviour match the reason the spec gives.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/completion.go:104-111` (the arm), `cmd/completion.go:85-103` (the doc comment), `specification.md:332` and `:496`. Commits `756d46a23` (code + tests) and `4b6fbd18e` (spec + knowledge store).
- Notes:
  - `completeOpenPositional` now opens with `if !completingPreDashPositional(cmd, args) { return nil, cobra.ShellCompDirectiveDefault }` (`cmd/completion.go:105-107`), ahead of the sigil arm (`:108-110`) and the session-name fallthrough (`:111`). Both pre-separator arms are byte-unchanged in behaviour; the restructure from one compound condition to a leading guard is the minimal shape for the new third answer and reads cleanly.
  - The gate itself (`cmd/open_search.go:46-49`) is untouched, so the boundary word immediately after the separator still reports pre-dash (`dash >= 0 && len(args) <= dash`) and keeps the two pre-separator arms — criterion 3 holds structurally, not just by test.
  - The new arm cannot mis-fire on a pre-separator word: `completingPreDashPositional` returns false only when a `--` was parsed *and* `len(args) > dash`, and cobra's probe parse leaves `dash == len(args)` for a separator-free line, so a word ahead of a real separator never reaches the Default arm.
  - `completeSessionNames` (`cmd/completion.go:32-40`, `NoFileComp` at `:39`) and `completeSearchTerm` (`:70-83`, `NoFileComp` at `:82`) are untouched — criterion 4's first half holds.
  - Doc comment (`:85-103`) now states the post-separator word is "passed to it untouched, so Portal offers it nothing and leaves the shell's own filename completion on", keeps the boundary-word exception and keeps the "separator is the only bound" paragraph with its tail corrected to "it is not Portal's to complete". No claim in it is falsified by the code; no process artefacts (task ids, phase numbers, spec §refs) appear in it.
  - Spec: `:332`'s falsified clause is replaced in place; `:334`, `:340`, `:376` and every earlier corrigendum are byte-untouched (confirmed against `git show 4b6fbd18e -- …/specification.md` — the diff touches exactly two lines plus the appended entry). The 2026-09-15 entry at `:494` has the self-contradicting clause replaced by "it is not Portal's to complete", keeping its "corrects two wrongs" conclusion, which now holds. One new dated entry lands at `:496`, quoting both corrected claims and stating the new behaviour; its date matches `date` on this machine (2026-09-15).
  - The knowledge re-index ran and is committed with the spec edit: `.workflows/.knowledge/metadata.json` and `.workflows/.knowledge/store.msp` are both in `4b6fbd18e`, and `git status` shows no dirty `.knowledge` files — criterion 5's last clause holds.
  - No other source or doc carries a falsified claim about this path: `README.md:175`'s "a path argument after the function does not fall through to filename completion" is about `x`'s own pre-separator positional and stays true; `README.md:157`'s separator sentence is about recognition and is unaffected.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/completion_test.go:601-612` — "it offers no session names for a word among a trailing command's arguments": `__complete open ~/Code/api -- ls /po` end-to-end through the real root, asserting `ShellCompDirectiveDefault` and zero candidates. The directive assertion is what discriminates: reverting to the session-name arm flips it to `NoFileComp` and the test fails.
  - `cmd/completion_test.go:614-625` — "it leaves filename completion on for a path argument to the trailing command": the exact reported failure `__complete open ~/Code/api -- ls src/`, same two assertions. Covers criterion 1 directly.
  - `cmd/completion_test.go:589-599` — the sigil-suppression subtest past the separator, unchanged and still green under the new arm.
  - `cmd/completion_test.go:657-675` — the two boundary-word subtests (`open -- /po`, `open api -- /po`) are unchanged; they are the guard that the new arm does not swallow the boundary position (criterion 3).
  - Criterion 2's pre-separator contract is pinned in three places: the plain-word table at `:477-511` (session names + `NoFileComp`, including the empty word), "it leaves a second positional on today's behaviour" at `:568-583` (`open /term <TAB>` → both session names + `NoFileComp`), and the `-f`-line subtest at `:647-655`, which exercises the same gate the change touched.
  - The old `"it answers a post-dash word exactly as the plain session-name completer does"` subtest is gone, replaced rather than deleted — correct, since its identity-with-`completeSessionNames` premise is exactly what stopped holding.
- Notes:
  - One test change beyond the task's Do-list is sound rather than drift: `"it leaves a second positional on today's behaviour"` (`:568`) moved from a direct `openCmd.ValidArgsFunction(...)` call to `completionResult(t, "__complete", "open", "/term", "")`, with a comment giving the reason. It is necessary: the new leading guard reads `cmd.ArgsLenAtDash()` on the first line, and a direct call inherits whatever dash index a sibling test's parse last left on the shared `openCmd` (a sibling running `__complete open -- /po` leaves `dash == 0`, which would make the direct call take the new Default arm and fail the test on ordering alone). Routing through `__complete` parses the line being asserted about.
  - Not over-tested: the two post-separator subtests overlap on their assertions but differ in the word being completed — `/po` proves no session names and no sigil, `src/` is the user-reported symptom the criterion names. Both are in the task's Tests list and neither is redundant against the other.

CODE QUALITY:
- Project conventions: Followed. The change stays inside `cmd`'s completion seam, adds no new package-level state, invents no log component or attr, and touches none of the `*Deps` / function-seam machinery. Test-side staging goes through `withFuncSeam` via `withCompletionSessions`, as the seam guard requires.
- SOLID principles: Good. The guard-first shape gives `completeOpenPositional` three exclusive arms with one reason each; the separator rule stays owned by `completingPreDashPositional` (`cmd/open_search.go:46-49`) rather than being restated at the call site.
- Complexity: Low. Three straight-line branches, no new state, no new dependency.
- Modern idioms: Yes. Early-return guard over a compound condition; `nil` candidates with an explicit directive matches how `--path` / `--zoxide` already hand the word to the shell (pinned at `cmd/completion_test.go:207-217`).
- Readability: Good. The doc comment states the three answers and the boundary exception in the order the code takes them.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` is green and `golangci-lint run` reports nothing new." — needs the unit lane and the linter run. Reading settles the logic (the changed arm compiles against the existing imports, `slices`/`strings` stay used in the test file, and no assertion in the touched suites contradicts the new behaviour), but neither a green suite nor a clean lint report can be established by reading.
