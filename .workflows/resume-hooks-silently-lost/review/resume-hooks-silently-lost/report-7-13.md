TASK: resume-hooks-silently-lost-7-13 — "The Doctor Renderers Are Unbound To The Reason Constants And Neither Is Exhaustive" (tick-79f063)

ACCEPTANCE CRITERIA:
- Every declared `skipReason*` value has a non-empty entry in both maps, including `lock-timeout` in `notEvaluableDetails` (absent at authoring time).
- Neither map holds a key outside the declared set.
- The two lookup functions are replaced by one, with the fall-through preserved as the runtime net.
- Adding a reason without a phrase fails the guard rather than compiling.
- Every rendered phrase is byte-identical to what it renders today except where Task 34 deliberately changes it.

STATUS: complete

SPEC CONTEXT:
`specification.md:255-257` signs off the `Skipped stale hook prune: …` copy for the `--fix` surface, and `:504` requires the sweep and the read-only check to stand down together, "under its own reason and phrase". Corrigendum `:552` is directly on point for this task's first criterion: it withdraws the third signed-off line ("could not read live panes"), splits the empty-live-set guard from the failed enumeration, and records that `notEvaluableDetails` "also gains its missing `lock-timeout` entry (`hooks.json is locked (not evaluable)`), so all five reasons render a phrase on both surfaces — vocabulary completeness rather than an observed leak". This task supplies the enforcement that keeps that completeness true; the entry itself was filled by T7-34.

IMPLEMENTATION:
- Status: Implemented (and legitimately carried forward by later phases)
- Location:
  - `internal/hooksweep/reason.go:12` (`type Reason string`), `:18-25` (the six declared reasons), `:35-42` (`Reasons`, the enumerable set)
  - `cmd/doctor.go:220-231` (shared phrase consts), `:236-243` (`skippedPrunePhrases`), `:247-254` (`notEvaluableDetails`), `:260-262` (the single `phraseFor`), `:266-268` / `:352-354` (the two renderers that consume it)
  - `cmd/doctor_stand_down_phrase_guard_test.go:47` (`TestStandDownPhraseCoverage`)
  - `internal/hooksweep/reason_enumeration_guard_test.go:22` (`TestReasonsEnumeratesEveryDeclaredConst`)
  - Delivering commit: `398c883f`; carried by `bb738fb4` (T8-48, typed vocabulary) and `a4898f41` (T9-12, sweep moved to `internal/hooksweep`)
- Notes:
  - Criterion 1 holds at HEAD: both maps carry a non-empty entry for all six declared reasons, including `hooksweep.ReasonLockTimeout` in `notEvaluableDetails` (`cmd/doctor.go:253`). The set grew from five to six (`ReasonMarkerReadFailed`) in a later task and both maps grew with it — which is exactly the guard doing its job.
  - Criterion 2 holds: each map holds precisely the six declared keys, and `cmd/doctor_stand_down_phrase_guard_test.go:64-71` fails on any key outside `hooksweep.Reasons`.
  - Criterion 3, the single lookup, holds (`cmd/doctor.go:260`); `skippedPrunePhrase` and `notEvaluableDetail` are gone from the tree (no `skipReason*` identifier survives anywhere). The **fall-through was later removed on purpose** by T8-48 (`bb738fb4`): `phraseFor` now returns the zero value for an unmapped reason, with the reasoning stated at `cmd/doctor.go:257-259` ("exhaustiveness is a test's to enforce, and a runtime fall-through would only make internal words look like copy") and pinned by an assertion that it renders nothing (`cmd/doctor_stand_down_phrase_guard_test.go:89-91`). This is a divergence from this task's wording, not a loss: the key type is now `hooksweep.Reason`, every value reaching the renderers originates from the declared const block (`internal/hooksweep/sweep.go:201`, `internal/hooksweep/standdown.go:42,48`), and both maps are guarded exhaustive — so the net has nothing left to catch. Judged sound; recorded rather than reported.
  - Criterion 4 holds through two guards rather than one, which is the right split: the coverage guard ranges over `hooksweep.Reasons`, and the source guard (`internal/hooksweep/reason_enumeration_guard_test.go:22`) reads the declarations off the AST keyed on the `Reason` *type*, so a reason declared and left out of the slice — the hole the first guard structurally cannot see — fails too.
  - Criterion 5 holds for this task's own commit: `398c883f` moves both map blocks verbatim (the diff is a pure relocation, no value edited). Subsequent wording changes are later tasks' deliberate work — the shared-phrase consts at `cmd/doctor.go:220-226` and the new marker-read reason.
  - Do-step 2 asked for the maps to sit beside the const block; they now sit beside the renderers in `cmd/doctor.go` instead, because the const block moved into `internal/hooksweep`, which is a library that must not own user-facing copy. `CLAUDE.md` records this as the intended home ("the user-facing copy tables … sit in `cmd` beside the renderers that print them"), so the record and the code agree.

TESTS:
- Status: Adequate
- Coverage:
  - Both criteria-1 assertions, one per surface (`doctor_stand_down_phrase_guard_test.go:52-62`), keyed on the live `hooksweep.Reasons` rather than a restated list.
  - The undeclared-key direction (`:64-71`), which is what a retired reason would strand.
  - The guard's own failure mode (`:73-93`): `missingPhrases` is exercised against a synthetic seventh reason and must report exactly it, so a rule that silently stopped catching omissions fails rather than passes.
  - The enumeration hole (`internal/hooksweep/reason_enumeration_guard_test.go:31-56`), including its own failure mode against synthetic source declaring an off-convention typed const.
  - The user-visible end of it stays covered by `cmd/doctor_stand_down_copy_test.go` — every reason has a copy case with no subtraction list (`:342-358`), phrases are distinct per surface (`:364-379`), and no phrase contains its own slug (`:411-420`).
- Notes: The plan's fifth named test, "it falls through to the raw reason for an unmapped value", existed as delivered and was inverted by T8-48 into the "renders nothing" assertion at `:89-91` — consistent with the deliberate fall-through removal above, not a dropped test. Nothing here is over-tested: the two guards police different failure modes (map coverage vs. set membership), and each carries exactly one self-check.

CODE QUALITY:
- Project conventions: Followed. Both guards run in the unit lane and scan their own package's sources, so `go test` cache invalidation is honest. The AST primitives are taken from `internal/sourceguardtest` (`ParsePackageSources`, `ParseSources`) rather than hand-rolled — matching the repo's ~20 other source guards.
- SOLID principles: Good. `phraseFor` takes the vocabulary as a parameter, so the two surfaces share a lookup without sharing words; `missingPhrases` / `undeclaredKeys` are named rules, which is what makes the guard's own behaviour testable.
- Complexity: Low. The lookup is one map read; the guards are flat traversals.
- Modern idioms: Yes — `slices.Clone`/`Contains`/`Equal`/`Sort`, `for key := range m`, a defined `Reason` type over stringly-typed constants.
- Readability: Good. Every declaration carries a comment saying what it defends and what would go wrong without it; `internal/hooksweep/reason.go:30-34` pre-empts the reader who finds `lock-timeout`'s phrase unreachable.
- Comment accuracy: Verified against the code. `cmd/doctor.go:257-259` correctly describes the post-T8-48 no-fall-through behaviour; `internal/hooksweep/reason_enumeration_guard_test.go:15-21` correctly describes what the type cannot hold and the guard still adds. `doctor_stand_down_phrase_guard_test.go:54` and `:60` say "would print the raw slug" where the current runtime would print nothing — the failure they name is stale by one task, but the remedy is comment text and the diagnostic still points at the right omission, so it is below the reporting bar.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
