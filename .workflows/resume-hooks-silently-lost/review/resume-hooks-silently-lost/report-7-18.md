TASK: resume-hooks-silently-lost-7-18 — "Three Doctor Drivers, Two Of Them Byte-Identical Apart From One Argv Element" (collapse `runDoctorFixCmd` / `runDoctorCmd` / `runDoctor` into one `runDoctorWith` driver plus a thin wrapper)

ACCEPTANCE CRITERIA:
- `cmd/doctor_test.go` holds one doctor driver plus the `runDoctor` wrapper.
- `terminals.json` isolation and the `doctorDeps` install/restore each appear once.
- Every converted call site drives the same argv and reads the same streams it did before.
- The `--fix` and read-only paths are distinguished only by the args passed.
- `go test ./cmd` passes with no doctor test renamed or weakened.

STATUS: complete

SPEC CONTEXT: The specification governs the bugfix behaviour (durable pane-token hook keys, shape-aware stale cleanup in §5, `hooks.json` locking in §6); it says nothing about test drivers. Per the shared verifier context, a phase 6–10 task's authority is its own body. The relevant spec-derived obligation this task must not damage is that `portal doctor` / `doctor --fix` behaviour — the stale-hook prune, its stand-down copy, and the read-only count — stays covered end-to-end through the CLI, which the collapsed driver still exercises (43 call sites across the `doctor_*`, `run_hook_stale_cleanup_*` and `hook_prune_*` suites, both `--fix` and read-only).

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/doctor_test.go:1318-1336` (the single `runDoctorWith` driver), `cmd/doctor_test.go:113-116` (the two-line `runDoctor` wrapper). Task commit `27093eda`; the helper's deps install was later routed through `withDoctorDeps` by task 7-25 (commit `616fead1`).
- Notes:
  - One driver: `runDoctorFixCmd` and `runDoctorCmd` are deleted and no reference to either survives anywhere in the tree (searched `*.go`, both lanes — zero matches). No other test composes `SetArgs([]string{"doctor"…})` (zero matches repo-wide), so `runDoctorWith` is the only argv-level doctor driver.
  - `isolateTerminalsFile` appears exactly once in the doctor suite (`cmd/doctor_test.go:1325`); its other two call sites (`cmd/open_burst_seams_test.go:17,91`, `cmd/spawn_seams_test.go:37`) belong to the spawn-seam suites and are not doctor runs. The `doctorDeps` install/restore appears once in the doctor path (`cmd/doctor_test.go:1326`, via the self-restoring `withDoctorDeps`); the remaining `withDoctorDeps` calls are in `cmd/deps_merge_convention_test.go` and `cmd/seam_staging_test.go`, whose subject *is* the seam-merge/staging convention rather than a doctor run.
  - Conversions are mechanical and stream-preserving: every converted site keeps its receiver positions (`outBuf, _, err` / `outBuf, errBuf, err` / `_, _, err`) and its argv, with `"--fix"` appended exactly where the deleted `runDoctorFixCmd` was called. `--fix` and read-only differ only by that arg.
  - `cmd/doctor_fix_transient_listpanes_integration_test.go:19`'s `runDoctorFixHookPrune` is not a fourth driver — it calls `pruneDoctorStaleHooks` directly with no `rootCmd.Execute`, so it is outside this consolidation.
  - `TestDoctorRejectsArgs` (`cmd/doctor_test.go:395`) gained `terminals.json` isolation and stream capture it previously lacked; argv (`doctor unexpected`) and the assertion are unchanged, and `cobra.NoArgs` rejects before `resolveDoctorDeps` runs, so the added isolation cannot alter the outcome. A strict-reading divergence from "same streams as before" that is strictly safer, not a loss.
  - Flag-leak hazard checked: `resetRootCmd` (`cmd/root_test.go:78-81`) resets `doctorCmd`'s `fix` flag value and `Changed` on every run, so the now-shared driver cannot carry `--fix` from one test into the next.
  - Value-copy semantics checked: `withDoctorDeps(t, *deps)` installs a copy rather than the caller's pointer, but `resolveDoctorDeps` (`cmd/doctor.go:78-80`) copies the installed struct into a fresh one before filling defaults, so no test could observe pointer identity either before or after. All 43 call sites pass a non-nil `*DoctorDeps` (`healthyDoctorDeps`, `staleDeps`, `seedStalePruneFixture`, `downServerDeferFixture`, or a literal), so the dereference is safe.

TESTS:
- Status: Adequate
- Coverage: No new test, correctly — this is a pure test-helper consolidation with no production change (the commit touches `*_test.go` only). Verification is the existing `cmd` doctor suite driven through the collapsed helper: read-only diagnosis (`TestDoctorAllStateChecksPassExitsZero`, `TestDoctorExecuteStaleEntryReturnsUnhealthy`, `TestAdvisories_*`, `TestUnjudgeableHookKeyRetention`'s read-only arm), `--fix` repair (`TestDoctorFixPrunesStaleEntriesThenRediagnosesClean`, `TestDoctorFixProtectsUserHooksWhenLiveSetEmptyOrErrored`, `TestDoctorFixDownServerPrunesProjectsButNotHooks`, `TestDoctorFixReportsLockedHookPrune`, `TestDoctorFixReportsStandDownOnReadFailures`), and stdout/stderr asserted separately still (`TestDoctorExecuteStaleEntryReturnsUnhealthy` reads `outBuf` and asserts `errBuf.Len() == 0`). No test was renamed, deleted or weakened — the diff outside the helper bodies is call-line-only.
- Notes: Judged by reading (no execution). Nothing in the change can alter a test's verdict: same argv, same buffers, same deps, same flag reset.

CODE QUALITY:
- Project conventions: Followed. The helper is `*testing.T`-first with `t.Helper()`, the deps seam is staged through the `withXDeps` helper that pairs its own `t.Cleanup` (the `cmd/seam_guard_test.go` rule — no test assigns `doctorDeps` directly any more), and no `t.Parallel()` is introduced. Lane placement is unchanged (unit-lane test file, no binary built or daemon spawned).
- SOLID principles: Good — the driver does one thing (isolate, install, execute), and the variadic arg is the only axis of variation.
- Complexity: Low — straight-line, no branching.
- Modern idioms: Yes — variadic `args ...string` with `append([]string{"doctor"}, args...)` is the idiomatic shape here.
- Readability: Good. The doc comment on `runDoctorWith` (`cmd/doctor_test.go:1318-1320`) and the inline `terminals.json` comment both hold true against the code, and neither references a task id, phase or spec section.
- Issues: None rising to a finding.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
