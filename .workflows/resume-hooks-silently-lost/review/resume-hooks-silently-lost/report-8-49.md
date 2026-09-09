TASK: resume-hooks-silently-lost-8-49 (tick-0202ee) — "Two Stated Outcomes Are Held By Contributor Discipline In A Repo Whose House Style Is Source Guards". Add two source guards: (1) `logtest.Install(t)` is the only route to a package-level capture handler; (2) an untagged `*_test.go` may not reference the portal-binary build helpers.

ACCEPTANCE CRITERIA:
- A hand-installed `SetTestHandler` over a `logtest.Sink` outside the sanctioned sites fails the first guard; the three survivors pass.
- An untagged test referencing a build helper fails the second guard.
- Both guards run in the unit lane and fail when they scan nothing.
- The tree passes both.

STATUS: issues_found (0 blocking)

SPEC CONTEXT: Phase 8 is an implementation-analysis phase; per the shared verifier context a phase 6–10 task's authority is its own body, not the specification. The task's substance is the repo's own house style, stated in CLAUDE.md: the lane rule ("**Building counts, not just running:** the fast lane builds no portal binary … `internal/portalbintest`'s own build test is integration-tagged") and the `logtest` rule ("`Install(t)` routes **every** component logger into a fresh sink for the test's duration", with three named survivors that construct a handler of their own — the JSON-rendering handler in `internal/hooks/store_test.go`, `cmd`'s `warnBypassHandler`, and `internal/log`'s in-package twins). Both were prose-only before this task.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Guard 1: `internal/logtest/install_guard_test.go:56` (`TestInstallIsTheOnlyRouteToACaptureHandler`), sanction table at `internal/logtest/install_guard_test.go:31-36`, owner-dir skip at `internal/logtest/install_guard_test.go:25` and `:61`, scanned-nothing tripwire at `internal/logtest/install_guard_test.go:127-136`.
  - Guard 2: `internal/portalbintest/lane_guard_test.go:50` (`TestBuildHelpersStayInTheIntegrationLane`), helper vocabulary at `internal/portalbintest/lane_guard_test.go:20-25`, lane predicate at `internal/portalbintest/lane_guard_test.go:89-92`, the two tripwires at `internal/portalbintest/lane_guard_test.go:113-125`.
  - Both are built on the shared primitives as the Do list required — `sourceguardtest.RepoSources` / `AllSources` / `TestSources` (`internal/sourceguardtest/reposources.go:87`), `ForEachFuncCall` (`internal/sourceguardtest/foreachfunccall.go:11`), `CalleeName`, `ParseMode` / `ParsedSource` (`internal/sourceguardtest/parsesources.go:18`), and `BuildConstraint` / `SatisfiedWithout` / `IntegrationTag` (`internal/sourceguardtest/buildconstraint.go:12`, `:22`, `:51`). No new scanning code was authored.
  - Both guard files are untagged and in the unit lane (`package logtest_test`, `package portalbintest_test`), satisfying the "run in the unit lane" criterion.
- Notes:
  - **The tree passes both, verified by enumeration.** Guard 1: the only `SetTestHandler` calls outside `internal/log/` are `internal/logtest/install.go:14` (in `Install`), `cmd/logging_capture_test.go:30` (in `initTestLogToStateDirAs`), `cmd/open_test.go:2918` (in `TestExecMarker_VisibleAtWARN`) and `internal/hooks/store_test.go:1260` (in `TestSetEmitsOpAsJSONField`) — exactly the four keys in the sanction table, file *and* enclosing function matching. Guard 2: every `_test.go` referencing one of the four build helpers carries `//go:build integration`, except the two guard files themselves, whose occurrences are string literals (`buildHelperNames`) and raw-string fixtures — neither is a `*ast.CallExpr`, so `buildHelperRefsIn` records nothing there.
  - The `logOwnerDir+string(filepath.Separator)` prefix at `internal/logtest/install_guard_test.go:61` correctly does *not* swallow `internal/logtest/` — the separator is load-bearing and present.
  - `compiledInUnitLane` (`internal/portalbintest/lane_guard_test.go:89`) treats an unreadable constraint as "in the lane", matching its own doc comment and `BuildConstraint`'s `(nil, false)` return for an unparseable line — the guard judges rather than excuses. Correct.
  - Guard 1 is deliberately broader than the Do list's wording ("with a `logtest.Sink`"): it flags *any* hand-rolled handler swap outside the sanctioned sites, whatever the argument. The reason is written down at `internal/logtest/install_guard_test.go:52-55` (sanctioned by name, because what legitimises each site is the handler it installs *instead of* a sink). That is a sound strengthening, not a divergence to report.
  - `unmatchedSanctions` (`internal/logtest/install_guard_test.go:110`) closes the usual rot in a named-exemption guard: a sanction whose site has moved fails rather than carrying a standing permission over a name a later test could take. Good.

TESTS:
- Status: Adequate
- Coverage: All four test names the plan asked for exist, one per guard for the scanned-nothing case:
  - `internal/logtest/install_guard_rule_test.go:75` `TestHandlerRuleFlagsAHandInstalledCaptureHandler` — "it flags a hand-installed capture handler".
  - `internal/logtest/install_guard_rule_test.go:91` `TestHandlerRuleAllowsTheThreeSanctionedHandlersAndInstall` — "it allows the three sanctioned handlers" (plus `Install` itself, and asserts `unmatchedSanctions` is empty over the full set).
  - `internal/portalbintest/lane_guard_rule_test.go:43` `TestLaneRuleFlagsABuildHelperReferencedFromAnUntaggedTest` — "it flags a portal-binary build helper referenced from an untagged test", paired with `:65` for the tagged-passes direction.
  - `internal/logtest/install_guard_rule_test.go:130` and `internal/portalbintest/lane_guard_rule_test.go:87` — "it fails when it scans nothing", one per guard.
  - Beyond the plan: `internal/logtest/install_guard_rule_test.go:115` covers a sanctioned site that has stopped installing a handler, and `internal/portalbintest/lane_guard_rule_test.go:98` covers the second lane-guard tripwire (the helper vocabulary drifting off the tree). Both are the guards' own failure paths, exercised through staged probes as Do item 4 asked.
- Notes:
  - The probes are miniature source files staged through `stageHandlerInstalls` / `stageTestFile`, driven through the *same* `handlerInstallsIn` / `buildHelperRefsIn` / `compiledInUnitLane` the repo scan uses — so the rule tests exercise the production rule rather than a restatement of it.
  - Not over-tested: each case pins one distinguishable behaviour, and the assertions check the finding's substance (that it names the offending file, and the helper or the route) rather than the exact message.
  - `TestLaneRuleFailsWhenItScansNothing` (`internal/portalbintest/lane_guard_rule_test.go:87`) calls `laneGuardFailure(0, 3, nil)` directly rather than through a staged fixture, unlike its guard-1 twin. It is the honest way to reach a zero-unit-lane-file reading (a staged fixture is a file, so it cannot produce one), and it is the same function the repo scan ends in. Fine.

CODE QUALITY:
- Project conventions: Followed. Both guards are untagged unit-lane source guards built on `sourceguardtest`, matching the ~20 existing guards' shape; no test uses `t.Parallel()`; no `*slog.Logger` is constructed; nothing here touches tmux, the filesystem outside the repo, or a process. Naming is `TestX_Behaviour`-style, which coexists with the `t.Run("it …")` style elsewhere in the same packages.
- SOLID principles: Good. Each guard is split into enumerate (`handlerInstallsIn` / `buildHelperRefsIn`), judge (`auditHandlerInstalls` / `auditBuildHelperLane`) and render (`handlerGuardFailure` / `laneGuardFailure`), which is exactly what lets the rule tests drive the judgement without the repo scan.
- Complexity: Low. No function exceeds a single loop plus a switch.
- Modern idioms: Yes — `slices.Contains`, `go/build/constraint` for the lane reading rather than string-matching the tag comment.
- Readability: Good. Every non-obvious decision carries its reason in a comment (why the sanctions are keyed by name, why an unreadable constraint counts as unit-lane, why the owner directory is skipped, why the rule is a guard rather than a manual check).
- Issues: `scannedTestFile.Rel` (`internal/portalbintest/lane_guard_test.go:38`) is written at `:56` and `:138` and never read — `helperRef.File` already carries the path every defect is built from. Dead weight only; nothing depends on it and nothing breaks. Not raised as a finding.

BLOCKING ISSUES:
- None. Both guards exist, both run in the unit lane, both fail when they scan nothing, both are covered by staged probes of their own failure paths, and the tree passes both by enumeration.

FINDINGS:
- [in-scope] [contained] internal/logtest/install_guard_test.go:61 — the owner-directory skip is by path prefix, so it exempts every file under `internal/log/`, while the reason recorded for it at `internal/logtest/install_guard_test.go:21-24` ("A Sink cannot reach there at all — logtest imports the package, so importing it back is an import cycle") holds only for the in-package `package log` files. Four files in that directory are external test packages — `internal/log/discard_guard_test.go`, `internal/log/exec_context_test.go`, `internal/log/migration_guard_test.go`, `internal/log/testmain_isolation_test.go` — and an external test package can import `internal/logtest` with no cycle. Narrow the skip to files the reason actually covers, by testing the parsed file's package name (`source.File.Name.Name == "log"`) alongside the directory prefix, and add a rule-test case staging a `package log_test` file under `internal/log/` that pairs a Sink with a hand-written swap. — FAILS: a `package log_test` file under `internal/log/` can write `sink := &logtest.Sink{}; log.SetTestHandler(t, sink)` by hand and the guard skips it without judging, which is precisely the pairing the guard was added to make impossible; the comment beside the skip states a constraint that does not apply to those four files, so a reader checking the exemption is told a reason the tree falsifies.
