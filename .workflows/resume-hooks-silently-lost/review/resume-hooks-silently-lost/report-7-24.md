TASK: resume-hooks-silently-lost-7-24 — "The Repo's Source Guards Hand-Roll The Primitives sourceguardtest Exists To Own"

ACCEPTANCE CRITERIA:
- Both hooks guards keep their own predicate and share one scan skeleton, including the scanned-zero fatal.
- `CalleeName` and `PackageDeps` are exported from `sourceguardtest` with their own coverage.
- No package hand-rolls `go list -deps` or a callee-name unwrapper.
- `sourceguardtest` stays stdlib-only and untagged, so every guard it drives still runs in the unit lane.
- Each converted guard still fails on the violation it was written to catch, and still fatals rather than passing when it scans nothing.

STATUS: complete

SPEC CONTEXT: This is a phase-7 implementation-analysis task, so its authority is its own body rather than
the specification (per the shared verifier context). The guards it consolidates do, however, police
spec-governed properties of this work unit: `internal/hooks/cleanstale_staleness_guard_test.go` pins the
"one exported staleness rule, every reader reaches it" invariant that CLAUDE.md and the spec's hook-staleness
section state (`hooks.StaleKeys` is the single home; the daemon sweep, `doctor --fix` and `doctor`'s
read-only count all route through it), and `internal/hooks/leaf_guard_test.go` pins the hooks store's leaf
status on the hydrate hot path. The task itself is duplication removal in the guard scaffolding beneath them.

IMPLEMENTATION:
- Status: Implemented (and legitimately moved past the task's wording by later phases — see Notes)
- Location:
  - `internal/sourceguardtest/calleename.go:13` — exported `CalleeName(call *ast.CallExpr) string`, the
    three-branch unwrap (Ident / SelectorExpr / neither), documented as ForEachFuncCall's companion.
  - `internal/sourceguardtest/packagedeps.go:152` — exported `PackageDeps(t, pkg, opts...)`, the single
    `go list -deps` exec-and-parse (`:93` `listDeps`, `:115` `parseDeps`) with one fatal wording for an
    unresolvable package (`:169`) and one for an empty set (`:173`).
  - `internal/hooks/cleanstale_staleness_guard_test.go:16,36,68` — both guards in the file now drive
    `sourceguardtest.ParsePackageSources(t, dir, false)` and carry only their own predicate
    (`:38` `CalleeName(call) == "StaleKeys"`; `:71` the mutation/forbidden/receiver triple). The local
    `calleeName` copy is gone; the surviving `calleeReceiverName` (`:81`) is a different unwrap (the
    receiver identifier, not the callee) and was already present before this task.
  - Four leaf guards converted and holding no local enumerator: `internal/nanoid/leaf_guard_test.go:16`,
    `internal/hooks/leaf_guard_test.go:46`, `internal/prefs/leaf_guard_test.go:18`,
    `internal/theme/leaf_guard_test.go:27` — all now route through `sourceguardtest.AssertDepsWithin`,
    which calls the same `packageDeps` (`internal/sourceguardtest/assertdepswithin.go:28`).
  - `internal/sourceguardtest/foreachfunccall_test.go:103` — the former duplicate `callName` now delegates
    to `CalleeName` and only adds the `<func literal>` label its ordering fixtures need.
- Notes: Two documented divergences from the task's literal wording, both sound and both a consolidation
  rather than a loss.
  (1) The task asked for a package-local `scanPackageCalls` in the hooks guard file. It was delivered
  (`internal/hooks/cleanstale_staleness_guard_scan_test.go`, commit 638e3788) and then removed by the later
  task 8-20 (commit 2583a929) in favour of `sourceguardtest.ParsePackageSources`, which owns the same
  enumerate/parse/empty-check skeleton for every guard in the tree rather than for one file. The
  scanned-zero fatal survives in the library: `PackageGoFiles` errors on an empty match
  (`internal/sourceguardtest/packagegofiles.go:31`), `ParsePackageSources` fatals on that error
  (`parsesources.go:46`), and `ParseSources` fatals on an empty parse result (`parsesources.go:85`). The
  criterion's substance — one shared skeleton, predicates only at the guards, no silent scan-nothing pass —
  is met more broadly than asked.
  (2) The four leaf guards now call `AssertDepsWithin` rather than `PackageDeps` directly. That is the same
  primitive with the allowlist assertion lifted into the library, and it adds two anti-vacuity fatals the
  hand-rolled versions never had (`assertdepswithin.go:32` set-does-not-hold-pkg, `:49` allowlist-has-drifted).
  `PackageDeps` remains exported and consumed (`cmd/capturetool/import_guard_test.go:33`), so it is not a
  dead export left behind by the change.
  `TestCleanStaleDoesNotCallStaleKeys` no longer exists under that name — a later phase inverted the rule
  (the clean must now reach the exported `StaleKeys`), and the guard is `TestStalenessRuleHasOneExportedFunction`
  (`:14`). Verified the two functions it names are real and do call it: `deleteStale`
  (`internal/hooks/store.go:318`, calling `StaleKeys` at `:332`) and `checkStaleHooks`
  (`cmd/doctor.go:363`, calling it at `:387`).

TESTS:
- Status: Adequate
- Coverage: All six named tests exist in substance.
  - `internal/sourceguardtest/calleename_test.go:10` identifier call, `:16` selector call, `:22` neither
    shape (immediately-invoked literal → empty) — the three branches of `CalleeName`, one test each.
  - `internal/sourceguardtest/packagedeps_test.go:14` transitive enumeration, `:30` fatal on an
    unresolvable package (asserts the fatal fired, the nil return, and that the message names the command).
  - "it fatals when the shared scan enumerates no files" now sits on the library primitive that replaced
    the local scan: `internal/sourceguardtest/parsesources_test.go:88`
    (`TestParsePackageSources_FatalsWhenThePackageYieldsNoSource`, asserting the fatal, the nil return, and
    that the message names the directory), with `:72` covering the empty-path-set fatal beside it.
- Notes: The transitivity assertion is non-vacuous, which is the property the recorded fix round for this
  task existed to restore. `packagedeps_test.go:23` asserts both `go/parser` and `go/scanner`; I enumerated
  every import block in the package's non-test sources (`assertdepswithin.go`, `buildconstraint.go`,
  `calleename.go`, `foreachfunccall.go`, `gosourcefiles.go`, `packagedeps.go`, `packagegofiles.go`,
  `parsesources.go`, `reposources.go`) — `go/parser` is imported directly by `parsesources.go:5` and
  `go/scanner` is imported by none of them, so it can only arrive through `go/parser`. A `PackageDeps`
  degraded to an immediate-import view would fail this test, which is what protects the three leaf guards
  that police transitive reach.
  The local `recordingT` stub (`packagedeps_test.go:49`) rather than `harnesstest.Recorder` is the
  exception CLAUDE.md names by hand ("`sourceguardtest`'s `PackageDeps` and `commandertest`'s strict
  `RunRaw`"), because the subject is what the helper returns after fatalling, which a stopping stand-in
  cannot observe. It is shared with the `parsesources_test.go` cases in the same test package, so it is one
  stub rather than the two the fix round flagged.
  No over-testing: five focused tests plus the empty-scan case, none redundant with another.

CODE QUALITY:
- Project conventions: Followed. `sourceguardtest` carries no build tag on any source (checked every file —
  the only `go:build` occurrences in the package are string literals inside test fixtures), so every guard
  it drives stays in the unit lane, and its own leaf guard pins both properties across both lanes:
  `internal/sourceguardtest/leaf_guard_test.go:27-32` confines the package to stdlib plus `harnesstest` and
  `portalbintest` under `ForbiddingThirdParty()`, and `:38-44` fails any build constraint on its sources.
  CLAUDE.md's `sourceguardtest` row describes `CalleeName` and `PackageDeps` accurately.
- SOLID principles: Good. `CalleeName` is one unwrap with one reason to change; `PackageDeps` separates the
  enumeration seam (`listDeps`, `:93`) from the policy (`AssertDepsWithin`), which is what let the anti-vacuity
  rules land in one place for all four guards.
- Complexity: Low. `CalleeName` is a two-case type switch; `packageDeps` is resolve-read-list-check.
- Modern idioms: Yes — `strings.SplitSeq` in `parseDeps` (`:117`), `strings.Cut` (`:121`), a functional-options
  `DepsOption` set, and the `harnesstest.TestingT` stand-in rather than a package-local interface.
- Readability: Good. Every fatal wording states the failure mode a guard would otherwise hide ("would pass
  vacuously", "would pass by having stopped looking"), and each guard file now reads as its predicate.
- Issues: None. I checked the remaining `*ast.SelectorExpr` sites across the tree for surviving callee-name
  unwrappers: the candidates (`cmd/open_theme_nomination_test.go:214`,
  `internal/tui/theme_persister_seam_test.go:132`, `internal/portaltest/teardown_guard_coverage_test.go:270`)
  are qualified-path matchers that require receiver identity too, not copies of `CalleeName`. No package
  hand-rolls `go list -deps`: the only `"-deps"` occurrence outside `internal/sourceguardtest` is none, and
  no `packageDeps`/`calleeName` local helper survives anywhere in the tree.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
