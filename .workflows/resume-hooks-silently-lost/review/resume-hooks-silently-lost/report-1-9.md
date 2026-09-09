TASK: resume-hooks-silently-lost-1-9 — Consolidate The Phase's cmd Test Files And Fixtures (tick-716ce1, severity: drift)

ACCEPTANCE CRITERIA:
- The four concern-named files are gone; every test they held survives with its cases intact
- Every remaining test file in `cmd` touching these two sources is named after a source file
- `seedStalePruneFixture` is the only implementation of that fixture
- The pruned-line output is asserted in one place, by exact equality
- The unit-lane test count for package `cmd` is unchanged
- `go test ./...` and `go test -tags integration -p 1 ./...` both pass

STATUS: complete

SPEC CONTEXT:
The two sources being consolidated around are the stale-hook sweep and `portal doctor --fix`. The spec
(specification.md:250-257, :294, :312, :374) fixes their user-visible contract: the sweep runs from two call
sites over one code path, `doctor --fix` supplies an `onRemoved` printing `Pruned stale hook: <key>` and an
`onSkipped` printing one `Skipped stale hook prune: <reason>` line beside it, and the exit code stays driven
by the post-repair diagnosis. Corrigenda 2026-08-30 and 2026-09-01 amend the reason vocabulary (five reasons,
not three) and the empty-live-set wording. None of that is what this task changes — this is a pure
test-organisation task whose subject is where the tests for that behaviour live, so the spec is context for
what must keep being asserted, not a source of new criteria.

IMPLEMENTATION:
- Status: Implemented (delivered in e34735b7; outcome intact in the final tree)
- Location:
  - Deletions (verified absent from the working tree and deleted in e34735b7): `cmd/hook_prune_output_test.go`,
    `cmd/hook_retention_shape_test.go`, `cmd/hook_sweep_restore_standdown_test.go`,
    `cmd/hook_sweep_standdown_report_test.go`
  - Merge targets at delivery: `cmd/doctor_test.go` (+192) and `cmd/run_hook_stale_cleanup_test.go` (+462)
  - Parameterised fixture, single implementation today: `cmd/doctor_test.go:815`
    (`seedStalePruneFixture(t, stateDir, lister)`), with its two named lister variants at
    `cmd/doctor_test.go:830` (`staleHookLister`) and `cmd/doctor_test.go:836` (`restoringHookLister`)
  - Exact-equality pruned-line assertion folded into the shared helper: `cmd/doctor_test.go:842`
    (`assertStalePrunesApplied`), the assertion itself at `cmd/doctor_test.go:863-873`
  - `restoringOption` today: `cmd/hookkey_vocabulary_test.go:60` (moved to the sweep fakes at delivery, re-homed
    to the shared hook-key vocabulary file by a later phase)
- Notes:
  - **Every Do item landed.** The four files were merged by which source each exercises, the fixture gained its
    `lister` parameter, both previously-inlined copies collapsed into it (the copy at
    `hook_sweep_restore_standdown_test.go:139-144` became `seedStalePruneFixture(t, t.TempDir(),
    restoringHookLister())`; the copy at `hook_sweep_standdown_report_test.go:222-244` became the fixture call
    inside `runDoctorFixWithLister`), and the substring pruned-line check was replaced by the exact-equality
    form inside the shared helper. All three then-existing `assertStalePrunesApplied` call sites were updated
    and none needed the weaker form.
  - **The collapse strengthened one assertion incidentally and correctly.** The old inlined stand-down fixture
    seeded only a live project, so `assertSkippedPruneLine`'s ordering check (`projectAt != -1 && skippedAt >
    projectAt`) was vacuous — no `Pruned stale project:` line was ever printed. `seedStalePruneFixture` seeds a
    gone project too, so that ordering branch is now genuinely exercised.
  - **One Do item was deferred, and is satisfied in the delivered tree.** "Drop the now-redundant standalone
    assertion" was not done at e34735b7: `TestDoctorFixPrunedHookOutput` survived the merge with its body
    rewritten to call `assertStalePrunesApplied`, making it a strict subset of
    `TestDoctorFixPrunesStaleEntriesThenRediagnosesClean`. Task 6-22 ("no case is a strict subset of its
    sibling", 2ad00331) removed it. Nothing is wrong in the tree today — the pruned line is asserted once by
    exact equality at `cmd/doctor_test.go:863-873` — so this is recorded, not reported as a finding.
  - **Later phases legitimately moved the merge target.** `cmd/run_hook_stale_cleanup_test.go` was deleted at
    a4898f41 (task 9-12) when the sweep moved to `internal/hooksweep`, and the remaining cmd-side tests were
    renamed to `hook_prune_*` / `doctor_fix_hook_prune_*`. Those names are a later task's decision, judged
    against its own body, not a regression of this task's second criterion — which held exactly as written at
    e34735b7, where the only cmd test files touching the two sources were `run_hook_stale_cleanup_test.go`,
    `doctor_test.go` and the doctor-named siblings.

TESTS:
- Status: Adequate (this is a movement task; the surviving cases are the coverage, as the task states)
- Coverage: Verified mechanically across package `cmd` at e34735b7^ vs e34735b7:
  - Top-level test functions: 936 before, 936 after, sets byte-identical (`diff` clean)
  - Subtest name multiset (`t.Run("…")` occurrences): identical before and after
  - Assertion calls (`t.Error/Errorf/Fatal/Fatalf`): 4138 → 4137, the single delta being the inline pruned-line
    loop in `TestDoctorFixPrunedHookOutput` replaced by the `assertStalePrunesApplied` call, which asserts the
    same line at the same exact-equality strength plus the two file-state checks
  - Bodies of the moved cases (`TestUnjudgeableHookKeyRetention`, `TestHookSweepStandsDownWhileRestoring`,
    `TestHookSweepReportsStandDown`, `TestDoctorFixReportsSkippedHookPrune`, `TestSkippedPrunePhrase`) carried
    over verbatim apart from the intended fixture substitutions
- Notes: No new test was required and none was added, which matches the task's Tests section. Per the verifier
  role I did not execute either lane; the final acceptance criterion (`go test ./...` and the integration lane
  pass) is asserted by reading only — the merged file was modified by ~20 subsequent task commits that
  compile against it, so a broken merge would not have survived to the tip.

CODE QUALITY:
- Project conventions: Followed. Files remain unit-lane (no build tags added, nothing daemon- or
  binary-spawning moved), no `t.Parallel()` introduced, and the fixture keeps routing dependency injection
  through the `*Deps` seam (`staleDeps` → `DoctorDeps`) rather than touching package state directly.
- SOLID principles: Good. `seedStalePruneFixture` now varies on exactly the one axis its three callers differ
  on (the live-pane enumeration), with the two variants named for what they mean (`staleHookLister`,
  `restoringHookLister`) rather than open-coded literals.
- Complexity: Low. The merged helpers are linear; `assertSkippedPruneLine`'s single pass over the output lines
  is the only loop with branching.
- Modern idioms: Yes — the folded assertion uses `strings.SplitSeq` with a range-over-func loop, consistent
  with the `modernize` linter the project enables.
- Readability: Good. Each moved helper carries a doc comment stating why the fixture is shaped as it is, and
  the comments hold true against the code: `seedStalePruneFixture`'s "one token-shaped hook entry and one
  stale project record" matches `hookstest.ReapableSeedA` + the `goneDir` record it seeds
  (`cmd/doctor_test.go:815-826`), and `staleHookLister`'s "excludes the seeded key … the set's non-emptiness
  keeps the hazard guard from deferring" matches `tokenRows(hookstest.LiveSeedB)`
  (`cmd/doctor_test.go:830-833`). No comment references a task id, phase or spec section.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
