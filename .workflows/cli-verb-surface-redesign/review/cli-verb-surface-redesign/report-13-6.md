TASK: cli-verb-surface-redesign-13-6 — Small DRY / Legibility Cleanups (Redundant And Misleading Code)

ACCEPTANCE CRITERIA:
- No `MatchSessions` identifier remains; `MatchGlob` compiles with all call sites updated.
- `buildProductionSpawnSeams` uses `spawnLogger`.
- `completionAliasKeys` loads the alias store once.
- No behaviour change; existing tests pass.

STATUS: Complete

SPEC CONTEXT: A chore-type maintainability task (severity: low) folding three unrelated
legibility nits surfaced by earlier review reports (report-2-3, report-5-2, report-6-4).
No spec behaviour is affected — this is a pure rename + single-source + dead-load-drop
refactor. The glob helper is reused across the session domain (query.go bare/session-pin
expansion) and the alias-key domain (alias-pin expansion), so the session-named identifier
was misleading; the spawn logger and alias-store load were each duplicated.

IMPLEMENTATION:
- Status: Implemented (all three nits addressed)
- Location:
  (1) internal/resolver/glob.go:28 — `MatchGlob` defined; doc comment (lines 22-27)
      rewritten to read domain-agnostically ("the names may be session names, alias keys,
      or any other string namespace"). Call sites updated: internal/resolver/query.go:191
      (expandSessionGlobAll, session domain) and query.go:267 (ResolveAliasPinAll, alias-key
      domain). Doc-comment references also updated (query.go:187, 258). Test renamed:
      internal/resolver/glob_test.go:36 TestMatchGlob (+ call/message sites 71,73,77).
      Grep for `MatchSessions` across the whole tree returns ZERO hits — identifier fully gone.
  (2) cmd/spawn_seams.go:59 — `Logger: spawnLogger` references the package-level
      `spawnLogger = log.For("spawn")` (line 16). Confirmed the only actual `log.For("spawn")`
      invocation in cmd is that package-level var; the other three cmd matches
      (open.go:673, open.go:918, open_burst_run.go:50) are comments, not calls. Single-sourced.
  (3) cmd/completion.go:60-66 — `completionAliasKeys` now calls `loadAliasStore()` once
      (which itself performs `store.Load()` at cmd/alias.go:97) and returns `store.Keys()`
      directly. No redundant second `store.Load()`. `Keys()` (internal/alias/store.go:227)
      reads the already-populated `s.aliases` map, so dropping the extra Load is
      behaviour-preserving.
- Notes: All three edits are mechanical and match the planned Solution exactly. No drift.

TESTS:
- Status: Adequate
- Coverage: This is a behaviour-preserving refactor, so per the plan ("existing tests pass
  unchanged") no new tests are warranted. TestMatchGlob (glob_test.go) exercises the renamed
  function directly (prefix match, no-match, malformed pattern, empty set). The resolver
  query_all tests exercise both call sites (session + alias-key glob expansion). Completion
  and spawn-seam behaviour is covered by existing cmd tests via the injectable seams
  (completionAliasKeys / productionSpawnSeams.Logger).
- Notes: No under-testing (rename + dead-code drop needs no incremental test); no over-testing
  (no redundant assertions added). Correct test posture for a no-behaviour-change chore.

CODE QUALITY:
- Project conventions: Followed. Identifier reads domain-agnostically (golang-naming: name by
  intrinsic role, not use-site), matching the codebase's preference. Single-source var reuse
  is idiomatic.
- SOLID principles: Good — MatchGlob's single responsibility now honestly named; spawn logger
  single-sourced (DRY).
- Complexity: Low — no control-flow change anywhere.
- Modern idioms: Yes.
- Readability: Improved on all three fronts (the stated Outcome). The MatchGlob doc comment
  explicitly documents the multi-domain reuse, which prevents the misleading-name regression
  from recurring.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
