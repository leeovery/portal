TASK: resume-hooks-silently-lost-8-41 — "Two Leaf Packages Have No Guard, And One Of Them Now Underwrites A Test-Isolation Guarantee" (tick-d5693e)

ACCEPTANCE CRITERIA:
- Both packages' transitive dependency sets are pinned by a guard.
- Each guard fails on an empty or unresolved set.
- Both guards run in the unit lane.
- Each guard states what its property underwrites.

STATUS: complete

SPEC CONTEXT:
This is a phase-8 task — one of the implementation-analysis cycles the implementation phase generated, not specified bugfix work — so per the shared verifier context its authority is its own body rather than the specification. The body asks for two leaf-dependency guards modelled on `internal/nanoid/leaf_guard_test.go`: one over `internal/xdg` (whose leaf status underwrites `internal/hookstest` resolving `hooks.json` by the same rule as the binary under test, so a destructive suite cannot seed a file nothing reads) and one over `internal/sourceguardtest` (whose stdlib-only, untagged contract is what keeps the module's ~20 source guards in the unit lane). CLAUDE.md states both properties in prose already; this task converts them into tests.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/xdg/leaf_guard_test.go:1-25 — `TestXDGPackage` / subtest `"it confines internal/xdg to the standard library"`, `AssertDepsWithin(t, xdgPkg, nil, ForbiddingThirdParty(), lane)` over `sourceguardtest.Lanes()`.
  - internal/sourceguardtest/leaf_guard_test.go:1-45 — `TestSourceGuardTestPackage` with two subtests: the dependency assertion (allowlist `internal/harnesstest` + `internal/portalbintest`, `ForbiddingThirdParty()`, both lanes) and `"it carries no build tag, so the guards built on it stay in the unit lane"` over `ParsePackageSources(t, ".", false)` + `BuildConstraint`.
  - Commit 84a5ed6a added both files; 33e01bda and b74712bd (task 9-19) later folded them onto the shared `Lanes()` / `BuildConstraint` primitives.
- Notes:
  - The dependency claims hold as written. `internal/xdg`'s three production sources import only `fmt`, `os`, `path/filepath`, `strings` (internal/xdg/xdg.go:5-9, internal/xdg/configfile.go:3-6, internal/xdg/configdir.go:3), so the `nil` allowlist under `ForbiddingThirdParty()` is satisfiable.
  - `internal/sourceguardtest` reaches `internal/harnesstest` (internal/sourceguardtest/assertdepswithin.go:6) and `internal/portalbintest` (internal/sourceguardtest/reposources.go:6-7), and both of those are themselves stdlib-only (internal/harnesstest/{poll,progress,recorder}.go, internal/portalbintest/build.go:3-9), so the two-entry allowlist is exactly the transitive set — `AssertDepsWithin`'s `sawAllowed` anchor (internal/sourceguardtest/assertdepswithin.go:48-50) will not fire.
  - The rationale the task asked for is stated in each guard: the `hookstest` seed-and-read parity at internal/xdg/leaf_guard_test.go:12-19, and the unit-lane placement of the module's source guards at internal/sourceguardtest/leaf_guard_test.go:16-21 and :35-37.
  - The `internal/sourceguardtest` guard's allowlist is not literally empty, unlike the task's "stdlib-only" wording. That is the correct reading rather than drift: the package genuinely depends on the two helpers, CLAUDE.md already records the contract as "stdlib-only beyond the shared `harnesstest.TestingT` and `portalbintest`", and both admitted packages are themselves stdlib-only and untagged, so they drag neither a dependency nor a lane onto the guards built here — which the comment at internal/sourceguardtest/leaf_guard_test.go:23-26 says.
  - The build-tag subtest deliberately scans production sources only (`includeTests=false`, internal/sourceguardtest/leaf_guard_test.go:39). That is the right scope: a tag on a `_test.go` file of this package gates nothing for a consumer, whereas a tag on a primitive's source would. A tag on `harnesstest`/`portalbintest` is not scanned, but it cannot land silently — `reposources.go` and `assertdepswithin.go` are untagged, so a tagged dependency would fail to build in the default lane rather than pass unnoticed.

TESTS:
- Status: Adequate
- Coverage: The guards *are* the tests this task was asked for, and both prescribed names are present verbatim — `"it confines internal/xdg to the standard library"` (internal/xdg/leaf_guard_test.go:20) and `"it confines internal/sourceguardtest to the standard library"` (internal/sourceguardtest/leaf_guard_test.go:22) — plus the untagged assertion the Do list's item 2 asked for.
- Notes:
  - Fail-on-empty/unresolved is inherited rather than restated: `packageDeps` fatals both on a `go list` error and on a zero-length set (internal/sourceguardtest/packagedeps.go:168-175), and `AssertDepsWithin` adds two further fatals for a set not holding the package itself and for a drifted non-empty allowlist (internal/sourceguardtest/assertdepswithin.go:30-34, :48-50). `ParsePackageSources` is likewise fatal on an empty enumeration and on an unparseable file (internal/sourceguardtest/parsesources.go:44-49, :84-87). Those fatal paths carry their own direct coverage in `TestPackageDeps_FatalsWhenGoListCannotResolveThePackage` (internal/sourceguardtest/packagedeps_test.go:30), `TestAssertDepsWithin_FatalsOnAnEmptyDepSet` (internal/sourceguardtest/assertdepswithin_test.go:43) and its two siblings at :54 and :65, so the criterion is proven at the primitive rather than duplicated per guard — the right place for it.
  - Both lanes are exercised via `Lanes()` (internal/sourceguardtest/packagedeps.go:42-44), so a dependency reachable only from an integration-tagged source is judged rather than resolved away.
  - Not over-tested: neither guard restates the primitive's own behaviour, and neither adds a negative-case fixture of the kind `internal/logtest/leaf_guard_test.go` needs (that guard narrows its own two-entry allowlist to show the assertion bites; here the empty and the fully-consumed allowlists give no equivalent free negative, and `assertdepswithin_test.go` already stages one over fixtures).
- Lane: neither guard file carries a build constraint, and neither `internal/xdg` nor `internal/sourceguardtest` holds a tagged production source, so both run under `go test ./...` as required.

CODE QUALITY:
- Project conventions: Followed. Both files sit in the `<pkg>_test` external package, name their subject through a package-level `const <pkg>Pkg` import path, and route through `sourceguardtest.AssertDepsWithin` — the same shape as `internal/harnesstest/leaf_guard_test.go`, `internal/shellquote/leaf_guard_test.go`, `internal/nanoid/leaf_guard_test.go`, `internal/prefs/leaf_guard_test.go` and `internal/theme/leaf_guard_test.go`. No `t.Parallel()`, consistent with the repo rule.
- SOLID principles: Good — the guards state a rule and delegate the enumeration, judgement and reporting to the shared primitive; no scanning logic is re-authored. The original commit's local `buildConstraintLine` helper was correctly folded onto the shared `BuildConstraint` by task 9-19.
- Complexity: Low. Two loops over `Lanes()` and one over the package's parsed sources.
- Modern idioms: Yes — `Lanes()` as a `[]DepsOption` range, variadic options, no reflection or string-prefix module sniffing (`isStdlib` keys on the empty module, not a path prefix).
- Readability: Good. Each guard's comment says what the property protects and what a violation would cost, and the failure message at internal/sourceguardtest/leaf_guard_test.go:41 names the file, the constraint and the consequence.
- Comment accuracy: The comments hold against the code and against the tree. The `hookstest` claim is true — internal/hookstest/hooks.go:16 and internal/hookstest/staging.go:10 both import `internal/xdg`. The bootstrapping note at internal/sourceguardtest/leaf_guard_test.go:11-14 is accurate: the guard is written with the primitives it polices, and because `go list -deps` reads the package's own non-test sources, a forbidden dependency there is reported rather than hidden by the suite that imports it.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
