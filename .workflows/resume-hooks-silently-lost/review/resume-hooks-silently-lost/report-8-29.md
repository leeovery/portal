TASK: resume-hooks-silently-lost-8-29 — "Two Inline Copies Of A Stand-Down Phrase Bypass The Vocabulary That Exists To Own It" (tick-23694e)

ACCEPTANCE CRITERIA:
- Neither vocabulary map holds a string literal that its sibling also holds.
- No branch of `checkStaleHooks` authors a stand-down phrase inline.
- Re-wording a map entry changes both early returns' output.
- Every rendered line is byte-identical to today's.

STATUS: complete

SPEC CONTEXT: This is a phase-8 implementation-analysis task, so its authority is its own body rather than the
specification. The surrounding subject is the hook stale-cleanup stand-down surface: `portal doctor --fix` renders a
"Skipped stale hook prune: …" repair line and the read-only diagnosis renders a "stale hooks: … (not evaluable)" check
line, both worded from one closed `hooksweep.Reason` vocabulary so the two surfaces cannot disagree about what the
reaper declined. The defect this task removes is copy duplication: two phrases authored in both vocabulary maps, and
`checkStaleHooks`'s two early returns authoring one of them a third and fourth time as bare literals, where the existing
key-binding guards cannot see them.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Task commit `3070e065` ("every stand-down phrase written once").
  - Phrase consts: cmd/doctor.go:220-226 (`restoreStandDownPhrase`, `markerReadStandDownPhrase`,
    `storeReadStandDownPhrase`, `paneReadStandDownPhrase`, `lockStandDownPhrase`).
  - Vocabularies composed from them: cmd/doctor.go:236-243 (`skippedPrunePhrases`) and cmd/doctor.go:247-254
    (`notEvaluableDetails`).
  - Shared render helper: cmd/doctor.go:352-354 (`staleHooksNotEvaluable`), routing through `phraseFor`
    (cmd/doctor.go:261-263).
  - The two early returns: cmd/doctor.go:367-373 — both now `staleHooksNotEvaluable(name, hooksweep.ReasonStoreReadFailed)`.
- Notes:
  - The task body names `cmd/run_hook_stale_cleanup.go` as the home of the vocabularies; that file was deleted later in
    the same plan by task 9-12 ("the sweep becomes internal/hooksweep", commit `a4898f41`), which moved the consts and
    both maps into cmd/doctor.go. The task's substance survived the move intact — this is a legitimate post-task
    relocation, not drift.
  - The task lifted `storeReadStandDownPhrase`, `paneReadStandDownPhrase` and `sweepFailedStandDownPhrase`;
    `lockStandDownPhrase` was lifted later by task 9-10. At this task's own commit the two maps already held no literal
    in common ("hooks.json is locked" vs "hooks.json is locked (not evaluable)" are different strings), so criterion 1
    held then and holds now.
  - Verified repo-wide: each of the six stand-down phrases is written exactly once in non-test sources, all six at
    cmd/doctor.go:221-231. No production branch re-authors one.
  - Every not-evaluable return of `checkStaleHooks` (cmd/doctor.go:368, :372, :376, :381) renders through the
    vocabulary; the remaining returns (:385, :389, :391) are pass/fail details, not stand-down phrases.
  - Byte-identity: the diff replaced `detail: "could not read hooks.json"` with the vocabulary entry whose value is the
    same string, so the rendered lines are unchanged — and cmd/doctor_stand_down_copy_test.go:87-88 pins both surfaces'
    whole rendered lines for that reason as string literals, so a drift would fail rather than pass silently.

TESTS:
- Status: Adequate
- Coverage:
  - The task's named assertion is delivered verbatim: cmd/doctor_stand_down_copy_test.go:480-505,
    `TestStaleHooksCheckStandDownCopy` / "it renders the nil-store and failed-load branches from the vocabulary". It
    re-words `notEvaluableDetails[ReasonStoreReadFailed]` for the test's duration
    (`rewordNotEvaluableDetail`, :473-478, with a `t.Cleanup` restore) and asserts both branches follow. This is the one
    assertion the existing guards cannot make: an assertion on today's literal cannot separate a branch that renders
    through the map from one that authors the same words inline.
  - The failed-load arm is genuinely exercised — `hookstest.StageStore(t, Staging{Unreadable: true})` stages a
    *directory* at the hooks.json path (internal/hookstest/staging.go:73-76), so `store.Load` returns a real error
    rather than an absent-file success.
  - New structural guard `TestStandDownVocabulariesShareNoInlineLiteral`
    (cmd/doctor_stand_down_phrase_guard_test.go:131-178) reads each map's inline literals from the AST and fails when
    one matches any runtime value of its sibling — the rule the key-binding guards structurally could not express. It
    fatals when either map declaration is not found, so it cannot pass having scanned nothing.
  - Byte-identity of the rendered output stays pinned by the pre-existing whole-line literals in
    cmd/doctor_stand_down_copy_test.go:65-121 and the coverage/undeclared-key guards at :47-70.
- Notes:
  - Not over-tested: two table rows and one guard, each asserting something none of the others can.
  - Mutating the package-level `notEvaluableDetails` map in a test is safe here — `cmd` uses no `t.Parallel()`
    anywhere (repo rule), and the helper restores the prior value on cleanup. It is an index assignment rather than a
    seam-var assignment, so `cmd/seam_guard_test.go` has nothing to say about it.
  - No new test builds or runs a portal binary or a daemon, so the unit lane is the correct home; no lane rule is
    touched.

CODE QUALITY:
- Project conventions: Followed. No new log component or attr key; no `*Deps`/function seam introduced or bypassed; the
  test lives in the unit lane and uses the canonical `hookstest.StageStore` staging route rather than a hand-rolled
  marshal-and-write.
- SOLID principles: Good. `staleHooksNotEvaluable` gives the four not-evaluable returns one construction point, and
  `phraseFor` stays the single rendering chokepoint for both vocabularies.
- Complexity: Low. The production change is a substitution; the guard is a flat AST walk with its own scanned-nothing
  tripwire.
- Modern idioms: Yes — `slices.Contains` in the guard, `strconv.Unquote` for literal values, `t.Cleanup` for restore.
- Readability: Good. Comments explain why rather than restate: cmd/doctor.go:215-220 says why the shared phrases are
  written once, cmd/doctor.go:365-366 says why a nil store and a failed read are one condition to a user, and
  cmd/doctor_stand_down_copy_test.go:469-472 says why re-wording is the only assertion that separates the two shapes.
  All three hold true against the code.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
