TASK: cli-verb-surface-redesign-8-2 — Remove the production-dead, divergent session-glob branch from `QueryResolver.Resolve`

ACCEPTANCE CRITERIA (from plan row 8-2):
1. Multi-match session glob never collapses to `matches[0]`.
2. A glob reaching the resolver expands to all matches or returns an explicit error (never silent first-match).
3. Single-target and burst glob expansion share one primitive.
4. Confirm callers (cmd/open.go, `ResolveBareAll`) never receive glob in the prod routing path.
5. An `os.Args`-assumption break can no longer silently fork glob semantics.

STATUS: Issues Found

SPEC CONTEXT:
Spec § Glob targets (specification.md:158-166) and § Target resolution precedence (l.51): a bare glob target is "session-domain by construction" and "expand[s] it against live session names" producing K targets that join the target list — never a first-match. § Domain-pinning flags (l.102, l.105): `-s/--session <name-or-glob>` and `-a/--alias <key-or-glob>` accept globs and expand them (over the session-name set / the finite alias-key namespace). Nowhere does the spec sanction a "first match at single-target arity" — that was a Phase-1 implementation artifact the multi-target routing gate is meant to override. § resolve log taxonomy (l.88): globs are deterministic (not guesses) so emit no resolve line.

IMPLEMENTATION:
- Status: Partial (core deliverable Implemented; two sibling branches left in place)
- Location: internal/resolver/query.go
- The 8-2 commit (95f37631) removed the `if HasGlobMeta(query) { … return &SessionResult{Name: matches[0], …} }` pre-check block from `QueryResolver.Resolve`. `Resolve` (query.go:132-166) is now a strict non-glob single-target resolver; a glob value falls through session→path→alias→zoxide to a `*MissResult` (a loud hard-fail via the caller). The doc comment (query.go:114-131) documents the new contract accurately.
- `ResolveBareAll` (query.go:203-214) pre-handles a glob via `expandSessionGlobAll` (all-match) BEFORE delegating to `Resolve`, so the fall-through inside `Resolve` is never reached from that caller in the glob case. Good.
- `expandSessionGlobAll` (query.go:174-184) is the single shared all-match primitive consumed by both `ResolveBareAll` and `ResolveSessionPinAll` (criterion 3 satisfied for the burst paths — this was Task 7-5).
- Production routing confirmed: `orderedOpenTargets` → `isMultiTarget` (cmd/open_burst.go:61-70) diverts any single glob-bearing bare/`-s`/`-a` target to the burst (`globExpandableDomain` + `HasGlobMeta`) BEFORE the single-pin dispatch table (cmd/open.go:259-272) and before the bare `qr.Resolve(query)` call (cmd/open.go:285). So in the normal prod path neither `Resolve` nor `ResolveBareAll` receives a glob (criterion 4 satisfied).

THE SURVIVING DIVERGENT BRANCHES (direct answer to the orchestrator's note):
8-2 removed ONLY the glob branch in `QueryResolver.Resolve`. A divergent first-match glob branch SURVIVES, unchanged, in BOTH single-pin (non-`All`) resolve paths:
- `ResolveSessionPin` — internal/resolver/query.go:295-298:
      if HasGlobMeta(query) {
          if matches := MatchSessions(query, names); len(matches) > 0 {
              return &SessionResult{Name: matches[0], Domain: "glob"}, nil   // ← query.go:297
          }
      }
- `ResolveAliasPin` — internal/resolver/query.go:347-355:
      if HasGlobMeta(value) {
          matches := MatchSessions(value, qr.aliases.Keys())
          ...
          path, _ := qr.aliases.Get(matches[0])   // ← query.go:353
          return qr.validatedPath(path, "alias")
      }
Both collapse a multi-match glob to `matches[0]` — the exact silent-first-match the acceptance says must not exist. `ResolvePathPin` / `ResolveZoxidePin` have no glob branch (correct — `-p`/`-z` never glob-expand).

Reachability: the only production callers of `ResolveSessionPin` / `ResolveAliasPin` are the pinDispatch table (cmd/open.go:263, :265), reached ONLY when `isMultiTarget` returns false. A glob-bearing `-s`/`-a` value makes `isMultiTarget` return true → burst → `ResolveSessionPinAll`/`ResolveAliasPinAll` (correct all-match). So the two surviving branches are production-dead — but production-dead SOLELY because `isMultiTarget` depends on the `os.Args`-based `openOwnArgs()` (cmd/open_burst.go:43-51). If that assumption breaks (`openOwnArgs()` returns nil — no `open` token found → `isMultiTarget` false), a glob-bearing `-s 'api-*'`/`-a 'workflow-*'` falls through to `ResolveSessionPin`/`ResolveAliasPin` and silently first-matches. This is precisely the fork acceptance criterion 5 says must be impossible — 8-2 closed it for the BARE path (glob → loud miss) but NOT for the `-s`/`-a` pin paths.

TESTS:
- Status: Adequate for the `Resolve` deliverable; the pin paths have tests that PIN the divergent behaviour.
- Coverage (good): `TestQueryResolver_Resolve_GlobFallsThroughToMiss` (internal/resolver/glob_test.go) now asserts a multi-match session glob does NOT collapse to a first-match `SessionResult` and instead yields `*MissResult` — directly locks criteria 1/2 for `Resolve`. The stale "glob pre-check" contrast test in query_test.go was correctly removed. `TestOpenCommand_ResolveLog_GlobEmitsNoLine` (cmd/open_test.go) was rewritten to drive the real burst route via an injected raw argv, confirming a glob routes to the burst and suppresses the resolve line. Solid.
- Concern: query_test.go:593 ("glob expansion attaches the first match with domain glob", asserts `matches[0]` = "api-1") and query_test.go:896 ("key glob multi-match mints the first sorted match (single-target)") explicitly assert and lock in the surviving first-match collapse in `ResolveSessionPin`/`ResolveAliasPin`. These tests are green precisely because the divergent branches were not touched — they encode the exact behaviour criterion 1 forbids. They confirm the survival is deliberate/unaddressed, not an oversight of a removed test.
- No test execution performed (assessed by reading only).

CODE QUALITY:
- Project conventions: Followed. internal/resolver stays a pure, log-free library; house-style capitalised user-facing errors with the //nolint:staticcheck annotation are consistent.
- SOLID / DRY: The burst paths correctly single-source glob expansion through `expandSessionGlobAll`. The two single-pin paths, however, still hand-roll their own `HasGlobMeta`/`MatchSessions`/`matches[0]` logic that diverges from the shared all-match primitive — a residual DRY/consistency gap and the seat of the divergence.
- Complexity: Low.
- Readability: Good; the `Resolve` doc comment is thorough and accurate.
- Issues: The surviving pin-path branches (above) are the substantive quality/correctness concern.

BLOCKING ISSUES:
- Acceptance criteria 1 ("Multi-match session glob never collapses to `matches[0]`") and 5 ("an `os.Args`-assumption break can no longer silently fork glob semantics") are NOT fully satisfied. The identical divergent first-match glob branch survives in `ResolveSessionPin` (internal/resolver/query.go:297) and `ResolveAliasPin` (internal/resolver/query.go:353). They are production-dead ONLY via the `os.Args`-dependent `isMultiTarget` diversion; on an `os.Args`-assumption break a glob-bearing `-s`/`-a` pin silently collapses to the first match — the exact fork the acceptance forbids. The core `Resolve` change is correct and complete, but the acceptance criteria as written extend beyond `Resolve`. Recommended resolution (likely a scoped follow-up task rather than reopening the `Resolve` change): either route the `ResolveSessionPin`/`ResolveAliasPin` glob branches through the shared `expandSessionGlobAll`/all-match primitive, or make a glob value reaching a single-pin an explicit loud error (mirroring `Resolve`'s loud-miss safety net), and retarget query_test.go:593 / :896 accordingly.

NON-BLOCKING NOTES:
- [bug] internal/resolver/query.go:297 (ResolveSessionPin) — multi-match session glob collapses to `matches[0]` (silent first-match); production-dead today only via the `os.Args`-based `isMultiTarget` gate, a latent fork on an argv-assumption break. Route through `expandSessionGlobAll` / all-match, or make a single-pin glob a loud error.
- [bug] internal/resolver/query.go:353 (ResolveAliasPin) — same divergent first-match collapse over the alias-key namespace; same latent fork and same fix.
- [quickfix] internal/resolver/query_test.go:593 and :896 — these tests assert and lock in the surviving first-match collapse; they must be retargeted (to a loud miss/error or an all-match expansion) together with whichever fix the two [bug] notes above receive, so the suite stops enshrining the divergent behaviour.
