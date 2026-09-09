TASK: resume-hooks-silently-lost-6-23 — Rename The Remaining Concern-Named Test Files In cmd (tick-b0fc73, severity low, source: bank)

ACCEPTANCE CRITERIA:
- Every test file in `cmd` exercising `doctor.go` or `run_hook_stale_cleanup.go` is named after a source file.
- Build tags and lane membership are unchanged.
- No test content changed.

STATUS: complete

SPEC CONTEXT:
This is a phase-6 implementation-analysis task, not specified work — per the shared verifier context, its authority is
its own body rather than the specification. The convention it enforces is the repo's own: a `cmd` test file is named
for the source file it exercises (`doctor_*_test.go`, `run_hook_stale_cleanup_*_test.go`), which is how a reader locates
a source file's coverage by filename. The bank entry named three surviving breaches predating the work unit; the task
body additionally records that a fourth named file (`cmd/hookkey_no_regression_upgrade_test.go`) no longer exists.

IMPLEMENTATION:
- Status: Implemented
- Location: commit `32161a93` — seven renames under `cmd/`, git-detected at similarity index 100%, 0 insertions /
  0 deletions:
    cmd/cleanstale_transient_listpanes_doctorfix_integration_test.go -> cmd/doctor_fix_transient_listpanes_integration_test.go
    cmd/cleanstale_transient_listpanes_shared_test.go                -> cmd/doctor_fix_transient_listpanes_shared_integration_test.go
    cmd/hooks_cleanstale_single_caller_guard_test.go                 -> cmd/run_hook_stale_cleanup_single_caller_guard_test.go
    cmd/rename_restore_cleanup_survival_integration_test.go          -> cmd/run_hook_stale_cleanup_rename_survival_integration_test.go
    cmd/hook_sweep_lock_timeout_test.go                              -> cmd/run_hook_stale_cleanup_lock_timeout_test.go
    cmd/hook_sweep_outcome_test.go                                   -> cmd/run_hook_stale_cleanup_outcome_test.go
    cmd/hook_sweep_snapshot_order_test.go                            -> cmd/run_hook_stale_cleanup_snapshot_order_test.go
- Notes:
  - The three bank-named files are all covered (renames 1, 2 and 3 above). The remaining four are the `hook_sweep_*`
    files this work unit had itself authored in earlier phases — i.e. Do item 3 ("re-check the package for any further
    breach introduced since") was actually executed rather than skipped.
  - The task body's claim that `cmd/hookkey_no_regression_upgrade_test.go` no longer exists is true: it was deleted in
    commit `227240d5` (task 2-1), so it is absent at `32161a93^`.
  - Rename 2 is the only one whose lane suffix changed (`_test.go` -> `_integration_test.go`). This corrects a name that
    previously did not advertise its tag; it does not move the file between lanes, because `integration` is not a GOOS
    or GOARCH so the filename fragment carries no implicit build constraint — lane membership is decided by the
    `//go:build integration` line, and the file's content was not touched.
  - No stale reference to any of the seven old filenames survives in Go source, `README.md`, `CLAUDE.md` or
    `.golangci.yml`; the only remaining occurrences are in `.workflows/` records of prior work units, which are
    historical documents and correctly left alone.
  - Judged and not reported: at `32161a93`, `cmd/noncontiguous_window_reboot_integration_test.go` called
    `runHookStaleCleanup` while carrying a concern name, which is a literal reading of criterion 1 left unmet. It is
    not a finding — its subject is a reboot round-trip that drives the cleanup as one step of a scenario rather than as
    the file's subject, and at HEAD the call is `hooksweep.Run`
    (`cmd/noncontiguous_window_reboot_integration_test.go:194`) with no `cmd/run_hook_stale_cleanup.go` left to be named
    after, so there is no remedy to prescribe.
  - Also judged and not reported: `cmd/hooks_read_lock_test.go` drives both `runDoctorDiagnosis` and
    `runHookStaleCleanup` but is named for `cmd/hooks.go`. That satisfies criterion 1 as written (it is named after a
    source file) and its subject really is the hooks store's degraded read, observed through those two callers.
  - Out of this task's change-set: phase 9 (`a4898f41`) later renamed five of these seven again to `hook_prune_*` /
    `hook_sweep_*` and moved the sweep to `internal/hooksweep`, retiring `cmd/run_hook_stale_cleanup.go`. That is a
    later, separately-planned task with its own review; it is not a defect in this one, and this task's output was
    correct for the tree it landed in.

TESTS:
- Status: Adequate (n/a by construction)
- Coverage: Unchanged. Seven byte-identical file moves within one package; no test body, table, assertion or helper was
  added, removed or edited. Verified per-file build tags at the task commit: the three `_integration_test.go` targets
  each begin `//go:build integration`; the four untagged targets each begin `package cmd` and carry no `_integration`
  suffix — so the name/tag correspondence holds in both directions after the rename and no test changed lane.
- Notes: The commit message's "unit 747 tests, integration 781, same names" is structurally consistent with what the
  diff can do — within a single package, a content-identical rename cannot change the set of test functions compiled
  into a lane unless a build tag moves, and none did. Not independently re-run (test execution is out of this
  reviewer's remit).

CODE QUALITY:
- Project conventions: Followed. Both new `doctor_*` names and the five `run_hook_stale_cleanup_*` names derive from the
  `cmd` source file each exercised at the time. The `_integration_test.go` suffix and `//go:build integration` tag are
  kept in agreement on every file, which is the repo's lane-purity convention from CLAUDE.md.
- SOLID principles: N/A (no code changed).
- Complexity: N/A.
- Modern idioms: N/A.
- Readability: Good — locating a source file's coverage by filename now works for every file the task touched.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
