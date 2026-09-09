TASK: resume-hooks-silently-lost-6-11 (tick-4acb93) — Collapse The Duplicate AllPaneLister Fakes And The Duplicated Seed-Key Aliases In cmd

ACCEPTANCE CRITERIA:
- One `AllPaneLister` fake exists in package `cmd`, in `cmd/hookkey_vocabulary_test.go`.
- No test file declares a package-level hook-key name that duplicates a seed the vocabulary already names.
- Every fixture named "live" holds a key from the live seed set.
- Verdict count is unchanged: no case is lost in the re-point.

STATUS: complete

SPEC CONTEXT:
This is a phase-6 implementation-analysis task, so per the shared verifier context its authority is its own
body rather than the specification. It carries no production surface: the task commit (ad9c7c0b) touches 15
files, every one of them a `_test.go`. The subject it serves is the spec's staleness machinery — the sweep
reader seam (`hooksweep.Reader` = `ListAllPaneHookKeys` + the `@portal-restoring` read) and the reapable /
live / unjudgeable seed-key halves that let a fixture say which side of the staleness rule it is measuring.
The duplication it closes is a real hazard for that subject: a fixture named "live" bound to a reapable seed
makes a retention assertion pass for the wrong reason.

IMPLEMENTATION:
- Status: Implemented (with a defensible, sound divergence from the Do list's literal wording)
- Location:
  - `cmd/hookkey_vocabulary_test.go:93-114` — the single sweep-seam fake `stubStaleSweepReader`
    (`rows`/`err`/`restoring`/`restoringErr`/`during`/`calls`), pinned by
    `var _ hooksweep.Reader = (*stubStaleSweepReader)(nil)` at :114.
  - `cmd/doctor_test.go:815` (`seedStalePruneFixture`), `:830` (`staleHookLister`), `:836`
    (`restoringHookLister`) — the doctor-side factories moved to the merged fake's pointer type.
  - `cmd/state_daemon_hook_cleanup_test.go:23` — the daemon fixture's live row now composes
    `hookstest.LiveSeedA`, replacing the deleted `livePaneToken` package-level var.
  - `cmd/noncontiguous_window_reboot_integration_test.go:240,246,254` — `staleKey` on
    `hookstest.ReapableSeedA`, the two stamped panes' tokens on `hookstest.LiveSeedA` / `LiveSeedB`
    (formerly all three re-derived from the same reapable index).
  - `internal/hookstest/hooks.go:171-198` — the named seed halves the fixtures now reach by name; the
    constructors behind them (`tokenShapedHookKey`, `unjudgeableHookKey`) are unexported, so re-deriving an
    index is no longer reachable from another package. (That move landed in a later phase; it is what makes
    criterion 2 structural rather than disciplinary today.)
- Notes:
  - The Do list said "delete `fakeHookLister` and re-point its call sites at `stubAllPaneLister`". The
    implementation went further and correctly: it deleted `fakeHookLister`, folded `recordingHookKeyLister`
    (the third fake, which the task's own note says this closes) into the survivor by adding its `calls`
    counter, and renamed the survivor `stubStaleSweepReader` so the name states the seam rather than one
    method. Criterion 1 is met in substance — one fake for that seam, in the named file. Verified: nothing in
    `cmd` implements the `ListAllPaneHookKeys` + `TryGetServerOption` pair except `stubStaleSweepReader`.
  - `recordingPaneHookLister` (`cmd/hookkey_vocabulary_test.go:73`) and `loudPaneHookLister`
    (`cmd/hooks_test.go:274`) remain, and both are correct to remain: they answer the separate `PaneHookLister`
    seam `hook list` reaches (`cmd/hooks.go:26`), the second being a deliberately-loud poison with no `rows`
    at all. Neither is one of the three fakes the task names.
  - The value→pointer receiver change is a quality gain beyond the stated outcome:
    `cmd/deps_merge_convention_test.go:138,201` compares the sentinel lister with `!=` through the
    `hooksweep.Reader` interface, which would panic at runtime on an interface holding a struct value carrying
    a slice field. Pointer identity makes that comparison legal.
  - `restoringHookLister` (`cmd/doctor_test.go:836`) mutates what `staleHookLister()` returns. Safe under the
    pointer type because the factory mints a fresh value per call, so no two callers share state.
  - The merged fake's shared-pointer reuse in `cmd/hook_prune_verdict_parity_test.go:32,35` (one `tc.lister`
    driven through both `checkStaleHooks` and `hooksweep.Run`) is deliberate and inert: the fake's only mutable
    field is `calls`, which that suite does not read, and CLAUDE.md forbids `t.Parallel()` in this tree.

TESTS:
- Status: Adequate — correctly, no new case was added
- Coverage: The task's own Tests section states the re-pointed suites are the test. Read against the diff,
  every hunk in all 15 files is a type substitution or a constant re-point: no `t.Run` was removed, no
  assertion weakened, no fixture's semantics changed. Confirmed case-by-case across `cmd/doctor_test.go`
  (~30 sites), `cmd/run_hook_stale_cleanup_test.go` (~25 sites, since re-homed by task 9-12),
  `cmd/hook_sweep_{lock_timeout,outcome,snapshot_order}_test.go`, `cmd/hooks_read_lock_test.go`,
  `cmd/state_daemon_{hook_cleanup,run}_test.go`, `cmd/doctor_{summary,fix_theme}_test.go`,
  `cmd/cleanstale_transient_listpanes_*` and `cmd/rename_restore_cleanup_survival_integration_test.go`.
  Criterion 4 holds.
- Notes:
  - The merge preserved rather than dropped the counting capability `recordingHookKeyLister` carried: the
    `calls` field it contributed is live today at `cmd/doctor_test.go:1080` and `:1102`, where two cases
    assert the marker read stands the check down *before* any live-pane enumeration runs — a property the
    rendered result alone cannot distinguish. That is the load-bearing use of the merge.
  - The one counting assertion that has since disappeared ("ListAllPaneHookKeys call count = 1", formerly
    `cmd/run_hook_stale_cleanup_test.go:253`) survived this commit intact and was removed later, by task 8-44.
    Not this task's loss.
  - Re-pointed fixtures were checked for value collision, not just for name: `ReapableSeedA` (index 0) and
    `LiveSeedA`/`LiveSeedB` (indices 4/5) are distinct, so the divergent-reboot fixture still measures a reap
    beside two retentions rather than three of one kind.

CODE QUALITY:
- Project conventions: Followed. Test-only change; no production file touched. The consolidated fake lives in
  the file whose header comment declares itself the home for "the seam fakes that answer with them"
  (`cmd/hookkey_vocabulary_test.go:1-7`), and the seed vocabulary is reached by name rather than re-derived,
  which is the CLAUDE.md rule for `internal/hookstest`. No `t.Parallel()` introduced. Lane discipline
  unchanged (integration-tagged files stayed integration-tagged).
- SOLID principles: Good. One fake per seam; the surviving fake is named for the seam
  (`stubStaleSweepReader`) rather than for one of its two methods, which is what the old `stubAllPaneLister`
  name got wrong.
- Complexity: Low. Mechanical substitution throughout.
- Modern idioms: Yes.
- Readability: Good. The merged doc comment (`cmd/hookkey_vocabulary_test.go:86-92`) states both halves of
  the fake's job — the fixed answers and the read count — and both claims hold against the code: `calls` is
  incremented at :103 and read by two live assertions.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
