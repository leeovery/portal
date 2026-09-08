## Attempt 1

ISSUES:
- `internal/logtest/leaf_guard_test.go:36-44` is executed a second time, verbatim, by `:49-59`. `AssertDepsWithin` with no options and with `WithBuildTags()` (no arguments) resolve identically: `newDepsConfig` yields `tags` of length 0 either way, so `packagedeps.go:87` appends no `-tags` flag and `depsConfig.lane()` returns `""`. The `Lanes()` loop's first iteration is therefore byte-for-byte the subtest above it. Two subtests assert the same thing, and the allowlist narrowing `[]string{harnessTestPkg}` is now hardcoded in two places beside `logTestMayImport` — if `logtest` ever drops its `harnesstest` edge, both fatal through `AssertDepsWithin`'s anchor check with a message about a drifted allowlist rather than about what actually changed.
  FIX: Fold the two into one negative subtest carrying the prescribed name — run `"it reports a dependency outside the allowlist"` over `sourceguardtest.Lanes()`, keeping one `harnesstest.Recorder` per lane, and delete `:46-59`. That satisfies both prescribed intents with one assertion and no duplication. While folding, name the lane in the failure message: `:56` currently reports `"one of the lanes the guard runs over judged nothing"` from inside the loop without saying which, so a real failure does not tell the reader which configuration to re-run. Deriving the label is awkward from a `DepsOption` — the simplest honest form is a `for i, lane := range` with the index in the message, or two explicit named cases.
  ALTERNATIVE: Keep both subtests but give the first a genuinely distinct subject — assert the positive guard is non-vacuous by narrowing to `[]string{logPkg}` and expecting `harnessTestPkg` reported, so the two probe opposite entries. Preserves both task test names literally but keeps two near-identical bodies and a second hardcoded narrowing. The reviewer recommends the fold.
  CONFIDENCE: high

- `internal/logtest/leaf_guard_test.go:29` omits `sourceguardtest.ForbiddingThirdParty()`, so a third-party import into `logtest` passes the guard silently — while `CLAUDE.md`'s new sentence claims "an `internal/fileutil` import — or any other new edge — fails the guard". That clause is false for a third-party edge, which is the same defect class this task exists to close: a written invariant the guard does not enforce. The two siblings that share `logtest`'s defining property — reachable from every test package in the tree — both pass the option: `internal/harnesstest/leaf_guard_test.go:18` and `internal/sourceguardtest/leaf_guard_test.go:28-31`, the latter demonstrating it composes fine with a non-empty internal allowlist. The stated rationale ("`logtest` makes no stdlib-only claim") misreads what the option does: `AssertDepsWithin`'s own doc says the stdlib-only meaning comes from the empty allowlist, and `ForbiddingThirdParty` only "brings those into the judgement". `logtest` has zero third-party dependencies today in both lanes, so the option passes immediately and costs nothing.
  FIX: Add `sourceguardtest.ForbiddingThirdParty()` to the call at `:29`, and extend the `logTestMayImport` rationale with the reason — a third-party import here lands in every test package in the tree, which is the harm the allowlist exists to prevent. Leave `CLAUDE.md` as written; the claim becomes true.
  ALTERNATIVE: Narrow the `CLAUDE.md` sentence instead — "pins the package's dependencies within this module". Cheaper, but leaves the hole open on the one package where a stray third-party edge propagates furthest. The reviewer recommends adding the option.
  CONFIDENCE: medium

COMMENT_CORRECTIONS:
- `internal/logtest/leaf_guard_test.go:22-23` — claims one edge directly above a two-entry allowlist; the line below declares `harnessTestPkg` as well as `logPkg`.
  OLD:
// would put that package there too. The logging machinery it captures is the
// one edge it needs.
  NEW:
// would put that package there too. It reaches the logging machinery it
// captures, and the stand-in its failing helpers report through.

NOTES:
- The substituted test name is adjudicated CORRECT and the prescribed test is NOT owed. The criterion is a property of this guard — that it takes a reading under the integration tag as well as the default one — and the `Lanes()` loop delivers it. The prescribed name names a property of the shared primitive: `internal/logtest` carries no build-tagged file, so the two lane readings are identical here and the case cannot be exhibited without staging a fixture module, which `sourceguardtest`'s own `TestAssertDepsWithin_WithBuildTags` already stages across four subtests. Do NOT re-stage it here.
- The allowlist is exact: `go list -deps` resolves `internal/harnesstest` + `internal/log` and nothing else internal, in both lanes.
- The re-voiced doc comment at `internal/logtest/assert.go:45-48` does exactly what the task asked. No correction needed.
- The `CLAUDE.md` row is otherwise accurate — it names the guard file, the allowlist and the two lanes.
- The comment at `:46-48` restates `WithBuildTags`'s own doc. True, and the rationale for the lanes loop — but if the fold is applied it should not survive verbatim onto the merged subtest, whose subject is the narrowing, not the tag.

## Attempt 2

ISSUES:
- `internal/logtest/leaf_guard_test.go:47-49` + `:55-58` — the negative subtest's match is satisfied by any complaint at all, not by one naming `internal/log`. `AssertDepsWithin` formats `"%s%s transitively depends on %s — …"` with the subject package first, so every message it emits about this package begins with `github.com/leeovery/portal/internal/logtest`, which *contains* `logPkg` (`…/internal/log`) as a substring. Confirmed empirically: a message reporting `internal/nanoid` as the offending dependency still returns `true` from `reported(rec, logPkg)`. The subtest therefore asserts "the guard said something", not "the guard named the dependency the narrowing exposed", and `reported`'s doc comment at `:55` — "says whether any complaint the guard recorded names pkg" — is false for this call site. `code-quality.md` lists "substring assertions in tests when exact output is deterministic" as an anti-pattern, and under the narrowed allowlist the guard's output is deterministic: exactly one error, naming `internal/log`.
  FIX: Anchor the match on the message's dependency slot rather than any occurrence — have `reported` test `strings.Contains(msg, "depends on "+pkg+" ")` (the guard writes the dep between `"depends on "` and `" — it may reach no further than"`), and re-word its doc comment to say it matches the dependency a complaint names. That removes the collision without needing the lane clause, which is not derivable from a `DepsOption` outside `sourceguardtest`.
  ALTERNATIVE: Assert the shape exactly instead — require `len(rec.Errors) == 1` and that the single error names `logPkg` in the dependency slot. Strictly tighter (it would also catch a stray extra complaint), at the cost of coupling the subtest to `logtest` having exactly two internal dependencies. The reviewer recommends the anchored match, optionally with the count added.
  CONFIDENCE: high

NOTES:
- All acceptance criteria verified met. The allowlist is exact against `go list -deps` (`internal/harnesstest` + `internal/log`, nothing else internal, no third-party), `ForbiddingThirdParty()` is passed at both call sites, and CLAUDE.md's "or any other new edge" clause is now true as written.
- The prior adjudication on the third prescribed test name holds and was not re-opened: `internal/sourceguardtest/assertdepswithin_test.go:134-192` stages a fixture module with both an integration-tagged and a `!integration`-gated dependency and proves `Lanes()` reaches each, so the property belongs to the shared primitive.
- No other leaf guard in the tree carries a negative subtest — they run the assertion alone and rely on `sourceguardtest`'s own suite for the primitive's behaviour. The one here is prescribed by the task and does show this allowlist is load-bearing, so it stays, but it is the only instance of the shape.
- `reported` (`:55-58`) is a near-copy of `sourceguardtest`'s unexported `errored` (`assertdepswithin_test.go:129-131`). Two instances, both unexported test helpers in different packages, so the Rule of Three is not met — noted only in case a third appears.
- The `"lane %d of the %d"` phrasing is awkward but honest: a `DepsOption` is an opaque func, so no label can be derived from it. Acceptable as written.
