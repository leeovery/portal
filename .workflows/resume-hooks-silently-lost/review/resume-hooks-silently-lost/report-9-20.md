TASK: resume-hooks-silently-lost-9-20 — "The Two Restore-Exe Literal Guards Are A Copy-Paste Pair That Each Walk The Repo" (tick-c65ae2). Collapse the orchestrator and session-restorer `Exe` literal guards into one parameterised guard over two descriptors, driven by a single repo walk.

ACCEPTANCE CRITERIA:
- [x] One collector, one scan and one fatals-when-empty test body serve both descriptors.
- [x] Each descriptor's scanned-nothing fatal keeps its own wording, including the "stopped looking" phrase both tests check for.
- [x] Both guards still fail on exactly the unpinned-`Exe` literals they failed on before, for their own type and their own file set.
- [x] The package's unit-lane run performs one repo walk and parse for the pair.

STATUS: complete

SPEC CONTEXT: None applicable in substance. This is a phase-9 implementation-analysis (duplication) task; its authority is its own body, sourced from `analysis-duplication-c4.md`. The specification never mentions `Exe`/`ExecutableResolver` (grepped: no hits), so the guard being consolidated is machinery the implementation phase produced, not specified behaviour. The underlying hazard the guard exists for is real and unchanged: `restore.Orchestrator.Exe` / `restore.SessionRestorer.Exe` are optional and fall back to `os.Executable()`, which under `go test` resolves to the test binary — a pane armed by such a value respawns into the suite and the session vanishes silently.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/restoretest/literal_guard_scan_test.go:26-32` — the `literalGuard{typeName, fileSet, include, constructors, fatalWording}` descriptor.
  - `internal/restoretest/literal_guard_scan_test.go:34-57` — the two instances (`orchestratorGuard`, `sessionRestorerGuard`) and the `restoreLiteralGuards` pair.
  - `internal/restoretest/literal_guard_scan_test.go:83-108` — the single scan: one `sourceguardtest.RepoSources` walk answering every descriptor in order, owning the scanned-zero fatal in each descriptor's own words.
  - `internal/restoretest/literal_guard_scan_test.go:114-125` — the single composite-literal collector `literalGuard.literalsIn`, taking the type name from the descriptor.
  - `internal/restoretest/literal_guard_test.go:24-38` — the one standing guard `TestNoTestComposesAPaneArmingRestoreType`, looping the descriptors.
  - Deleted by commit 9a7b54ec: `internal/restoretest/orchestrator_literal_guard_test.go`, `internal/restoretest/session_restorer_literal_guard_test.go`.
- Notes:
  - Verdict parity confirmed against the pre-change files (`git show 9a7b54ec^`): the orchestrator descriptor keeps `everyTestFile` as its include, the session-restorer descriptor keeps `isIntegrationTagged`, and both route through the unchanged `isRestorePkgType`, so each guard flags exactly the literals of its own type in its own file set as before. Message wording ("test files" / "integration-tagged test files", the constructor names, "of N scanned") is preserved through `fileSet` and `constructors`.
  - Sole behavioural change in the standing guard: findings are now reported with `t.Errorf` per descriptor rather than `t.Fatalf`, so a tree violating both rules reports both. Still a failing test; nothing lost.
  - The single walk is real: `scanGuardTestFiles` calls `RepoSources` once and loops descriptors over the same `[]ParsedSource` (`:91-100`). The standing guard passes both descriptors in one call (`literal_guard_test.go:25`), and no other file in `internal/restoretest` calls `RepoSources` (grepped).
  - Checked for orphans: no production source, doc or `CLAUDE.md` reference survives to the deleted files or their removed identifiers (`scanTestOrchestratorLiterals`, `sessionRestorerLiteralsIn`, `stagedConstructor`, `writeGuardFile`, …) — only historical workflow artifacts mention them.
  - The three constructor names quoted in the descriptors all exist: `NewRestoreOrchestrator` (`internal/restoretest/orchestrator_staged.go:21`), `NewFakeExeOrchestrator` (`internal/restoretest/orchestrator.go:29`), `NewSessionRestorer` (`internal/restoretest/session_restorer_staged.go:20`).
  - The guard passes on the current tree: the only `restore.Orchestrator{` occurrences under a `_test.go` are inside this file's own fixture string literals (`literal_guard_test.go:56,58,89`), which an AST walk does not see as composite literals; every `restore.SessionRestorer{` literal lives in the unit-lane `internal/restore/session_test.go`, which the integration-scoped descriptor does not police.
  - Unasked-for but sound: `writeGuardFixture`/`writeGuardFile` were collapsed into one variadic `writeGuardFixture(t, ...guardFixtureFile)` (`literal_guard_scan_test.go:138-152`), which is what let the multi-file fixture cases join the same table.

TESTS:
- Status: Adequate
- Coverage: Every subtest of the two deleted suites survives as a table case, and the two new claims the task introduced are pinned.
  - `TestRestoreLiteralGuard_FailsForAnUnpinnedExeLiteralOfEitherType` (`literal_guard_test.go:40-180`) carries all seven prior cases across both descriptors: bare orchestrator literal (2 findings), orchestrator via constructors, production-file literal ignored, integration `SessionRestorer` literal (2 findings), explicit `Exe: nil` still flagged, constructor route ignored, unit-lane literal ignored — each asserting both `scanned` and finding count, so a descriptor that silently stopped matching files fails.
  - `TestRestoreLiteralGuard_FatalsWhenADescriptorsFileSetEnumeratesNothing` (`:184-237`) keeps the `harnesstest.Recorder` + exactly-one-fatal + `"stopped looking"` assertions, adds the per-descriptor wording checks (`:209-236`).
  - `TestRestoreLiteralGuard_WalksTheRepoOnceForBothDescriptors` (`:243-268`) observes the shared walk the right way — by `*ast.File` pointer identity across the two descriptors' `include` callbacks, which a second walk could not satisfy.
- Notes:
  - Not over-tested: the fixture cases are the union of the two prior suites, deduplicated, and the two added tests each pin an acceptance criterion.
  - Worth knowing when reading `:184-204`: an empty tree is refused by `sourceguardtest.ParseSources` ("parsed no sources, so a guard over them would pass by having stopped looking"), one message upstream of the descriptors, so those two table cases pass through the walk's own fatal rather than a descriptor's. The in-source comment at `:206-208` states exactly this, and the descriptor-owned fatal is genuinely observed by `:209-224` (a tree holding only a unit-lane `_test.go`, asserted equal to `sessionRestorerGuard.fatalWording`) — remove the fatal loop at `literal_guard_scan_test.go:101-106` and that subtest fails. The orchestrator descriptor's own wording is unreachable at runtime by construction (its include admits every test file, so a walk that produced any source gives it a non-zero count), and is pinned as a value by `:226-236`; that reachability property is inherited from the pre-change code, not introduced here.
  - No lane or isolation concern: pure source-reading unit-lane tests, no tmux, no daemon, no binary build.

CODE QUALITY:
- Project conventions: Followed. Unit-lane source guard routed through `sourceguardtest` (the stated single home for repo scans), reporting through `harnesstest.TestingT` so its own fatal path is testable; `t.Fatalf("%s", guard.fatalWording)` keeps the format string constant; no `t.Parallel()`; file naming matches the repo's `*_guard_test.go` practice.
- SOLID principles: Good. The descriptor separates *what varies* (type name, lane filter, copy) from *what is fixed* (walk, collect, scanned-zero rule); the scan depends on the `include` seam rather than on either concrete lane.
- Complexity: Low. One nested loop over sources × descriptors, one AST inspect, one post-loop validation.
- Modern idioms: Yes — variadic fixture writing, `slices.Equal` for the pointer-identity assertion, table-driven subtests.
- Readability: Good. Each descriptor field is documented at its declaration, and the doc comments state the *reason* for each choice (why the rule is "never compose the struct", why the session-restorer half is integration-scoped, why one walk).
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
