TASK: resume-hooks-silently-lost-9-8 — "The Sweep Reads hooks.json Twice And Only One Read's Failure Has A Reason" (phase 9, implementation-analysis cycle; severity low, source: architecture)

ACCEPTANCE CRITERIA:
- A `hooks.json` unreadable at the pre-read stands the cycle down under `store-read-failed`, as today.
- A `hooks.json` unreadable only at `deleteStale`'s load — readable when the snapshot was taken, unreadable under the exclusive hold — also stands the cycle down under `store-read-failed`, where today it escapes the vocabulary.
- A failing save still returns an unclassified error, and `portal doctor --fix` still renders it through the failed-sweep phrase.
- `errors.Is` against the shared sentinel matches both read failures and neither save failure.

STATUS: complete

SPEC CONTEXT:
The specification's change C ("the stale sweep becomes shape-aware and names what it deleted", §1.2) is what made silent hook loss diagnosable; the sweep's decline vocabulary is the machinery that keeps a cycle that removed nothing from reading as a cycle that ran and found nothing. This task is a phase-9 analysis item, so its authority is its own body (per the shared verifier context): the vocabulary was leaking on one of the clean's two reads of `hooks.json`, and the task closes that leak. Nothing in the spec body contradicts the change.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hooks/store.go:276-280` — `ErrSnapshotRead` is gone; the sentinel is now `ErrStoreRead`, doc-commented as covering "whichever of the clean's two reads it was … A clean that read, judged and then failed to write carries it from neither".
  - `internal/hooks/store.go:299-302` — `CleanStale`'s pre-read failure wraps `ErrStoreRead` (`fmt.Errorf("%w: %w", …)`), and the enumeration is never called.
  - `internal/hooks/store.go:327-330` — `deleteStale`'s `s.load()` failure under the exclusive hold now wraps the same sentinel, replacing the bare `fmt.Errorf("failed to load hooks: %w", …)`.
  - `internal/hooks/store.go:343-346` — the save failure is untouched by any read sentinel (`"failed to save after cleaning stale hooks: %w"`), so it still reaches the unclassified path.
  - `internal/hooksweep/sweep.go:186-190` — the classification arm is `errors.Is(err, hooks.ErrStoreRead)` → `ReasonStoreReadFailed`; no new arm was needed, and the comment now names the reads ("at either of the two reads it takes") rather than the phase, as the Do list asked.
  - `CLAUDE.md:72` — the `hooks` row now reads "Either read of the file failing returns `ErrStoreRead` — the pre-read, which never calls the enumeration, and `deleteStale`'s own load under the exclusive hold alike — so both classify as one condition, while a failed save carries neither."
- Notes:
  - The Do list and AC3 name `cmd/run_hook_stale_cleanup.go`'s `declinedSweep` and a `skipReasonSweepFailed` renderer. Both moved during this plan: the cycle now lives in `internal/hooksweep` (that file no longer exists) and the failed-sweep renderer is `reportFailedPrune` / `sweepFailedStandDownPhrase` (`cmd/doctor.go:228-231`, `:274-276`). The substance the criteria describe is intact at the new homes — the code is the source of truth here, and the task's own body is the thing that moved past its citations, not the implementation.
  - `Set` and `Remove` still wrap their own load failures bare (`internal/hooks/store.go:129`, `:185`). That is correct and in scope-boundary: the sentinel's contract is scoped to the clean's two reads, neither mutation is on the sweep path, and no caller classifies their errors through `declinedSweep`.
  - The end-to-end path holds: a delete-phase read failure returns from `CleanStale` (`store.go:309` returns `deleteStale`'s error directly) → `hooksweep.Run` → `declinedSweep` → `standDownOutcome(declineWarn(ReasonStoreReadFailed, "error", err))`, so both the WARN's `reason` attr and `Outcome.DeclineReason` land in the closed vocabulary; the mutation lock is released either way by the `defer` at `store.go:325`.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/hooks/cleanstale_read_sentinel_test.go:28-63` — the sentinel contract at the store layer: pre-read failure wraps it, delete-phase load failure wraps it (the fixture swaps the file for a directory from inside the enumeration, so only the second read fails — exactly the "readable when the snapshot was taken" case AC2 names), and a denied-write save wraps it in neither direction (`!errors.Is`). That is AC4 in full.
  - `internal/hooksweep/read_failure_test.go:57-91` — the three named plan tests verbatim ("it classifies a failed pre-read as store-read-failed", "it classifies a failed delete-phase load as store-read-failed", "it leaves a failed save on the unclassified path"), each asserting the cycle stands down rather than erroring, plus a fourth pinning that both reads report one reason.
  - `cmd/doctor_fix_hook_prune_report_test.go:45-63` and `:69-77` — AC3's second half on the user's surface: reads succeed, the write is denied, and `portal doctor --fix` prints "Skipped stale hook prune: the sweep could not complete" with exactly one `hooks` WARN under `sweepFailedMsg`.
  - `cmd/doctor_stand_down_copy_test.go:81-90, 364-392` — `ReasonStoreReadFailed` has a row in the reason-complete copy table, pinning its repair line ("Skipped stale hook prune: could not read hooks.json"), its not-evaluable line, its WARN level, and an `error` attr carrying `hooks.ErrStoreRead.Error()`; a companion subtest proves no two reasons share rendered words.
- Notes:
  - The plan's fourth test, "it renders the same stand-down phrase for both read failures on both surfaces", is delivered as a composition rather than a single case: `read_failure_test.go:86-90` pins that both reads yield one `Reason`, and the cmd copy table pins that reason's phrase on both surfaces. Since a phrase is a pure function of the reason (`phraseFor`, `cmd/doctor.go:260-262`), the property the plan named is covered without a second doctor-level fixture — an end-to-end delete-phase doctor run would have been redundant coverage of a map lookup.
  - The `errors.Is`/`!errors.Is` pairing is what makes these tests fail if the change regresses: dropping the sentinel from `deleteStale` fails both the store-layer and the hooksweep-layer delete-phase cases, and wrapping the save in it fails the two negative cases.

CODE QUALITY:
- Project conventions: Followed. The sweep's whole emission stays on the `hooks` component with no second line added by either caller (`internal/hooksweep/sweep.go:14`, and the `hooksweep` row of CLAUDE.md); the copy tables stay in `cmd` beside the renderers; no new log component or attr key was invented; both new tests are unit-lane and hermetic, staging through `hookstest.StageStore` rather than hand-rolling a hooks.json.
- SOLID principles: Good. The sentinel is the store's own vocabulary and the reason is the sweep's; `declinedSweep` translates between them at one point, so neither layer restates the other's words.
- Complexity: Low — one sentinel rename plus one wrapped return.
- Modern idioms: Yes (`fmt.Errorf` with two `%w` verbs, matching the sibling wrap in `CleanStale`).
- Readability: Good. The sentinel's doc comment names both reads and explicitly excludes the write, which is the distinction the whole change exists to make.
- Issues: None. Every comment touched by the change holds against the code: `ErrStoreRead`'s doc, `CleanStale`'s "A failed snapshot read returns ErrStoreRead and never calls it" (`store.go:296-297` — the enumeration is genuinely unreached on that branch), and `declinedSweep`'s "at either of the two reads it takes".

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
