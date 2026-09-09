TASK: resume-hooks-silently-lost-1-6 — Single-Source The Restore-Window Rule And Its Phrase (severity: duplication)

ACCEPTANCE CRITERIA:
- `state.IsRestoringSet` is called from exactly one place in package `cmd`
- The failed-read-counts-as-set rationale comment exists exactly once
- `restore may be in progress` appears as one string literal in `cmd`; both renderings derive from it
- `checkStaleHooks` still reads the marker after the store guards and before the enumeration
- Every existing test passes with no assertion, fixture or name changed
- `go test ./...` and `go test -tags integration -p 1 ./...` both pass

STATUS: complete

SPEC CONTEXT:
The specification requires the hook-staleness cycle to stand down whenever the `@portal-restoring`
window may be open, because a restore's panes carry no `@portal-pane-id` token between skeleton
construction and the re-stamp — judging keys in that window would report (and, on `--fix`, reap)
every token-keyed entry on the machine. The load-bearing half of the rule is that a *failed* marker
read counts as set, so the cycle stands down absent proof the window is clear. §5.1 pins the user
copy: `Skipped stale hook prune: restore in progress` (spec line 255). The Corrigendum of 2026-09-04
splits a failed marker read off under its own reason (`marker-read-failed`) and its own phrase
(`could not read the restore marker`), while keeping the same fail-safe posture — which is why the
plan's deliberately weaker `restore may be in progress` wording (a phase-1 deviation recorded in
`planning/.../phase-1-tasks.md`) was later returned to the spec's verbatim phrase. This task itself
is a pure duplication fix: it introduces no behaviour.

IMPLEMENTATION:
- Status: Implemented, then superseded in place by later phases (all substance preserved)
- Location (task commit `e03c9093`, cmd/doctor.go + cmd/run_hook_stale_cleanup.go only):
  extracted `restoreWindowActive(checker) (bool, error)` carrying the rationale verbatim, called it
  from `runHookStaleCleanup` and `checkStaleHooks` at their existing positions, deleted the second
  rationale comment, added `const restoreStandDownPhrase` and composed both renderings from it.
  The diff is behaviour-neutral: `active` is exactly the former `restoring || err != nil`, and the
  read error is still returned alongside for the caller that logs it.
- Location (current tree, source of truth):
  - `internal/state/markers.go:102-110` — the rule and its rationale, now `state.RestoreWindowActive`
    (moved out of `cmd` by task 6-8, "one home for the stand-down rule").
  - `internal/hooksweep/sweep.go:71-80` — `StalenessStandDown`, the sweep's reader (the sweep moved
    out of `cmd` by task 9-12; `cmd/run_hook_stale_cleanup.go` no longer exists).
  - `cmd/doctor.go:375` — `checkStaleHooks` takes the same gate, after `store == nil` (367) and
    `store.Load` (370), before `hooksweep.JudgeAgainstLivePanes` (379). Ordering criterion holds.
  - `cmd/doctor.go:221` — `restoreStandDownPhrase = "restore in progress"`, the single literal;
    `:237` uses it as the `skippedPrunePhrases` value and `:248` composes
    `restoreStandDownPhrase + " (not evaluable)"` for `notEvaluableDetails`.
- Notes:
  - Criterion 1 ("exactly one place in package `cmd`") is not literally true of the tree and never
    was: at the task's own commit `cmd/state_commit_now.go` and two sites in `cmd/state_daemon.go`
    already read `IsRestoringSet` for the daemon's capture-suppression posture, which was never in
    this task's scope. The criterion's substance — one home for the rule — is delivered and has
    since been strengthened: all four production readers (hooksweep, daemon tick, daemon shutdown
    flush, commit-now) now route through `state.RestoreWindowActive`, and no site restates
    `restoring || err != nil` (`internal/state/markers.go:109` is the only occurrence in the repo).
  - Criterion 2 holds in the current tree: the rationale is written once, on `RestoreWindowActive`.
    `cmd/state_daemon.go:337` *refers* to it ("the window's rule presumes set") rather than
    restating it, and `hooksweep.StalenessStandDown`'s doc explains the reason vocabulary, not the
    rule.
  - The phrase has since been re-worded from `restore may be in progress` to the spec's
    `restore in progress` (the marker-read-failed split gave the failed read its own phrase). The
    criterion's substance — one literal, both renderings derived — holds under the new wording, and
    the plan's own record of the deviation is now moot.

TESTS:
- Status: Adequate (and materially stronger than the task asked for)
- Coverage:
  - `internal/state/markers_test.go:366` `TestRestoreWindowActive` — marker set, marker absent, and
    failed read presumed active with the error returned alongside: the rule's three cases.
  - `internal/hooksweep/sweep_test.go:255` `TestHookSweepStandsDownWhileRestoring` — stands down
    before enumerating with the marker set, on a failed marker read, and before loading the store;
    sweeps normally with the marker absent (the sweep-side pin of the extracted predicate).
  - `internal/hooksweep/sweep_test.go:339` `TestHookSweepReportsStandDown` — the reported reason and
    its log line.
  - `cmd/doctor_stand_down_copy_test.go:338` `TestStandDownCopy` — one row per stand-down reason,
    pinning the `--fix` line, the not-evaluable line, the log level/attrs and an untouched
    hooks.json; `:397` "it composes a shared phrase from the const both surfaces name" asserts
    exactly this task's property — `skippedPrunePhrases[restoring]` *is* the const and
    `notEvaluableDetails[restoring]` *contains* it.
  - `cmd/doctor_stand_down_phrase_guard_test.go:47` / `:299` — a source guard that every declared
    reason has a phrase on both surfaces, and that no production literal in `cmd` re-spells a
    declared phrase (containment, so `phrase + " (not evaluable)"` written inline would fail). The
    single-literal property is now structurally enforced rather than asserted once.
  - `cmd/hook_prune_verdict_parity_test.go:14` — the diagnosis and the reaper agree on judgeability
    for the restore-marker case, so the two call sites cannot drift apart again.
- Notes:
  - The task's "no new test" call was right: the predicate has no behaviour beyond what the two
    sides already exercise, and the task commit changed no test file (its whole diff is
    `cmd/doctor.go`, `cmd/run_hook_stale_cleanup.go`, plus the tick/manifest bookkeeping).
  - `TestDoctorFixReportsSkippedHookPrune`, named in the task's Tests section, no longer exists
    under that name — its subject was absorbed into the reason-per-row `TestStandDownCopy` table by
    a later phase. Coverage of the behaviour is greater, not smaller.
  - `checkStaleHooks`'s *ordering* (marker read after the store guards, before the enumeration) is
    correct in code but is not itself pinned by an assertion; the consequence of a re-ordering would
    be a phrase swap between two stand-down reasons in an edge branch, not a wrong verdict, and no
    test claims to pin it, so nothing here reads as covered when it is not.

CODE QUALITY:
- Project conventions: Followed. `RestoreWindowActive` sits beside `IsRestoringSet` in
  `internal/state`'s marker helpers; `internal/state` stays a leaf; the doctor phrase vocabulary
  keeps its declared-const convention (`*StandDownPhrase`) that the source guard reads.
- SOLID principles: Good. One rule, one home, four readers; the phrase vocabulary is a table the
  renderers index rather than branch over.
- Complexity: Low. The predicate is one expression; both call sites keep their original guard
  ladder position.
- Modern idioms: Yes. `RestoreWindowActive(restoring bool, err error)` taking another call's results
  is unusual for Go, but it is documented as the composition point and it is what lets a caller with
  its own `IsRestoring` seam (`cmd/state_commit_now.go:107`) reach the same rule.
- Readability: Good. The rationale is stated once where the rule lives; the daemon's comment points
  at it instead of paraphrasing.
- Issues: None found.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
