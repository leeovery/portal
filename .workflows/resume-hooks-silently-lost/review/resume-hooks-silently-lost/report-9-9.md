TASK: resume-hooks-silently-lost-9-9 — "The Stand-Down Reason Vocabulary Is Guarded Three Ways And Carries Members No Surface Reaches" (tick-e4a6b3, phase 9, severity low, source: architecture)

ACCEPTANCE CRITERIA:
1. `skipReasons` holds six members and every one of them is a reason a cycle declined under.
2. `phraseFor` has no fallback branch; a reason absent from a map renders empty and the coverage guard fails.
3. The `--fix` output for a failed sweep is byte-identical to today's.
4. The copy-uniqueness guard ranges over `skipReasons` with no subtraction list and passes.
5. Both phrase maps are exhaustive over the six and carry no extras.

STATUS: complete

SPEC CONTEXT:
This is a phase-9 implementation-analysis task, so its authority is its own body rather than the specification. The only adjacent spec material is §5.1's signed-off stand-down copy and the 2026-09-01 corrigendum, which withdrew "could not read live panes", split the empty-live-set guard from the failed enumeration, and added the `lock-timeout` not-evaluable entry "for vocabulary completeness … since lock-timeout cannot reach that path today". That corrigendum's five reasons plus `lock-timeout` are exactly the six the task leaves standing; nothing in §5.1 or the corrigenda names a "sweep-failed" reason, so removing it from the reason type contradicts no signed-off copy. CLAUDE.md's `hooksweep` row is likewise consistent with the end state: it describes `Reason` as "the closed stand-down vocabulary", `Reasons` as its enumerable form, and the `skippedPrunePhrases` / `notEvaluableDetails` copy tables as living in `cmd` beside their renderers.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hooksweep/reason.go:9-25` — the `Reason` type and its six-member const block; `sweep-failed` is absent and the block's doc says so explicitly ("A sweep that ran and failed is not among them: it declined nothing").
  - `internal/hooksweep/reason.go:35-42` — `Reasons` enumerates exactly those six.
  - `cmd/doctor.go:236-243` / `cmd/doctor.go:247-254` — `skippedPrunePhrases` and `notEvaluableDetails`, six rows each, keyed on `hooksweep.Reason`, no `sweep-failed` row.
  - `cmd/doctor.go:260-262` — `phraseFor` is now the bare map lookup; the `if phrase, ok := …; ok { … } return string(reason)` fallback is gone.
  - `cmd/doctor.go:228-231` — `sweepFailedStandDownPhrase` survives as a plain const outside the shared stand-down const block.
  - `cmd/doctor.go:274-276` — the new `reportFailedPrune(w)` renderer, called from `cmd/doctor.go:203` in place of the old `reportSkippedPrune(w, skipReasonSweepFailed)`.
  - `cmd/doctor_stand_down_copy_test.go:342-358` — the copy-uniqueness/coverage guard now ranges over `hooksweep.Reasons` whole; no subtraction list remains.
  - The identifiers `skipReason`, `skipReasonSweepFailed` and `notStandDownReasons` return zero matches across every `.go` file in the tree (verified by a repo-wide grep, `skipReason`/`notStandDownReasons`/`ReasonSweepFailed`), so nothing was left half-retired. `phraseFor` has exactly two production call sites (`cmd/doctor.go:267` and `cmd/doctor.go:353`) and `reportSkippedPrune` exactly one (`cmd/doctor.go:210`).
- Notes:
  - Byte-identity holds by construction. Before: `reportSkippedPrune(w, skipReasonSweepFailed)` formatted `"Skipped stale hook prune: %s\n"` against `skippedPrunePhrases[skipReasonSweepFailed]`, which was `sweepFailedStandDownPhrase`. After: `reportFailedPrune` formats the same verb against the same const. Confirmed against the 9-9 diff (`git show e274117b -- cmd/doctor.go cmd/run_hook_stale_cleanup.go`).
  - Deleting the fallback cannot strand a reason at runtime. `Outcome.DeclineReason` is only ever set by `standDownOutcome` (`internal/hooksweep/sweep.go:199-202`), whose `StandDown` values all originate in `declineDebug`/`declineWarn` (`internal/hooksweep/standdown.go:41-49`) with one of the six named constants; `cmd/doctor.go:209` gates on a non-empty reason before rendering. The two guards close the remaining gap in opposite directions: `TestReasonsEnumeratesEveryDeclaredConst` (`internal/hooksweep/reason_enumeration_guard_test.go:22`) proves `Reasons` matches the declared const set (keyed on the type, so an off-convention name is still caught), and `TestStandDownPhraseCoverage` (`cmd/doctor_stand_down_phrase_guard_test.go:47`) proves both maps are exhaustive over `Reasons` and hold no undeclared keys.
  - The `sweepFailedStandDownPhrase` name still carries the `StandDownPhrase` suffix its own doc disclaims — that is load-bearing, not residue: `phraseConstSuffix` (`cmd/doctor_stand_down_phrase_guard_test.go:211`) is how `TestStandDownPhrasesAreSpelledOnlyInTheirDeclaredHome` finds the consts it forbids re-spelling, so renaming it would drop "the sweep could not complete" out of the respelling guard's reach.
  - Later phase-9 commits moved this code: `cmd/run_hook_stale_cleanup.go` no longer exists (9-12 moved the cycle to `internal/hooksweep` and renamed `skipReason*` → `Reason*`), and 9-10 lifted `lockStandDownPhrase`. The 9-9 outcome survives both moves intact.

TESTS:
- Status: Adequate
- Coverage: All four named tests exist and each pins the criterion it is named for.
  - `"it renders the same failed-sweep line for --fix after sweep-failed leaves the reason type"` — `cmd/doctor_fix_hook_prune_report_test.go:45-63`. Pins the literal `Skipped stale hook prune: the sweep could not complete` twice over: directly from `reportFailedPrune` into a buffer, and end-to-end through `runDoctorWith(t, deps, "--fix")` over `failingSweepDeps` (a genuinely stale entry with `WritesDenied`, which is the one path leaving the cycle an error rather than a stand-down). It also pins the single WARN under the `hooks` component. This is the AC-3 test and it would fail on any re-wording.
  - `"it fails the coverage guard when a declared reason is absent from a phrase map"` — `cmd/doctor_stand_down_phrase_guard_test.go:73-93`. Exercises the guard's own rule against a synthetic unmapped reason and then asserts `phraseFor(vocabulary, unmapped) == ""` for both maps. This is the AC-2 test: it is precisely what would fail if the fallback branch were restored.
  - `"it enumerates every stand-down reason with no subtraction list"` — `cmd/doctor_stand_down_copy_test.go:342-358`. Ranges over `hooksweep.Reasons` whole in both directions (no case reuses a reason; no declared reason lacks a case) with nothing subtracted. AC-4.
  - `"it renders a distinct phrase for each of the six reasons on both surfaces"` — `cmd/doctor_stand_down_copy_test.go:364-379`. Reads the *rendered* phrases rather than the table's expectations, which is the right subject for a uniqueness rule. I checked the twelve rendered strings by hand: all six skipped-prune phrases and all six not-evaluable details are pairwise distinct, so the assertion is not vacuously satisfied.
  - Supporting: `TestStandDownPhraseCoverage`'s first three subtests (`cmd/doctor_stand_down_phrase_guard_test.go:52-71`) carry AC-5 — exhaustive over the six, no extras — and the AST guard at `internal/hooksweep/reason_enumeration_guard_test.go:22-56` carries AC-1's "six and only six" from the declaration side, including its own negative case over synthetic source.
  - Would the tests fail if the change regressed? Yes at each point: restoring the fallback fails `cmd/doctor_stand_down_phrase_guard_test.go:89`; re-adding a `sweep-failed` member fails both `undeclaredKeys`/`missingPhrases` arms and the no-subtraction-list enumeration; re-wording the failed-sweep line fails `cmd/doctor_fix_hook_prune_report_test.go:46`.
- Notes: The failed-sweep line is asserted in three places — `cmd/doctor_fix_hook_prune_report_test.go:49` (direct), `:59` (end-to-end) and `cmd/hook_prune_one_component_test.go:92` (through `pruneDoctorStaleHooks`). This is not redundant testing of one thing: only the first pins the byte string (which is the acceptance criterion), the second pins that the `--fix` path reaches it and prints no competing line, and the third's subject is the one-record/one-component rule with the line composed from the const rather than restated. No over-testing worth flagging.

CODE QUALITY:
- Project conventions: Followed. Emission stays wholly inside `internal/hooksweep` under the `hooks` component (`internal/hooksweep/sweep.go:14`), with `cmd` keeping only its rendered lines — exactly the split CLAUDE.md's `hooksweep` row describes; the daemon caller discards the outcome (`cmd/state_daemon.go:214`) and adds no second line. The reason-type/const/enumerable-slice/AST-guard shape matches the closed-vocabulary pattern used elsewhere in the tree. No new log component or attr key was invented.
- SOLID principles: Good. Moving `sweep-failed` out of `Reason` restores the type's stated contract — the vocabulary now holds only what a caller may report as a decline, and the failure gets its own renderer instead of borrowing the decline path's.
- Complexity: Low. `phraseFor` reduces to a single expression; `reportFailedPrune` is a single `Fprintf`.
- Modern idioms: Yes. The guards use `slices.Contains` / `slices.Equal` / `slices.Clone`, and the copy suite composes filter chains through the sanctioned `logtest` query surface (`sink.Records().Matching(…).AtExactLevel(…)`).
- Readability: Good. The two renderers sit adjacent and each names why it is not the other.
- Issues: None. Every comment the task touched holds against the code:
  - `cmd/doctor.go:214-219` claims the const block holds phrases "both surface vocabularies share" — checked all five (`restore`, `markerRead`, `storeRead`, `paneRead`, `lock`); each is composed into both maps.
  - `cmd/doctor.go:228-231` claims no reason names `sweepFailedStandDownPhrase` — true; it appears in production only at `cmd/doctor.go:275`.
  - `cmd/doctor.go:256-259` claims exhaustiveness is a test's to enforce — true, and the test exists.
  - `internal/hooksweep/reason.go:31-34` claims `lock-timeout`'s not-evaluable phrase is unreachable "for vocabulary completeness". Still true: `staleHooksNotEvaluable` is reached only from `checkStaleHooks` (`cmd/doctor.go:363-392`), which passes `ReasonStoreReadFailed`, or a reason from `StalenessStandDown` (restoring / marker-read-failed) or `JudgeAgainstLivePanes` (pane-read-failed / empty-pane-read) — never the lock. The task's Solution and Do list deliberately did not touch this, so it is not drift.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
