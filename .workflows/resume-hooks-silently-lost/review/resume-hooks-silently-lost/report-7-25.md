TASK: resume-hooks-silently-lost-7-25 — "Nine *Deps Seams In cmd, One Of Them Guarded" (phase 7, implementation-analysis consolidation; severity: duplication)

ACCEPTANCE CRITERIA:
- Each of the nine seams has an install helper that registers its own restore.
- No `*_test.go` in `cmd` assigns a seam pointer directly.
- The guard covers all nine identifiers and fails when a bare assignment is reintroduced for any of them.
- The guard fatals rather than passing when it scans no files.
- Every converted test drives the same deps it drove before, and `go test ./cmd` plus the integration lane pass.

STATUS: complete

SPEC CONTEXT:
This is a phase-7 task, so its authority is its own body rather than the specification (per the shared verifier context: phases 6–9 are implementation-analysis cycles the implementation generated). The specification mentions `*Deps` seams only incidentally (`specification.md:152`, `:483`, `:518` — that a `cmd` test Executing a real body must inject its seam), and states no requirement about how a seam is staged. The binding convention is CLAUDE.md's "DI / testing pattern" section, which this task also updated: seams are installed through `withXDeps(t, deps)` helpers in `cmd/testhelpers_test.go`, each registering its own `t.Cleanup`, with a source guard failing a direct assignment.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Helpers: `cmd/testhelpers_test.go:26` (`withHooksDeps`), `:35` (`withoutHooksDeps`), `:49` (`withBootstrapDeps`), `:57` (`withOpenDeps`), `:65` (`withOpenBurstDeps`), `:73` (`withDoctorDeps`), `:81` (`withKillDeps`), `:89` (`withListDeps`), `:97` (`withCommitNowDeps`), `:105` (`withUninstallDeps`).
  - Guard: `cmd/seam_guard_test.go:39` (`TestSeamsInstalledOnlyThroughTheStagingHelpers`), `:53` (`runSeamAssignmentGuard`), `:123` (`declaredDepsSeams`).
  - Staging table + restore proof: `cmd/seam_staging_test.go:22` (`seamStagingCases`), `:36` (`TestSeamStagingHelpers`).
  - Delivered by commit `616fead1`; the guard file was later renamed `deps_seam_guard_test.go` → `seam_guard_test.go` and extended with the function-var arm by phase-8 task 8-30 (`c1a52b57`), which is the state verified here.
- Notes:
  - Nine `var xDeps *XDeps` declarations exist in the cmd production sources — `cmd/doctor.go:73`, `cmd/list.go:11`, `cmd/open_burst_run.go:13`, `cmd/hooks.go:40`, `cmd/kill.go:9`, `cmd/open.go:36`, `cmd/state_commit_now.go:30`, `cmd/root.go:36`, `cmd/uninstall.go:20` — and all nine have a helper and a row in `seamStagingCases()`. The task text names two of them by the wrong identifier (`rootDeps` for the `bootstrapDeps` declared at `cmd/root.go:36`, `openBurstRunDeps` for `openBurstDeps` at `cmd/open_burst_run.go:13`); the real declarations are covered, so this is a slip in the plan's prose rather than a gap.
  - No `*_test.go` under `cmd/` assigns any of the nine seams: a scan of every `cmd/*_test.go` for `<seam> =` returns one hit, `cmd/seam_staging_test.go:66`, which is a `==` comparison inside the without-helper test, not an assignment.
  - Guard derivation is structural, not a hardcoded list of nine (`declaredDepsSeams` → `depsSeamDecls`, `cmd/seam_guard_test.go:148`), so a tenth seam is guarded when it is declared, and `requireSeams` (`:137`) fatals if the arm ever comes back empty.
  - Conversion equivalence: across every converted cmd test file in `616fead1` (excluding the four infrastructure files — the new guard, the new staging table, `testhelpers_test.go`, and the deleted `hooks_deps_guard_test.go`), **every** added line is a `withXDeps(t,` opener or a closing `})`/`)`/`}`, and **every** removed line is a bare seam assignment, its paired `t.Cleanup`, a `prev := <seam>` capture, or a closing brace. No dependency content was added, dropped or altered.
  - The pointer-to-value conversions are safe: only two sites installed a pointer variable rather than a literal (`cmd/doctor_test.go`'s `runDoctorWith`, now `withDoctorDeps(t, *deps)` at `cmd/doctor_test.go:1305`; `cmd/open_multitarget_test.go:26`'s `withOpenDeps(t, *deps)`), plus `cmd/state_commit_now_test.go:110` and `cmd/uninstall_test.go:28`. No production code writes a field through a seam pointer (a scan for `<seam>.<Field> =` across `cmd/` finds nothing), no caller mutates its deps after installing, and no `*Deps` struct carries a lock-bearing field, so the value copy neither loses a write-back nor trips `copylocks`.
  - The three `prev`-restoring sites converted to nil-restoring: no test installs the same seam twice in nested scopes and `TestMain` (`cmd/testmain_isolation_test.go:49`) installs no `*Deps` seam, so `prev` was always nil at those sites — restoring nil is the same behaviour.
  - Integration-lane files were converted too (`reattach_integration_test.go`, `abridged_integration_test.go`, `concurrent_coldboot_integration_test.go`); `testhelpers_test.go` is untagged, so the helpers are in both lanes, and the guard's `PackageGoFiles(".", true)` is a directory listing with no build-tag filtering, so integration-tagged sources are scanned from the unit lane.

TESTS:
- Status: Adequate
- Coverage:
  - "it restores the seam when the test that installed it finishes" — `cmd/seam_staging_test.go:39`, run per seam over the nine-row table, with a nested `t.Run` so the inner cleanups have run when the assertion fires, plus a pre-check that the seam was not already installed.
  - "it leaves the seam unset for a test that asks for the production default" — `cmd/seam_staging_test.go:63`.
  - "it fails a test file assigning a seam outside its helper" — present as `cmd/seam_guard_test.go:294` ("it flags a direct assignment to a *Deps seam"), driven per declared identifier against a staged offender fixture, asserting both that the guard failed and that its complaint names the seam. The func-seam twin is `:434`.
  - "it covers every declared seam identifier" — `cmd/seam_guard_test.go:280`, comparing the derived declaration set against the staging table.
  - "it fatals when the guard scans no files" — `cmd/seam_guard_test.go:337`, across three shapes (no paths, no test source, only the helper file); the fatal now arrives from `sourceguardtest.ParseSources` (`internal/sourceguardtest/parsesources.go:85`), whose message carries the "stopped looking" phrase the assertion pins.
  - Negative-control coverage: `cmd/seam_guard_test.go:316` builds a fixture installing every seam through its helper and asserts the guard stays silent.
- Notes: Not over-tested — the guard's own failure paths run through `harnesstest.Recorder` rather than a bespoke stand-in, and the per-seam parameterisation is the coverage the acceptance criteria ask for rather than redundant repetition. The plan's test name "it fails a test file assigning a seam outside its helper" was reworded to "it flags a direct assignment to a *Deps seam" when phase 8 added the function-var arm beside it; the substance is unchanged.

CODE QUALITY:
- Project conventions: Followed. The `t.Parallel()` prohibition still holds (nothing added parallelism); CLAUDE.md's "Build & Test" and "DI / testing pattern" sections were updated in the same commit and the current text names the current filename (`cmd/seam_guard_test.go`), with no stale reference to `deps_seam_guard_test.go` or `hooks_deps_guard_test.go` anywhere outside `.workflows/`. Test-only code stays in `_test.go` files; the guard reaches `internal/sourceguardtest` and `internal/harnesstest` as the repo's other ~20 source guards do.
- SOLID principles: Good. `runSeamAssignmentGuard` takes the seam set, the paths and the helper filename as parameters and reports through `harnesstest.TestingT`, which is what makes its own failure paths testable; the derivation (`declaredDepsSeams`/`declaredFuncSeams`) is separated from the assertion.
- Complexity: Low. The helpers are three lines each; the AST walk is a flat inspect with one predicate.
- Modern idioms: Yes — `slices.IndexFunc`/`slices.SortFunc`/`slices.Equal`, a generic `withFuncSeam[F any]`, and `t.Helper()` on every helper.
- Readability: Good. Each helper's comment states why the install and the restore are written together, and the guard's header states why the seam set is derived rather than listed.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
