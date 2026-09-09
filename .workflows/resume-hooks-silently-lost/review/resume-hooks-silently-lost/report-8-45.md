TASK: resume-hooks-silently-lost-8-45 — "Two Stand-Down Reasons Cannot Reach The Surface That Renders Them"
(record at the reason-vocabulary declaration that the set is deliberately complete rather than fully
reachable; change no behaviour)

ACCEPTANCE CRITERIA:
- The declaration states which reasons are unreachable on which surface, and why.
- No code path changes; the five-reason completeness guards still pass.
- The degrade-to-unlocked read is untouched.

STATUS: complete

SPEC CONTEXT:
The specification's 2026-09-01 corrigendum (specification.md:552) settles this exact point: `notEvaluableDetails`
gains its `lock-timeout` entry "so all five reasons render a phrase on both surfaces — vocabulary completeness
rather than an observed leak, since `lock-timeout` cannot reach that path today (a lock acquisition failure
degrades to an unlocked read)". §6.5 and its second 2026-09-01 corrigendum (specification.md:367, 554-556) pin
the degrade-to-unlocked read as the settled behaviour on every read path, `checkStaleHooks` named among them
(specification.md:505). The task's job was therefore annotation only — making the settled position visible where
the enum is declared — and explicitly not making the pair reachable.

IMPLEMENTATION:
- Status: Implemented (delivered, then relocated and condensed by later plan tasks — both deliberate)
- Location:
  - Delivering commit 832258e1: 18 inserted lines in `cmd/run_hook_stale_cleanup.go`, every one a comment on the
    `skipReasons` declaration. No non-comment line in the diff.
  - Present home: `internal/hooksweep/reason.go:30-34` — the annotation now sits on `var Reasons`, immediately
    under the const block that declares the vocabulary (`internal/hooksweep/reason.go:18-25`). It was moved there
    by task 9-12 (a4898f41, "the sweep becomes internal/hooksweep") and condensed by task 9-13 (6373a1aa,
    "comments state conclusions, not the debate").
- Notes:
  - The surviving text is accurate against the code as it stands, and I verified each half:
    * Unreachable at the diagnosis surface — `checkStaleHooks` (`cmd/doctor.go:363-392`) has exactly three
      stand-down exits: `store == nil` and a failed `store.Load` (both `ReasonStoreReadFailed`, lines 367-373),
      `hooksweep.StalenessStandDown` (restoring / marker-read-failed, lines 375-377) and
      `hooksweep.JudgeAgainstLivePanes` (pane-read-failed / empty-pane-read, lines 379-382). None can yield
      `ReasonLockTimeout`, because `Store.Load` degrades: `loadSharedBounded`
      (`internal/hooks/store.go:70-79`) logs one `load-unlocked` DEBUG on any acquisition failure and falls
      through to the unlocked `load()`, so a held sidecar never surfaces as an error to this caller.
    * Reachable at the repair surface — `hooksweep.declinedSweep` maps `hooks.ErrLockHeld` to
      `ReasonLockTimeout` (`internal/hooksweep/sweep.go:181-185`), which `skippedPrunePhrases`
      (`cmd/doctor.go:242`) words. The annotation is scoped to "lock-timeout's *not-evaluable* phrase", so it
      does not overclaim.
    * Only `ReasonLockTimeout` is asymmetric. I checked all six declared reasons against both surfaces; the
      other five (`restoring`, `marker-read-failed`, `store-read-failed`, `pane-read-failed`,
      `empty-pane-read`) are producible on both, so "lock-timeout" is the complete list the annotation owes.
  - Criterion 1's "and why" clause: the delivered comment carried the reason (a read that cannot take the sidecar
    reads anyway, unlocked); task 9-13 removed it as debate, leaving the conclusion. That is a deliberate,
    recorded divergence and not a loss — the "why" survives at the two places that own it
    (`internal/hooks/store.go:62-79`, and `cmd/doctor_stand_down_copy_test.go:50-53`, which states "the lock is
    the one reason a repair declines under that a read degrades past"), and the annotation still discharges the
    task's Outcome: a reader meeting the phrase is told it exists for completeness rather than hunting a missing
    path. Not reported as a finding.
  - Criteria 2 and 3 hold literally: the delivering commit contains no executable line, and the degrade-to-unlocked
    read is byte-untouched by it. The vocabulary is now six reasons rather than the task's "five" (the sixth
    arrived with the corrigenda earlier in the plan); the guards range over `hooksweep.Reasons` rather than a
    hardcoded count, so they cover the set whole.

TESTS:
- Status: Adequate
- Coverage:
  - Completeness guards, both still keyed on the live set rather than a literal count:
    `internal/hooksweep/reason_enumeration_guard_test.go:22` (every const declared with the `Reason` type is
    enumerated in `Reasons`, and vice versa, with its own failure mode exercised over synthetic source at
    lines 45-56) and `cmd/doctor_stand_down_phrase_guard_test.go:47-93` (both `skippedPrunePhrases` and
    `notEvaluableDetails` word every declared reason, hold no undeclared key, and the rule's own omission case
    is proved).
  - The unreachable pair's diagnosis line is still covered by the real renderer over a synthetic result, exactly
    as the task's Tests section directed: `renderStaleHooksLine` (`cmd/doctor_stand_down_copy_test.go:230-243`)
    builds the `checkResult` and drives `renderDoctorReport`, asserted for every reason at line 387 — including
    `hooks.json is locked (not evaluable)` (line 120).
  - The asymmetry the annotation asserts is now additionally pinned behaviourally rather than only in prose: the
    lock row (`cmd/doctor_stand_down_copy_test.go:112-121`) leaves `postRepairNotEvaluable` unset, alone among the
    six, and the sweep half is exercised end to end against a really-held sidecar in
    `internal/hooksweep/lock_timeout_test.go:28-101`.
- Notes: No new test was owed — the task changed no behaviour — and none was added. Nothing is over-tested here:
  the guards and the copy table predate this task and are the coverage it was told to leave in place.

CODE QUALITY:
- Project conventions: Followed. The annotation carries no process artefacts — no task id, no phase, no spec
  section number — consistent with the repo's comment style and with the "no §refs" rule.
- SOLID principles: N/A (comment-only change).
- Complexity: Low — no code path added or altered.
- Modern idioms: N/A.
- Readability: Good. The note sits on `Reasons`, immediately below the const block a reader meets the vocabulary
  in, and states the conclusion in one sentence.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
