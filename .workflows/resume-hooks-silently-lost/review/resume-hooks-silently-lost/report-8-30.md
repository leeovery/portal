TASK: resume-hooks-silently-lost-8-30 — "cmd Carries A Second Seam Family — Nine Package-Level Function Vars Installed By Hand At 113 Sites" (tick-f8a2c7)

ACCEPTANCE CRITERIA:
- No test file assigns a function-var seam directly.
- The guard derives the family from the production declarations, so a seam declared tomorrow is covered without editing a list.
- Every install restores the captured production default, not nil.
- The guard fails on a staged direct assignment and passes on the tree.

STATUS: complete

SPEC CONTEXT: This is a phase-8 implementation-analysis task, not specified bugfix work — per the shared verifier context its authority is its own body rather than the specification. Its subject is the `cmd` package's test-seam discipline: the repo already closed the install-without-restore leak vector for the `*Deps` pointer seams (a named `withXDeps` helper per seam plus a source guard deriving the guarded set from `var xDeps *XDeps` declarations). The task extends both halves — helper and guard — to the second seam family, the package-level function vars a command body calls through, whose production default is a real value rather than nil.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/testhelpers_test.go:113-118` — `withFuncSeam[F any](t, seam *F, replacement F)`: captures `original := *seam`, installs, registers `t.Cleanup(func() { *seam = original })`.
  - `cmd/seam_guard_test.go` — the guard, renamed from `cmd/deps_seam_guard_test.go`. `seamDecl{name, helper}` (73-76) replaces the old bare `[]string`, so the diagnostic names the right helper per family; `declaredSeams` (114-119) unions the two arms; `declaredDepsSeams` (123-126) / `declaredFuncSeams` (130-133) both route through `requireSeams` (137-144), which fatals when an arm comes back empty.
  - `cmd/seam_guard_test.go:176-240` — `funcSeamDecls` + `holdsFunc`: the recogniser. Explicit `*ast.FuncType`, an `*ast.Ident` naming a func type the package declares, else (type absent) a `*ast.FuncLit` initialiser, an `*ast.Ident` naming a package function, or an `*ast.SelectorExpr`.
  - `cmd/seam_guard_test.go:86-110` — `seamAssignments` gained the `TestMain` skip, needed because `persistTranslation` (`cmd/config.go:155`) is now a derived seam and `cmd/testmain_isolation_test.go:75` installs it package-wide with no `*testing.T` to restore into.
  - `cmd/seam_staging_test.go` (renamed from `deps_seam_staging_test.go`) — `TestWithFuncSeam` at :84-103.
  - `CLAUDE.md` — the DI/testing-pattern paragraph rewritten to document the second family, the recogniser's five arms, the generosity trade-off and the `TestMain` exemption; the guard's new filename is correct.
- Notes:
  - Criterion 1 verified by enumeration, not by trust. I derived the seam set by hand from every package-level `var` in `cmd`'s production sources (`grep "^var"` plus the four grouped `var (` blocks in `hooks.go:34`, `root.go:54`, `state_common.go:7`, `state_daemon.go:65`) and ran it against the recogniser's rules: 18 function-var seams — `acquireDaemonLock`, `commanderFactory`, `completionAliasKeys`, `completionSessionNames`, `daemonRunFunc`, `daemonShutdownFunc`, `daemonTickLoopFunc`, `hydrateRunFunc`, `openPathFunc`, `openRawArgs`, `openSessionFunc`, `openTUIFunc`, `osExit`, `persistTranslation`, `runOpenBurstFunc`, `saverMembershipProbe`, `signalHydrateRunFunc`, `versionChecker` — a superset of the nine the task named. Grepping every one of them for a direct assignment or a shadowing `:=` across all `cmd/*_test.go` returns exactly one hit: the `TestMain` install the guard exempts. So the tree is clean and criterion 4's "passes on the tree" holds.
  - The remaining hand-rolled capture-restore pairs in `cmd` tests (`version`, `daemonLockFile`, `fatalErrorStderr`, `hydrateLogger`) are all non-function package vars, correctly outside the family the recogniser derives — a `BasicLit`, a `*os.File` `StarExpr` type, an `io.Writer` `SelectorExpr` type and a `log.For(...)` `CallExpr` respectively. No function-var seam is left hand-installed.
  - Criterion 3 holds by construction and is pinned by `TestWithFuncSeam`, which asserts both that the restore is non-nil and that it is pointer-identical to the captured default (`funcPointer`, `reflect.Value.Pointer`, because func values are not `==`-comparable).
  - The guard's own reach is unchanged and correct: `sourceguardtest.PackageGoFiles(".", true)` enumerates by filename, so integration-tagged test files (`cmd/reattach_integration_test.go`, `cmd/abridged_integration_test.go`) are scanned in the unit lane too — and both were re-pointed.
  - No drift from the task body. The `*Deps` coverage table (`seamStagingCases`) is deliberately not extended to the function family; `cmd/seam_staging_test.go:14-15` states why (one generic helper serves every member, so there is no per-seam helper for a table to cover).

TESTS:
- Status: Adequate
- Coverage: All four tests the task named exist and assert what their names say.
  - `cmd/seam_staging_test.go:85` "it restores the captured production default after the test" — installs over `openTUIFunc` in an inner `t.Run`, then after the inner cleanups have run asserts both non-nil and pointer-identity against the captured production value. This is the one assertion that distinguishes capture-and-restore from restore-to-nil, and it makes it.
  - `cmd/seam_guard_test.go:434` "it flags a direct assignment to a function-var seam" — table-driven per derived seam (not one representative), and asserts the complaint names `withFuncSeam`, so the message routes the reader to the right helper.
  - `cmd/seam_guard_test.go:379` "it derives the function-var seam set from the production sources" — drives `funcSeamDecls` over a staged fixture covering all five accept arms (explicit func type, package-declared func type name, func literal, package-func identifier, qualified name), all four reject arms (qualified explicit type, string literal, non-func identifier, `*Deps` pointer), the grouped-`var` case, and a function-local `var` that must not be picked up. The negative cases are what stop the generous `SelectorExpr` arm from silently becoming "everything".
  - `cmd/seam_guard_test.go:454` "it leaves TestMain's package-wide install alone" — new, and load-bearing: without it the exemption added at :89 would be untested, and it is what makes the guard green on `cmd/testmain_isolation_test.go`.
  - The scanned-nothing tripwire survives the rewrite (`:337`, three shapes: no paths, no test source, only the helper file), backed by `ParseSources`' own fatal at `internal/sourceguardtest/parsesources.go:84-87`.
- Notes:
  - Not over-tested. The derivation test is one table over one fixture rather than a case per arm; the two assignment tests reuse one `runSeamAssignmentGuard` + `captureSeamGuardFailure` pair rather than restating the harness.
  - The ~129 re-pointed call sites keep their existing assertions, verified rather than assumed: filtering the commit's diff over every non-guard test file leaves *only* `withFuncSeam(` additions and `})` closers on the `+` side, and only `orig/prev :=` captures, seam assignments and `t.Cleanup` restores on the `-` side. Test and subtest counts are unchanged in every re-pointed file (`cmd/open_test.go` holds 68 `func Test` before and after); the only count changes in the whole commit are the guard files themselves, where the old 8 subtests are retained (one renamed to "it flags a direct assignment to a *Deps seam") and 5 added.
  - Cleanup ordering was preserved where it is load-bearing: `installStubVersionChecker` (`cmd/version_guard_test.go:26-30`) still registers the seam restore before `resetVersionCheckForTest`, matching the pre-change sequence.

CODE QUALITY:
- Project conventions: Followed. The helper lives in `cmd/testhelpers_test.go` beside the `withXDeps` family as CLAUDE.md's DI/testing section requires; the guard is driven through `internal/sourceguardtest` (`PackageGoFiles`, `ParsePackageSources`, `ParseSources`) rather than re-authoring the enumerate-parse-count loop, and reports through `harnesstest.TestingT` / `harnesstest.Recorder` so its own failure paths are exercisable — both the stated house patterns. CLAUDE.md was updated in the same commit and its claims check out against the code (the `io.Writer`-typed `fatalErrorStderr` it cites is at `cmd/root.go:188`, and `versionChecker` at `cmd/version_guard.go:9`).
- SOLID principles: Good. One generic helper rather than eighteen; `holdsFunc` is a pure predicate separated from `funcSeamDecls`' collection and from `forEachPackageVar`'s traversal, so each of the three is readable and testable on its own.
- Complexity: Low. `holdsFunc` is two flat type switches; the deepest nesting is `forEachPackageVar`'s decl/spec/name walk, which is inherent to the AST shape.
- Modern idioms: Yes. Type parameter on `withFuncSeam`, `slices.SortFunc` / `slices.IndexFunc` / `slices.Equal`, `strings.Compare` as the sort comparator.
- Readability: Good. Every non-obvious decision carries its reason at the declaration — why one helper instead of per-seam, why capture-and-restore instead of restore-to-nil, why the `SelectorExpr` arm is deliberately generous and which mistake that trade prefers, and why `TestMain` is exempt.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
