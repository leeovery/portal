TASK: resume-hooks-silently-lost-9-30 — `internal/logtest` States A Dependency-Shape Invariant That No Guard Enforces (tick-adad45)

ACCEPTANCE CRITERIA:
- [x] `internal/logtest` carries a leaf guard asserting its transitive dependency set.
- [x] Adding an `internal/fileutil` import to `internal/logtest` fails that guard.
- [x] The guard resolves the tagged configuration as well as the default one.
- [x] The guard refuses to pass over an empty or drifted dependency set, as `AssertDepsWithin` already enforces.

STATUS: complete

SPEC CONTEXT: None applicable. This is a phase-9 implementation-analysis task; per the shared verifier context, a phase 6–10 task's authority is its own body rather than the specification. The relevant standing context is CLAUDE.md's `logtest` row (the closed test-support leaf reachable from every test package in the tree) and the sibling precedents `internal/harnesstest/leaf_guard_test.go` and `internal/sourceguardtest/leaf_guard_test.go`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/logtest/leaf_guard_test.go:1-63` (new) — the leaf guard.
  - `internal/logtest/assert.go:44-49` — re-voiced `AssertWriteFailure` doc comment.
  - `CLAUDE.md:84` — `logtest` row now states the shape is guarded and names the guard file, the allowlist and the two lanes.
  - Commit `8ec4b819` (3 files, +68/-4); fix-tracking record at `.workflows/resume-hooks-silently-lost/implementation/resume-hooks-silently-lost/fix-tracking-resume-hooks-silently-lost-9-30.md` (two fix rounds, both applied).
- Notes:
  - AC1: `leaf_guard_test.go:29-33` calls `sourceguardtest.AssertDepsWithin` for `github.com/leeovery/portal/internal/logtest`, matching the shape of `internal/harnesstest/leaf_guard_test.go:16-21`.
  - AC2: the allowlist `logTestMayImport` (`:26`) is `{internal/harnesstest, internal/log}`. Verified exact against the tree: `internal/logtest`'s non-test files import only `harnesstest` (`assert.go:7`, `capture.go:21`) and `log` (`install.go:6`), and `internal/log`'s non-test files carry no in-module import at all (checked file by file, tests excluded), so the transitive in-module set is exactly those two. An `internal/fileutil` import lands in the `default:` arm of `assertdepswithin.go:43-45` and is reported.
  - `sourceguardtest.ForbiddingThirdParty()` is passed at both call sites (`:31`, `:44`), so CLAUDE.md's "or any other new edge" clause is true for a third-party import too, not only an in-module one. This was the attempt-1 fix and it landed.
  - AC3: both call sites loop over `sourceguardtest.Lanes()` (`packagedeps.go:42-44` = default + `-tags integration`), so the tagged configuration is resolved as well as the default one.
  - AC4: inherited from `AssertDepsWithin` (`assertdepswithin.go:30-34` fatal on a set not holding the subject, `:48-50` fatal on a drifted allowlist, `packagedeps.go:172-175` fatal on an empty set). Nothing in the guard opts out of those.
  - `go list -deps` without `-test` covers only the package's non-test imports, which is the correct scoping here: what propagates into every consuming test package is what `internal/logtest` itself imports, not what its own `_test.go` files do (they import `sourceguardtest`, which is deliberately outside the allowlist and correctly invisible to the guard).
  - Cache-input coverage is in place: the guard passes no `InDir`, so `packageDeps` → `readPackageSources(".")` (`packagedeps.go:166`, `111-113`) reads `internal/logtest`'s own directory, which is the phase-10 fix (`ac02d961`) that makes a newly added tagged source re-key the judging binary. Without it a tag-gated addition would move nothing about the cache key.
  - Drift from the plan's `Do` list: none. All four Do items landed.

TESTS:
- Status: Adequate
- Coverage:
  - `"it holds logtest's transitive dependencies inside the declared allowlist"` (`:29-33`) — the positive assertion, over both lanes.
  - `"it reports a dependency outside the allowlist"` (`:38-51`) — narrows the allowlist to `harnessTestPkg` alone and requires `internal/log` to be reported, per lane, with a fresh `harnesstest.Recorder` each iteration. This is what shows the positive assertion bites without a package having to import something it must not.
  - The third test name the task prescribes — `"it sees a dependency reachable only from an integration-tagged file"` — is deliberately absent, adjudicated in the fix-tracking record. Verified and I agree it is not owed: `internal/logtest` carries no build-tagged file (checked every `.go` in the directory — all `package logtest` / `package logtest_test` with no constraint line), so the two lane readings are identical here and the case cannot be exhibited without staging a fixture module. That property belongs to the shared primitive and is already staged there: `internal/sourceguardtest/assertdepswithin_test.go:134-197` writes a fixture module holding both an integration-tagged and a `!integration`-gated dependency and proves across four subtests that `WithBuildTags` and the `Lanes()` composition each reach it. Re-staging it in `internal/logtest` would be duplicate coverage of another package's primitive. AC3 — the property this consumer owes — is delivered by the `Lanes()` loops.
- Notes:
  - Would fail if the feature broke: yes, on both directions. Adding an in-module or third-party import fails the positive subtest; a primitive that stopped judging would fail the negative one.
  - Not vacuous under a fatal: `Recorder.Fatalf` panics with the sentinel `Run` absorbs (`internal/harnesstest/recorder.go:58-76`), so an early fatal leaves `rec.Errors` empty, `reported` false, and `:47-49` reports.
  - The attempt-2 defect is genuinely fixed: `reported` (`:59-63`) anchors on `"depends on "+pkg+" "` rather than a bare substring. Confirmed against the emitting format string `assertdepswithin.go:44` (`"%s%s transitively depends on %s — it may reach no further than %v"`) — the subject `…/internal/logtest` is named first and extends `…/internal/log`, and the trailing space in the anchor is what stops `…/internal/logtest` matching in the dependency slot as well.
  - Not over-tested: two subtests, no redundancy (attempt 1's byte-identical duplicate pair was folded), no mocking, no setup.

CODE QUALITY:
- Project conventions: Followed. Untagged unit-lane guard, consistent with the ~20 `sourceguardtest`-driven guards; no `t.Parallel()`; no hand-rolled `go build`; no `slog.Logger` construction; reports through `harnesstest.Recorder` rather than a local stub, per the CLAUDE.md `harnesstest` row.
- SOLID principles: Good. The guard states its rule and delegates enumeration and judgement to `sourceguardtest`; no primitive is restated locally.
- Complexity: Low. Two loops and one predicate.
- Modern idioms: Yes — `slices.ContainsFunc`, per-iteration loop variables, table-free subtests.
- Readability: Good. `logTestMayImport`'s comment (`:19-25`) says why the set is what it is rather than restating it; `reported`'s comment (`:54-58`) states the reason for the anchoring, which is the non-obvious part.
- Comment accuracy: Verified line by line. `:22-23` now says "It reaches the logging machinery it captures, and the stand-in its failing helpers report through" — two edges, matching the two-entry allowlist below it (attempt 1's one-edge claim is gone). `:35-37`, `:54-58`, and the re-voiced `assert.go:46-49` all hold against the code. No process-artifact references (no task ids, phases or spec sections) anywhere in the changed code or the CLAUDE.md row.
- Security: N/A.
- Performance: Four `go list` subprocess invocations per run (two subtests × two lanes), cached by the Go build cache. Negligible and inherent to the guard's shape.
- Issues: None rising above preference. For the record and NOT reported as findings: `reported` (`:59-63`) is a near-twin of the package-local `containsSubstring` (`install_guard_rule_test.go:146`) and of `sourceguardtest`'s unexported `errored` (`assertdepswithin_test.go:129-132`); the failure message's `"lane %d of the %d"` phrasing (`:48`) is awkward but honest, since a `DepsOption` is an opaque func and no label is derivable from it outside `sourceguardtest`.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
