TASK: cli-verb-surface-redesign-3-4 — Atomic aggregated pre-flight abort (report every miss; single-target `-f` carve-out)

ACCEPTANCE CRITERIA (edge cases from the Phase 3 task table):
- multiple misses all reported (one re-run fixes all)
- any single miss in a mixed set aborts atomically (nothing opens/mints)
- single-target miss keeps `-f` suggestion (Phase 1), multi-target miss omits `-f`
- zero-match glob counts as a miss
- N=1 all-hit falls through to the single-target connect, not the burst

STATUS: Complete

SPEC CONTEXT:
Spec § "Atomic pre-flight & partial failure": pre-flight is a read-only resolve of the
whole target set; any unresolvable target ⇒ atomic abort (nothing opens, nothing created);
the abort reports EVERY unresolvable target (not just the first) so one re-run fixes them
all; the `-f <text>` suggestion appears ONLY in the single-target case (because `-f` is
mutually exclusive with targets and cannot carry multi-target intent). Spec § "Glob targets":
zero matches ⇒ unresolvable ⇒ atomic hard fail. Spec § "Miss handling": single-target miss
uses `nothing resolved for 'blog' — try -f blog` (U+2014 em-dash).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/open_burst.go:126-152 (dispatchOpenBurst) — the atomic pre-flight + routing.
  - cmd/open_burst.go:91-93 (aggregatedMissError) — multi-target message, no `-f`, QuoteJoin.
  - cmd/open_burst.go:108-110 (singleMissError) — single-target message, keeps `-f`, U+2014.
  - cmd/open_burst.go:61-70 (isMultiTarget) / :76-83 (globExpandableDomain) — the routing gate.
  - cmd/open_burst.go:158-163 (surfaceToResult) — single-surface degenerate → openResolved.
  - cmd/open.go:230-233 — RunE wires the gate; :285-305 — the unchanged single-target path
    (single non-glob miss uses the same singleMissError, single-sourced per Task 7-4).
  - cmd/open_surfaces.go:37-101 (resolveOpenSurfaces) — the strictly read-only resolve that
    produces the misses slice (Task 3-3; the input to 3-4's aggregation).
- Notes:
  - Atomicity holds: dispatchOpenBurst resolves the WHOLE set first, then checks
    `len(misses) > 0` and returns the abort BEFORE any surface opens (no burst, no
    openResolved connect, no mint). resolveOpenSurfaces performs no mint / no tmux mutation
    (it only reads sessions/aliases/zoxide/filesystem and reduces mints to literal dirs),
    so "nothing opens/mints" is guaranteed.
  - Arity discriminator is `len(ordered) == 1` (the input arity), which correctly maps a
    single glob-expanding-to-zero to the single-target `-f` message while a mixed set with
    a single miss still uses the aggregated (no-`-f`) wording — matching the spec's
    arity-based carve-out.
  - N=1 all-hit split into its two forms is handled: a single non-glob target never enters
    dispatchOpenBurst (isMultiTarget=false); a single glob expanding to exactly one surface
    enters the burst dispatch but degenerates via `len(surfaces) == 1` → openResolved (so
    the command-on-attach guard + `--ack` write + inside/outside dispatch all apply).
  - Safe indexing: `misses[0]` in the len(ordered)==1 arm is guarded by the enclosing
    `if len(misses) > 0`, and a single target can produce at most one MissResult, so it
    never over-indexes. All-variant resolvers always return >=1 result (verified:
    expandSessionGlobAll / ResolveBareAll / ResolveSessionPinAll / ResolveAliasPinAll each
    return a MissResult on zero-match), so resolveOpenSurfaces' results[0] cannot panic.
  - ErrZoxideNotInstalled is an immediate whole-resolve abort (env fault), deliberately NOT
    folded into the "report every miss" aggregation — documented and reasoned in
    resolveOpenSurfaces and consistent with the Phase 2 pinned-`-z` contract. Correct, not
    a drift.

TESTS:
- Status: Adequate
- Coverage (cmd/open_multitarget_test.go):
  - TestOpenCommand_MultiTarget_MixedSetTwoMisses_ReportsBothAtomically — every miss
    reported ("nothing resolved for: 'gone1', 'gone2'"), no burst/session/path/tui.
  - TestOpenCommand_MultiTarget_SingleMissInThreeSet_AbortsAtomically — one miss in a
    3-set aborts atomically; hits do not connect.
  - TestOpenCommand_MultiTargetMiss_OmitsMinusF — multi-target message omits `-f`.
  - TestOpenCommand_SingleTargetMiss_KeepsMinusFSuggestion — single non-glob bare keeps
    the `-f` suggestion on the unchanged single-target path.
  - TestOpenCommand_SingleGlobExpandingToZero_KeepsMinusF — zero-match glob is a miss AND
    keeps the single-target `-f` wording (N=1 arity through the burst dispatch).
  - TestSingleMissError_ByteIdenticalFormat — golden-string guard for the U+2014 em-dash
    and the target substituted twice.
  - TestOpenCommand_SingleGlobExpandingToMany_Bursts — glob → K≥2 bursts (all-hit → burst).
  - TestOpenCommand_MultiTarget_AllHitRepeatedPin_Bursts — `-s a -s b` all-hit → burst.
  - TestOpenCommand_SingleGlobExpandingToOne_SingleConnectNotBurst — N=1 all-hit degenerate
    → openSessionFunc single connect, not the burst.
- Notes:
  - Every acceptance/edge point has a dedicated, focused test; each drives openCmd.RunE
    through cobra with the raw argv injected via the openRawArgs seam and captures the
    routing arm — behaviour, not implementation detail.
  - Not over-tested: the byte-identical helper test is a deliberate cheap wording-drift
    guard, not redundant with the through-command tests.
  - The per-domain miss classification that FEEDS the aggregation (`-p` non-existent dir,
    `-z` no-match, glob-named dir, ErrZoxideNotInstalled, read-only guarantee) is thoroughly
    covered at the source in cmd/open_surfaces_test.go (Task 3-3). Since aggregatedMissError
    is domain-agnostic (it only joins the misses slice), testing one miss shape through the
    aggregation is sufficient; no gap.

CODE QUALITY:
- Project conventions: Followed. Seam-based DI (runOpenBurstFunc / openRawArgs / openDeps),
  no t.Parallel, single-sourced user-facing strings (singleMissError / aggregatedMissError /
  commandAttachOnlyMessage), plain vs UsageError error classes matched to exit codes
  (aggregated miss = plain error → exit 1, asserted not a UsageError).
- SOLID principles: Good. dispatchOpenBurst has a single responsibility (pre-flight + route);
  the resolve engine, the message helpers, and the routing gate are cleanly separated.
- Complexity: Low. dispatchOpenBurst is a linear 5-step guard chain; each branch is obvious.
- Modern idioms: Yes (errors.Is sentinel branch for ErrZoxideNotInstalled, type-switch
  collect, strings.Cut in the argv scan).
- Readability: Good. Doc comments are precise and cite the governing spec sections; the
  arity-vs-miss-count distinction is explained inline.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
