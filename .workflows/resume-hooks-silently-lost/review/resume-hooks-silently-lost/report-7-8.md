TASK: resume-hooks-silently-lost-7-8 — "The Hook-Fire Assertion In cmd/bootstrap Is The Unpolled Read Already Fixed One Package Over"

ACCEPTANCE CRITERIA:
1. `cmd/bootstrap`'s hook-fire assertion polls to the shared budget and no longer reads once immediately after `WaitForSkeletonMarkersCleared`.
2. `hydrateBudget` and `hydrateTick` are declared once, in `internal/restoretest`, and every consumer reads them.
3. No `10*time.Second` / `50*time.Millisecond` literal and no `divergent*` duplicate remains at the four named sites.
4. An absent marker file still reads as a count of 0 rather than fatalling.
5. The meta test's writer goroutine cannot panic the process.
6. The integration lane passes and the hook-fire assertion still detects a hook that fires twice (cross-fire) and one that never fires.

STATUS: complete

SPEC CONTEXT:
The specification does not govern this task — it is a phase-7 implementation-analysis consolidation whose authority is its own body (per the shared verifier context), and the spec contains no text on the reboot round-trip test, the hook-fire marker file, or cross-fire assertions (grep for `HOOK_FIRED` / `round-trip` / `exactly once` returns nothing; none of the ten corrigenda touch this area). The one spec-adjacent fact the task rests on is the hydrate helper's ordering, and it holds in production: `cmd/state_hydrate.go:145` unsets the skeleton marker and `:149` then execs the shell-or-hook, so a marker-clear says nothing about whether the hook's `echo >>` has landed. That is exactly the race the task names.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/restoretest/marker_count.go:14-31` — the promoted `HydrateBudget` (10s) / `HydrateTick` (50ms) pair, now the single declaration in the tree (a later task, 4b9bbad6, added the sibling `PaneReactionBudget`/`PaneReactionTick` beside them).
  - `internal/restoretest/marker_count.go:43-112` — exported `AssertMarkerCount` over the unexported `markerAssertion{path,marker,want,budget,tick}` with `run`/`wait`/`read`. Semantics are byte-equivalent to the deleted `internal/restore` original (`git show e9b27db7^:internal/restore/marker_assert_test.go`): same loop predicate `got > want || (got == want && want > 0) || deadline`, same three distinct failure messages (never-appeared / CROSS-FIRE / cumulative-count), same absent-file-is-zero read.
  - `cmd/bootstrap/reboot_roundtrip_test.go:157` — `verifyHookFiredOnce` (bare `os.ReadFile` + `strings.Count` + ENOENT `t.Fatalf`) deleted and replaced by `restoretest.AssertMarkerCount(t, hookFireFile, "HOOK_FIRED", 1)`, three lines after the `WaitForSkeletonMarkersCleared` at `:152`.
  - `internal/restore/multipane_legacy_integration_test.go:51-54` — the four `assertMarkerCount` calls routed to `restoretest.AssertMarkerCount`; both raw-literal `WaitForSkeletonMarkersCleared` calls now pass the promoted pair (`:139` region, current `:44`/`:199` — the `time` import is gone from the file).
  - `cmd/noncontiguous_window_reboot_integration_test.go:399,404` — `divergentHydrateBudget`/`divergentPollTick` deleted from the const block at `:23-32`; the hydrate wait reads the promoted pair, and a later task upgraded the per-pane `WaitForFileExists` to `AssertMarkerCount`.
  - `internal/restore/rename_reboot_shared_test.go:13-21` — local `hydrateBudget`/`hydrateTick` deleted (`time` import gone); `hookFiredMarker` retained and still used by the two rename suites.
  - `internal/restore/marker_assert_test.go` and `internal/restore/marker_assert_meta_test.go` are gone from the tree.
- Notes: Verified by enumeration, not by inspection of the named sites alone — a repo-wide grep for `AssertMarkerCount|HydrateBudget|HydrateTick|hydrateBudget|hydrateTick|divergent(HydrateBudget|PollTick)` across `cmd` and `internal` returns 16 call sites, every one of them reading `restoretest.HydrateBudget`/`HydrateTick` or `restoretest.PaneReactionBudget`/`PaneReactionTick`; no second declaration of either pair exists. Criterion 2's "declared once" therefore holds tree-wide, not just at the four sites.
  The `internal/restoretest/doc.go` mixed-lane rule is respected: `marker_count.go` is untagged because its own unit-lane test is a caller, while every production caller is integration-tagged.
  Criterion 6's "the integration lane passes" is not directly verifiable here (no test execution). Judged by reading, the edit compiles: every file that lost its last duration literal also lost its `time` import (`internal/restore/multipane_legacy_integration_test.go`, `internal/restore/rename_reboot_shared_test.go`), while `cmd/bootstrap/reboot_roundtrip_test.go` and `cmd/noncontiguous_window_reboot_integration_test.go` retain `time` for other uses (`:492`, `:530` and `:282` respectively), and `os`/`strings` remain used in the bootstrap file after `verifyHookFiredOnce` was deleted.

TESTS:
- Status: Adequate
- Coverage: `internal/restoretest/marker_count_test.go` (unit lane, `package restoretest`, white-box so it can drive `markerAssertion` at a 400ms `probeBudget` instead of paying the real 10s). All six named micro-acceptance tests are present and each would fail if its subject broke:
  - late marker still satisfies → `TestAssertMarkerCount_MarkerArrivingAfterTheAssertionStarts:59` (marker written at 150ms against a 400ms budget — fails if the poll is removed).
  - never appears → `:74`, and it asserts `elapsed >= probeBudget`, so a give-up-early regression fails it.
  - overshoot fails at once → `:90`, asserting `elapsed < probeBudget`, so a version that waited out the budget on an unrecoverable count fails.
  - want-0 waits out the budget → `:123`, asserting `elapsed >= probeBudget`; paired with `:109`, which proves a want-0 with the marker present fails (without it the absence assertion would prove nothing).
  - absent file is a count of 0 → `:140`, asserting `len(rep.Fatals) == 0` specifically, which is precisely criterion 4 (the old `cmd/bootstrap` behaviour was an ENOENT `t.Fatalf`).
  - writer failure reported not panicked → `:154`, which drains `writeMarkerAfter`'s error channel for a path under a missing directory; criterion 5 is structural (`marker_count_test.go:40-57` returns the error on a buffered channel; the deleted `internal/restore/marker_assert_meta_test.go:57` `panic(err)` is gone).
  `TestAssertMarkerCount_ExportedEntryPointUsesTheSharedBudget:163` pins criterion 6's cross-fire arm and that the exported entry point actually uses the 10s budget: the marker lands at `pastProbeBudget` (600ms), which the shared budget absorbs and the probe budget cannot — and the contrast sub-test at `:175` is what stops the first from passing under any budget.
- Notes: Not over-tested. The one apparent duplication — `:191` "it fails when more markers fire than expected" against `:90` — is the criterion-6 assertion against the *exported* entry point rather than the internal one, and the sub-test's own message says so. No redundant mocking or setup; all fixtures are `t.TempDir()` files.
  The tests assert *that* the assertion fails, not *which* of the three messages it emits, so folding CROSS-FIRE into the default wording would not be caught. That is diagnostic text, no acceptance criterion asks for it, and no test misnames its subject as a result — noted, not raised.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()` (CLAUDE.md prohibits it tree-wide). Test file is named after its source (`marker_count.go` → `marker_count_test.go`). The helper lives in `internal/restoretest`, which production code must not import, and reports through `harnesstest.TestingT` — the single declaration of that subset per CLAUDE.md — rather than declaring a local `markerReporter`, which the deleted original did. The lane rule is satisfied: nothing here builds, spawns or execs a portal binary, so the untagged unit-lane test is correctly placed.
- SOLID principles: Good. The `markerAssertion` value type separates the assertion's policy (budget/tick) from its behaviour, which is what lets the exported entry point pin the shared budget while the tests drive a fast one — without exporting budget parameters no production caller wants.
- Complexity: Low. One loop, one switch, one read; no branch is unreachable-by-construction.
- Modern idioms: Yes. `os.IsNotExist` guard, buffered error channel over a panic in the writer goroutine, value receiver on a small config struct.
- Readability: Good. Every non-obvious decision carries its reason: why the budgets are declared together (`:12-13`), why a want of 0 must wait out the budget (`:41-42`), why an overshoot is final (`:88-90`), why an absent file is a zero (`:98-99`).
- Comment accuracy: Verified against the code and against production. `marker_count.go:36-38`'s claim that "a pane's helper clears its skeleton marker before it execs the hook" is true of `cmd/state_hydrate.go:145` then `:149`. `HydrateBudget`'s "the pane has still to be spawned, respawned into the hydrate exe and scheduled" matches what the reboot fixtures do before the wait. No comment references a task id, phase or spec section.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
