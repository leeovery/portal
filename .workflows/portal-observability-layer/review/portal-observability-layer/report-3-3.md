TASK: Instrument hooks.Store CleanStale batch (per-entry DEBUG, INFO summary, per-entry WARN) and the migrate-rename Save (portal-observability-layer-3-3)

ACCEPTANCE CRITERIA:
1. CleanStale removing N>0 → N DEBUG hooks: clean-stale hook_key=<k> via=internal + one INFO summary hooks: clean-stale entries=N via=internal took=<d>.
2. entries_failed present only when M>0.
3. Whole-batch Save failure → one WARN with error_class from ClassifyWriteError (write-failed-*), NOT unexpected.
4. Zero-removal → no INFO summary, no Save (decision (a)).
5. migrate-rename internal Save emits via=internal breadcrumb at store seam (INFO success / WARN failure); load/collision/save WARN under component hooks.
6. single-batched-Save per-entry-WARN [needs-info] flagged in code comment + PR description.

STATUS: Complete

SPEC CONTEXT:
Spec § State-mutation audit trail (658-727). Store mutation methods are the seam. Batch ops: per-entry DEBUG, one INFO summary (op/entries/entries_failed only if M>0), per-entry WARN (unexpected) on mid-loop failure. error_class rule (710) distinguishes whole-mutation (write-failed-*) from per-entry (unexpected). Cycle catalog lists Hooks CleanStale with batch-summary shape.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/hooks/store.go:249-298 (CleanStale: start, partition, zero-removal early return 277-279, per-key DEBUG 281-283, single batched Save, summary via storelog.EmitCleanStaleSummary success 295/failure 289); internal/storelog/clean_stale.go:48-60 (shared helper, WARN error_class=ClassifyWriteError + log.Took, INFO omits entries_failed; import-cycle rationale documented); cmd/state_migrate_rename.go:51-94 (SaveAudited, hand-rolled WARN removed); store.go:96-105 (SaveAudited INFO/WARN).
- Notes: Decision (a) zero-removal documented (273-276). [needs-info] (no reachable per-entry unexpected WARN under single batched Save) recorded in code comment (242-248) + storelog helper (57-58). Bulk migrate-rename emits entries=N (not per-key), documented.

TESTS:
- Status: Adequate
- Coverage: store_test.go TestCleanStaleLogging (N DEBUG + INFO summary entries=2/via/took, hook_key set; entries_failed omitted; WARN error_class=write-failed-temp-create via 0500 + errors.Is; zero-removal 0 records + mtime); storelog/clean_stale_test.go (success/failure helper); TestSaveAuditedLogging; state_migrate_rename_test.go (one INFO modify component=hooks entries=3 via=internal; WARN modify component=hooks error_class; collision+load WARN under hooks).
- Notes: Behaviour-focused; save-failure asserts raw wrapped error as the value. chmod 0500 + root-skip established pattern. No over-testing.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; log.For("hooks"); terse messages; SetTestHandler/injected-logger paths explained in tests).
- SOLID: Good — storelog extraction DRY (two real consumers, import-cycle documented); SaveAudited keeps emission at store chokepoint.
- Complexity: Low.
- Modern idioms: Yes (log.Took, %w, errors.Is).
- Readability: Good — deliberate spec/code divergence documented inline.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] CleanStale doc comment duplicates the [needs-info] rationale now also in storelog.EmitCleanStaleSummary; a one-line "see storelog.EmitCleanStaleSummary" pointer would prevent drift.
- [idea] SaveAudited accepts arbitrary op string with no validation against the closed op space; sole caller passes correct "modify"; a doc note/debug assertion would harden the seam.
- [idea] AC6 requires the [needs-info] flag in the PR description too; code-comment half verified, PR-description half is a human-reviewer check at PR time.
