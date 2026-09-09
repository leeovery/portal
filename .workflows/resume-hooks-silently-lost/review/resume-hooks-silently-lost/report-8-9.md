TASK: resume-hooks-silently-lost-8-9 — "doctor --fix Prints Nothing At All When The Sweep Genuinely Fails" (tick-fde1db)

ACCEPTANCE CRITERIA:
- A sweep that returns an error prints one line naming the failure, and still logs the WARN.
- No `--fix` invocation that reaches the hook sweep prints nothing about it.
- The printed phrase comes from `skippedPrunePhrases`, not an inline literal.
- `runHookStaleCleanup(reader, store, nil)` attributes the cycle's DEBUG counts to the hooks component.

STATUS: complete

SPEC CONTEXT:
This is a phase-8 implementation-analysis task, so its authority is its own body rather than the
specification. The spec's neighbouring material is §5.1's signed-off `--fix` copy plus the
2026-09-01 and 2026-09-04 corrigenda, which fix the stand-down vocabulary at six *decline* reasons
(`restoring`, `marker-read-failed`, `store-read-failed`, `pane-read-failed`, `empty-pane-read`,
`lock-timeout`) and record the withdrawal of "could not read live panes". None of that governs a
sweep that ran and *failed*, which is the hole this task fills: the spec's own argument — a repair
that printed nothing is indistinguishable from one that found nothing — applied to the one path the
stand-downs never covered.

IMPLEMENTATION:
- Status: Implemented (reshaped by a later phase-9 task; substance preserved)
- Location:
  - `cmd/doctor.go:197-212` — `pruneDoctorStaleHooks`; the error arm at `:202-205` now renders a
    user-facing line via `reportFailedPrune(w)` and returns, instead of falling through to a zero
    outcome that printed nothing.
  - `cmd/doctor.go:231` — `sweepFailedStandDownPhrase = "the sweep could not complete"`.
  - `cmd/doctor.go:264-268` / `:270-276` — `reportSkippedPrune` (stand-down vocabulary) and
    `reportFailedPrune` (failed sweep), the two renderers of the `Skipped stale hook prune: …` line.
  - `internal/hooksweep/sweep.go:14` — the cycle's own `log.For("hooks")` binding.
  - `internal/hooksweep/sweep.go:192` — `logger.Warn(sweepFailedMsg, "error", err)` on the
    unclassified-failure arm of `declinedSweep`, the only path that returns a non-nil error to
    `pruneDoctorStaleHooks`.
- Notes:
  Two of the four criteria are met in substance rather than to the letter, both because a later
  task in the same plan moved the cycle into `internal/hooksweep`. Judged against intent, both
  divergences are sound and neither is a loss:

  1. Criterion 3 asked for the phrase to come from `skippedPrunePhrases`, and the task commit
     (5f544ad) did exactly that — enrolling `skipReasonSweepFailed` in `skipReasons` and both phrase
     maps. The extraction then made `Reason` a closed vocabulary of *declines only*
     (`internal/hooksweep/reason.go:16-17`: "A sweep that ran and failed is not among them: it
     declined nothing"), so the phrase became its own const at `cmd/doctor.go:231` with its own
     renderer. The criterion's substance — one single-sourced declaration, no literal at the call
     site — holds: the const carries the `*StandDownPhrase` suffix that enrols it in
     `TestStandDownPhrasesAreSpelledOnlyInTheirDeclaredHome`
     (`cmd/doctor_stand_down_phrase_guard_test.go:299-311`), so no production literal may respell it.
     Enrolling a `Reason` instead would have put a value in the closed enum that `hooksweep` never
     returns, which the `Reasons`-coverage guards would then require every surface to word.

  2. Criterion 4 names `runHookStaleCleanup(reader, store, nil)`; that function and its
     `countsLogger` parameter no longer exist. The nil-fallback question is settled more strongly
     than the task asked — there is no logger parameter at all, and the whole cycle (counts,
     stand-downs and failures) rides `log.For("hooks")` from
     `internal/hooksweep/sweep.go:14`, pinned by
     `internal/hooksweep/sweep_move_test.go:140-173`.

  Criterion 2 verified by enumerating every exit from `pruneDoctorStaleHooks`: error →
  `reportFailedPrune`; removals → one `Pruned stale hook:` per key; decline → `reportSkippedPrune`;
  nil store → silent, but the post-repair diagnosis reports `stale hooks` as not-evaluable with
  "could not read hooks.json" (`cmd/doctor.go:363-373`), so the user is not left silent there
  either. The remaining silent path is a cycle that ran and removed nothing — which is the baseline
  the whole feature reads against, not a gap.

  Criterion 4's exit-code clause (Do item 4) holds: `runDoctorFix` (`cmd/doctor.go:186-191`) returns
  nothing and the exit comes solely from the post-repair diagnosis (`cmd/doctor.go:164-176`).

  The failed line cannot lie about partial work: `hooks.Store.deleteStale`
  (`internal/hooks/store.go:318-358`) returns `nil, err` on every failure and `save` is an atomic
  rename, so "the sweep could not complete" is never printed over keys that were in fact removed.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/doctor_fix_hook_prune_report_test.go:17-63` — the task's three named cases: one pruned line
    per reaped key (asserted both exactly and by prefix count, so a second line fails), a
    stand-down line rendered through the vocabulary rather than restated, and the failed-sweep case,
    which pins `reportFailedPrune`'s bytes directly *and* through a full `--fix` run *and* asserts
    exactly one WARN at `hooks`/`stale-hook cleanup failed`. That last assertion is what carries
    the "still logs the WARN" half of criterion 1 across the emission's move into `hooksweep`.
  - `cmd/hook_prune_one_component_test.go:85-107` — the same failure from the other angle: the
    rendered line is exactly `Skipped stale hook prune: ` + `sweepFailedStandDownPhrase` with
    nothing else on stdout, and exactly one record at or above WARN, so a caller adding a second
    log line for one event fails here.
  - `cmd/hook_prune_one_component_test.go:41-64` — daemon and `doctor --fix` emit an identical
    component/message sequence, with a non-vacuity guard on the daemon side.
  - `internal/hooksweep/sweep_move_test.go:140-173` — every record of a reaping, a standing-down and
    a failing cycle carries `component=hooks`, which is criterion 4's substance.
  - `cmd/doctor_stand_down_phrase_guard_test.go:47-93` — `skippedPrunePhrases` and
    `notEvaluableDetails` cover exactly `hooksweep.Reasons`, with the rule's own failure mode
    exercised against a synthetic reason (`:73-93`), so the guard cannot pass having stopped looking.
  - `cmd/hook_sweep_caller_guard_test.go:18-51` — pins Do item 3's premise: no bootstrap step may
    reach `hooksweep` at all, which is why the cycle's counts have no caller-supplied component to
    default to.
  - Fixtures verified rather than taken on trust: `failingSweepDeps`
    (`cmd/doctor_fix_hook_prune_report_test.go:69-77`) stages the sidecar *before* the 0500 chmod
    (`internal/hookstest/staging.go:85-95`), so the lock and both reads succeed and only
    `AtomicWrite`'s temp create fails — the one shape that reaches
    `internal/hooksweep/sweep.go:192` rather than a stand-down.
  - `assertSkippedPruneLine` (`cmd/doctor_test.go:1587-1607`) is an exactly-one assertion that also
    fails on any *other* `Skipped stale hook prune:` line, so a path printing two lines, or the
    right line with the wrong words, fails rather than passes.
- Notes: Not over-tested. The failed-sweep path is asserted twice, but from genuinely different
  positions — one on the rendered bytes of the renderer in isolation plus a full `--fix` run, the
  other on the emission's single-record property — and neither subsumes the other.

CODE QUALITY:
- Project conventions: Followed. The WARN rides the `hooks` component per CLAUDE.md's rule that
  `hooksweep` owns the whole cycle's emission and a caller adds no second line; the new tests are
  unit-lane and untagged, spawn no daemon and build no binary; no `slog.Logger` is constructed
  outside `internal/log`; `sweepFailedMsg` is deliberately respelled in `cmd`
  (`cmd/hookkey_vocabulary_test.go:214-221`) with the reason stated — a cmd suite asserting the
  operator-visible message must fail when the cycle re-words it rather than silently agreeing.
- SOLID principles: Good. `pruneDoctorStaleHooks` renders and nothing else; the classification of a
  failure versus a decline stays inside `hooksweep`; the two renderers are separated by the
  distinction they exist to preserve.
- Complexity: Low. One added early return and one three-line renderer.
- Modern idioms: Yes.
- Readability: Good. Every non-obvious choice carries its reason in-source — why the failure phrase
  sits outside the stand-down vocabulary (`cmd/doctor.go:228-231`, `:270-273`), why the caller adds
  no second log line (`cmd/hook_prune_one_component_test.go:97-99`), and why the fixture stages its
  sidecar before denying writes.
- Issues: None reportable. Noted and deliberately not raised: the `"Skipped stale hook prune: "`
  prefix is spelled in both `cmd/doctor.go:267` and `:275`, which is a duplication I would fold, but
  nothing breaks today and no test or contract is falsified by it — a preference, not a defect.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
