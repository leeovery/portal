TASK: resume-hooks-silently-lost-8-8 — "cmd Carries Two Opposing *Deps Merge Conventions, And The Fail-Silent One Is Unguarded" (tick-214d8a, phase 8)

ACCEPTANCE CRITERIA:
- Neither resolver overwrites a production default from the injected struct; both fill unset fields only.
- Every field a test injects is the one the command runs against (existing doctor and commit-now suites pass unchanged).
- A resolver performs no config load for a field the test injected.
- A field added to either *Deps without a fill line is a nil dereference at first use, not a silently ignored mock.

STATUS: issues_found

SPEC CONTEXT: The specification (`.workflows/resume-hooks-silently-lost/specification/.../specification.md`) names neither `DoctorDeps` nor `CommitNowDeps` — grepped, zero hits. This is a phase-8 implementation-analysis task, so per the shared verifier context its authority is its own body. The surrounding bugfix context is the hook-staleness path it serves: `resolveDoctorDeps` supplies the `HookLister`/`HookStore` seams that `checkStaleHooks` (cmd/doctor.go:326) and `pruneDoctorStaleHooks` (cmd/doctor.go:197) run against, so a silently-ignored mock there would let a stale-hook test assert against the real config rather than its fixture.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/doctor.go:79-140` — `resolveDoctorDeps` rewritten in the fill-in direction: `deps := &DoctorDeps{}`, `*deps = *doctorDeps` when non-nil (cmd/doctor.go:80-83), then one `if deps.X == nil` fill per seam. The eleven `if doctorDeps.X != nil` overrides are gone (confirmed against the commit diff, 7212757e).
  - `cmd/state_commit_now.go:54-79` — `resolveCommitNowDeps` rewritten the same way, six fills, six overrides deleted.
  - Reference shape it converges on: `hookSeams` (`cmd/hooks.go:83-104`) — same `var seams; if hooksDeps != nil { seams = *hooksDeps }` then fill-the-unset structure. The task description's cited line (cmd/hooks.go:83) is accurate.
- Notes:
  - AC1 holds: neither resolver reads the package-level var after the initial struct copy. Verified by grep — the only `doctorDeps`/`commitNowDeps` references in non-test sources are the declaration and that copy (cmd/doctor.go:73,81-82; cmd/state_commit_now.go:30,56-57).
  - AC3 holds: each of `loadHookStore`, `loadProjectStore`, `loadPrefsStoreNoMigrate` and `themesDirPath` is now nested under its own `deps.X == nil` / `deps.ThemesDir == ""` gate (cmd/doctor.go:110-133), and `buildProductionSpawnSeams` — which loads `terminals.json` through `buildResolver` (cmd/spawn_seams.go:36,51-57) — is gated on `Detector == nil || Resolve == nil` (cmd/doctor.go:100). Do-item 2's "keep `loadPrefsStoreNoMigrate` as the doctor's non-migrating route" is honoured (cmd/doctor.go:129), so `doctor_persisted_theme_test.go:573`'s guard (that call site must be `doctor.go:resolveDoctorDeps` and nothing else) still reads true.
  - AC4 holds structurally: with the whole injected struct copied in one assignment, a new field is never ignored; a missing fill line leaves the production default nil, which is a nil dereference at first use. Nothing in the resolver can silently substitute a default over an injection.
  - Production behaviour is unchanged: `doctorDeps`/`commitNowDeps` are nil in production, so every fill applies and the side-effect ordering (spawn seams → hook/project/prefs/themes loads) is the same as before.
  - `doctor_spawn_seams_guard_test.go:16-43` (Detector/Resolve must originate from `buildProductionSpawnSeams`) still passes — the call survives, now inside a conditional, and the guard inspects the whole func body.
  - Task Outcome ("one *Deps merge convention across cmd") is met. Grepped every `*Deps` consumer in the package: the four per-field mergers (`hookSeams`, `resolveDoctorDeps`, `resolveCommitNowDeps`, `buildOpenBurstDeps` at cmd/open_burst_run.go:32) now all run the fill-in direction; `buildListDeps` (cmd/list.go:89), `buildKillDeps` (cmd/kill.go:41) and `buildQueryResolver` (cmd/open.go:702) take the injection wholesale with no per-field merge at all, and `buildUninstallDeps` (cmd/uninstall.go:24) fills unset fields inside its injected branch. No fail-silent overwrite remains.

TESTS:
- Status: Adequate
- Coverage (`cmd/deps_merge_convention_test.go`, 436 lines, new in this commit; unit lane, correctly untagged — it builds and execs nothing):
  - `"it runs against the injected seam for every field a test sets"` (line 28) — the plan's table over `DoctorDeps`' exported fields, one `seamCase` per field, each staged through `withDoctorDeps` (so `cmd/seam_guard_test.go`'s no-direct-assignment rule is respected).
  - `"every exported field of DoctorDeps is covered"` (line 37) / same for `CommitNowDeps` (line 105) — `assertSeamCasesCoverFields` (line 366) derives the wanted set from `reflect.TypeFor[T]().Fields()`, so a field added later fails the table rather than silently escaping it. (`reflect.Type.Fields() iter.Seq[StructField]` exists in the Go 1.26 toolchain this module targets — checked in the local GOROOT.)
  - `"it falls through to the production default for an unset field"` (lines 41, 109).
  - `"it loads no hook, project or prefs store when one is injected"` (line 66) — the one test that would actually fail on a regression to the overwrite direction. Its observable is well chosen: the one-shot Application Support migration in `migrateConfigFile` (cmd/config.go:16-48) fires only when production resolves that file's path for itself, so "the file did not move" is proof no load ran. Paired with `"it loads each store the injection left unset"` (line 82) asserting the same observable inverted, which is what stops the first test passing vacuously.
  - Isolation is sound: `stageApplicationSupportConfig` (line 397) re-points `HOME` and `XDG_CONFIG_HOME` at temp dirs and clears the three `PORTAL_*_FILE` overrides (empty ⇒ unset per `xdg.ConfigFilePath`, internal/xdg/configfile.go:76), so nothing outside the test's temp dirs is read or moved. No `t.Parallel`.
- Notes:
  - Mild redundancy, not worth acting on: in the new fill-in shape the whole per-field table is one struct-copy statement's property, so the eleven `DoctorDeps` subtests assert one assignment eleven times — but the table was prescribed verbatim by the task's Tests list, and it doubles as the input to the coverage guard.
  - The fall-through subtests enumerate their six/five fields by hand rather than deriving them; a seam added without a fill line would not fail them. That is by design — AC4's contract is a loud nil dereference in production, not a test that catches it — and the store fills that the hand list omits are covered by the migration-observable pair above.

CODE QUALITY:
- Project conventions: Followed. Unit-lane placement correct; seams staged through `withDoctorDeps`/`withCommitNowDeps` per the `withXDeps` rule in CLAUDE.md; no `t.Parallel`; no logger constructed in the test; no `*Deps` assigned directly.
- SOLID principles: Good. The resolver keeps its single responsibility (merge + default) and the merge direction is now uniform across the package.
- Complexity: Low. Flat sequence of independent guards; the only nesting is the `Detector`/`Resolve` pair, which shares one `buildProductionSpawnSeams` construction between two fills.
- Modern idioms: Yes — `reflect.TypeFor`, range-over-func `Fields()`, `slices.Sort`/`slices.Equal`. Consistent with the `modernize` linter the repo enables.
- Readability: Good. Both resolver doc comments state the direction and why it is the safe one; the `// Each best-effort load is constructed only for a seam left unset` comment (cmd/doctor.go:107-108) names the property AC3 asks for. No process-artifact references (no task ids, phases or spec sections) in any comment added by this commit.
- Issues: None beyond the finding below.

BLOCKING ISSUES:
- None. All four acceptance criteria are met in substance.

FINDINGS:
- [in-scope] [contained] cmd/deps_merge_convention_test.go:303 — the `Commit` seam case calls `resolveCommitNowDeps().Commit("", state.Index{}, false, nil)` with an empty state dir; pass `t.TempDir()` instead (the injected seam ignores its arguments, so the assertion is unaffected). FAILS: on the exact regression this case exists to catch — the resolver overwriting the injected `Commit` with `state.Commit` — the production function runs with `dir == ""`, so `SessionsJSON("")` resolves to the relative path `sessions.json`, `structuralChange` finds no prior file and returns true (internal/state/commit.go:31,46-50), and `fileutil.AtomicWrite0600` writes `sessions.json` into the test's working directory, i.e. `cmd/` in the repo (internal/state/commit.go:35). That is a test writing outside its temp dirs, which CLAUDE.md's ABSOLUTE INVARIANT forbids without qualification. The sibling cases are already safe under the same regression — `ReadIndex("")` only reads, `TouchSaveRequested("/sentinel/state")` fails at `os.OpenFile` (internal/state/paths.go:60), and the tmux-touching doctor seams dial the socket `TestMain` poisons (cmd/testmain_isolation_test.go:72) — so `Commit` is the single site.

VERIFICATION METHOD: implementation and tests judged by reading, per the reviewer's no-execution rule; no suite was run and no build was performed. AC2 ("existing doctor and commit-now suites pass unchanged") is supported by the commit touching no existing test file (7212757e changes only `cmd/doctor.go`, `cmd/state_commit_now.go` and the new `cmd/deps_merge_convention_test.go`) plus a read of the guards that name these resolvers, not by execution.
