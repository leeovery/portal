TASK: cli-verb-surface-redesign-13-1 (bug) — Sweep the surviving divergent silent-first-match glob branches out of the single-pin resolve paths (ResolveSessionPin / ResolveAliasPin)

ACCEPTANCE CRITERIA:
1. Neither ResolveSessionPin nor ResolveAliasPin can return a matches[0] collapse for a multi-match glob.
2. A glob-bearing -s/-a value reaching the single-pin path yields an all-match expansion or an explicit loud error (never silent first-match), independent of the isMultiTarget/os.Args gate.
3. query_test.go:593 and :896 assert the new behaviour and no longer enshrine first-match.
4. The ResolveSessionPin doc comment no longer claims first-match-at-single-target-arity behaviour.
5. QueryResolver.Resolve is unchanged.

STATUS: Complete

SPEC CONTEXT:
Spec § Domain-pinning flags / § Glob targets: glob fan-out is EXCLUSIVELY the burst's job. A pinned -s/-a names its domain explicitly and matches by EXACT name/key; it never mints, never falls back to the picker, and (per this bug fix) must never silently collapse a multi-match glob to matches[0]. The single-pin resolve is a defense-in-depth loud-fail path: in production the multi-target routing gate diverts any glob-bearing pin to the *All burst variants, but if that os.Args assumption breaks, the single-pin path must still hard-fail loudly rather than fork glob semantics.

IMPLEMENTATION:
- Status: Implemented (solution (b) — explicit loud error mirroring Resolve's loud-miss).
- Location: internal/resolver/query.go
  - ResolveSessionPin (query.go:309-327): no glob branch; isExactSession() then hard-fail `fmt.Errorf("No session found: %s", query)` (:326). A glob value is never a literal session name → falls straight through to the loud miss.
  - ResolveAliasPin (query.go:366-372): no glob branch; aliases.Get(value) then unknownAliasError(value) → `"No alias found: %s"` (:407-409). A glob value is never a literal alias key → loud "No alias found".
  - Doc comments updated: ResolveSessionPin (:302-308) and ResolveAliasPin (:358-365) now explicitly state the pin does NOT glob-expand and that a glob falls through to the hard-fail miss ("a LOUD failure mirroring Resolve's loud-miss, never a silent first-match … independent of the multi-target routing gate"). The former "first match at single-target arity" clause is gone.
  - The three surviving HasGlobMeta branches (query.go:220 ResolveBareAll, :240 ResolveSessionPinAll, :266 ResolveAliasPinAll) are the correct burst-side all-match paths (via expandSessionGlobAll / MatchGlob-over-Keys) — not single-pin, correctly untouched.
  - Grep confirms zero `matches[0]` collapse in resolver production code.
- Notes: Committed as 6a6bdca4 (Tcli-verb-surface-redesign-13-1); the 14-1/14-2 refactors (typed Domain, extracted isExactSession) further unified the lister-error policy through the shared helper, so all three session entry points (Resolve, ResolveSessionPin, ResolveSessionPinAll) now share one exact-match rule. No drift from the plan.

TESTS:
- Status: Adequate.
- Coverage:
  - query_test.go:593 "multi-match glob does NOT collapse to the first match — loud miss": lister returns [api-1, api-2, web-abc], `api-*` → nil result + err "No session found: api-*". Formerly enshrined first-match; now retargeted. Would have returned api-1 under the old branch, so it fails if the branch is reintroduced.
  - query_test.go:615 zero-match glob and :630 exact miss guards reinforce the same loud message.
  - Alias pin (formerly ~:896, now :1019) "multi-match key glob does NOT mint the first match — loud miss": aliases {workflow-b, workflow-a} both existing, `workflow-*` → nil + "No alias found: workflow-*". Retargeted off the old "mints the first sorted match".
  - query_test.go:996 "single-match key glob no longer expands in the single-pin — loud miss": proves the branch is gone at ANY arity, not just multi (strong guard).
  - query_test.go:1060 zero-match alias glob confirms byte-identical message.
  - Failing alias/zoxide/session seams (failingAliasLookup / failingZoxideQuerier / failingSessionLister) double every pin case as a "never consults the other domains" guard.
- Notes: Not over-tested — each subtest guards a distinct arity/domain path with a single focused assertion set. Not under-tested for this task's scope. One adjacent gap (non-blocking, see below): there is no Resolve-level glob→MissResult guard test; the plan's Tests bullet referenced mirroring a `TestQueryResolver_Resolve_GlobFallsThroughToMiss`, but no test by that name (or covering Resolve's documented glob-falls-through-to-miss behaviour at query.go:137-142) exists. That is 8-2's territory (criterion 5 only requires Resolve be unchanged, which it is), not a 13-1 blocker.

CODE QUALITY:
- Project conventions: Followed. House-style capitalised user-facing messages with the //nolint:staticcheck ST1005 directive; unknownAliasError single-sources the "No alias found" message for byte-identical exact-miss vs zero-match-glob output; typed resolver.Domain used consistently.
- SOLID principles: Good. isExactSession is the single authority for the session match + lister-error policy; expandSessionGlobAll single-sources glob fan-out for the burst variants. Single-pin and all-match paths are cleanly separated.
- Complexity: Low. The single-pin paths are now straight-line (membership/lookup → hard-fail).
- Modern idioms: Yes (slices.Contains, errors.Is/As in siblings).
- Readability: Good. Doc comments precisely describe the loud-fail contract and the os.Args-gate independence.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/resolver/query_test.go:63 (TestQueryResolver_Resolve) — add a subtest asserting a glob value passed to QueryResolver.Resolve falls through to a *MissResult (guarding the behaviour documented at query.go:137-142), mirroring the pin-level loud-miss guards. Closes the coverage gap implied by the plan's reference to a `TestQueryResolver_Resolve_GlobFallsThroughToMiss` that does not exist. Arguably 8-2 scope; harmless to add here.
