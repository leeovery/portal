TASK: cli-verb-surface-redesign-2-3 — `-a/--alias` pin — alias-domain mint, key globs, shadow bypass, hard-fails on unknown key

ACCEPTANCE CRITERIA (Phase 2 row 2-3 + edge cases):
- alias key shadowed by a same-named session → `-a` mints at the aliased dir (bypasses session→path→alias precedence)
- unknown key hard-fails, never pops the picker
- key glob (`-a 'workflow-*'`) single-match mints; multi-match → burst deferred to Phase 3
- glob matches over the finite Portal-owned key namespace (enumerate via alias List/Keys)
- aliased dir no longer on disk → error
- Phase-level AC #1 (`-a` mints at aliased dir, accepts key globs, hard-fails on unknown key); AC #3 (every pin hard-fails, never picker); AC #4 (`-a` reaches a shadowed key)

STATUS: Complete

SPEC CONTEXT:
Spec § Domain-pinning flags: `-a/--alias <key-or-glob>` pins the alias domain — "mint at aliased dir; hard fail on unknown key"; "`-a` is the only way to reach an alias key shadowed by a same-named session (the pin bypasses precedence)". Spec § Glob targets: "`-a` accepts key globs (alias keys are a finite Portal-owned namespace)". Spec § Pinned-domain contract: every pin hard-fails on unresolvable and never falls back to the TUI picker. Axiom 2: a directory-domain hit (alias) always mints a fresh `{project}-{nanoid}` session.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/resolver/query.go:346-362 (`ResolveAliasPin`) — glob branch (Keys()+MatchSessions, first-sorted match) and exact branch; both route through `validatedPath(path,"alias")`.
  - internal/resolver/query.go:391-399 (`unknownAliasError`) — single-sourced "No alias found: <value>" for unknown-key + zero-match-glob.
  - internal/resolver/query.go:404-409 (`validatedPath`) — disk existence check → `PathResult{Domain:"alias"}` or `*DirNotFoundError`.
  - internal/resolver/query.go:21-24 — `AliasLookup` interface gains `Keys()`; internal/alias/store.go:227-234 adds `Store.Keys()` (sorted, deduped).
  - cmd/open.go:259-272 — pinDispatch table maps "alias" → `(*resolver.QueryResolver).ResolveAliasPin`; cmd/open.go:317-328 (`resolvePinAndOpen`) drives resolve + `openResolved` handoff; the pin block sits before the bare-resolution path, so precedence is bypassed structurally.
  - cmd/open_targets.go:27-35 — `-a`/`--alias` mapped to "alias" domain in the ordered-target scan.
- Notes: Shadow bypass is correct on two levels — (1) the cmd dispatch reaches the `-a` pin arm before any bare session/path resolution, and (2) `ResolveAliasPin` itself never touches `qr.sessions`/`qr.zoxide`. Gone-dir vs unknown-key are distinct failure classes (`*DirNotFoundError` vs plain "No alias found"). Task text says "enumerate via alias List"; implementation added a dedicated `Keys()` method instead of reusing `List()` (which returns []Alias) — a cleaner, semantically-correct choice, not a drift.

TESTS:
- Status: Adequate
- Coverage:
  - internal/resolver/query_test.go:832-978 (`TestQueryResolver_ResolveAliasPin`) — 6 focused subcases: known key → PathResult{Domain:alias}; glob single-match; glob multi-match → first-sorted-match; unknown key → "No alias found: nope"; zero-match glob → "No alias found: web-*"; gone dir → "Directory not found: /gone/dir" + errors.As(*DirNotFoundError). The resolver is built with `failingSessionLister`/`failingZoxideQuerier`, so every case doubles as a "never consults session/zoxide" (shadow-bypass) guard.
  - cmd/open_test.go:710-901 — `TestOpenCommand_AliasPin_Mints_NoPicker` (same-named shadowing session present; asserts mint + no attach + no picker), `..._UnknownKey_HardFailsNoPicker` (no picker/mint; plain error, not UsageError), `..._ThreadsCommandIntoMint` (-e claude threads through), `..._EmitsNoResolveLine` (pin is deterministic → no resolve log line).
  - cmd/open_targets_test.go:96 + open_targets_guard_test.go — `-a` value attribution and the openTargetPins↔live-flag-set drift guard.
- Notes: Test balance is good — no redundant assertions; each subcase pins a distinct behaviour. The gone-dir path is covered at the resolver layer (not duplicated at cmd layer), which is proportionate since it propagates through the identical `resolvePinAndOpen` path already exercised by the unknown-key cmd test. Not under- or over-tested.

CODE QUALITY:
- Project conventions: Followed — small interface (`AliasLookup` = Get+Keys), method-value dispatch table (a new pin is one row, not a fifth copy), single-sourced error/message helpers, house-style capitalised user-facing strings with the documented `//nolint:staticcheck` directive.
- SOLID principles: Good — `ResolveAliasPin` has a single alias-domain responsibility; the pinDispatch table is open/closed.
- Complexity: Low — clear glob/exact two-branch structure.
- Modern idioms: Yes — slices.Sort, filepath.Match, method values, typed error sentinels.
- Readability: Good — doc comments cross-reference the spec sections and explain each decision.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/resolver/query.go:347-355 — `ResolveAliasPin`'s glob branch (first-sorted-match collapse via `matches[0]`) is production-unreachable: the cmd-layer `isMultiTarget` gate (cmd/open_burst.go:61-70, globExpandableDomain includes "alias") routes every glob-bearing `-a` value to the burst (`ResolveAliasPinAll`), so the single-pin dispatch table (open.go:265) only ever invokes `ResolveAliasPin` with a glob under the test harness (where `openOwnArgs` returns nil). This is the same silent-first-match dead branch Phase-8 task 8-2 removed from `Resolve`, but the pin variants (`ResolveAliasPin` and `ResolveSessionPin` at query.go:295-298) were left. Decide: remove the branch to align with 8-2 (also updating the single-target glob unit subcases), or add a comment noting it is deliberately retained as a tested single-target library primitive only reachable at test arity — the current doc comment implies the first-match path is a live single-target behaviour.
- [quickfix] internal/resolver/query.go:252,348 (and internal/resolver/glob.go:27) — `MatchSessions` is reused to glob-match the alias-key namespace (`qr.aliases.Keys()`), a session-named helper applied to a non-session domain. Rename to a domain-agnostic `MatchGlob` for legibility; mechanical cross-file rename, no logic change.
