TASK: open-with-forced-filter-11-1 (tick-333efc) — Make the search-form collision check fail closed on a flag it does not name

ACCEPTANCE CRITERIA: [from the plan task]
1. `validateSearchFormCollisions` names exactly `"exec"`, `"filter"` and `"ack"` as literals (plus the `anyOpenDomainPin` call) and nothing more; the rest of the verdict is read from the registration through `LocalFlags().Visit`.
2. A search form beside a flag no named arm covers is refused with `cannot use a /term search with --<name>`, and the line opens no picker, resolves no target and starts no bootstrap.
3. Every refusal today's seven flags produce is byte-identical and still fires from its own named arm, in the current order — the collision suite at `cmd/open_search_test.go:337-401` passes with no edit.
4. A search-form line that set no flag is still admitted: `portal open /port` and `portal open /` reach the search.
5. `portal open /port --help` still prints usage, and the arm carries no carve-out for cobra's auto-registered `help` flag.
6. The walk is over `LocalFlags` rather than `Flags`, so a persistent flag inherited from the root command stays outside the rule.

STATUS: complete

SPEC CONTEXT:
§5.1 states the rule as a closed one over the set: "`/term` composes with nothing. Any other target, any trailing command, and any of `open`'s own flags on the same line is a usage error." Two carve-outs sit beside it: "Flags that answer before the command body runs are outside the rule: `portal open /term --help` prints help, and root-level persistent flags apply as they do to any other invocation." The section also fixes two further properties the task inherits — "A refusal names what collided" (one message naming the search form and the element beside it) and "A refused line starts nothing" (decided from the arguments alone, so no server, no restore, no frame). Because the rule is closed over `open`'s own flag set, reading the verdict from the registration rather than from a restated list is exactly what the spec's wording authorises.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/open_search.go:98-106` — the derived final arm, appended after the six existing arms inside `validateSearchFormCollisions`.
  - `cmd/open_search.go:109-126` — `firstSetLocalFlag`.
  - `cmd/open_search.go:83-96` — the six named arms, unchanged in body and order (verified against `git show f25d236d0`: the commit is a pure insertion of 31 lines in this file, no line deleted or modified).
  - `cmd/open.go:774-781` — the seven registered flags the named arms cover.
- Notes:
  - Criterion 1 holds exactly: the switch carries the three literals `"exec"` (`:88`), `"filter"` (`:90`), `"ack"` (`:94`) plus `anyOpenDomainPin(cmd)` (`:92`, which reads `openDomainPinFlags` at `cmd/open.go:250`), and no other flag name appears in the function.
  - **Deliberate, correct divergence from the task's `Do` step.** The plan prescribed `cmd.LocalFlags().Visit`; the implementation uses `LocalFlags().VisitAll` filtered on `f.Changed` (`cmd/open_search.go:120-124`) and documents why at `:113-117`. The prescribed form would have been silently inert, leaving the check failing open exactly as before: cobra builds `c.lflags` with `AddFlag` (`cobra@v1.10.2/command.go:930-936`), which populates only `formal` (`pflag@v1.0.9/flag.go:871-891`), while `Visit` iterates `f.actual` (`pflag@v1.0.9/flag.go:348-367`) — a map only a parse of *that* set fills, and nothing ever parses `lflags`. The `*pflag.Flag` values are shared with `c.Flags()`, so the `Changed` field the implementation reads is accurate. Substance of criterion 1's second half ("the rest of the verdict is read from the registration") and criterion 6 (LocalFlags scope) are both preserved; only the mechanism differs, and the written mechanism was the broken one. The comment's claims were checked against pflag v1.0.9 and cobra v1.10.2 and all hold, including "pflag visits lexically" (`NewFlagSet` sets `SortFlags: true`, and `LocalFlags` copies `c.Flags().SortFlags` onto `c.lflags`).
  - Criterion 5 verified structurally: `openCmd`'s `help` flag is in `LocalFlags` and carries no carve-out, but cobra resolves it at `cobra@v1.10.2/command.go:927-936` (`helpVal` → `return flag.ErrHelp`), ahead of `c.ValidateArgs(argWoFlags)` at `:968`, so the validator never sees a line that set it. The completion path cannot reach the arm either: `cobra@v1.10.2/completions.go` never calls `ValidateArgs` (grepped — no match), which keeps `cmd/completion.go:85-103`'s deliberate "complete the word, refuse on Enter" contract intact.
  - Criterion 6 verified structurally: `LocalFlags` excludes a parent persistent flag by pointer comparison against `c.parentsPflags` (`cobra@v1.10.2/command.go:930-936`), whereas `c.Flags()` holds the merged set after `mergePersistentFlags`. The carve-out is currently vacuous — `PersistentFlags` is registered nowhere in `cmd/` or `internal/` — so the choice is future-proofing that matches the spec rather than a live behaviour difference.
  - Criterion 2's "starts no bootstrap" half holds by construction: the arm returns from `validateOpenArgs`, wired as `openCmd`'s `Args` validator (`cmd/open.go:160`), and cobra runs `ValidateArgs` (`:968`) before the `PersistentPreRunE` walk that follows it.
  - The arm is unreachable for every line that exists today — all seven registered flags are caught by an earlier arm — so no refusal `open` currently emits changes wording or origin.
  - No drift: `validateOpenArgs`, `searchFormPositionals` and `preDashPositionals` are untouched, as the task required.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/open_search_test.go:414-429` — `TestValidateSearchFormCollisions_RefusesAFlagNoNamedArmCovers`: crafted `open`-shaped command carrying `--zzz`, set, asserts the `*UsageError` message is exactly `cannot use a /term search with --zzz`. This test is the guard against the `Visit` regression: with `Visit` the walk reports nothing, the function returns nil, and the test fails.
  - `cmd/open_search_test.go:431-437` — `TestValidateSearchFormCollisions_AdmitsASearchFormThatSetNoFlag`: same command with the flag registered but unset, asserts admission. Covers criterion 4's shape at the arm.
  - `cmd/open_search_test.go:406-412` — `craftedOpenCommand` helper, with the reason the real `openCmd` is never used stated in its doc comment (pflag cannot un-register a flag).
  - The fall-through the crafted command depends on was verified rather than assumed: an unparsed `pflag.FlagSet` reports `argsLenAtDash == -1` (`NewFlagSet`), and `FlagSet.Changed` on an absent flag returns false, so the six named arms all decline on a crafted command.
  - Criteria 3/4/5 rest on existing unchanged tests, which the diff confirms were not touched: the seven per-flag refusal tests at `:337-404`, `TestValidateOpenArgs_RefusedLineStartsNoBootstrap` at `:439`, `TestValidateOpenArgs_StillAnswersHelpOnASearchFormLine` at `:454`, and the real-`Execute` admission tests (`TestOpenCommand_SearchForm_*`, e.g. `:302`) which drive `rootCmd.Execute()` with the real flag set — including the auto-registered `help` flag — and so also cover "the new arm fires on no line that exists today".
- Notes:
  - Not over-tested: two tests, one assertion each, no mocking, no setup beyond a four-line command constructor.
  - Independence holds — each test builds its own `*cobra.Command`, so neither depends on `resetRootCmd`'s per-flag `Changed` reset (`cmd/root_test.go:39-71`), which does cover all seven `openCmd` flags plus the help flag, so the new arm cannot mis-fire from cross-test leakage on the real command either.
  - Criterion 6 is settled by reading but is not test-guarded, and cannot be without inventing a root persistent flag that does not exist. Nothing observable breaks today if the walk were swapped to `Flags()`; noted as an observation, not a finding.

CODE QUALITY:
- Project conventions: Followed. Tests live in the file named after the source file under test, carry no `t.Parallel()` (prohibited package-wide), and the new arm adds no package-level seam, so `cmd/seam_guard_test.go` is unaffected. `firstSetLocalFlag` is placed directly beneath its only caller, matching the file's existing helper-after-caller layout.
- SOLID principles: Good. The helper answers one question, takes the `*cobra.Command` it reads from, and the arm composes it with the named arms without touching them.
- Complexity: Low — one loop over ~8 flags, entered only on a line that already carries a search form.
- Modern idioms: Yes. The `first == ""` accumulation is forced by pflag's no-early-exit visitor and is documented as such at `cmd/open_search.go:117`.
- Readability: Good. Both comments explain why rather than restating what, and neither references a task id, phase or spec section.
- Comment accuracy: Verified against pflag v1.0.9 and cobra v1.10.2 — every claim in the `firstSetLocalFlag` doc comment (`:109-117`) and the arm's comment (`:98-102`) holds.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "Every refusal today's seven flags produce is byte-identical and still fires from its own named arm, in the current order — the collision suite at `cmd/open_search_test.go:337-401` passes with no edit." — reading settles the "no edit" and the arm ordering (the commit is a pure insertion; the six arms are byte-identical and the derived arm is last), but "passes" needs `go test ./cmd -run 'TestValidateOpenArgs|TestValidateSearchFormCollisions|TestOpenCommand_SearchForm'` to be observed. The same run settles the two new tests and the unchanged `--help` and admission tests behind criteria 2, 4 and 5.
