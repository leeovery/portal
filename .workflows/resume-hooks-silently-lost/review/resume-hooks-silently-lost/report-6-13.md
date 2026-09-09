TASK: resume-hooks-silently-lost-6-13 — Make The hooksDeps Install-And-Restore Pair Inseparable (tick-bd43fd, phase 6 consolidation)

ACCEPTANCE CRITERIA:
- No test file assigns `hooksDeps` directly.
- Every conversion preserves the exact deps struct the site installed.
- The package's verdict count is unchanged.
- Tests: a unit guard asserting no `*_test.go` in `cmd` assigns `hooksDeps` outside the helper (source-walking, via `sourceguardtest`, in the repo's existing guard style).

STATUS: complete

SPEC CONTEXT: None governing. This is a phase-6 implementation-analysis consolidation task, and per the shared verifier context its authority is its own body. The specification only touches the `cmd` `*Deps` seam family in passing (`.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md:152,483` — the `hook` command reaches tmux "through the `hook` command's existing `*Deps` seam", and a `cmd`-level test asserts CLI failure propagation "with its `*Deps` seam injected"). It says nothing about how a test stages that seam, so nothing here can drift from it. The binding rule is `CLAUDE.md`'s DI/testing section: mocks are installed "through the `withXDeps(t, deps)` staging helpers in `cmd/testhelpers_test.go`, each of which registers its own restore in the same breath" — which this task is what created for the hooks seam.

IMPLEMENTATION:
- Status: Implemented (and since generalised by a later task, correctly)
- Location:
  - `cmd/testhelpers_test.go:26-30` — `withHooksDeps(t *testing.T, deps HooksDeps)`, assigning `hooksDeps = &deps` and registering `t.Cleanup(func() { hooksDeps = nil })` in the same body.
  - `cmd/testhelpers_test.go:35-39` — `withoutHooksDeps(t *testing.T)`, the counterpart for a case whose subject is the production default (this was not asked for by the task; it is what let the one `hooksDeps = nil` site in `cmd/hooks_seams_test.go:14` state its precondition rather than keep a bare assignment the guard would flag).
  - `cmd/seam_guard_test.go:39-69` — the guard at HEAD (`TestSeamsInstalledOnlyThroughTheStagingHelpers` → `runSeamAssignmentGuard`), source-walking the `cmd` package via `sourceguardtest.PackageGoFiles(".", true)` and reporting every `*_test.go` outside `testhelpers_test.go` that assigns a declared seam.
- Notes:
  - Completeness verified two ways. At the task's own commit `b0ae1294`, `git grep "hooksDeps = "` over `cmd/*_test.go` returns only the four lines inside `testhelpers_test.go` (the two helper bodies). At HEAD, `grep -rn "hooksDeps" cmd/*.go` returns only `cmd/hooks.go:40,85,86` (the declaration and the production read), `cmd/testhelpers_test.go:28,29,37,38` (the helpers), and `cmd/seam_staging_test.go:66-75` (comparisons against nil, not assignments). No test file assigns the seam directly — criterion 1 holds.
  - The conversion reached **more** sites than the task inventoried, which is the right direction: the description listed 69 across six files, and the commit converted 72 `withHooksDeps` sites across seven (`hooks_test.go` 38, `hooks_rm_exit_test.go` 11, `hooks_write_lock_test.go` 8, `hooks_pane_token_test.go` 8, `hooks_seams_test.go` 3, `hooks_read_lock_test.go` 2, `root_test.go` 2) plus the one `withoutHooksDeps`. `cmd/hooks_seams_test.go` was absent from the task's file list and was converted anyway; had it been missed, the new guard would have failed.
  - Criterion 2 (exact struct preserved) holds at every site: reading the whole diff of `b0ae1294`, each conversion is the same composite literal moved inside the call, with no field added, dropped or re-pointed. The value-parameter signature (`deps HooksDeps`, stored as `&deps`) is safe here because no site constructs a named `HooksDeps` variable and mutates it after installing — `grep -rn "HooksDeps" cmd/*_test.go` outside the `withHooksDeps(t, HooksDeps{` form returns only the helper declarations and the staging-test names. Fakes are still shared by pointer (`resolver`, `lister`, `stamper`), so the post-run assertions on their recorded calls are unaffected.
  - The guard file this task authored (`cmd/hooks_deps_guard_test.go`) no longer exists at HEAD: task 7-25 (`616fead1`) generalised it into `cmd/seam_guard_test.go`, which derives the whole seam vocabulary from the production sources instead of hardcoding one identifier. That is a legitimate later evolution, not a loss — `hooksDeps` is still in the guard's vocabulary by construction (`depsSeamDecls`, `cmd/seam_guard_test.go:148-162`, matches `var hooksDeps *HooksDeps` at `cmd/hooks.go:40` and derives the helper name `withHooksDeps`, which is the real function), and `TestSeamGuard/it flags a direct assignment to a *Deps seam` (`cmd/seam_guard_test.go:294-314`) sub-tests per declared seam, so the `hooksDeps` arm is demonstrated individually rather than assumed.
  - Criterion 3 (verdict count unchanged) is not directly observable by reading, but the change is mechanical: no test function was added, removed or renamed by the conversion, and the only new verdicts are the guard's own.

TESTS:
- Status: Adequate
- Coverage:
  - The rule itself: `cmd/seam_guard_test.go:39-45` runs the assignment guard over the live `cmd` package. Its own behaviour is pinned by `TestSeamGuard` (`:279-356`) — it flags an offender fixture per declared seam and names the seam in the complaint, it passes a fixture installing every seam through its helper, and it fatals in three ways when it scanned nothing ("no paths at all", "no test source among them", "only the helper file"), so a guard that had quietly stopped looking cannot pass.
  - The helper's contract: `cmd/seam_staging_test.go:22-56` tables `hooksDeps` alongside the other eight `*Deps` seams (`:27`) and asserts, per seam, that it is unset before, set inside the installing sub-test, and unset again once that sub-test's cleanups have run — i.e. the test fails if the restore were ever dropped from the helper.
  - The counterpart route: `TestWithoutHooksDeps` (`cmd/seam_staging_test.go:62-78`) pins that `withoutHooksDeps` clears an installed seam and leaves it clear afterwards.
- Notes: Would the tests fail if the work broke? Yes on both halves — deleting the `t.Cleanup` from `withHooksDeps` fails the `hooksDeps` row of `TestSeamStagingHelpers`; reintroducing a bare `hooksDeps = &HooksDeps{…}` in any `cmd` test file fails `TestSeamsInstalledOnlyThroughTheStagingHelpers`. Not over-tested: the three suites have distinct subjects (the source rule, the restore behaviour, the without-route), and the per-seam sub-test loops are one assertion set per seam rather than repeated coverage of one.

CODE QUALITY:
- Project conventions: Followed. The helper sits in `cmd/testhelpers_test.go`, the staging home `CLAUDE.md` names, and matches the shape of the other eight `withXDeps` helpers beside it (`cmd/testhelpers_test.go:48-106`) — same signature form, same body, same doc-comment register. The guard is source-walking via `sourceguardtest` in the repo's established guard style and runs in the unit lane, as required.
- SOLID principles: Good. One helper, one job; the seam pointer stays package-private and the production read at `cmd/hooks.go:85-86` is untouched.
- Complexity: Low. Four-line helper; the guard's traversal is a single `ast.Inspect` over assignment statements.
- Modern idioms: Yes. `t.Helper()` on both helpers so a failure is attributed to the call site; `t.Cleanup` rather than a returned restore func the caller could forget to defer.
- Readability: Good. Both doc comments state the hazard (the seam outlives the test that set it) rather than restating the code, and neither references a task id, phase or spec section.
- Issues: None found.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
