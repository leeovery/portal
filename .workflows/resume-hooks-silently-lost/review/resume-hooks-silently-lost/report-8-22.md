TASK: resume-hooks-silently-lost-8-22 — "Four Leaf-Package Guards Each Restate The Transitive Deps Confined To An Allowlist Check, And Its Vacuity Check Survives In One Copy Of Four" (tick-2cf66d, phase 8)

ACCEPTANCE CRITERIA:
- Five guards assert through one shared function; none writes the loop itself.
- Every guard fails on an empty or unresolved dep set.
- An offending dep is reported by all five, under one failure verb.
- The four existing callers resolve their package argument exactly as before, and capturetool's guard still fails on a forbidden import.

STATUS: complete

SPEC CONTEXT:
Phase 8 is an implementation-analysis cycle, not specified bugfix work — per the shared verifier context, a phase 6–10 task's authority is its own body rather than the specification. The task's subject is duplication among the repo's leaf-package dependency guards: four packages (`internal/hooks`, `internal/nanoid`, `internal/prefs`, `internal/theme`) each restated "this package's transitive deps stay inside an allowlist" in a different shape, with different failure verbs, and with the anti-vacuity check surviving in only one of the four. A fifth hand-rolled `go list -deps` sat outside the family in `cmd/capturetool/import_guard_test.go`, unable to fold in because `PackageDeps` had no working-directory knob. CLAUDE.md's `sourceguardtest` row is the project-convention anchor and now describes both `AssertDepsWithin` ("reports every dep outside a declared allowlist under one verb and refuses to pass over a vacuous set, exempting other modules unless `ForbiddingThirdParty` is passed") and `PackageDeps` ("taking `InDir` to anchor resolution at a chosen directory") — so the delivered surface and the binding conventions file agree.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/sourceguardtest/assertdepswithin.go:24` — `AssertDepsWithin(t, pkg, allowed, opts...)`: skips pkg itself and the stdlib (`:39`), skips other modules unless `ForbiddingThirdParty` (`:40`), reports every remaining dep outside the allowlist under a single `t.Errorf` (`:44`), and fatals on a set that does not hold pkg itself (`:32`) and on a non-empty allowlist none of whose entries is present (`:49`).
  - `internal/sourceguardtest/packagedeps.go:17` — `InDir(dir)`, the additive working-directory knob; `:93` `listDeps` sets `cmd.Dir = cfg.dir`, so a caller passing no option gets `cmd.Dir = ""` (inherited cwd) exactly as a bare `exec.Command` did.
  - `internal/sourceguardtest/packagedeps.go:152` — `PackageDeps`' signature is unchanged apart from the variadic `opts`, so the four existing callers are source-compatible; `:162` `packageDeps` is the shared enumeration carrying the vacuity check for both entry points (fatal on an unresolvable package at `:169`, fatal on an empty set at `:173`).
  - Guards reduced to an allowlist plus rationale: `internal/hooks/leaf_guard_test.go:22`/`:46`, `internal/nanoid/leaf_guard_test.go:16`, `internal/prefs/leaf_guard_test.go:18`, `internal/theme/leaf_guard_test.go:27`.
  - `cmd/capturetool/import_guard_test.go:31` — the hand-rolled `go list -deps` is gone; the `deps` helper now routes through `sourceguardtest.PackageDeps(..., InDir(sourceguardtest.ProjectRoot(t)))`, and the file no longer imports `portalbintest`.
- Notes:
  - Do item 3's escape hatch ("keep nanoid's stdlib-only predicate as its own allowlist form if a path list cannot express it") was resolved better than the fallback: `ForbiddingThirdParty()` (`packagedeps.go:25`) makes "stdlib alone" expressible as an empty allowlist, so nanoid keeps no bespoke predicate.
  - AC1 reads literally as "five guards assert through one shared function", and capturetool's two assertions go through the shared *enumeration* (`PackageDeps`) plus `slices.Contains`, not through `AssertDepsWithin`. This is a sound divergence, not a loss: capturetool's property is a denylist ("the portal binary must not reach `internal/capture`") over the whole transitive set of the main package, which an allowlist assertion cannot state without enumerating every dependency the binary has. The task's substance — delete the fifth hand-rolled `go list -deps`, fold it onto the shared enumeration through the new knob, restore its anti-vacuity protection — is delivered, and Do item 4's own parenthetical (`it needs cmd.Dir = portalbintest.ProjectRoot()`, "deleting its hand-rolled `go list -deps`") describes exactly what landed.
  - Adoption ran wider than the four named packages. I read five further guards that assert through the same function: `internal/shellquote/leaf_guard_test.go:16`, `internal/xdg/leaf_guard_test.go:22`, `internal/logtest/leaf_guard_test.go:31`, `internal/harnesstest/leaf_guard_test.go:18`, `internal/sourceguardtest/leaf_guard_test.go:28`. Some of these carry the `Lanes()` / `WithBuildTags` machinery that postdates this task; none writes a dependency loop of its own.
  - No strictness was lost at any of the four. hooks' allowlist keeps the same module-scoped judgement its `map[string]bool` had; prefs' and theme's old forbidden-lists were narrower than the allowlist that replaced them; nanoid's stdlib-only reach is preserved by `ForbiddingThirdParty()`.
  - Correct separation of the two directory concerns: `cfg.dir` drives the `go list` subprocess (`packagedeps.go:99`) while `cfg.sourceDir()` (`:54`) defaults to `"."` only for the cache-input read, so adding the knob did not move where the four existing callers' `go list` runs.

TESTS:
- Status: Adequate
- Coverage: All four test subjects named in the plan's Tests section exist and verify the behaviour rather than the implementation:
  - "it reports every dep outside the allowlist" → `internal/sourceguardtest/assertdepswithin_test.go:28`, which narrows hooks' allowlist to `internal/fileutil` and asserts all three of `internal/log`, `internal/nanoid`, `internal/storelog` are reported in one run (`:33`), and that nothing fatalled (`:38`) — the "every, under one verb" property.
  - "it fatals on an empty dep set" → `:43`, driving the `listDeps` seam (`packagedeps.go:93`) to return an empty set, which is the one shape `go list` cannot be asked to produce.
  - "it fatals when no allowlisted internal dep is present" → `:65`, and it additionally asserts the fatal names the allowlist it could not see (`:73`).
  - "it resolves a package relative to the given working directory" → `internal/sourceguardtest/assertdepswithin_test.go:108`, which pairs the `InDir("../nanoid")` reading with a control reading taken without the knob (`:114`) — so the case cannot pass by the resolution being directory-insensitive.
  - Beyond the named four: `:54` covers the "set does not hold pkg itself" fatal, `:78`/`:88` cover the other-module default and `ForbiddingThirdParty`, `:98` covers the stdlib-only-with-empty-allowlist pass (nanoid's shape).
  - The capturetool fold keeps its own anti-vacuity case: `cmd/capturetool/import_guard_test.go:22` `TestCaptureToolDoesImportCapture` fails if the enumeration stops finding `internal/capture`, and `PackageDeps` fatals rather than returning an empty set, so `TestPortalBinaryDoesNotImportCapture` (`:16`) cannot pass over nothing.
  - The suite pins its own preconditions: `assertdepswithin_test.go:78` would fail if `internal/theme` stopped depending on `internal/log`, which is exactly the anchor `internal/theme/leaf_guard_test.go:22` rests on.
- Notes: The seam swap at `assertdepswithin_test.go:122` mutates the package-level `listDeps` var; no case in the file calls `t.Parallel()`, consistent with the repo-wide rule. No redundancy found — each case pins a distinct branch of the four-arm switch or a distinct fatal. Nothing here builds or runs a portal binary or a daemon, so the unit lane is the correct home and no `//go:build integration` is owed.

CODE QUALITY:
- Project conventions: Followed. Test-only package, stdlib + `harnesstest.TestingT` only, untagged — which `internal/sourceguardtest/leaf_guard_test.go:28` now polices through the very function this task added. `AssertDepsWithin` takes `harnesstest.TestingT` rather than `*testing.T`, so its own failure paths are assertable, matching the tree's stand-in convention.
- SOLID principles: Good. The enumeration (`packageDeps`) and the judgement (`AssertDepsWithin`) are separate, which is what let capturetool reuse the former without being forced into the latter's allowlist shape. `DepsOption` is an idiomatic functional-options seam that leaves the existing signature intact.
- Complexity: Low. The judgement is one four-arm switch over a flat list; the fatal conditions are three named checks.
- Modern idioms: Yes — `slices.Contains`, `strings.Cut`, `strings.SplitSeq` (`packagedeps.go:117`), a variadic option type, and a swappable func var for the one seam that needs it.
- Readability: Good. Each fatal's message states what the failure means for the guard ("this guard is judging some other package", "the allowlist no longer describes the package, so this guard proves nothing"), which is the failure text a future maintainer actually needs.
- Issues: None rising to a finding. The doc comments on `AssertDepsWithin` (`assertdepswithin.go:9-23`), `InDir` (`packagedeps.go:14-19`) and `ForbiddingThirdParty` (`:21-27`) hold against the code they describe, and none references a task id, phase or spec section.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
