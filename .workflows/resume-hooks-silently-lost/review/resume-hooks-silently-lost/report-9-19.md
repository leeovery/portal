TASK: resume-hooks-silently-lost-9-19 — "The Build-Tag Question Has Three Readers, And Every Leaf Guard But One Is Blind To It" (tick-262787, phase 9, severity medium)

ACCEPTANCE CRITERIA:
- [ ] Build-constraint extraction is implemented once; the three former copies call it and hold only their own policy.
- [ ] Three readings of the same file now classify it identically, including a file whose constraint fails to parse.
- [ ] The `integration` tag name is declared once in the tree's guard family.
- [ ] Each of the seven leaf guards fails when a forbidden dependency is reachable only from an `integration`-tagged file in that package.
- [ ] `internal/sourceguardtest`'s own leaf guard keeps its coverage with the hand-rolled companion check removed.

STATUS: issues_found (0 blocking; the single finding's whole remedy is documentation text)

SPEC CONTEXT: This is a phase-9 implementation-analysis task, so its authority is its own body rather than the
specification (per the shared verifier context). The task is a consolidation of the guard family's build-tag
machinery: three packages each hand-rolled a walk of `file.Comments` bounded by `file.Package`, ran
`constraint.Parse`, and evaluated the `integration` tag — disagreeing on the unparseable case and declaring the tag
literal twice; and `AssertDepsWithin` resolved `go list -deps` under the default configuration only, so a
dependency reachable only from a build-tagged file was invisible to every leaf guard in the tree.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/sourceguardtest/buildconstraint.go:12` (`IntegrationTag`), `:22-39` (`BuildConstraint`),
    `:44-46` (`SatisfiedWith`), `:51-53` (`SatisfiedWithout`) — the new single reading.
  - `internal/sourceguardtest/packagedeps.go:34-36` (`WithBuildTags`), `:42-44` (`Lanes`), `:65-70`
    (`depsConfig.lane`, the "under -tags …" clause appended to a finding), `:93-105` (`listDeps` passes `-tags`).
  - Callers routed through it: `internal/portalbintest/lane_guard_test.go:89-92` (`compiledInUnitLane`),
    `internal/restoretest/literal_guard_scan_test.go:64-67` (`isIntegrationTagged`, the file the former
    `session_restorer_literal_guard_test.go` half was later merged into),
    `internal/sourceguardtest/leaf_guard_test.go:40` (the former `buildConstraintLine`).
  - Nine guards run both lanes via `sourceguardtest.Lanes()`: the seven named — `internal/nanoid`,
    `internal/shellquote`, `internal/harnesstest`, `internal/xdg`, `internal/prefs`, `internal/hooks`,
    `internal/theme` — plus `internal/logtest` and `internal/sourceguardtest` itself.
- Notes:
  - Criterion 1 holds. The three copies are gone (verified against the task commit b74712bd); each caller now
    holds one expression stating only its own policy — unit lane, integration lane, and "any tag at all".
  - Criterion 2 holds. `BuildConstraint` answers `(nil, false)` for an unparseable constraint, so all three
    callers read the same file the same way. The prior per-caller *verdicts* are preserved exactly:
    `compiledInUnitLane` still returns true (old code returned true on parse error; `!stated` gives the same),
    and `isIntegrationTagged` still returns false. `SatisfiedWithout(expr, tag)` reproduces the old
    `expr.Eval(func(t string) bool { return t != integrationTag })` semantics verbatim, and `SatisfiedWith`
    reproduces `t == integrationTag`, so neither guard's reach moved.
  - The extraction also fixes a latent mis-read: `BuildConstraint` identifies a constraint line with
    `constraint.IsGoBuild`/`IsPlusBuild` before parsing, where the old `buildConstraintLine` parsed every
    pre-package comment and took the first that happened to parse. It also now reads a legacy `// +build` line,
    which `compiledInUnitLane` previously ignored.
  - Criterion 3 holds for the guard family's constraint readers: no `integrationTag` identifier survives anywhere
    in the tree, and the only `"integration"` string literals left are `internal/sourceguardtest/buildconstraint.go:12`
    (the declaration), a `go build` argv in `internal/portalbintest/build.go:67`, and a `go list` argv in
    `internal/tmux/target_composition_guard_test.go:63` — neither of the latter two is a build-constraint reading,
    and neither file was in this task's scope.
  - Criterion 4 holds structurally: every one of the seven guards loops `sourceguardtest.Lanes()`, which is
    `{default, -tags integration}`, and the option is proven to bite by the staged fixture module in
    `assertdepswithin_test.go`.
  - Criterion 5: the hand-rolled `buildConstraintLine` copy is deleted and the subtest it served now routes
    through the shared primitive (`internal/sourceguardtest/leaf_guard_test.go:38-44`). The subtest itself was
    *kept* rather than retired, re-justified as lane purity for the primitives ("a tag here gates every guard
    built on these primitives out of the unit lane") — a property `Lanes()` does not cover, since `Lanes()`
    judges the *judged* package's dependencies, not whether the guard machinery itself stays in the unit lane.
    This is a divergence from the Do list's "retire … in favour of the option", and it is a gain rather than a
    loss: coverage is strictly wider than the plan asked for. Not reported as a finding.

TESTS:
- Status: Adequate
- Coverage: All four named micro-acceptance tests exist and assert what they name:
  - `internal/sourceguardtest/buildconstraint_test.go:13` — "it returns the parsed build constraint for a tagged
    file and reports none for an untagged one" (5 cases: gated on the tag, gated against it, no constraint, a
    doc comment preceding nothing, and a constraint stated *after* the package clause where it constrains
    nothing). The last two are the boundary cases the `group.Pos() > file.Package` bound exists for.
  - `:66` — "it classifies an unparseable constraint the same way for every caller", asserting both `found=false`
    and `expr==nil` for `//go:build integration &&`.
  - `:79` — "it evaluates the integration tag against a parsed constraint", asserting `SatisfiedWith` *and*
    `SatisfiedWithout` across four constraint shapes including "gated on some other tag" and "gated on the tag
    alongside another" — the two shapes that separate the two evaluators.
  - `internal/sourceguardtest/assertdepswithin_test.go:146` — "it reports a forbidden dependency reachable only
    from an integration-tagged file", over a staged two-package fixture module (`stageTaggedFixtureModule`,
    `:203-225`), and it additionally asserts the finding names the configuration it was resolved under, so the
    command a reader would run to confirm it is the one that produced it.
- Notes:
  - Not over-tested. The three subtests beyond the named one each hold a distinct property that the named one
    cannot: `:135` is the control (the default reading must *pass* the fixture, or the tagged reading proves
    nothing), `:162` proves `Lanes()` reaches the integration configuration, `:177` proves it reaches the
    default one via a package whose forbidden import sits behind `//go:build !integration`. Dropping any of the
    three would leave a direction unproven.
  - Not under-tested. Both callers of the primitive are one-line expressions over it, and `portalbintest`'s rule
    tests (`lane_guard_rule_test.go`) still drive `compiledInUnitLane` over tagged and untagged fixtures, so the
    routed policy is exercised end to end rather than only at the primitive.
  - Test execution was not attempted (assessment by reading, per the verifier's remit). Imports and referenced
    symbols were checked by hand across the five changed test files and all resolve.

CODE QUALITY:
- Project conventions: Followed. The primitive is stdlib-only (`go/ast`, `go/build/constraint`, `slices`), so
  `internal/sourceguardtest`'s own leaf guard — which forbids anything beyond `harnesstest` + `portalbintest`
  across both lanes — still passes, and the package stays untagged, keeping every guard built on it in the unit
  lane per CLAUDE.md's lane rule. No test runs a binary or a daemon, so nothing crosses into the integration lane.
- SOLID principles: Good. `BuildConstraint` (reading) is separated from `SatisfiedWith`/`SatisfiedWithout`
  (policy), which is exactly what lets three callers share one reading while each keeps its own lane question.
  `WithBuildTags` is one more `DepsOption` on the existing option shape rather than a new entry point.
- Complexity: Low. `BuildConstraint` is a two-level loop with one early break; the evaluators are one line each.
- Modern idioms: Yes — `slices.Contains` for the tag set, variadic options, `constraint.IsGoBuild`/`IsPlusBuild`
  rather than string prefix matching.
- Readability: Good. `depsConfig.lane()` is a genuinely good touch: a finding from the tagged reading names its
  configuration, so it is reproducible.
- Issues: One nuance, below the reporting bar and noted only for the record: `BuildConstraint`'s doc says an
  unparseable constraint "states none" because "the file carrying it does not build either". That is true of a
  malformed `//go:build` line but not of a malformed `// +build` one, which the toolchain ignores, leaving the
  file unconstrained. The answer the code gives is correct in both cases; only half the stated reason is.

BLOCKING ISSUES:
- None. All five acceptance criteria are met in substance, and no behaviour was lost: each routed caller's
  verdict for every input class is identical to what its deleted copy produced.

FINDINGS:
- [in-scope] [contained] CLAUDE.md:89 — the `sourceguardtest` architecture-table row enumerates the package's
  primitives exhaustively (down to `Rooted`, `ParsedSource.Position` and `PackageSource`) and names
  `AssertDepsWithin`'s options individually ("taking `InDir` to anchor resolution at a chosen directory",
  "exempting other modules unless `ForbiddingThirdParty` is passed"), but it names none of the six exported
  symbols this task added: `BuildConstraint`, `SatisfiedWith`, `SatisfiedWithout` and `IntegrationTag`
  (`internal/sourceguardtest/buildconstraint.go:12,22,44,51`), and `WithBuildTags` and `Lanes`
  (`internal/sourceguardtest/packagedeps.go:34,42`). Extend that row with the build-constraint reader and its
  two evaluators, the single `IntegrationTag` declaration, and `WithBuildTags`/`Lanes` as the option that takes
  a dependency reading under a chosen configuration — FAILS: CLAUDE.md is the binding description of this
  package, and it is the document a contributor consults before adding a guard. A fourth hand-rolled
  `file.Comments` walk, or a new leaf guard resolving only the default configuration, is exactly the state this
  task existed to end, and the table as it stands gives no sign that either primitive is available — the
  omission re-opens the gap for the next guard author rather than for this one.
