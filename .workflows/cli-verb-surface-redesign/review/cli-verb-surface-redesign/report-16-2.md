TASK: Remove the fabricated-Domain latent trap in the degenerate single-surface burst (cli-verb-surface-redesign-16-2)

ACCEPTANCE CRITERIA:
1. The degenerate single-surface path no longer passes a fabricated Domain into openResolved (preferred), OR — if the round-trip is retained — the invariant is explicitly documented at surfaceToResult.
2. attach-vs-mint dispatch, command-guard, ack-write, and connector-dispatch behaviour are unchanged.
3. The upstream resolve-decision log line (emitted in resolveOpenSurfaces) is unchanged — no duplicate or incorrect resolve line is produced.

STATUS: Complete

SPEC CONTEXT: Multi-target open burst (spec § Atomic pre-flight & partial failure; § Burst exec-argv & mint responsibility). dispatchOpenBurst runs the read-only pre-flight over the ordered target set; a single surviving surface degenerates to a single connect through the shared openResolved dispatch (command-on-attach guard + hidden --ack write + inside/outside connector). The resolve-decision INFO line (component `resolve`, message `resolved`) is emitted once per non-glob bare target inside resolveOpenSurfaces via emitResolveDecision; globs are deterministic and emit no line.

IMPLEMENTATION:
- Status: Implemented — PREFERRED approach taken (thread the true resolver.QueryResult; no fabrication). surfaceToResult has been fully removed (grep across cmd/ and internal/ returns zero references).
- Location:
  - cmd/open_surfaces.go:37 — resolveOpenSurfaces signature now returns a 4th value `results []resolver.QueryResult`, retained in lockstep with surfaces (results[i] produced surfaces[i]), appended together in the same switch arm of the `collect` closure (lines 50-63). Lockstep invariant documented at lines 44-49.
  - cmd/open_burst.go:132 — dispatchOpenBurst captures `results` alongside surfaces.
  - cmd/open_burst.go:147-156 — degenerate arm passes `results[0]` (the exact QueryResult that produced surfaces[0], carrying its real Domain provenance) to openResolved; no domain fabricated. Comment explains the reuse is only for command guard + ack write + connector dispatch, none of which read Domain.
  - cmd/open.go:360-378 — openResolved unchanged; dispatches by concrete TYPE (*SessionResult attach / *PathResult mint), never reads .Domain.
- Notes: The trap is fully eliminated, not merely documented. Even a future openResolved that consulted r.Domain would now see the true provenance (DomainGlob attach / DomainAlias mint) rather than a fabricated DomainSession/DomainPath. spawn.Surface was left lossy (Kind+Value only, no Domain added) — the correct, minimal choice, since the lossy Surface is simply no longer round-tripped for the degenerate case.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/open_surfaces_test.go:433 TestResolveOpenSurfaces_DegenerateSingle_ThreadsTrueResultDomain — directly satisfies the task's added-test bullet. Sub-test "session glob to one attach carries DomainGlob" asserts results[0].(*SessionResult).Domain == DomainGlob (not fabricated DomainSession); sub-test "alias glob to one mint carries DomainAlias" asserts results[0].(*PathResult).Domain == DomainAlias (not fabricated DomainPath). Both assert exactly one surface AND one result (lockstep). The test comment correctly notes dispatchOpenBurst forwards results[0] VERBATIM into openResolved, so results[0] IS the value at the openResolved boundary.
  - cmd/open_multitarget_test.go:306 TestOpenCommand_SingleGlobExpandingToOne_SingleConnectNotBurst — confirms the degenerate glob→1 case connects via openResolved (openSessionFunc), not the burst, with correct session name; the openPathFunc/pathCalled seam exists for the mint side.
  - Existing single-target / attach / mint burst-routing tests (open_multitarget_test.go, open_surfaces_test.go) remain intact and unaffected by the added return value.
- Notes: Tests are focused, not redundant. One informational point (not a finding): the task's test bullet mentions "zoxide-origin mints", but a zoxide target is not glob-expandable (globExpandableDomain admits only DomainBare/DomainSession/DomainAlias), so a single -z target never routes through the multi-target gate and can never reach the degenerate burst arm. The reachable domains are exactly session-glob (→DomainGlob attach) and alias-glob (→DomainAlias mint), which are precisely the two the test covers. Coverage is correct for what is actually reachable.

CODE QUALITY:
- Project conventions: Followed. Small-interface/seam DI honoured; resolver stays a pure log-free library (emitResolveDecision remains the sole cmd-layer resolve emitter). No colour/log-vocabulary concerns.
- SOLID principles: Good. resolveOpenSurfaces keeps single responsibility (read-only classify); returning results in lockstep is a natural extension. openResolved untouched (open/closed preserved — the shared dispatch didn't need to change).
- Complexity: Low. One extra slice threaded; no new branches.
- Modern idioms: Yes. Idiomatic Go named-return + slice append in the same arm.
- Readability: Good. The lockstep invariant is documented at both the producer (collect closure) and the consumer (dispatchOpenBurst degenerate arm).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
