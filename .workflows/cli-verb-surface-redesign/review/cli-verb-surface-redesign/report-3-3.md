TASK: cli-verb-surface-redesign-3-3 — Read-only resolve/classify engine → ordered attach/mint surfaces + glob/alias-glob K-expansion + literal-dir reduction

ACCEPTANCE CRITERIA (from plan row 3-3):
- bare runs the Phase-1 chain, pins skip to their Phase-2 domain
- session glob (bare or `-s`) expands to K user-visible session surfaces joining in place, zero-match = miss
- `-a` key glob expands to K alias mints
- overlapping globs may duplicate surfaces (honored, never deduped)
- mint targets reduced to a literal existing dir so the window never re-resolves
- glob-metacharacter dir path unreachable as bare, reachable only via `-p`
- strictly read-only (no mint, no tmux mutation)

STATUS: Complete

SPEC CONTEXT:
Spec § "Burst exec-argv & mint responsibility" mandates a mint target be reduced to a
literal existing directory at resolve time (alias/zoxide/-p never travel to the spawned
window; only the literal dir does, so `--path <dir>` cannot diverge). Spec § "Target-set
composition" defines the union of positionals + pins, each resolving by its own rule
(bare → precedence chain; pins → their domain). Spec § "Glob targets": bare glob metacharacters
are session-domain by construction, expand to K targets joining the list, zero-match = atomic
hard fail; `-a` accepts key globs over the finite Portal-owned namespace; a directory whose name
contains glob metacharacters is unreachable as a bare positional and reachable only via `-p`.
Spec § "Atomic pre-flight & partial failure": the engine is a strictly read-only resolve of the
whole set (no mint, nothing created). No dedup — overlapping globs may duplicate surfaces.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/open_surfaces.go:37-101 — resolveOpenSurfaces, the read-only classify engine. Walks the
    ordered []Target, routing each domain: "bare" → ResolveBareAll (+ single-sourced
    emitResolveDecision), "session" → ResolveSessionPinAll, "alias" → ResolveAliasPinAll,
    "path" → ResolvePathPin (collected-miss on error), "zoxide" → ResolveZoxidePin (only
    ErrZoxideNotInstalled aborts; else collected miss). collect() classifies SessionResult→attach,
    PathResult→mint (Value = resolved literal dir), MissResult→appended raw target.
  - internal/resolver/query.go:174-279 — expandSessionGlobAll (shared K-expansion primitive, Task
    7-5 extraction), ResolveBareAll, ResolveSessionPinAll, ResolveAliasPinAll. All return collected
    *MissResult (never a hard error) except ErrZoxideNotInstalled; alias-glob validates each matched
    key's dir and reduces to *PathResult, per-key gone-dir → *MissResult carrying THAT key.
  - internal/spawn/surface.go — Surface{Kind, Value} output shape; SurfaceMint.Value documented as
    the resolved literal dir, never the query.
  - cmd/open_burst.go:126-163 — dispatchOpenBurst consumes the engine output (routing/abort is the
    3-4 boundary; surfaceToResult degenerate single-surface path).
  - Glob detection via resolver.HasGlobMeta / MatchSessions (glob.go), filepath.Match (glob not regex).
- Notes: Each ACC criterion maps cleanly to an implementation site. Bare→chain / pins→domain via the
  switch. Session glob (bare) and alias key glob route through the shared expandSessionGlobAll /
  ResolveAliasPinAll. Literal-dir reduction: PathResult.Path is the on-disk-validated dir; the raw
  alias key / zoxide query never becomes a Surface value. Glob-named dir bare→miss, -p→mint (ResolvePathPin
  stats the literal path, bypassing glob detection). Read-only: engine only READS (session set, alias store,
  zoxide, fs existence) — no CreateFromDir/QuickStart/tmux mutation. No dedup: results appended in order.
  The Resolve method's production-dead glob branch was already removed (Task 8-2), so a glob never
  silently first-matches; the whole K-expansion is single-sourced through expandSessionGlobAll.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/open_surfaces_test.go — engine-level: MixedOrderedSet (bare+pins ordered surfaces),
    SessionGlobExpandsInPlace (bare glob joins between neighbours), AliasKeyGlobExpandsToMints,
    OverlappingGlobsDuplicate (honored, not deduped), MintReducedToLiteralDir (+ explicit assert the
    query strings did NOT travel), GlobNamedDir_BareIsMiss_PathIsMint, ZoxideNotInstalled_ImmediateHardError,
    CollectedMisses (-z no-match / -p nonexistent / -p non-dir file), ReadOnly_NoMintOrAttach
    (openPathFunc/openSessionFunc fatal-if-called guard), ResolveLog_BareNonGlobOnly (exactly 3 lines,
    INFO, correct attrs; pins+globs emit none).
  - internal/resolver/query_all_test.go — resolver-primitive coverage of ResolveBareAll /
    ResolveSessionPinAll / ResolveAliasPinAll: glob→K SessionResults(domain=glob), zero-match→single miss,
    exact hit, bad path→collected miss, gone alias dir→collected miss, total miss, alias key glob→K
    PathResults reduced to dirs, per-key gone-dir→miss-for-that-key-others-survive, unknown key, zero-key glob.
  - Each ACC criterion has a failing-if-broken assertion. Read-only is proven structurally (fatal seams).
- Notes: Balanced, not over-tested — the resolver-primitive tests and the cmd-engine tests exercise
  different layers (primitive fan-out vs. engine routing/ordering/in-place-join), not the same thing twice.

CODE QUALITY:
- Project conventions: Followed. Small interface DI (SessionLister/AliasLookup/ZoxideQuerier/DirValidator),
  no t.Parallel in package cmd, table-free switch on the domain string, internal/resolver stays log-free
  (resolve line emitted from the cmd layer via emitResolveDecision), error sentinels in tmuxerr/resolver.
- SOLID principles: Good. resolveOpenSurfaces has a single responsibility (classify → surfaces/misses);
  the All-variants are the reusable primitives; expandSessionGlobAll is single-sourced (Task 7-5) so the
  bare and -s glob paths cannot drift.
- Complexity: Low. One linear pass with a five-arm switch; the inner collect closure has one type switch.
- Modern idioms: Yes (slices.Contains, strings.Cut for the equals-form pin, errors.Is sentinel branch).
- Readability: Good. Comments accurately document the immediate-hard-error vs collected-miss split and
  the literal-dir-reduction contract with spec section references.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/open_surfaces_test.go — add an engine-level subtest for a `Domain:"session"` (i.e. `-s`)
  glob that expands in place within a mixed target set. The ACC explicitly calls out "session glob (bare
  or `-s`)"; the bare-glob in-place join is engine-tested (SessionGlobExpandsInPlace) and ResolveSessionPinAll
  glob is resolver-tested, but the `-s`-domain routing through the engine's "session" arm with in-place
  expansion is only covered transitively. Low priority — the "session" arm is a one-line delegation identical
  in shape to the engine-tested bare arm.
