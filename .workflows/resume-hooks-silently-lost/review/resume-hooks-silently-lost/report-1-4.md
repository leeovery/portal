TASK: resume-hooks-silently-lost-1-4 — A Stood-Down Cycle Reaches The Caller

ACCEPTANCE CRITERIA:
- Both stand-down branches invoke `onSkipped` exactly once with their reason, and a nil `onSkipped` is safe on both
- The empty-live-set stand-down logs at WARN under the `hooks` component with `op=clean-stale-skipped`, `via=internal`, `reason=empty-pane-read` and the persisted `entries` count; the restore stand-down keeps its DEBUG level
- An empty live set with zero persisted entries returns silently: no log record, no `onSkipped` call, no output
- The enumeration-error branch is unchanged — its existing WARN on the injected logger, `return nil`, and no skipped line
- `portal doctor --fix` prints `Skipped stale hook prune: restore may be in progress` / `… could not read live panes`, on the same writer and in the same repair block as `Pruned stale hook:` lines
- `portal doctor --fix`'s exit code is unchanged by a stand-down — driven solely by the post-repair diagnosis
- `hooks.json` is byte-identical across every stand-down path

STATUS: complete

SPEC CONTEXT:
§5.1 requires a stood-down cycle to reach its caller the way a removal does, so `portal doctor --fix` prints a fixed line rather than nothing; §5.4 requires one greppable line shape (`op=clean-stale-skipped`, `via=internal`, `reason=…`) covering the decline reasons, WARN for anomalies and DEBUG for an expected restore window; §6.5 requires the stand-down to leave the exit code to the post-repair diagnosis. Three corrigenda govern this task's letter and are authoritative over both the spec body and the task text:
- 2026-08-30 / 2026-09-04: the closed reason set grew from three to six — `store-read-failed`, `pane-read-failed` and `marker-read-failed` are now named declines rather than paths that recorded nothing. This deliberately supersedes the task's "the enumeration-error branch is unchanged / emits no skipped line".
- 2026-09-01: `Skipped stale hook prune: could not read live panes` is **withdrawn**, not reassigned — the guard is a successful read that returned no panes, so it renders `live pane list came back empty` and the failed enumeration renders `could not enumerate live panes`.
- 2026-09-04: `marker-read-failed` splits out of `restoring`, which is why the task's deliberately-weakened `restore may be in progress` is no longer needed: `restoring` now means a marker genuinely read as set, so `restore in progress` is true of it, and a failed read (the ordinary case on a down server for this bootstrap-exempt command) gets its own `could not read the restore marker`.

IMPLEMENTATION:
- Status: Implemented (delivered at e2061594, then legitimately re-homed by the later consolidation phases from `cmd/run_hook_stale_cleanup.go` into `internal/hooksweep`)
- Location:
  - `internal/hooksweep/reason.go:19-25` — the closed `Reason` vocabulary, `internal/hooksweep/reason.go:35-42` the enumerable `Reasons`.
  - `internal/hooksweep/standdown.go:12-16` — the single line shape (`op`/`via`/`reason`), `:35-37` its one emission point, `:41-49` the DEBUG/WARN level split.
  - `internal/hooksweep/sweep.go:98-105` — the empty-live-set guard, declining WARN with `reason=empty-pane-read` and the `entries` count; `:155-157` the restore stand-down taken before the store is read; `:174-194` the store-side failure classification; `:199-202` `standDownOutcome`, which emits once and reports the reason back through `Outcome.DeclineReason`.
  - `cmd/doctor.go:197-212` — `pruneDoctorStaleHooks` renders `Pruned stale hook:` and the skipped line on the same writer; `:220-254` the two shared phrase vocabularies; `:260-268` `phraseFor` / `reportSkippedPrune`; `:164-177` the exit code driven solely by the post-repair diagnosis.
  - `cmd/state_daemon.go:214` — the daemon discards the outcome (the "nil `onSkipped`" call shape, structurally safe now that the report is a return value).
- Notes: the callback the task specified (`onSkipped func(reason string)` plus `skipReason*` string constants) was superseded by a typed `Outcome{Removed, DeclineReason}` return and a typed `Reason` vocabulary. This is a strict improvement on the criterion's substance: a nil callback can no longer be got wrong, the reason cannot be reported as an untyped string, and the logged value and the printed line still cannot drift (both resolve from the same `hooksweep.Reason`). The task's instruction to fall back to the raw reason value for an unmapped reason was also deliberately reversed — `phraseFor` (`cmd/doctor.go:260-262`) returns nothing and exhaustiveness is enforced structurally instead (see TESTS). That is defensible and better: the enum slug can never leak into user-facing copy, and the guard chain makes an unmapped reason unreachable rather than merely tolerable.
  One further supersession worth recording, not a loss: the zero-persisted-entries path now emits a DEBUG counts line (`internal/hooksweep/sweep.go:134-136`) where the criterion said "no log record". It is per-cycle DEBUG detail in the house convention, carries no `panes` figure because none was taken, and reports no stand-down — the criterion's substance (no `onSkipped`/decline report, no user-visible output) holds, verified at `internal/hooksweep/sweep_test.go:340-357`.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/doctor_stand_down_copy_test.go:56-123` is the single table binding, per reason, the log level and attrs, the exact `--fix` repair line, the read-only diagnosis line, and the untouched `hooks.json`. `:338-358` proves the table covers `hooksweep.Reasons` whole with no subtraction list; `:381-392` drives both the sweep (`assertStandDownSweep`, `:289-309`) and a real `doctor --fix` Execute (`assertStandDownRepair`, `:313-336`) for every row.
  - Criterion 2: `emptyPaneReadStandDownAttrs` (`:160-166`) pins `entries=2` and the absence of an `error` attr; `assertStandDown` (`internal/hooksweep/helpers_test.go:114-138`, mirrored at `cmd/hookkey_vocabulary_test.go:236-261`) pins level, message, `component=hooks`, `op=clean-stale-skipped`, `via=internal` and `reason`. The restore row pins DEBUG (`cmd/doctor_stand_down_copy_test.go:63`).
  - Criterion 5: `assertSkippedPruneLine` (`cmd/doctor_test.go:1587-1615`) asserts the exact line, that it appears exactly once, that no other `Skipped stale hook prune:` line appears, that no `Pruned stale hook:` line accompanies it, and — the placement question the task called out — that it precedes the project-prune lines, i.e. sits in the hook-prune repair block.
  - Criterion 6: `:440-466` runs three Executes per reason — read-only healthy (nil error), `--fix` with an injected failing check (`ErrDoctorUnhealthy`), read-only with the same failing check — so the stand-down is shown to move neither exit code.
  - Criterion 7: `assertHooksPathUnchanged` (`:280-285`) is applied on both the sweep and the repair arm for all six reasons, over a state renderer that handles the file, absent and directory cases.
  - Criterion 3: `internal/hooksweep/sweep_test.go:340-357` (no decline, no WARN) plus `:504-520` (the counts line carries `entries=0` and no `panes` attr).
  - Exhaustiveness backing the no-fallback decision: `cmd/doctor_stand_down_phrase_guard_test.go:47-94` fails any declared reason missing from either vocabulary and any vocabulary key naming no declared reason, and `internal/hooksweep/reason_enumeration_guard_test.go:22-57` fails a declared `Reason` const left out of `Reasons`. Both guards carry their own rule tests over synthetic sources, so a guard that stopped catching the omission fails rather than passing quietly.
- Notes: no over-testing found. The six-row table multiplies executions, but each row is a distinct decline path with distinct copy, and the shared assertion helpers keep one expectation per property. The uniqueness subtest (`:364-379`) reads *rendered* phrases rather than the table's expectations, so it catches two reasons converging on one wording — the drift that matters — rather than an author copying a row.

CODE QUALITY:
- Project conventions: Followed. The `hooks` component binding is the package's own (`internal/hooksweep/sweep.go:14`) with the whole cycle emitting under it, which is what keeps a caller from adding a second line for the same event (proved by `cmd/hook_prune_single_report_test.go:18-68`). `op=clean-stale-skipped` is the spec-governed new value and `reason` an existing attr key; no new component or attr was invented. Unit-lane placement is correct — nothing here builds or spawns a binary.
- SOLID principles: Good. Emission is the sweep's alone; the diagnosis calls `StalenessStandDown`/`JudgeAgainstLivePanes` for their verdicts and deliberately does not emit (`internal/hooksweep/standdown.go:20-21`, `cmd/doctor.go:375-382`), so the reaper and the diagnosis cannot disagree and cannot double-report.
- Complexity: Low. `declinedSweep` is a flat classification switch; the stand-down carries its own level and attrs so no call site restates them.
- Modern idioms: Yes — typed `Reason` over stringly-typed reasons, `errors.As` on a `declinedError` carrying the stand-down in transit.
- Readability: Good. The comments in the changed code hold against it: the `View.PaneRows` "meaningful only where Enumerated is set" claim is true at its one consumer (`cmd/doctor.go:384`, reached only past a non-declined view), and `phraseFor`'s "exhaustiveness is a test's to enforce" is backed by the two guards above rather than asserted.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
