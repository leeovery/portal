TASK: cli-verb-surface-redesign-1-3 — Glob pre-check → session-domain expansion + zero-match hard-fail

ACCEPTANCE CRITERIA (Phase 1, row 1-3 + phase-level AC #1/#3):
- A bare target containing glob metacharacters (*, ?, […]) is session-domain by construction: expand against the user-visible session set, never run the path/alias/zoxide chain; zero matches is a hard fail.
- All session-domain matching (exact name + glob) matches only the leading-underscore-filtered ListSessions view (_portal-saver/_portal-bootstrap never matchable).
Edge cases: glob matching only _-prefixed internal sessions counts as zero; a path whose name contains glob metacharacters (foo[1]) is unreachable as a bare positional (zero-match hard-fail); glob skips the chain even when a same-named alias/dir exists; multi-match expansion into a burst deferred to Phase 3 (now implemented in final code).

STATUS: Complete

SPEC CONTEXT:
Spec § Target resolution precedence step 1 (Glob pre-check) and § Glob targets: a bare positional with glob metacharacters is session-domain by construction — expanded against live (user-visible) session names, skipping path/alias/zoxide; zero matches ⇒ unresolvable ⇒ atomic hard fail. A directory whose name contains glob metacharacters is unreachable as a bare positional (reach it via -p). § Session set — user-visible only: session-domain resolution matches only the leading-underscore-filtered ListSessions view. § Miss handling: single-target miss keeps the `-f` escape-hatch suggestion; no implicit TUI-picker fallback.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/resolver/glob.go:11 (globMeta = "*?["), :18 HasGlobMeta, :27 MatchSessions (filepath.Match; malformed pattern → zero matches, no panic).
  - internal/resolver/query.go:174 expandSessionGlobAll (zero → single *MissResult{pattern}; ≥1 → K *SessionResult{Domain:"glob"}), :203 ResolveBareAll (HasGlobMeta gate → expandSessionGlobAll against ListSessionNames, early return before the session→path→alias→zoxide chain).
  - internal/tmux/tmux.go:250-257 leading-underscore filter in ListSessions; :265 ListSessionNames delegates to it → user-visible set only.
  - cmd/open.go:230-233 multi-target routing gate (diverts glob-bearing single target to the burst before the bare Resolve).
  - cmd/open_burst.go:61 isMultiTarget (single glob-expandable target bursts), :76 globExpandableDomain (bare/session/alias), :108 singleMissError (keeps -f), :139-145 dispatchOpenBurst zero-match arity branch.
  - cmd/open_surfaces.go:56-68 resolveOpenSurfaces bare arm (ResolveBareAll → expand, resolve line suppressed for globs via emitResolveDecision's !HasGlobMeta gate).
  - cmd/open_targets.go:48 orderedOpenTargets classifies a bare positional as Domain "bare".
- Notes: Behaviour is structurally correct end-to-end. A single glob routes bare→burst→ResolveBareAll; the HasGlobMeta early-return guarantees the path/alias/zoxide chain is never consulted for a glob (this is exactly why a literal glob-named dir like foo[1] is unreachable bare). Multi-match now expands to K burst surfaces (Phase 3 landed) while single-match degenerates to a plain connect and zero-match hard-fails — all three arities handled. Defensive net: QueryResolver.Resolve no longer expands globs at all; a glob that ever slipped into it falls through to a loud miss (never a silent first-match), pinned by TestQueryResolver_Resolve_GlobFallsThroughToMiss. No drift from plan/spec.

TESTS:
- Status: Adequate
- Coverage:
  - internal/resolver/glob_test.go: HasGlobMeta (asterisk/question/bracket/bare-`[`, plain, nanoid, path-like, lone `]`, empty); MatchSessions (in-order subset, no-match, malformed → none, empty set); Resolve glob-fallthrough safety net incl. "matching only internal sessions", malformed glob.
  - internal/resolver/query_all_test.go: ResolveBareAll glob→K SessionResults{glob}, zero→single collected miss carrying the pattern; non-glob exact/alias/miss single-result paths.
  - cmd/open_surfaces_test.go: SessionGlobExpandsInPlace, OverlappingGlobsDuplicate (no dedup), GlobNamedDir_BareIsMiss_PathIsMint (the foo[1] edge case — bare = collected miss, -p = mint of the literal dir), ResolveLog_BareNonGlobOnly (glob emits no resolve line).
  - cmd/open_multitarget_test.go: SingleGlobExpandingToZero_KeepsMinusF (byte-exact "nothing resolved for 'nomatch-*' — try -f nomatch-*", burst NOT called), SingleGlobExpandingToMany_Bursts, SingleGlobExpandingToOne_SingleConnectNotBurst.
- Notes: Every acceptance edge case has a dedicated test at the right layer. Would fail if the feature broke (e.g. if the chain were consulted for foo[1], the bare arm would mint instead of miss; the test asserts a miss). Not over-tested: the resolver-level zero-match tests exercise the pure expansion primitive while the cmd-level tests exercise full command dispatch/arity routing — distinct layers, not redundant.

CODE QUALITY:
- Project conventions: Followed. Miss/hard-fail strings for this task (singleMissError, aggregatedMissError) are lowercase, no trailing punctuation, single-sourced (matches golang-error-handling skill). The capitalised "No session found"/"No alias found" messages are out of this task's scope (pin misses) and are deliberate documented byte-compat carve-outs with ST1005 nolint.
- SOLID principles: Good. HasGlobMeta/MatchSessions are single-purpose pure functions; expandSessionGlobAll is single-sourced and consumed by both ResolveBareAll and ResolveSessionPinAll; globExpandableDomain isolates the domain predicate.
- Complexity: Low. Clear early-return gates; no nested branching of note.
- Modern idioms: Yes (strings.ContainsAny, filepath.Match with ErrBadPattern-as-no-match, slices.Contains).
- Readability: Good. Comments precisely explain the `]`-not-a-starter choice, the malformed-glob-as-zero contract, and the single-glob-bursts routing rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
