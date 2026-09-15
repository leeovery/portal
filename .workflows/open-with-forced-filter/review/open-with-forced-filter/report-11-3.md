TASK: open-with-forced-filter-11-3 (tick-13a587) — Bound the positional completer's sigil arm where the parser bounds it

ACCEPTANCE CRITERIA:
1. `portal __complete open ~/Code/api -- ls /po` offers no `/`-prefixed candidate: it answers exactly what `completeSessionNames("/po")` answers, with `ShellCompDirectiveNoFileComp` — the arm every other non-sigil word takes.
2. Pre-dash completion is unchanged: `portal __complete open /po` still offers `/portal-a1b2`, `portal __complete open /` still offers the whole searched set, `portal __complete open api /po` still takes the sigil arm, and every non-sigil word answers as it does today.
3. The separator bound is stated once, in `cmd/open_search.go` beside `preDashPositionals` and read from `cmd.ArgsLenAtDash()`; `cmd/completion.go` states no rule of its own about `--`.
4. A line carrying no `--` completes as pre-dash even though cobra's probe parse leaves a non-negative dash index on the flag set.
5. Accepted residual, pinned rather than fixed: the word immediately after the separator keeps today's sigil arm.

STATUS: complete

SPEC CONTEXT:
specification.md:56 (§2.3) bounds sigil *recognition* at the `--` separator — words past it are the trailing command's own and a `/word` among them is never a sigil. §8.1 originally stated the completion rule with no such bound; three 2026-09-15 corrigenda (specification.md:492, :494, :496) close the gap. The final rule now sits at specification.md:332: "Completion is bounded by the `--` separator, as recognition is (§2.3), and by nothing else. A `/word` the separator leaves among the trailing command's own arguments … is answered with no candidates and the shell's own filename completion left on … One position is outside the bound and stays as it is: the word immediately after the separator is, at the flag layer the completer reads, indistinguishable from the next ordinary positional, so it keeps the sigil arm." specification.md:334 pins the other half — the separator is the *only* bound, so a sigil is still offered on a line `validateSearchFormCollisions` will refuse.

IMPLEMENTATION:
- Status: Implemented (later superseded in one detail by a subsequent plan task — see Notes)
- Location:
  - `cmd/open_search.go:36-49` — `completingPreDashPositional(cmd, args) bool`, sited immediately below `preDashPositionals` (`:29-34`), reading `cmd.ArgsLenAtDash()` and returning `dash < 0 || len(args) <= dash`.
  - `cmd/completion.go:104-112` — `completeOpenPositional(cmd *cobra.Command, args []string, toComplete string)`, gating on the predicate before the sigil arm.
  - `cmd/open.go:785` — `openCmd.ValidArgsFunction = completeOpenPositional`, the closure that discarded `cmd`/`args` removed; the `-p`/`-z` note above it and the two `RegisterFlagCompletionFunc` registrations below it are untouched.
  - Delivered at c797fc9b3; `cmd/completion.go`'s post-dash arm subsequently rewritten at 756d46a23 (task 12-6).
- Notes:
  - **Criterion 1's letter no longer holds, deliberately, and the substance does.** As delivered at c797fc9b3 the post-dash word fell through to `completeSessionNames` exactly as criterion 1 prescribes. Task 12-6 (756d46a23, "let a word past the separator complete as the trailing command's own") replaced that with an early `return nil, cobra.ShellCompDirectiveDefault` at `cmd/completion.go:105-107`. That is a later task of the same plan and the spec was amended to match (specification.md:496 records the reason: the session-name arm carries `NoFileComp`, so falling to it past the separator suppressed the very filenames §8.1 twice names as the cost worth avoiding — `portal open ~/Code/api -- ls src/<TAB>` answered with silence). Criterion 1's operative clause — "offers no `/`-prefixed candidate" — is met, and the change is a strict improvement on the word it superseded. Not a finding.
  - The `<=` in the predicate is correct and load-bearing, verified against the dependencies rather than taken on the task's word: cobra probe-parses with an appended separator at `github.com/spf13/cobra@v1.10.2/completions.go:369` (`_ = finalCmd.ParseFlags(append(finalArgs, "--"))`) and re-parses the real line at `:373`; `pflag@v1.0.9/flag.go:1207-1209` (`ParseAll`) resets `f.args` but never `argsLenAtDash`, which is assigned only when a `--` is met (`:1145`) and set to -1 only by `NewFlagSet`/`Init` (`:1268`, `:1286`). So a separator-free line reports `dash == len(args)`, and `<` would silently delete the sigil arm outright.
  - Traced the boundary by hand across the shapes that matter: `open ~/Code/api -- ls /po` → args=["~/Code/api","ls"], dash=1 → post-dash; `open -- ls /po` → args=["ls"], dash=0 → post-dash; `open /po` → args=[], dash=0 → pre-dash; `open api /po` → args=["api"], dash=1 → pre-dash; `open --filter api /po` → args=[], dash=0 → pre-dash; `open -- /po` and `open api -- /po` → byte-identical to the separator-free forms, which is precisely the residual criterion 5 pins.
  - One rule, one home: `grep` over `cmd/` non-test sources finds `completingPreDashPositional` declared once and called once, and `cmd/completion.go` encodes no `--` test of its own.

TESTS:
- Status: Adequate
- Coverage (`cmd/completion_test.go:586-695`, `TestCompleteOpenPositionalSeparatorBound`, every end-to-end case driven through `completionResult`/`completionCandidates` so cobra's own two-pass parse populates the boundary rather than a hand-set index):
  - `:589` "it offers no sigil completion for a word among a trailing command's arguments" — the reported failing line, asserting no `/`-prefixed candidate.
  - `:599` "it offers no session names for a word among a trailing command's arguments" — candidates *and* directive (`ShellCompDirectiveDefault`); this is 12-6's replacement for the task's named `"it answers a post-dash word exactly as the plain session-name completer does"`, and it is the stronger assertion.
  - `:612` "it leaves filename completion on for a path argument to the trailing command" (`-- ls src/`) — the non-sigil post-dash word, the case specification.md:496 names.
  - `:625` / `:635` / `:645` — pre-dash unchanged on a separator-free line, beside another target, and beside `-f`.
  - `:655` / `:665` — the criterion-5 residual pinned at both arities (`open -- /po`, `open api -- /po`).
  - `:675` "it reads a line with no separator as pre-dash despite cobra's probe parse" — driven over the predicate directly on a command parsed the way cobra parses it (`ParseFlags(["api","--"])` then `ParseFlags(["api"])`), asserting first that the dash index really is non-negative (`:688`) so the test cannot pass vacuously if cobra's probe ever stops leaking.
  - The two direct callers the task named were updated: `:467` and the table at `:497` now pass `&cobra.Command{}, nil` (fresh command ⇒ `ArgsLenAtDash() == -1` ⇒ pre-dash, so those cases still exercise the arms they name); `:520` already supplied both.
  - Every one of these would fail if the gate were removed, inverted, or tightened to `<`.
- Notes:
  - `:589` and `:599` drive the identical argv and the second subsumes the first (no candidates at all implies no `/`-prefixed candidate). Both are plan-named cases from 11-3 and 12-6 respectively, and the narrower one states the originally-reported symptom in its own words; the overlap is deliberate rather than bloat.
  - Criterion 2's `portal __complete open /` clause has no end-to-end case of its own; the searched set for a bare `/` is pinned at the unit level (`completeSearchTerm("/")`, `:445-453` and `:476-483`). The gate this task added is independent of `toComplete`, so nothing about the bound is left unobserved by the absence.

CODE QUALITY:
- Project conventions: Followed. The predicate lives beside the parser-side rule it mirrors rather than in the completer, which is what keeps the two surfaces from restating the bound; the registration hands cobra the function itself instead of wrapping it in a closure that discards arguments.
- SOLID principles: Good — one exported behaviour per function, the boundary decision separated from the arm selection.
- Complexity: Low. `completeOpenPositional` is three straight-line branches; the predicate is one expression.
- Modern idioms: Yes.
- Readability: Good. The guard-clause form 12-6 landed reads better than the compound `IsSearchSigil(...) && completingPreDashPositional(...)` it replaced, since the boundary now visibly precedes both arms.
- Comment accuracy: Verified rather than assumed. The `<=` rationale at `cmd/open_search.go:42-45` is true of cobra v1.10.2 and pflag v1.0.9 as vendored (citations above). The residual described at `cmd/completion.go:90-93` is true: `open -- /po` and `open /po` both reach the completer as `len(args)==0, dash==0`. No process artifacts (task ids, phase numbers, spec section numbers) appear in either comment.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
